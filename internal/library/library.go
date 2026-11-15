// Package library reads the tree of folders and documents of one root
// generation.
//
// A snapshot holds each document of a root index with its parsed
// .metadata and .content. Blobs never change, so the builder keeps parsed
// metadata in an LRU cache by blob hash, and a new snapshot reuses each
// document whose index hash did not change.
package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/hashtree"
	"github.com/brianmcgee/rmhub/internal/lru"
	"github.com/brianmcgee/rmhub/internal/rootlog"
)

// Entry types.
const (
	TypeFolder   = "folder"
	TypePDF      = "pdf"
	TypeEPUB     = "epub"
	TypeNotebook = "notebook"
)

// Trash is the parent ID of a trashed entry.
const Trash = "trash"

const (
	// maxIndex is the maximum size of an index blob.
	maxIndex = 4 << 20
	// maxMeta is the maximum size of a .metadata or .content blob.
	maxMeta = 4 << 20
	// workers is the number of parallel document loads.
	workers = 16
)

// Entry is the API view of a document or a folder.
type Entry struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	Parent    string     `json:"parent"`
	Trashed   bool       `json:"trashed"`
	Modified  *time.Time `json:"modified"`
	Hash      string     `json:"hash"`
	PageCount int        `json:"pageCount"`
}

// Doc is one document of a root index. A Doc is not changed after the
// snapshot is built; copy Meta and Content before an edit.
type Doc struct {
	Entry
	// Index is the entry of the document in the root index.
	Index hashtree.Entry
	// Files are the entries of the document index.
	Files []hashtree.Entry
	// Meta and Content are the parsed .metadata and .content. They are
	// nil if the file is missing or is not a JSON object.
	Meta    map[string]any
	Content map[string]any
	// Visible is false for a deleted document and for a document without
	// .metadata.
	Visible bool
}

// File returns the index entry of the file with the extension ext, such
// as ".pdf".
func (d *Doc) File(ext string) (hashtree.Entry, bool) {
	for _, f := range d.Files {
		if f.Name == d.ID+ext {
			return f, true
		}
	}
	return hashtree.Entry{}, false
}

// Snapshot is the library of one root generation.
type Snapshot struct {
	Account    string
	Generation uint64
	// RootHash is empty at generation 0.
	RootHash string
	// Schema is the schema of the root index, or 0 at generation 0.
	Schema int
	Source rootlog.Source

	// docs holds each document of the root index by ID.
	docs     map[string]*Doc
	children map[string][]*Doc
}

// Doc returns the visible document with the ID.
func (s *Snapshot) Doc(id string) (*Doc, bool) {
	d, ok := s.docs[id]
	if !ok || !d.Visible {
		return nil, false
	}
	return d, true
}

// Docs returns each document of the root index, also the documents that
// are not visible, in no order.
func (s *Snapshot) Docs() []*Doc {
	out := make([]*Doc, 0, len(s.docs))
	for _, d := range s.docs {
		out = append(out, d)
	}
	return out
}

// Len returns the number of documents of the root index.
func (s *Snapshot) Len() int { return len(s.docs) }

// Children returns the visible entries with the parent ID, sorted by name
// and then by ID.
func (s *Snapshot) Children(parent string) []*Doc { return s.children[parent] }

// IsAncestor tells if the visible folder a is d or an ancestor of d.
func (s *Snapshot) IsAncestor(a, d string) bool {
	for range len(s.docs) + 1 {
		if d == a {
			return true
		}
		doc, ok := s.Doc(d)
		if !ok {
			return false
		}
		d = doc.Parent
	}
	return false
}

// Path returns the names from the top level to the entry. A trashed path
// starts with /trash. Top lets a caller stop at another top entry: the
// walk stops at the first ancestor (or the entry itself) for which top
// returns true, and the path starts with its name.
func (s *Snapshot) Path(id string, top func(string) bool) string {
	var names []string
	cur := id
	for range len(s.docs) + 1 {
		d, ok := s.Doc(cur)
		if !ok {
			break
		}
		names = append(names, d.Name)
		if top != nil && top(cur) {
			break
		}
		if d.Parent == Trash && top == nil {
			names = append(names, Trash)
			break
		}
		cur = d.Parent
	}
	slices.Reverse(names)
	return "/" + strings.Join(names, "/")
}

