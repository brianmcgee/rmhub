package app

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/rootlog"
)

// tree returns a tree that writes documents as a tablet does and
// commits them as a device. It starts with the current root.
func (e *env) tree(account string) *libtest.Env {
	e.t.Helper()
	return libtest.On(e.t, e.app.Library.Blobs, e.app.Roots, account)
}

type entry struct {
	ID, Name, Type, Parent, Hash, Path string
	Trashed                            bool
	Modified                           *time.Time
	PageCount                          int
}

type hresp struct {
	status int
	header http.Header
	body   []byte
}

func (r hresp) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
}

func (r hresp) entry(t *testing.T) entry {
	t.Helper()
	var en entry
	r.json(t, &en)
	return en
}

func (r hresp) names(t *testing.T) string {
	t.Helper()
	var es []entry
	r.json(t, &es)
	var out []string
	for _, en := range es {
		out = append(out, en.Name)
	}
	return strings.Join(out, ",")
}

// req sends an API request with headers and a raw body.
func (e *env) req(method, path, token string, body []byte, hdr ...string) hresp {
	e.t.Helper()
	r, err := http.NewRequest(method, e.api.URL+path, bytes.NewReader(body))
	if err != nil {
		e.t.Fatal(err)
	}
	r.Header.Set("Authorization", "Bearer "+token)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return hresp{status: res.StatusCode, header: res.Header, body: b}
}

func (e *env) jreq(method, path, token string, body any, hdr ...string) hresp {
	e.t.Helper()
	b, _ := json.Marshal(body)
	return e.req(method, path, token, b, append([]string{"Content-Type", "application/json"}, hdr...)...)
}

func TestLibraryRead(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("work", "Work", "")
	pdf := []byte("%PDF-1.7 paper body")
	tr.PDF("paper", "Paper", "work", pdf)
	tr.PDF("n1", "Notes", "", []byte("%PDF-1.7 a"))
	tr.PDF("n2", "Notes", "", []byte("%PDF-1.7 b"))
	tr.Folder("old", "Old", library.Trash)
	tr.PDF("inold", "Draft", "old", []byte("%PDF-1.7 c"))
	tr.Put("nb", libtest.Meta("Sketch", "DocumentType", "work"),
		map[string]any{"fileType": "notebook", "cPages": map[string]any{"pages": []any{map[string]any{"id": "p1"}, map[string]any{"id": "p2"}}}},
		map[string][]byte{"/p1.rm": []byte("page 1"), "/p2.rm": []byte("page 2")})
	gen := tr.Commit(4).Generation

	r := e.req("GET", "/api/v1/library", owner, nil)
	if r.status != 200 || r.names(t) != "Notes,Notes,Work" || r.header.Get("RMHub-Generation") != fmt.Sprint(gen) {
		t.Fatalf("top level: %d %s %v", r.status, r.body, r.header)
	}
	if r := e.req("GET", "/api/v1/library?parent=trash", owner, nil); r.names(t) != "Old" {
		t.Fatalf("trash: %s", r.body)
	}
	if r := e.req("GET", "/api/v1/library?parent=work", owner, nil); r.names(t) != "Paper,Sketch" {
		t.Fatalf("folder: %s", r.body)
	}
	if r := e.req("GET", "/api/v1/library?parent=nothing", owner, nil); r.status != 404 {
		t.Fatalf("unknown folder: %d", r.status)
	}

	r = e.req("GET", "/api/v1/library/paper", owner, nil)
	en := r.entry(t)
	if r.status != 200 || en.Path != "/Work/Paper" || en.Type != "pdf" || en.Name != "Paper" || r.header.Get("ETag") != `"`+tr.Docs["paper"].Hash+`"` {
		t.Fatalf("read: %d %s %v", r.status, r.body, r.header)
	}
	if en := e.req("GET", "/api/v1/library/inold", owner, nil).entry(t); !en.Trashed || en.Path != "/trash/Old/Draft" {
		t.Fatalf("trashed entry %+v", en)
	}
	if en := e.req("GET", "/api/v1/library/nb", owner, nil).entry(t); en.Type != "notebook" || en.PageCount != 2 {
		t.Fatalf("notebook %+v", en)
	}

	if r := e.req("GET", "/api/v1/library/resolve?path=/Work/Paper", owner, nil); r.entry(t).ID != "paper" {
		t.Fatalf("resolve: %s", r.body)
	}
	if r := e.req("GET", "/api/v1/library/resolve?path=/trash/Old/Draft", owner, nil); r.entry(t).ID != "inold" {
		t.Fatalf("resolve trash: %s", r.body)
	}
	r = e.req("GET", "/api/v1/library/resolve?path=/Notes", owner, nil)
	if r.status != 409 || !bytes.Contains(r.body, []byte("n1")) || !bytes.Contains(r.body, []byte("n2")) {
		t.Fatalf("two entries: %d %s", r.status, r.body)
	}
	if r := e.req("GET", "/api/v1/library/resolve?path=/Work/None", owner, nil); r.status != 404 {
		t.Fatalf("resolve missing: %d", r.status)
	}
	if r := e.req("GET", "/api/v1/library/search?q=NOTE", owner, nil); r.names(t) != "Notes,Notes" {
		t.Fatalf("search: %s", r.body)
	}
	if r := e.req("GET", "/api/v1/library/search?q=draft", owner, nil); r.names(t) != "" {
		t.Fatalf("search found a trashed entry: %s", r.body)
	}

	r = e.req("GET", "/api/v1/library/paper/content", owner, nil)
	if r.status != 200 || !bytes.Equal(r.body, pdf) || r.header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("content: %d %v", r.status, r.header)
	}
	for _, id := range []string{"nb", "work"} {
		if r := e.req("GET", "/api/v1/library/"+id+"/content", owner, nil); r.status != 404 {
			t.Fatalf("content of %s: %d", id, r.status)
		}
	}
	r = e.req("GET", "/api/v1/library/nb/archive", owner, nil)
	if r.status != 200 || r.header.Get("Content-Type") != "application/zip" {
		t.Fatalf("archive: %d %s", r.status, r.body)
	}
	zr, err := zip.NewReader(bytes.NewReader(r.body), int64(len(r.body)))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	if strings.Join(names, " ") != "nb.content nb.metadata nb/p1.rm nb/p2.rm" {
		t.Fatalf("archive files %v", names)
	}
	if r := e.req("GET", "/api/v1/library/work/archive", owner, nil); r.status != 404 {
		t.Fatalf("archive of a folder: %d", r.status)
	}
}

