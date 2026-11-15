package libops

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/hashtree"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/rootlog"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

var now = time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)

func setup(t *testing.T) (*libtest.Env, *Ops) {
	e := libtest.New(t)
	o := &Ops{
		Lib:    library.New(e.Roots, e.Blobs),
		Blobs:  e.Blobs,
		Roots:  e.Roots,
		Schema: func(context.Context, string) (int, error) { return 3, nil },
		Now:    func() time.Time { return now },
	}
	return e, o
}

func owner(e *libtest.Env) Request {
	return Request{Account: e.Account, Source: rootlog.Source{Type: rootlog.SourceToken, ID: "tok1"}}
}

func kind(t *testing.T, err error, k Kind) {
	t.Helper()
	var oe *Error
	if !errors.As(err, &oe) || oe.Kind != k {
		t.Fatalf("error %v, want kind %d", err, k)
	}
}

func TestReapply(t *testing.T) {
	e, o := setup(t)
	e.PDF("d", "D", "", []byte("%PDF d"))
	e.PDF("e", "E", "", []byte("%PDF e"))
	e.Commit(4)

	o.beforeCommit = func(attempt int) {
		if attempt <= 2 {
			// The tablet renames E first.
			e.PDF("e", "E"+string(rune('0'+attempt)), "", []byte("%PDF e"))
			e.Commit(4)
		}
	}
	s, id, err := o.Run(e.Ctx, owner(e), Patch("d", new("New D"), nil))
	if err != nil {
		t.Fatal(err)
	}
	if id != "d" || s.Generation != 4 || s.Schema != 4 {
		t.Fatalf("after: %s gen %d schema %d", id, s.Generation, s.Schema)
	}
	d, _ := s.Doc("d")
	en, _ := s.Doc("e")
	if d.Name != "New D" || en.Name != "E2" {
		t.Fatalf("names %q %q", d.Name, en.Name)
	}
	cur, _ := e.Roots.Current(e.Ctx, e.Account)
	if cur.Source.Type != rootlog.SourceToken || cur.Source.ID != "tok1" {
		t.Fatalf("source %+v", cur.Source)
	}

	// Five lost races give a conflict.
	o.beforeCommit = func(int) {
		e.PDF("e", "E", "", []byte("%PDF e"))
		e.Docs["d"] = mustDoc(t, o, e, "d").Index
		e.Commit(4)
	}
	_, _, err = o.Run(e.Ctx, owner(e), Patch("d", new("Again"), nil))
	kind(t, err, KindConflict)
}

func mustDoc(t *testing.T, o *Ops, e *libtest.Env, id string) *library.Doc {
	t.Helper()
	s, err := o.Lib.Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := s.Doc(id)
	if !ok {
		t.Fatalf("no entry %s", id)
	}
	return d
}

func TestMoveIntoTrashedFolder(t *testing.T) {
	e, o := setup(t)
	e.Folder("f", "F", "")
	e.PDF("d", "D", "", []byte("%PDF"))
	e.Commit(4)
	o.beforeCommit = func(attempt int) {
		if attempt == 1 {
			e.Folder("f", "F", library.Trash)
			e.Commit(4)
		}
	}
	s, _, err := o.Run(e.Ctx, owner(e), Patch("d", nil, new("f")))
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Doc("d"); d.Parent != "f" || !d.Trashed {
		t.Fatalf("entry %+v", d.Entry)
	}
}

func TestKeepUnknownMetadata(t *testing.T) {
	e, o := setup(t)
	m := libtest.Meta("D", "DocumentType", "")
	m["pinned"] = true
	m["x"] = float64(1)
	m["version"] = float64(7)
	e.Put("d", m, map[string]any{"fileType": "pdf", "y": "keep"}, map[string][]byte{".pdf": []byte("%PDF")})
	e.Commit(3)
	s, _, err := o.Run(e.Ctx, owner(e), Patch("d", new("Renamed"), nil))
	if err != nil {
		t.Fatal(err)
	}
	d, _ := s.Doc("d")
	if d.Meta["pinned"] != true || d.Meta["x"] != float64(1) || d.Meta["version"] != float64(8) || d.Meta["metadatamodified"] != true || d.Meta["visibleName"] != "Renamed" {
		t.Fatalf("metadata %v", d.Meta)
	}
	if d.Meta["lastModified"] != strconv.FormatInt(now.UnixMilli(), 10) {
		t.Fatalf("lastModified %v", d.Meta["lastModified"])
	}
	if d.Content["y"] != "keep" {
		t.Fatalf("content %v", d.Content)
	}
	// The root keeps schema 3, and the .pdf file is the same blob.
	pdf, _ := d.File(".pdf")
	if s.Schema != 3 || pdf.Hash != hashtree.FileHash([]byte("%PDF")) {
		t.Fatalf("schema %d pdf %s", s.Schema, pdf.Hash)
	}
}

