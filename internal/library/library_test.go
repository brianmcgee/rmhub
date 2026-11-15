package library_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/library/libtest"
)

func ids(ds []*library.Doc) string {
	var out []string
	for _, d := range ds {
		out = append(out, d.ID)
	}
	return strings.Join(out, ",")
}

func TestSnapshotReuse(t *testing.T) {
	e := libtest.New(t)
	for i := range 20 {
		e.PDF("doc"+string(rune('a'+i)), "Doc", "", []byte("%PDF-1.7 "+string(rune('a'+i))))
	}
	lib := library.New(e.Roots, e.Blobs)
	e.Commit(4)
	if _, err := lib.Current(e.Ctx, e.Account); err != nil {
		t.Fatal(err)
	}
	e.Blobs.Reads()

	// Rename one document.
	m := libtest.Meta("Renamed", "DocumentType", "")
	changed := e.Put("docc", m, map[string]any{"fileType": "pdf", "pageCount": float64(1)}, map[string][]byte{".pdf": []byte("%PDF-1.7 c")})
	ent := e.Commit(4)
	e.Blobs.Reads()
	s, err := lib.Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	reads := e.Blobs.Reads()
	meta, _ := (&library.Doc{Entry: library.Entry{ID: "docc"}, Files: e.Files["docc"]}).File(".metadata")
	// The root, the changed document index and its .metadata. The
	// .content blob did not change, so the cache has it.
	want := map[string]int{ent.Hash: 1, changed.Hash: 1, meta.Hash: 1}
	if len(reads) != len(want) {
		t.Fatalf("reads %v, want %v", reads, want)
	}
	for k := range want {
		if reads[k] != 1 {
			t.Fatalf("reads %v, want %v", reads, want)
		}
	}
	if d, _ := s.Doc("docc"); d.Name != "Renamed" {
		t.Fatalf("name %q", d.Name)
	}
	// The same generation comes from the cache with no read.
	if _, err := lib.Current(e.Ctx, e.Account); err != nil || len(e.Blobs.Reads()) != 0 {
		t.Fatal("second read of one generation read blobs")
	}
}

func TestEntryModel(t *testing.T) {
	e := libtest.New(t)
	e.Folder("work", "Work", "")
	e.PDF("paper", "Paper", "work", []byte("%PDF-1.7"))
	e.Folder("old", "Old", library.Trash)
	e.Notebook("nb", "Sketch", "old", 2)
	del := libtest.Meta("Gone", "DocumentType", "")
	del["deleted"] = true
	e.Put("gone", del, map[string]any{"fileType": "pdf"}, nil)
	e.Put("epub", libtest.Meta("Book", "DocumentType", ""), map[string]any{"fileType": "epub"}, map[string][]byte{".epub": []byte("PK")})
	e.Commit(3)

	s, err := library.New(e.Roots, e.Blobs).Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	if s.Schema != 3 || s.Generation != 1 {
		t.Fatalf("snapshot %d %d", s.Schema, s.Generation)
	}
	p, ok := s.Doc("paper")
	if !ok || p.Name != "Paper" || p.Type != library.TypePDF || p.Parent != "work" || p.Trashed || p.PageCount != 1 || p.Modified == nil || p.Hash != e.Docs["paper"].Hash {
		t.Fatalf("paper %+v", p.Entry)
	}
	if got := s.Path("paper", nil); got != "/Work/Paper" {
		t.Fatalf("path %q", got)
	}
	nb, _ := s.Doc("nb")
	if nb.Type != library.TypeNotebook || !nb.Trashed || nb.PageCount != 2 {
		t.Fatalf("notebook %+v", nb.Entry)
	}
	if got := s.Path("nb", nil); got != "/trash/Old/Sketch" {
		t.Fatalf("trashed path %q", got)
	}
	if f, _ := s.Doc("old"); f.Type != library.TypeFolder || !f.Trashed {
		t.Fatalf("folder %+v", f.Entry)
	}
	if b, _ := s.Doc("epub"); b.Type != library.TypeEPUB {
		t.Fatalf("epub %+v", b.Entry)
	}
	if _, ok := s.Doc("gone"); ok {
		t.Fatal("deleted document is visible")
	}
	if got := ids(s.Children("")); got != "epub,work" {
		t.Fatalf("top level %s", got)
	}
	if got := ids(s.Children(library.Trash)); got != "old" {
		t.Fatalf("trash %s", got)
	}
	// A deleted document stays in the root entries.
	if len(s.RootEntries()) != 6 {
		t.Fatalf("root entries %d", len(s.RootEntries()))
	}
}

func TestSearch(t *testing.T) {
	e := libtest.New(t)
	e.PDF("a", "Tax 2025", "", []byte("%PDF"))
	e.PDF("b", "taxonomy", "", []byte("%PDF"))
	e.PDF("c", "Other", "", []byte("%PDF"))
	e.PDF("d", "Tax trashed", library.Trash, []byte("%PDF"))
	e.Commit(4)
	s, err := library.New(e.Roots, e.Blobs).Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(s.Search("TAX", 100, nil)); got != "a,b" {
		t.Fatalf("search %s", got)
	}
	if got := ids(s.Search("TAX", 1, nil)); got != "a" {
		t.Fatalf("limit %s", got)
	}
}

func TestEmptyAccount(t *testing.T) {
	e := libtest.New(t)
	s, err := library.New(e.Roots, e.Blobs).Current(e.Ctx, e.Account)
	if err != nil || s.Generation != 0 || len(s.Children("")) != 0 {
		t.Fatalf("empty: %+v %v", s, err)
	}
}

// TestLargeRoot makes sure that the library reads a root index of each
// size that the sync service accepts.
func TestLargeRoot(t *testing.T) {
	e := libtest.New(t)
	// 50 long IDs give a root index of more than 5 MiB.
	for i := range 50 {
		e.Folder(strings.Repeat("x", 110<<10)+strconv.Itoa(i), "F"+strconv.Itoa(i), "")
	}
	e.Commit(4)
	s, err := library.New(e.Roots, e.Blobs).Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s.Children("")); n != 50 {
		t.Fatalf("%d entries, want 50", n)
	}
}