func epub() []byte {
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.CreateHeader(&zip.FileHeader{Name: "mimetype", Method: zip.Store})
	_, _ = io.WriteString(w, "application/epub+zip")
	w, _ = zw.Create("META-INF/container.xml")
	_, _ = io.WriteString(w, "<container/>")
	_ = zw.Close()
	return b.Bytes()
}

func TestLibraryWrites(t *testing.T) {
	e := newEnvWith(t, true, "[api]\nmax_upload_bytes = 4096\n")
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.PDF("d", "D", "", []byte("%PDF-1.7 d"))
	tr.Commit(4)
	user := e.device(owner, "RM110-1")
	ws := e.listen(user, true)

	r := e.jreq("POST", "/api/v1/library/folders", owner, map[string]string{"name": "Inbox"})
	inbox := r.entry(t)
	if r.status != 201 || inbox.Type != "folder" || len(inbox.ID) != 36 || inbox.Path != "/Inbox" {
		t.Fatalf("folder: %d %s", r.status, r.body)
	}
	// The tablet hears of the API write.
	msg, ok := ws.next(5 * time.Second)
	if !ok || !bytes.Contains(msg, []byte("SyncComplete")) {
		t.Fatalf("no SyncComplete after an API write: %s", msg)
	}

	pdf := []byte("%PDF-1.7 uploaded")
	r = e.req("POST", "/api/v1/library/documents?name=Paper&parent="+inbox.ID, owner, pdf, "Content-Type", "application/pdf")
	paper := r.entry(t)
	if r.status != 201 || paper.Type != "pdf" || paper.Path != "/Inbox/Paper" {
		t.Fatalf("upload: %d %s", r.status, r.body)
	}
	if r := e.req("GET", "/api/v1/library/"+paper.ID+"/content", owner, nil); !bytes.Equal(r.body, pdf) {
		t.Fatalf("download after upload: %s", r.body)
	}
	r = e.req("POST", "/api/v1/library/documents?name=Book", owner, epub(), "Content-Type", "application/epub+zip")
	if r.status != 201 || r.entry(t).Type != "epub" {
		t.Fatalf("epub: %d %s", r.status, r.body)
	}
	for name, c := range map[string]struct {
		body  []byte
		media string
	}{
		"text as PDF":    {[]byte("hello"), "application/pdf"},
		"PDF as EPUB":    {pdf, "application/epub+zip"},
		"text media":     {pdf, "text/plain"},
		"above limit":    {append([]byte("%PDF-"), make([]byte, 5000)...), "application/pdf"},
		"zip, no mime":   {func() []byte { b := epub(); return bytes.ReplaceAll(b, []byte("mimetype"), []byte("mimetypx")) }(), "application/epub+zip"},
		"text with name": {[]byte("not a pdf"), "application/pdf"},
	} {
		if r := e.req("POST", "/api/v1/library/documents?name=X", owner, c.body, "Content-Type", c.media); r.status != 400 {
			t.Errorf("%s: %d %s", name, r.status, r.body)
		}
	}

	// Rename and move with If-Match.
	tag := e.req("GET", "/api/v1/library/d", owner, nil).header.Get("ETag")
	r = e.jreq("PATCH", "/api/v1/library/d", owner, map[string]string{"name": "D2", "parent": inbox.ID}, "If-Match", tag)
	if en := r.entry(t); r.status != 200 || en.Name != "D2" || en.Parent != inbox.ID || en.Path != "/Inbox/D2" {
		t.Fatalf("patch: %d %s", r.status, r.body)
	}
	// The tablet changes D after the read.
	tag = r.header.Get("ETag")
	tr = e.tree(acct)
	tr.PDF("d", "D tablet", inbox.ID, []byte("%PDF-1.7 d"))
	tr.Commit(4)
	if r := e.jreq("PATCH", "/api/v1/library/d", owner, map[string]string{"name": "D3"}, "If-Match", tag); r.status != 412 {
		t.Fatalf("stale If-Match: %d %s", r.status, r.body)
	}
	if r := e.jreq("PATCH", "/api/v1/library/"+inbox.ID, owner, map[string]string{"parent": "trash"}); r.status != 400 {
		t.Fatalf("move into trash: %d", r.status)
	}
	r = e.req("POST", "/api/v1/library/d/trash", owner, nil)
	if en := r.entry(t); r.status != 200 || !en.Trashed || en.Parent != "trash" {
		t.Fatalf("trash: %d %s", r.status, r.body)
	}
	// The API commit records the token as the source.
	cur, _ := e.app.Roots.Current(context.Background(), acct)
	if cur.Source.Type != rootlog.SourceToken {
		t.Fatalf("source %+v", cur.Source)
	}
}

