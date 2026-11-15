// Package libtest makes real trees in real stores for the tests of the
// library packages. A tablet makes the same blobs.
package libtest

import (
	"context"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/blob"
	blobfs "github.com/brianmcgee/rmhub/internal/blob/fs"
	"github.com/brianmcgee/rmhub/internal/hashtree"
	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/rootlog"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

// Counting is a blob store that counts the reads of each key.
type Counting struct {
	blob.Store
	mu    sync.Mutex
	reads map[string]int
}

// Get counts the read and reads the blob.
func (c *Counting) Get(ctx context.Context, ns blob.Namespace, key string) (io.ReadCloser, int64, error) {
	c.mu.Lock()
	c.reads[key]++
	c.mu.Unlock()
	return c.Store.Get(ctx, ns, key)
}

// Reads returns the keys that were read since the last call.
func (c *Counting) Reads() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	r := c.reads
	c.reads = map[string]int{}
	return r
}

// Env is one account with a blob store and a root log.
type Env struct {
	T       testing.TB
	Ctx     context.Context
	Blobs   *Counting
	Roots   *rootlog.Log
	Account string
	NS      blob.Namespace
	// Docs is the tree that the next Commit writes, by document ID.
	Docs map[string]hashtree.Entry
	// Files holds the file entries of each document by ID.
	Files map[string][]hashtree.Entry
	Gen   uint64
}

// New returns an Env on new stores in a temporary directory.
func New(t testing.TB) *Env {
	t.Helper()
	dir := t.TempDir()
	bs, err := blobfs.New(filepath.Join(dir, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := sqlite.Open(filepath.Join(dir, "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	acct := ids.NewAccountID()
	ns, _ := blob.NewNamespace(acct)
	return &Env{
		T: t, Ctx: context.Background(),
		Blobs: &Counting{Store: bs, reads: map[string]int{}},
		Roots: rootlog.New(st), Account: acct, NS: ns,
		Docs: map[string]hashtree.Entry{}, Files: map[string][]hashtree.Entry{},
	}
}

// On returns an Env on existing stores. Docs starts with the entries of
// the current root of the account, so a test can change a tree that an
// API write made.
func On(t testing.TB, blobs blob.Store, roots *rootlog.Log, account string) *Env {
	t.Helper()
	ns, err := blob.NewNamespace(account)
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{
		T: t, Ctx: context.Background(),
		Blobs: &Counting{Store: blobs, reads: map[string]int{}},
		Roots: roots, Account: account, NS: ns,
		Docs: map[string]hashtree.Entry{}, Files: map[string][]hashtree.Entry{},
	}
	s, err := library.New(roots, blobs).Current(e.Ctx, account)
	if err != nil {
		t.Fatal(err)
	}
	for _, re := range s.RootEntries() {
		e.Docs[re.Name] = re
	}
	e.Gen = s.Generation
	return e
}

// Meta returns a .metadata object like the tablet writes.
func Meta(name, typ, parent string) map[string]any {
	return map[string]any{
		"visibleName":      name,
		"type":             typ,
		"parent":           parent,
		"lastModified":     strconv.FormatInt(time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC).UnixMilli(), 10),
		"version":          float64(1),
		"pinned":           false,
		"synced":           true,
		"modified":         false,
		"deleted":          false,
		"metadatamodified": false,
	}
}

// Put writes a document with the metadata, the content and the extra
// files by extension, and adds it to Docs.
func (e *Env) Put(id string, meta, content map[string]any, extra map[string][]byte) hashtree.Entry {
	e.T.Helper()
	var files []hashtree.Entry
	add := func(f hashtree.Entry, err error) {
		if err != nil {
			e.T.Fatal(err)
		}
		files = append(files, f)
	}
	add(library.PutJSON(e.Ctx, e.Blobs, e.NS, id, ".metadata", meta))
	add(library.PutJSON(e.Ctx, e.Blobs, e.NS, id, ".content", content))
	for _, ext := range slices.Sorted(maps.Keys(extra)) {
		add(library.PutFile(e.Ctx, e.Blobs, e.NS, id, ext, extra[ext]))
	}
	d, err := library.PutDocument(e.Ctx, e.Blobs, e.NS, id, files)
	if err != nil {
		e.T.Fatal(err)
	}
	e.Docs[id] = d
	e.Files[id] = files
	return d
}

// Folder adds a folder.
func (e *Env) Folder(id, name, parent string) hashtree.Entry {
	return e.Put(id, Meta(name, "CollectionType", parent), map[string]any{}, nil)
}

// PDF adds a PDF document.
func (e *Env) PDF(id, name, parent string, pdf []byte) hashtree.Entry {
	return e.Put(id, Meta(name, "DocumentType", parent), map[string]any{"fileType": "pdf", "pageCount": float64(1)}, map[string][]byte{".pdf": pdf})
}

// Notebook adds a notebook with the pages.
func (e *Env) Notebook(id, name, parent string, pages int) hashtree.Entry {
	extra := map[string][]byte{}
	var list []any
	for i := range pages {
		pid := fmt.Sprintf("p%d", i)
		list = append(list, map[string]any{"id": pid})
		extra["/"+pid+".rm"] = fmt.Appendf(nil, "reMarkable .lines file, version=6 page %d", i)
	}
	return e.Put(id, Meta(name, "DocumentType", parent), map[string]any{"fileType": "notebook", "cPages": map[string]any{"pages": list}}, extra)
}

// Commit writes the root index of Docs and commits it as a device.
func (e *Env) Commit(schema int) rootlog.Entry {
	e.T.Helper()
	h, err := library.PutRoot(e.Ctx, e.Blobs, e.NS, schema, slices.Collect(maps.Values(e.Docs)))
	if err != nil {
		e.T.Fatal(err)
	}
	cur, err := e.Roots.Current(e.Ctx, e.Account)
	if err != nil {
		e.T.Fatal(err)
	}
	ent, err := e.Roots.Commit(e.Ctx, e.Account, cur.Generation, h, rootlog.Source{Type: rootlog.SourceDevice, ID: "RM110-1"}, true)
	if err != nil {
		e.T.Fatal(err)
	}
	e.Gen = ent.Generation
	return ent
}