// Search returns at most limit visible entries that are not trashed and
// whose name contains q, without regard to case. keep filters the
// entries, or is nil.
func (s *Snapshot) Search(q string, limit int, keep func(*Doc) bool) []*Doc {
	q = strings.ToLower(q)
	var out []*Doc
	for _, d := range s.docs {
		if d.Visible && !d.Trashed && strings.Contains(strings.ToLower(d.Name), q) && (keep == nil || keep(d)) {
			out = append(out, d)
		}
	}
	sortDocs(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func sortDocs(ds []*Doc) {
	slices.SortFunc(ds, func(a, b *Doc) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
}

// RootEntries returns the entries of the root index.
func (s *Snapshot) RootEntries() []hashtree.Entry {
	out := make([]hashtree.Entry, 0, len(s.docs))
	for _, d := range s.docs {
		out = append(out, d.Index)
	}
	return out
}

// Builder reads snapshots from the blob store.
type Builder struct {
	blobs blob.Store
	meta  *lru.Cache[string, map[string]any]
}

// NewBuilder returns a builder with a cache of parsed metadata for
// cacheSize blobs.
func NewBuilder(blobs blob.Store, cacheSize int) *Builder {
	return &Builder{blobs: blobs, meta: lru.New[string, map[string]any](cacheSize)}
}

// Build reads the snapshot of the root log entry e. It reuses the
// documents of prev whose index hash did not change. prev can be nil.
func (b *Builder) Build(ctx context.Context, account string, e rootlog.Entry, prev *Snapshot) (*Snapshot, error) {
	s := &Snapshot{Account: account, Generation: e.Generation, RootHash: e.Hash, Source: e.Source, docs: map[string]*Doc{}}
	if e.Generation == 0 {
		s.link()
		return s, nil
	}
	ns, err := blob.NewNamespace(account)
	if err != nil {
		return nil, err
	}
	raw, err := b.read(ctx, ns, e.Hash, maxIndex)
	if err != nil {
		return nil, fmt.Errorf("root index %s: %w", e.Hash, err)
	}
	root, err := hashtree.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("root index %s: %w", e.Hash, err)
	}
	s.Schema = root.Schema

	var load []hashtree.Entry
	for _, re := range root.Entries {
		if prev != nil {
			if old, ok := prev.docs[re.Name]; ok && old.Index.Hash == re.Hash {
				d := *old
				d.Index = re
				s.docs[re.Name] = &d
				continue
			}
		}
		load = append(load, re)
	}

	docs := make([]*Doc, len(load))
	errs := make([]error, len(load))
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	for i, re := range load {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			docs[i], errs[i] = b.loadDoc(ctx, ns, re)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	for _, d := range docs {
		s.docs[d.ID] = d
	}
	s.link()
	return s, nil
}

// link sets the trashed flags and the child lists.
func (s *Snapshot) link() {
	s.children = map[string][]*Doc{}
	for _, d := range s.docs {
		if !d.Visible {
			continue
		}
		d.Trashed = s.trashed(d)
		s.children[d.Parent] = append(s.children[d.Parent], d)
	}
	for _, c := range s.children {
		sortDocs(c)
	}
}

func (s *Snapshot) trashed(d *Doc) bool {
	cur := d
	for range len(s.docs) + 1 {
		if cur.Parent == Trash {
			return true
		}
		next, ok := s.Doc(cur.Parent)
		if !ok {
			return false
		}
		cur = next
	}
	return false
}

func (b *Builder) read(ctx context.Context, ns blob.Namespace, hash string, limit int64) ([]byte, error) {
	rc, size, err := b.blobs.Get(ctx, ns, hash)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	if size > limit {
		return nil, fmt.Errorf("blob %s is larger than %d bytes", hash, limit)
	}
	return io.ReadAll(io.LimitReader(rc, limit))
}

// loadDoc reads the document index and the metadata of a root entry. A
// missing blob makes the document invisible, so one broken document does
// not hide the library.
func (b *Builder) loadDoc(ctx context.Context, ns blob.Namespace, re hashtree.Entry) (*Doc, error) {
	d := &Doc{Index: re}
	d.ID = re.Name
	d.Hash = re.Hash
	raw, err := b.read(ctx, ns, re.Hash, maxIndex)
	if errors.Is(err, blob.ErrNotFound) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	ix, err := hashtree.Parse(raw)
	if err != nil {
		return d, nil
	}
	d.Files = ix.Entries
	if f, ok := d.File(".metadata"); ok {
		if d.Meta, err = b.json(ctx, ns, f.Hash); err != nil {
			return nil, err
		}
	}
	if f, ok := d.File(".content"); ok {
		if d.Content, err = b.json(ctx, ns, f.Hash); err != nil {
			return nil, err
		}
	}
	fill(d)
	return d, nil
}

// json reads a JSON object blob through the cache. A missing blob or a
// blob that is not a JSON object gives nil.
func (b *Builder) json(ctx context.Context, ns blob.Namespace, hash string) (map[string]any, error) {
	key := ns.String() + "/" + hash
	if m, ok := b.meta.Get(key); ok {
		return m, nil
	}
	raw, err := b.read(ctx, ns, hash, maxMeta)
	if errors.Is(err, blob.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		m = nil
	}
	b.meta.Put(key, m)
	return m, nil
}

// fill sets the entry fields from Meta and Content.
func fill(d *Doc) {
	if d.Meta == nil {
		return
	}
	if del, _ := d.Meta["deleted"].(bool); del {
		return
	}
	d.Visible = true
	d.Name, _ = d.Meta["visibleName"].(string)
	d.Parent, _ = d.Meta["parent"].(string)
	if ms, ok := d.Meta["lastModified"].(string); ok {
		if n, err := strconv.ParseInt(ms, 10, 64); err == nil {
			t := time.UnixMilli(n).UTC()
			d.Modified = &t
		}
	}
	if t, _ := d.Meta["type"].(string); t == "CollectionType" {
		d.Type = TypeFolder
		return
	}
	switch ft, _ := d.Content["fileType"].(string); ft {
	case "pdf":
		d.Type = TypePDF
	case "epub":
		d.Type = TypeEPUB
	default:
		d.Type = TypeNotebook
	}
	d.PageCount = pageCount(d.Content)
}

// pageCount reads the page count of .content: the field pageCount, or the
// length of pages, or the pages of cPages that are not deleted.
func pageCount(c map[string]any) int {
	if n, ok := c["pageCount"].(float64); ok && n > 0 {
		return int(n)
	}
	if p, ok := c["pages"].([]any); ok && len(p) > 0 {
		return len(p)
	}
	cp, _ := c["cPages"].(map[string]any)
	pages, _ := cp["pages"].([]any)
	n := 0
	for _, p := range pages {
		if pm, ok := p.(map[string]any); ok && pm["deleted"] != nil {
			continue
		}
		n++
	}
	return n
}

// Library keeps the current snapshot of each account.
type Library struct {
	Roots   *rootlog.Log
	Builder *Builder

	mu  sync.Mutex
	cur map[string]*Snapshot
}

// New returns a library on the root log and the blob store.
func New(roots *rootlog.Log, blobs blob.Store) *Library {
	return &Library{Roots: roots, Builder: NewBuilder(blobs, 8192), cur: map[string]*Snapshot{}}
}

// Current returns the snapshot of the current generation of the account.
func (l *Library) Current(ctx context.Context, account string) (*Snapshot, error) {
	e, err := l.Roots.Current(ctx, account)
	if err != nil {
		return nil, err
	}
	return l.snapshot(ctx, account, e)
}

// At returns the snapshot of a generation. prev is the snapshot to reuse
// documents from, or nil.
func (l *Library) At(ctx context.Context, account string, generation uint64, prev *Snapshot) (*Snapshot, error) {
	if generation == 0 {
		return l.Builder.Build(ctx, account, rootlog.Entry{}, nil)
	}
	e, err := l.Roots.Get(ctx, account, generation)
	if err != nil {
		return nil, err
	}
	if prev == nil {
		prev = l.cached(account)
	}
	return l.Builder.Build(ctx, account, e, prev)
}

func (l *Library) cached(account string) *Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cur[account]
}

func (l *Library) snapshot(ctx context.Context, account string, e rootlog.Entry) (*Snapshot, error) {
	prev := l.cached(account)
	if prev != nil && prev.Generation == e.Generation {
		return prev, nil
	}
	s, err := l.Builder.Build(ctx, account, e, prev)
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	if old := l.cur[account]; old == nil || old.Generation < s.Generation {
		l.cur[account] = s
	}
	l.mu.Unlock()
	return s, nil
}