// scoped creates a token with grants with the token tok.
func (e *env) scoped(tok string, body map[string]any) hresp {
	return e.jreq("POST", "/api/v1/account/tokens", tok, body)
}

func grantsOf(pairs ...string) []map[string]string {
	var out []map[string]string
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]string{"folder": pairs[i], "access": pairs[i+1]})
	}
	return out
}

func tokenText(t *testing.T, r hresp) (id, text string) {
	t.Helper()
	if r.status != 201 {
		t.Fatalf("create token: %d %s", r.status, r.body)
	}
	var v struct{ ID, Token string }
	r.json(t, &v)
	return v.ID, v.Token
}

func TestScopedTokens(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("agent", "Agent", "")
	tr.Folder("sub", "Sub", "agent")
	tr.PDF("doc", "Doc", "agent", []byte("%PDF-1.7 a"))
	tr.Folder("ref", "Ref", "")
	tr.PDF("refdoc", "RefDoc", "ref", []byte("%PDF-1.7 r"))
	tr.PDF("private", "Private", "", []byte("%PDF-1.7 p"))
	tr.Folder("gone", "Gone", library.Trash)
	tr.Commit(4)

	for name, g := range map[string][]map[string]string{
		"document":       grantsOf("doc", "read"),
		"trashed folder": grantsOf("gone", "read"),
		"unknown folder": grantsOf("nothing", "read"),
		"bad access":     grantsOf("agent", "admin"),
	} {
		if r := e.scoped(owner, map[string]any{"label": "x", "grants": g}); r.status != 400 {
			t.Errorf("grant on %s: %d %s", name, r.status, r.body)
		}
	}
	expiry := e.now.Add(24 * time.Hour)
	tid, tok := tokenText(t, e.scoped(owner, map[string]any{"label": "agent", "expiresAt": expiry, "grants": grantsOf("agent", "write", "ref", "read")}))

	if r := e.req("GET", "/api/v1/library", tok, nil); r.names(t) != "Agent,Ref" {
		t.Fatalf("scoped top level: %s", r.body)
	}
	if r := e.req("GET", "/api/v1/library/private", tok, nil); r.status != 404 {
		t.Fatalf("entry outside the grants: %d", r.status)
	}
	if r := e.req("GET", "/api/v1/library/search?q=doc", tok, nil); r.names(t) != "Doc,RefDoc" {
		t.Fatalf("scoped search: %s", r.body)
	}
	if en := e.req("GET", "/api/v1/library/doc", tok, nil).entry(t); en.Path != "/Agent/Doc" {
		t.Fatalf("scoped path %q", en.Path)
	}
	if en := e.req("GET", "/api/v1/library/resolve?path=/Ref/RefDoc", tok, nil).entry(t); en.ID != "refdoc" {
		t.Fatalf("scoped resolve %+v", en)
	}
	if r := e.req("GET", "/api/v1/library/resolve?path=/Private", tok, nil); r.status != 404 {
		t.Fatalf("scoped resolve outside: %d", r.status)
	}
	if r := e.jreq("PATCH", "/api/v1/library/agent", tok, map[string]string{"name": "X"}); r.status != 403 {
		t.Fatalf("rename grant folder: %d", r.status)
	}
	if r := e.jreq("PATCH", "/api/v1/library/doc", tok, map[string]string{"parent": "ref"}); r.status != 403 {
		t.Fatalf("move into read-only: %d", r.status)
	}
	if r := e.jreq("POST", "/api/v1/library/folders", tok, map[string]string{"parent": "agent", "name": "Out"}); r.status != 201 {
		t.Fatalf("create in grant: %d %s", r.status, r.body)
	}
	for _, p := range []string{"POST /api/v1/account/pairing-codes", "GET /api/v1/account/devices", "GET /api/v1/account", "GET /api/v1/admin/users"} {
		m, path, _ := strings.Cut(p, " ")
		if r := e.req(m, path, tok, nil); r.status != 403 {
			t.Errorf("%s with a scoped token: %d", p, r.status)
		}
	}

	// Child tokens.
	if r := e.scoped(tok, map[string]any{"grants": grantsOf("ref", "write")}); r.status != 403 {
		t.Fatalf("wider grant: %d", r.status)
	}
	if r := e.scoped(tok, map[string]any{"grants": grantsOf("private", "read")}); r.status != 403 {
		t.Fatalf("grant outside: %d", r.status)
	}
	if r := e.scoped(tok, map[string]any{"label": "owner"}); r.status != 403 {
		t.Fatalf("token without grants: %d", r.status)
	}
	if r := e.scoped(tok, map[string]any{"expiresAt": expiry.Add(time.Hour), "grants": grantsOf("sub", "read")}); r.status != 403 {
		t.Fatalf("later expiry: %d", r.status)
	}
	cid, child := tokenText(t, e.scoped(tok, map[string]any{"grants": grantsOf("sub", "read")}))
	// The child takes the expiry of the parent.
	var kids []struct {
		ID        string
		ExpiresAt *time.Time
	}
	e.req("GET", "/api/v1/account/tokens", tok, nil).json(t, &kids)
	if len(kids) != 1 || kids[0].ID != cid || kids[0].ExpiresAt == nil || !kids[0].ExpiresAt.Equal(expiry) {
		t.Fatalf("child token %+v", kids)
	}
	if r := e.req("GET", "/api/v1/library", child, nil); r.names(t) != "Sub" {
		t.Fatalf("child top level: %s", r.body)
	}
	if r := e.req("GET", "/api/v1/library/doc", child, nil); r.status != 404 {
		t.Fatalf("child sees the parent grant: %d", r.status)
	}
	// The chain has at most 4 scoped tokens.
	last := child
	for i := range 3 {
		r := e.scoped(last, map[string]any{"grants": grantsOf("sub", "read")})
		if i == 2 {
			if r.status != 403 {
				t.Fatalf("fifth scoped token: %d", r.status)
			}
			break
		}
		_, last = tokenText(t, r)
	}

	// A scoped token lists only its descendants.
	var list []struct{ ID, Parent string }
	e.req("GET", "/api/v1/account/tokens", tok, nil).json(t, &list)
	if len(list) != 3 || list[0].ID == tid {
		t.Fatalf("scoped list %+v", list)
	}
	if r := e.req("DELETE", "/api/v1/account/tokens/"+tid, child, nil); r.status != 404 {
		t.Fatalf("child revokes its parent: %d", r.status)
	}

	// Revoke the parent: the child is gone.
	if r := e.req("DELETE", "/api/v1/account/tokens/"+tid, owner, nil); r.status != 204 {
		t.Fatalf("revoke: %d", r.status)
	}
	if r := e.req("GET", "/api/v1/library", child, nil); r.status != 401 {
		t.Fatalf("child after parent revocation: %d", r.status)
	}
	e.req("GET", "/api/v1/account/tokens", owner, nil).json(t, &list)
	for _, tk := range list {
		if tk.ID == tid || tk.ID == cid {
			t.Fatalf("revoked token in the list: %+v", list)
		}
	}
}

