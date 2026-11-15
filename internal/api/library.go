package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/libops"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/pages"
	"github.com/brianmcgee/rmhub/internal/render"
	"github.com/brianmcgee/rmhub/internal/rendercache"
	"github.com/brianmcgee/rmhub/internal/rootlog"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// SearchLimit is the maximum number of search results.
const SearchLimit = 100

// Library holds the dependencies of the library endpoints and the change
// feed.
type Library struct {
	Lib    *library.Library
	Ops    *libops.Ops
	Blobs  blob.Store
	Roots  *rootlog.Log
	Tokens *tokens.Store
	Kinds  []TokenKind
	Log    *slog.Logger
	// MaxUpload is the maximum size of a document upload in bytes.
	MaxUpload int64
	// TempDir holds the upload spool files. Empty means the default
	// temporary directory.
	TempDir string
	// Feed has the timers of the change feed.
	Feed FeedTimers
	// Stop ends each open change feed when it closes, so that a server
	// shutdown does not wait for the streams.
	Stop <-chan struct{}
	// Render renders pages for the render endpoint.
	Render *Renderer
}

// Register adds the library endpoints and the change feed to the mux.
func (l *Library) Register(m *Mux) {
	auth := func(h HandlerFunc) HandlerFunc { return Auth(l.Kinds, AccountToken(h)) }
	m.Handle("GET /api/v1/library", auth(l.list))
	m.Handle("GET /api/v1/library/resolve", auth(l.resolve))
	m.Handle("GET /api/v1/library/search", auth(l.search))
	m.Handle("GET /api/v1/library/{id}", auth(l.get))
	m.Handle("GET /api/v1/library/{id}/content", auth(l.content))
	m.Handle("GET /api/v1/library/{id}/archive", auth(l.archive))
	m.Handle("GET /api/v1/library/{id}/pages", auth(l.pages))
	m.Handle("GET /api/v1/library/{id}/pages/{number}/render", auth(l.renderPage))
	m.Handle("POST /api/v1/library/folders", auth(l.createFolder))
	m.Handle("POST /api/v1/library/documents", auth(l.upload))
	m.Handle("PATCH /api/v1/library/{id}", auth(l.patch))
	m.Handle("POST /api/v1/library/{id}/trash", auth(l.trash))
	m.Handle("GET /api/v1/changes", auth(l.changes))
}

// EntryView is the JSON form of an entry. Path is set on single reads.
type EntryView struct {
	library.Entry
	Path string `json:"path,omitempty"`
}

// view is the library of one request: one snapshot and the scope of the
// token.
type view struct {
	s  *library.Snapshot
	sc *grants.Scope
}

func (l *Library) open(w http.ResponseWriter, r *http.Request) (view, error) {
	p := PrincipalFrom(r.Context())
	s, err := l.Lib.Current(r.Context(), p.Account)
	if err != nil {
		return view{}, err
	}
	w.Header().Set("RMHub-Generation", strconv.FormatUint(s.Generation, 10))
	return view{s: s, sc: grants.FromChain(p.Chain)}, nil
}

func (v view) visible(id string) bool {
	_, ok := v.s.Doc(id)
	return ok && (v.sc == nil || v.sc.Visible(v.s, id))
}

// doc returns the visible entry with the ID, or code not_found.
func (v view) doc(id string) (*library.Doc, error) {
	d, ok := v.s.Doc(id)
	if !ok || !v.visible(id) {
		return nil, Errorf(CodeNotFound, "entry %s does not exist", id)
	}
	return d, nil
}

// entry returns the view of an entry. A grant folder of a scoped token
// is a top-level entry for that token.
func (v view) entry(d *library.Doc, withPath bool) EntryView {
	e := EntryView{Entry: d.Entry}
	var top func(string) bool
	if v.sc != nil {
		top = v.sc.IsTop
		if v.sc.IsTop(d.ID) {
			e.Parent = ""
		}
	}
	if withPath {
		e.Path = v.s.Path(d.ID, top)
	}
	return e
}

