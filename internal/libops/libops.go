// Package libops changes the library of an account with semantic
// operations.
//
// An operation reads the current snapshot, checks itself against it,
// writes new blobs and commits a new root. If a tablet commits first, the
// loop reads the new root and does the operation again, so no tablet
// change is lost.
package libops

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/hashtree"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/rootlog"
)

// Attempts is the number of commits before an operation fails with
// KindConflict.
const Attempts = 5

// Kind is the class of an operation error.
type Kind int

// Error kinds.
const (
	KindBadRequest Kind = iota
	KindForbidden
	KindNotFound
	KindPrecondition
	KindConflict
)

// Error is an operation error that the client can fix.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func errorf(k Kind, format string, args ...any) error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, args...)}
}

// Ops runs operations.
type Ops struct {
	Lib   *library.Library
	Blobs blob.Store
	Roots *rootlog.Log
	// Schema returns the announced schema of the account. A root at
	// generation 0 uses it.
	Schema func(ctx context.Context, account string) (int, error)
	Now    func() time.Time

	// beforeCommit runs before each commit. Tests use it to commit as a
	// tablet first.
	beforeCommit func(attempt int)
}

// Request is the caller of an operation.
type Request struct {
	Account string
	Source  rootlog.Source
	// Scope is nil for an owner token.
	Scope *grants.Scope
	// IfMatch is the document index hash from If-Match, or "".
	IfMatch string
}

// Tx is one attempt of an operation on one snapshot.
type Tx struct {
	S   *library.Snapshot
	ctx context.Context
	req Request
	o   *Ops
	ns  blob.Namespace
	// changed holds the new root entries by document ID.
	changed map[string]hashtree.Entry
}

// Op checks and applies an operation. It returns the ID of the entry
// that the response shows.
type Op func(t *Tx) (string, error)

// Run does the operation with at most Attempts commits. It returns the
// snapshot after the commit and the entry ID of the operation.
func (o *Ops) Run(ctx context.Context, req Request, op Op) (*library.Snapshot, string, error) {
	ns, err := blob.NewNamespace(req.Account)
	if err != nil {
		return nil, "", err
	}
	for attempt := 1; attempt <= Attempts; attempt++ {
		s, err := o.Lib.Current(ctx, req.Account)
		if err != nil {
			return nil, "", err
		}
		t := &Tx{S: s, ctx: ctx, req: req, o: o, ns: ns, changed: map[string]hashtree.Entry{}}
		id, err := op(t)
		if err != nil {
			return nil, "", err
		}
		schema := s.Schema
		if schema == 0 {
			if schema, err = o.Schema(ctx, req.Account); err != nil {
				return nil, "", err
			}
		}
		entries := s.RootEntries()
		for i, e := range entries {
			if c, ok := t.changed[e.Name]; ok {
				entries[i] = c
				delete(t.changed, e.Name)
			}
		}
		for _, c := range t.changed {
			entries = append(entries, c)
		}
		hash, err := library.PutRoot(ctx, o.Blobs, ns, schema, entries)
		if err != nil {
			return nil, "", err
		}
		if o.beforeCommit != nil {
			o.beforeCommit(attempt)
		}
		_, err = o.Roots.Commit(ctx, req.Account, s.Generation, hash, req.Source, true)
		if errors.Is(err, rootlog.ErrStale) {
			continue
		}
		if err != nil {
			return nil, "", err
		}
		after, err := o.Lib.Current(ctx, req.Account)
		if err != nil {
			return nil, "", err
		}
		return after, id, nil
	}
	return nil, "", errorf(KindConflict, "the root changed during %d attempts; try again", Attempts)
}

// CheckNew checks the name and the parent of a new entry against the
// current snapshot, and changes nothing. Call it before work that costs
// storage. Run does the same checks again at the commit.
func (o *Ops) CheckNew(ctx context.Context, req Request, parent, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	s, err := o.Lib.Current(ctx, req.Account)
	if err != nil {
		return err
	}
	t := &Tx{S: s, ctx: ctx, req: req, o: o}
	return t.destination(parent)
}

// Lookup returns the entry with the ID if the scope can see it.
func (t *Tx) Lookup(id string) (*library.Doc, error) {
	d, ok := t.S.Doc(id)
	if !ok || (t.req.Scope != nil && !t.req.Scope.Visible(t.S, id)) {
		return nil, errorf(KindNotFound, "entry %s does not exist", id)
	}
	return d, nil
}

// target returns the entry that a write changes. It checks the scope and
// If-Match.
func (t *Tx) target(id string) (*library.Doc, error) {
	d, err := t.Lookup(id)
	if err != nil {
		return nil, err
	}
	if sc := t.req.Scope; sc != nil {
		if sc.IsGrantFolder(id) {
			return nil, errorf(KindForbidden, "entry %s is a grant folder of the token; a scoped token cannot change it", id)
		}
		if !sc.CanWrite(t.S, id) {
			return nil, errorf(KindForbidden, "the token has no write access to entry %s", id)
		}
	}
	if t.req.IfMatch != "" && t.req.IfMatch != d.Hash {
		return nil, errorf(KindPrecondition, "entry %s has the hash %s, not %s", id, d.Hash, t.req.IfMatch)
	}
	return d, nil
}