// sse reads the events of a change feed.
type sse struct {
	events chan sseEvent
	done   chan struct{}
	cancel func()
}

type sseEvent struct {
	ID   string
	Data struct {
		Generation uint64
		Source     struct{ Type, ID string }
		Changes    []struct {
			ID, Op string
			Entry  *entry
		}
	}
}

func (e *env) feed(token string, hdr ...string) (*sse, int) {
	e.t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r, _ := http.NewRequestWithContext(ctx, "GET", e.api.URL+"/api/v1/changes", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	for i := 0; i+1 < len(hdr); i += 2 {
		r.Header.Set(hdr[i], hdr[i+1])
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		e.t.Fatal(err)
	}
	if res.StatusCode != 200 {
		cancel()
		_ = res.Body.Close()
		return nil, res.StatusCode
	}
	s := &sse{events: make(chan sseEvent, 1024), done: make(chan struct{}), cancel: cancel}
	e.t.Cleanup(cancel)
	go func() {
		defer close(s.done)
		defer func() { _ = res.Body.Close() }()
		sc := bufio.NewScanner(res.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "id: "):
				ev.ID = line[4:]
			case strings.HasPrefix(line, "data: "):
				_ = json.Unmarshal([]byte(line[6:]), &ev.Data)
			case line == "" && ev.ID != "":
				s.events <- ev
				ev = sseEvent{}
			}
		}
	}()
	return s, 200
}