func TestChecks(t *testing.T) {
	e, o := setup(t)
	e.Folder("a", "A", "")
	e.Folder("b", "B", "a")
	e.PDF("d", "D", "", []byte("%PDF"))
	e.Commit(4)
	req := owner(e)
	for name, c := range map[string]struct {
		op   Op
		kind Kind
	}{
		"parent is a document": {Patch("b", nil, new("d")), KindBadRequest},
		"parent is trash":      {Patch("d", nil, new(library.Trash)), KindBadRequest},
		"into a descendant":    {Patch("a", nil, new("b")), KindBadRequest},
		"into itself":          {Patch("a", nil, new("a")), KindBadRequest},
		"unknown entry":        {Patch("zz", new("x"), nil), KindNotFound},
		"unknown parent":       {CreateFolder("n", "zz", "N"), KindNotFound},
		"empty name":           {CreateFolder("n", "", " "), KindBadRequest},
		"folder in document":   {CreateFolder("n", "d", "N"), KindBadRequest},
		"no change":            {Patch("d", nil, nil), KindBadRequest},
	} {
		_, _, err := o.Run(e.Ctx, req, c.op)
		var oe *Error
		if !errors.As(err, &oe) || oe.Kind != c.kind {
			t.Errorf("%s: %v", name, err)
		}
	}
	// A folder and a trash at the top level.
	s, id, err := o.Run(e.Ctx, req, CreateFolder("new", "", "Inbox"))
	if err != nil || id != "new" {
		t.Fatal(err)
	}
	if f, _ := s.Doc("new"); f.Type != library.TypeFolder || f.Content == nil || len(f.Content) != 0 {
		t.Fatalf("folder %+v %v", f.Entry, f.Content)
	}
	s, _, err = o.Run(e.Ctx, req, Trash("d"))
	if err != nil {
		t.Fatal(err)
	}
	if d, _ := s.Doc("d"); !d.Trashed {
		t.Fatal("not trashed")
	}
}

func TestIfMatch(t *testing.T) {
	e, o := setup(t)
	e.PDF("d", "D", "", []byte("%PDF"))
	e.Commit(4)
	h1 := e.Docs["d"].Hash
	req := owner(e)
	req.IfMatch = h1
	// The tablet changes D between the read and the write.
	o.beforeCommit = func(attempt int) {
		if attempt == 1 {
			e.PDF("d", "D tablet", "", []byte("%PDF"))
			e.Commit(4)
		}
	}
	_, _, err := o.Run(e.Ctx, req, Patch("d", new("API"), nil))
	kind(t, err, KindPrecondition)
	if d := mustDoc(t, o, e, "d"); d.Name != "D tablet" {
		t.Fatalf("name %q", d.Name)
	}
	// A stale tag before the first attempt.
	o.beforeCommit = nil
	_, _, err = o.Run(e.Ctx, req, Trash("d"))
	kind(t, err, KindPrecondition)
}

func TestScopedWrites(t *testing.T) {
	e, o := setup(t)
	e.Folder("f", "F", "")
	e.Folder("r", "R", "")
	e.PDF("d", "D", "f", []byte("%PDF"))
	e.PDF("x", "X", "", []byte("%PDF"))
	e.Commit(4)
	req := owner(e)
	req.Scope = grants.FromChain([]tokens.Token{
		{ID: "agent", Grants: []tokens.Grant{{Folder: "f", Access: "write"}, {Folder: "r", Access: "read"}}},
		{ID: "owner"},
	})
	for name, c := range map[string]struct {
		op   Op
		kind Kind
	}{
		"rename grant folder": {Patch("f", new("G"), nil), KindForbidden},
		"trash grant folder":  {Trash("f"), KindForbidden},
		"move into read-only": {Patch("d", nil, new("r")), KindForbidden},
		"move to top level":   {Patch("d", nil, new("")), KindForbidden},
		"entry outside":       {Patch("x", new("Y"), nil), KindNotFound},
		"create in read-only": {CreateFolder("n", "r", "N"), KindForbidden},
	} {
		_, _, err := o.Run(e.Ctx, req, c.op)
		var oe *Error
		if !errors.As(err, &oe) || oe.Kind != c.kind {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := o.Run(e.Ctx, req, Patch("d", new("Mine"), nil)); err != nil {
		t.Fatal(err)
	}
}

func TestUploadOnEmptyAccount(t *testing.T) {
	e, o := setup(t)
	pdf := []byte("%PDF-1.7 body")
	f, err := library.PutFile(e.Ctx, e.Blobs, e.NS, "", "", pdf)
	if err != nil {
		t.Fatal(err)
	}
	s, _, err := o.Run(e.Ctx, owner(e), Upload("u1", "", "Paper", "pdf", f.Hash, f.Size))
	if err != nil {
		t.Fatal(err)
	}
	d, ok := s.Doc("u1")
	if !ok || d.Type != library.TypePDF || d.Name != "Paper" || s.Schema != 3 {
		t.Fatalf("upload %+v schema %d", d, s.Schema)
	}
	if got, _ := d.File(".pdf"); got.Hash != f.Hash || got.Size != int64(len(pdf)) {
		t.Fatalf("pdf entry %+v", got)
	}
}