func (v view) entries(ds []*library.Doc) []EntryView {
	out := make([]EntryView, 0, len(ds))
	for _, d := range ds {
		if v.visible(d.ID) {
			out = append(out, v.entry(d, false))
		}
	}
	return out
}

func (l *Library) list(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	parent := r.URL.Query().Get("parent")
	switch {
	case parent == "" && v.sc != nil:
		WriteJSON(w, http.StatusOK, v.entries(v.sc.Tops(v.s)))
		return nil
	case parent == "" || (parent == library.Trash && v.sc == nil):
	default:
		d, err := v.doc(parent)
		if err != nil {
			return err
		}
		if d.Type != library.TypeFolder {
			return Errorf(CodeBadRequest, "entry %s is not a folder", parent)
		}
	}
	WriteJSON(w, http.StatusOK, v.entries(v.s.Children(parent)))
	return nil
}

func (l *Library) get(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	d, err := v.doc(r.PathValue("id"))
	if err != nil {
		return err
	}
	w.Header().Set("ETag", `"`+d.Hash+`"`)
	WriteJSON(w, http.StatusOK, v.entry(d, true))
	return nil
}

func (l *Library) resolve(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	var names []string
	for n := range strings.SplitSeq(r.URL.Query().Get("path"), "/") {
		if n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return Errorf(CodeBadRequest, "the query path must name an entry, such as /Work/Paper")
	}
	// cur holds the IDs of the matches of the names so far. The ID
	// "trash" stands for the trash of an owner token.
	var cur []string
	rest := names
	if v.sc != nil {
		for _, d := range named(v.sc.Tops(v.s), names[0]) {
			cur = append(cur, d.ID)
		}
		rest = names[1:]
	} else {
		cur = []string{""}
	}
	for _, n := range rest {
		var next []string
		for _, id := range cur {
			if id == "" && n == library.Trash {
				next = append(next, library.Trash)
			}
			for _, d := range named(v.s.Children(id), n) {
				next = append(next, d.ID)
			}
		}
		cur = next
	}
	var found []*library.Doc
	for _, id := range cur {
		if d, ok := v.s.Doc(id); ok && v.visible(id) {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 0:
		return Errorf(CodeNotFound, "no entry has the path %s", r.URL.Query().Get("path"))
	case 1:
		w.Header().Set("ETag", `"`+found[0].Hash+`"`)
		WriteJSON(w, http.StatusOK, v.entry(found[0], true))
		return nil
	}
	idList := make([]string, len(found))
	for i, d := range found {
		idList[i] = d.ID
	}
	return Errorf(CodeConflict, "%d entries have the path %s: %s", len(found), r.URL.Query().Get("path"), strings.Join(idList, ", "))
}

func named(ds []*library.Doc, name string) []*library.Doc {
	var out []*library.Doc
	for _, d := range ds {
		if d.Name == name {
			out = append(out, d)
		}
	}
	return out
}

func (l *Library) search(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	q := r.URL.Query().Get("q")
	if q == "" {
		return Errorf(CodeBadRequest, "the query q must not be empty")
	}
	keep := func(d *library.Doc) bool { return v.visible(d.ID) }
	WriteJSON(w, http.StatusOK, v.entries(v.s.Search(q, SearchLimit, keep)))
	return nil
}

func (l *Library) content(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	d, err := v.doc(r.PathValue("id"))
	if err != nil {
		return err
	}
	media := map[string]string{library.TypePDF: "application/pdf", library.TypeEPUB: "application/epub+zip"}[d.Type]
	if media == "" {
		return Errorf(CodeNotFound, "entry %s is a %s and has no source file", d.ID, d.Type)
	}
	f, ok := d.File("." + d.Type)
	if !ok {
		return Errorf(CodeNotFound, "entry %s has no .%s file", d.ID, d.Type)
	}
	rc, size, err := l.Blobs.Get(r.Context(), mustNS(v.s.Account), f.Hash)
	if errors.Is(err, blob.ErrNotFound) {
		return Errorf(CodeNotFound, "the .%s file of entry %s is not in the blob store", d.Type, d.ID)
	}
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", media)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("ETag", `"`+d.Hash+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, rc)
	return nil
}

// maxContent is the maximum size of a .content file that the page list
// reads.
const maxContent = 4 << 20

func (l *Library) pages(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	d, err := v.doc(r.PathValue("id"))
	if err != nil {
		return err
	}
	ps, err := l.pageList(r.Context(), v.s.Account, d)
	if err != nil {
		return err
	}
	WriteJSON(w, http.StatusOK, map[string]any{"pages": ps})
	return nil
}

// pageList returns the pages of a document. A folder gives code
// not_found. A .content file that is not valid gives an internal error
// that names the document.
func (l *Library) pageList(ctx context.Context, account string, d *library.Doc) ([]pages.Page, error) {
	if d.Type == library.TypeFolder {
		return nil, Errorf(CodeNotFound, "entry %s is a folder and has no pages", d.ID)
	}
	var raw []byte
	if f, ok := d.File(".content"); ok {
		rc, _, err := l.Blobs.Get(ctx, mustNS(account), f.Hash)
		switch {
		case errors.Is(err, blob.ErrNotFound):
		case err != nil:
			return nil, err
		default:
			raw, err = io.ReadAll(io.LimitReader(rc, maxContent+1))
			_ = rc.Close()
			if err != nil {
				return nil, err
			}
			if len(raw) > maxContent {
				return nil, fmt.Errorf("the .content file of document %s is larger than %d bytes", d.ID, maxContent)
			}
		}
	}
	ps, err := pages.List(d.ID, raw, d.Files)
	if err != nil || len(ps) > 0 || d.Type != library.TypePDF {
		return ps, err
	}
	return l.pdfPages(ctx, account, d)
}

// pdfPages returns one page for each page of the PDF of a document whose
// .content has no page list. A PDF that the API uploaded has no page list
// until a tablet opens it. These pages have no ID and no .rm file. The
// count goes through the renderer, so it obeys the render limit and the
// cache keeps it for each PDF.
func (l *Library) pdfPages(ctx context.Context, account string, d *library.Doc) ([]pages.Page, error) {
	f, ok := d.File(".pdf")
	if !ok {
		return []pages.Page{}, nil
	}
	key := rendercache.Key(account, strconv.Itoa(render.Version), "pagecount", f.Hash)
	b, err := l.Render.image(ctx, account, key, func(ctx context.Context) ([]byte, error) {
		data, err := l.blob(ctx, mustNS(account), f.Hash, d.ID)
		if err != nil {
			return nil, err
		}
		pool, err := l.Render.pdfPool()
		if err != nil {
			return nil, err
		}
		n, err := pool.PageCount(ctx, data)
		if err != nil {
			return nil, err
		}
		return []byte(strconv.Itoa(n)), nil
	})
	if err != nil {
		return nil, fmt.Errorf("count the pages of the PDF of document %s: %w", d.ID, err)
	}
	n, err := strconv.Atoi(string(b))
	if err != nil || n < 0 {
		return nil, fmt.Errorf("the cached page count of the PDF of document %s is not a number", d.ID)
	}
	ps := make([]pages.Page, n)
	for i := range ps {
		ps[i] = pages.Page{Number: i + 1, PDFPage: new(i + 1)}
	}
	return ps, nil
}

func mustNS(account string) blob.Namespace {
	ns, err := blob.NewNamespace(account)
	if err != nil {
		panic("api: principal has an invalid account ID: " + account)
	}
	return ns
}

func (l *Library) archive(w http.ResponseWriter, r *http.Request) error {
	v, err := l.open(w, r)
	if err != nil {
		return err
	}
	d, err := v.doc(r.PathValue("id"))
	if err != nil {
		return err
	}
	if d.Type == library.TypeFolder {
		return Errorf(CodeNotFound, "entry %s is a folder and has no archive", d.ID)
	}
	ns := mustNS(v.s.Account)
	// Check each file before the first byte of the response.
	keys := make([]string, len(d.Files))
	for i, f := range d.Files {
		keys[i] = f.Hash
	}
	have, err := l.Blobs.Exists(r.Context(), ns, keys)
	if err != nil {
		return err
	}
	for _, f := range d.Files {
		// The names come from a tablet. A name such as ../x would make
		// an unsafe ZIP file for a client that extracts it.
		if !fs.ValidPath(f.Name) || f.Name == "." || strings.Contains(f.Name, `\`) {
			return Errorf(CodeConflict, "the file name %q of entry %s is not a safe path for an archive", f.Name, d.ID)
		}
		if !have[f.Hash] {
			return Errorf(CodeNotFound, "the file %s of entry %s is not in the blob store", f.Name, d.ID)
		}
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.ID + ".zip"}))
	w.Header().Set("ETag", `"`+d.Hash+`"`)
	w.WriteHeader(http.StatusOK)
	zw := zip.NewWriter(w)
	for _, f := range d.Files {
		if err := l.copyToZip(r, zw, ns, f.Name, f.Hash); err != nil {
			l.Log.Warn("archive stopped", "entry", d.ID, "file", f.Name, "err", err)
			return nil
		}
	}
	if err := zw.Close(); err != nil {
		l.Log.Warn("archive stopped", "entry", d.ID, "err", err)
	}
	return nil
}

func (l *Library) copyToZip(r *http.Request, zw *zip.Writer, ns blob.Namespace, name, hash string) error {
	rc, _, err := l.Blobs.Get(r.Context(), ns, hash)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()
	fw, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(fw, rc)
	return err
}

// run does a write operation and answers with the entry.
func (l *Library) run(w http.ResponseWriter, r *http.Request, status int, op libops.Op) error {
	req := opRequest(r)
	s, id, err := l.Ops.Run(r.Context(), req, op)
	if err != nil {
		return opError(err)
	}
	v := view{s: s, sc: req.Scope}
	w.Header().Set("RMHub-Generation", strconv.FormatUint(s.Generation, 10))
	d, ok := s.Doc(id)
	if !ok {
		return fmt.Errorf("entry %s is missing after the commit", id)
	}
	w.Header().Set("ETag", `"`+d.Hash+`"`)
	// A scoped token cannot see an entry after a trash. The answer then
	// has no path.
	WriteJSON(w, status, v.entry(d, v.visible(id)))
	return nil
}

// opRequest returns the write request of the principal of r.
func opRequest(r *http.Request) libops.Request {
	p := PrincipalFrom(r.Context())
	return libops.Request{
		Account: p.Account,
		Source:  rootlog.Source{Type: rootlog.SourceToken, ID: p.TokenID},
		Scope:   grants.FromChain(p.Chain),
		IfMatch: ifMatch(r),
	}
}

// ifMatch returns the hash of the If-Match header without quotes.
func ifMatch(r *http.Request) string {
	h := strings.TrimSpace(r.Header.Get("If-Match"))
	h = strings.TrimPrefix(h, "W/")
	return strings.Trim(h, `"`)
}

func opError(err error) error {
	var oe *libops.Error
	if !errors.As(err, &oe) {
		return err
	}
	code := map[libops.Kind]string{
		libops.KindBadRequest:   CodeBadRequest,
		libops.KindForbidden:    CodeForbidden,
		libops.KindNotFound:     CodeNotFound,
		libops.KindPrecondition: CodePreconditionFailed,
		libops.KindConflict:     CodeConflict,
	}[oe.Kind]
	return Errorf(code, "%s", oe.Msg)
}

type folderRequest struct {
	Parent string `json:"parent"`
	Name   string `json:"name"`
}

func (l *Library) createFolder(w http.ResponseWriter, r *http.Request) error {
	var req folderRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	return l.run(w, r, http.StatusCreated, libops.CreateFolder(ids.UUID(), req.Parent, req.Name))
}

type patchRequest struct {
	Name   *string `json:"name"`
	Parent *string `json:"parent"`
}

func (l *Library) patch(w http.ResponseWriter, r *http.Request) error {
	var req patchRequest
	if err := DecodeJSON(r, &req); err != nil {
		return err
	}
	return l.run(w, r, http.StatusOK, libops.Patch(r.PathValue("id"), req.Name, req.Parent))
}

func (l *Library) trash(w http.ResponseWriter, r *http.Request) error {
	return l.run(w, r, http.StatusOK, libops.Trash(r.PathValue("id")))
}

// epubMime is the content of the mimetype entry of an EPUB.
const epubMime = "application/epub+zip"

func (l *Library) upload(w http.ResponseWriter, r *http.Request) error {
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var fileType string
	switch media {
	case "application/pdf":
		fileType = library.TypePDF
	case epubMime:
		fileType = library.TypeEPUB
	default:
		return Errorf(CodeBadRequest, "the media type must be application/pdf or application/epub+zip")
	}
	q := r.URL.Query()
	name := q.Get("name")
	if strings.TrimSpace(name) == "" {
		return Errorf(CodeBadRequest, "the query name must not be empty")
	}
	if r.ContentLength > l.MaxUpload {
		return Errorf(CodeBadRequest, "the body is larger than %d bytes (api.max_upload_bytes)", l.MaxUpload)
	}
	// Check the access before the body goes to the blob store, so that a
	// token without write access cannot fill the store.
	if err := l.Ops.CheckNew(r.Context(), opRequest(r), q.Get("parent"), name); err != nil {
		return opError(err)
	}

	f, err := os.CreateTemp(l.TempDir, "upload-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(r.Body, l.MaxUpload+1))
	if err != nil {
		return Errorf(CodeBadRequest, "cannot read the request body: %v", err)
	}
	if n > l.MaxUpload {
		return Errorf(CodeBadRequest, "the body is larger than %d bytes (api.max_upload_bytes)", l.MaxUpload)
	}
	if err := checkUpload(f, n, fileType); err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	hash := hex.EncodeToString(h.Sum(nil))
	acct := PrincipalFrom(r.Context()).Account
	if err := l.Blobs.Put(r.Context(), mustNS(acct), hash, f); err != nil {
		return err
	}
	return l.run(w, r, http.StatusCreated, libops.Upload(ids.UUID(), q.Get("parent"), name, fileType, hash, n))
}

// checkUpload checks the magic bytes of a PDF and the mimetype entry of
// an EPUB.
func checkUpload(f *os.File, size int64, fileType string) error {
	if fileType == library.TypePDF {
		head := make([]byte, 5)
		if _, err := f.ReadAt(head, 0); err != nil || !bytes.Equal(head, []byte("%PDF-")) {
			return Errorf(CodeBadRequest, "the body is not a PDF file: it does not start with %%PDF-")
		}
		return nil
	}
	zr, err := zip.NewReader(f, size)
	if err != nil || len(zr.File) == 0 || zr.File[0].Name != "mimetype" {
		return Errorf(CodeBadRequest, "the body is not an EPUB file: the first ZIP entry must be mimetype")
	}
	rc, err := zr.File[0].Open()
	if err != nil {
		return Errorf(CodeBadRequest, "the body is not an EPUB file: %v", err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, int64(len(epubMime))+1))
	if err != nil || string(b) != epubMime {
		return Errorf(CodeBadRequest, "the body is not an EPUB file: the mimetype entry must be %s", epubMime)
	}
	return nil
}