func (s *sse) next(t *testing.T) sseEvent {
	t.Helper()
	select {
	case ev := <-s.events:
		return ev
	case <-time.After(10 * time.Second):
		t.Fatal("no feed event")
		return sseEvent{}
	}
}

func TestChangeFeed(t *testing.T) {
	e := newEnv(t, true)
	e.app.Library.Feed.TokenCheck = 50 * time.Millisecond
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("f", "F", "")
	tr.PDF("d", "D", "f", []byte("%PDF-1.7 d"))
	tr.PDF("x", "X", "", []byte("%PDF-1.7 x"))
	g1 := tr.Commit(4).Generation

	if _, status := e.feed(owner, "Last-Event-ID", fmt.Sprint(g1+1)); status != 400 {
		t.Fatalf("after above the current generation: %d", status)
	}
	f, _ := e.feed(owner)
	_, scopedTok := tokenText(t, e.scoped(owner, map[string]any{"grants": grantsOf("f", "read")}))
	sf, _ := e.feed(scopedTok)

	tr.PDF("d", "D", library.Trash, []byte("%PDF-1.7 d"))
	g2 := tr.Commit(4).Generation
	ev := f.next(t)
	if ev.ID != fmt.Sprint(g2) || ev.Data.Generation != g2 || len(ev.Data.Changes) != 1 || ev.Data.Changes[0].Op != "trashed" || ev.Data.Source.Type != "device" {
		t.Fatalf("event %+v", ev)
	}
	if ev := sf.next(t); ev.Data.Changes[0].ID != "d" || ev.Data.Changes[0].Op != "removed" || ev.Data.Changes[0].Entry != nil {
		t.Fatalf("scoped event %+v", ev)
	}
	// A change outside the grant gives the scoped feed no event.
	tr.PDF("x", "X2", "", []byte("%PDF-1.7 x"))
	g3 := tr.Commit(4).Generation
	if ev := f.next(t); ev.Data.Generation != g3 {
		t.Fatalf("event %+v", ev)
	}

	// Reconnect after g1: the feed sends g2 and g3 in order.
	r, _ := e.feed(owner, "Last-Event-ID", fmt.Sprint(g1))
	if a, b := r.next(t), r.next(t); a.ID != fmt.Sprint(g2) || b.ID != fmt.Sprint(g3) {
		t.Fatalf("replay %s %s", a.ID, b.ID)
	}

	// Many commits overflow the broker buffer. The feed sends each one.
	e.app.Library.Feed.Poll = 50 * time.Millisecond
	for i := range rootlog.BufferSize * 2 {
		tr.PDF("x", fmt.Sprintf("X%d", i), "", []byte("%PDF-1.7 x"))
		tr.Commit(4)
	}
	for i := range rootlog.BufferSize * 2 {
		if ev := f.next(t); ev.Data.Generation != g3+uint64(i)+1 {
			t.Fatalf("event %d has generation %d", i, ev.Data.Generation)
		}
	}
	if ev := sf.events; len(ev) != 0 {
		t.Fatalf("scoped feed has %d events for changes outside the grant", len(ev))
	}

	// Revocation ends the stream.
	var list []struct{ ID string }
	e.req("GET", "/api/v1/account/tokens", owner, nil).json(t, &list)
	for _, tk := range list {
		e.req("DELETE", "/api/v1/account/tokens/"+tk.ID, owner, nil)
	}
	for _, s := range []*sse{f, sf} {
		select {
		case <-s.done:
		case <-time.After(5 * time.Second):
			t.Fatal("feed did not end after revocation")
		}
	}
}