// destination checks a parent for a new or moved entry.
func (t *Tx) destination(parent string) error {
	if parent == library.Trash {
		return errorf(KindBadRequest, "use the trash operation to move an entry into the trash")
	}
	if parent == "" {
		if t.req.Scope != nil {
			return errorf(KindForbidden, "the token has no write access to the top level")
		}
		return nil
	}
	d, err := t.Lookup(parent)
	if err != nil {
		return err
	}
	if d.Type != library.TypeFolder {
		return errorf(KindBadRequest, "entry %s is not a folder", parent)
	}
	if t.req.Scope != nil && !t.req.Scope.CanWrite(t.S, parent) {
		return errorf(KindForbidden, "the token has no write access to folder %s", parent)
	}
	return nil
}

func checkName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errorf(KindBadRequest, "the name must not be empty")
	}
	if strings.Contains(name, "/") {
		return errorf(KindBadRequest, "the name must not contain /")
	}
	return nil
}

func (t *Tx) now() string {
	return strconv.FormatInt(t.o.Now().UnixMilli(), 10)
}

// editMeta writes a new .metadata with the changed fields and keeps each
// other field. It increments version and sets lastModified and
// metadatamodified.
func (t *Tx) editMeta(d *library.Doc, set map[string]any) error {
	m := maps.Clone(d.Meta)
	maps.Copy(m, set)
	v, _ := m["version"].(float64)
	m["version"] = v + 1
	m["lastModified"] = t.now()
	m["metadatamodified"] = true
	f, err := library.PutJSON(t.ctx, t.o.Blobs, t.ns, d.ID, ".metadata", m)
	if err != nil {
		return err
	}
	e, err := library.PutDocument(t.ctx, t.o.Blobs, t.ns, d.ID, library.ReplaceFile(d.Files, f))
	if err != nil {
		return err
	}
	t.changed[d.ID] = e
	return nil
}

// newMeta returns the .metadata of a new entry.
func (t *Tx) newMeta(name, typ, parent string) map[string]any {
	return map[string]any{
		"deleted":          false,
		"lastModified":     t.now(),
		"metadatamodified": false,
		"modified":         false,
		"parent":           parent,
		"pinned":           false,
		"synced":           true,
		"type":             typ,
		"version":          float64(0),
		"visibleName":      name,
	}
}

func (t *Tx) add(id string, meta, content map[string]any, files ...hashtree.Entry) error {
	mf, err := library.PutJSON(t.ctx, t.o.Blobs, t.ns, id, ".metadata", meta)
	if err != nil {
		return err
	}
	cf, err := library.PutJSON(t.ctx, t.o.Blobs, t.ns, id, ".content", content)
	if err != nil {
		return err
	}
	e, err := library.PutDocument(t.ctx, t.o.Blobs, t.ns, id, append([]hashtree.Entry{mf, cf}, files...))
	if err != nil {
		return err
	}
	t.changed[id] = e
	return nil
}

// CreateFolder makes a folder with the ID in the parent.
func CreateFolder(id, parent, name string) Op {
	return func(t *Tx) (string, error) {
		if err := checkName(name); err != nil {
			return "", err
		}
		if err := t.destination(parent); err != nil {
			return "", err
		}
		return id, t.add(id, t.newMeta(name, "CollectionType", parent), map[string]any{})
	}
}

// Upload makes a document with the ID in the parent. The source file is
// in the blob store with the hash and the size. fileType is pdf or epub.
func Upload(id, parent, name, fileType, hash string, size int64) Op {
	return func(t *Tx) (string, error) {
		if err := checkName(name); err != nil {
			return "", err
		}
		if err := t.destination(parent); err != nil {
			return "", err
		}
		content := map[string]any{
			"dummyDocument":      false,
			"extraMetadata":      map[string]any{},
			"fileType":           fileType,
			"fontName":           "",
			"lastOpenedPage":     float64(0),
			"lineHeight":         float64(-1),
			"margins":            float64(180),
			"orientation":        "portrait",
			"pageCount":          float64(0),
			"pages":              []any{},
			"pageTags":           []any{},
			"redirectionPageMap": []any{},
			"tags":               []any{},
			"textScale":          float64(1),
		}
		src := hashtree.Entry{Hash: hash, Type: hashtree.TypeFile, Name: id + "." + fileType, Size: size}
		return id, t.add(id, t.newMeta(name, "DocumentType", parent), content, src)
	}
}

// Patch renames the entry if name is not nil, and moves it if parent is
// not nil.
func Patch(id string, name, parent *string) Op {
	return func(t *Tx) (string, error) {
		if name == nil && parent == nil {
			return "", errorf(KindBadRequest, "the request must have name or parent")
		}
		d, err := t.target(id)
		if err != nil {
			return "", err
		}
		set := map[string]any{}
		if name != nil {
			if err := checkName(*name); err != nil {
				return "", err
			}
			set["visibleName"] = *name
		}
		if parent != nil {
			if err := t.destination(*parent); err != nil {
				return "", err
			}
			if *parent != "" && t.S.IsAncestor(id, *parent) {
				return "", errorf(KindBadRequest, "cannot move entry %s into itself or into its descendant", id)
			}
			set["parent"] = *parent
		}
		return id, t.editMeta(d, set)
	}
}

// Trash moves the entry into the trash.
func Trash(id string) Op {
	return func(t *Tx) (string, error) {
		d, err := t.target(id)
		if err != nil {
			return "", err
		}
		return id, t.editMeta(d, map[string]any{"parent": library.Trash})
	}
}