// TestUploadChecksAccessFirst makes sure that an upload that the token
// cannot do leaves no blob in the store.
func TestUploadChecksAccessFirst(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("agent", "Agent", "")
	tr.PDF("d", "D", "", []byte("%PDF-1.7 d"))
	tr.Commit(4)
	_, tok := tokenText(t, e.scoped(owner, map[string]any{"label": "a", "grants": grantsOf("agent", "read")}))

	pdf := []byte("%PDF-1.7 not permitted")
	sum := sha256.Sum256(pdf)
	for _, tc := range []struct {
		name, tok, query string
		status           int
	}{
		{"read-only folder", tok, "name=X&parent=agent", 403},
		{"top level with a scoped token", tok, "name=X", 403},
		{"parent that is not a folder", owner, "name=X&parent=d", 400},
		{"parent that does not exist", owner, "name=X&parent=none", 404},
		{"name with a slash", owner, "name=a%2Fb", 400},
	} {
		r := e.req("POST", "/api/v1/library/documents?"+tc.query, tc.tok, pdf, "Content-Type", "application/pdf")
		if r.status != tc.status {
			t.Errorf("%s: %d %s", tc.name, r.status, r.body)
		}
	}
	ns, err := blob.NewNamespace(acct)
	if err != nil {
		t.Fatal(err)
	}
	rc, _, err := e.app.Library.Blobs.Get(context.Background(), ns, hex.EncodeToString(sum[:]))
	if err == nil {
		_ = rc.Close()
	}
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("the refused upload is in the blob store: %v", err)
	}
}
