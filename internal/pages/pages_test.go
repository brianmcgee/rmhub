package pages

import (
	"testing"

	"github.com/brianmcgee/rmhub/internal/hashtree"
)

func ids(ps []Page) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCPages(t *testing.T) {
	content := `{"fileType":"notebook","cPages":{"pages":[
		{"id":"C","idx":{"timestamp":"1:3","value":"bc"}},
		{"id":"B","idx":{"timestamp":"1:2","value":"bb"},"deleted":{"timestamp":"1:9","value":1}},
		{"id":"A","idx":{"timestamp":"1:1","value":"ba"}},
		{"id":"D","idx":{"timestamp":"1:4","value":"bc"},"deleted":{"timestamp":"1:5","value":0}}
	]}}`
	files := []hashtree.Entry{{Name: "doc/A.rm", Hash: "ha"}, {Name: "doc/C.rm", Hash: "hc"}, {Name: "doc.content", Hash: "x"}}
	ps, err := List("doc", []byte(content), files)
	if err != nil {
		t.Fatal(err)
	}
	// B is deleted. C and D have one idx value, so the ID decides.
	if !eq(ids(ps), []string{"A", "C", "D"}) {
		t.Fatalf("order %v", ids(ps))
	}
	for i, p := range ps {
		if p.Number != i+1 || p.PDFPage != nil {
			t.Errorf("page %+v", p)
		}
	}
	if ps[0].RMHash == nil || *ps[0].RMHash != "ha" || ps[2].RMHash != nil {
		t.Errorf("rm hashes %v %v", ps[0].RMHash, ps[2].RMHash)
	}
}

func TestInsertedBlankPDFPage(t *testing.T) {
	content := `{"fileType":"pdf","cPages":{"pages":[
		{"id":"p1","idx":{"value":"ba"},"redir":{"value":0}},
		{"id":"new","idx":{"value":"bb"}},
		{"id":"p2","idx":{"value":"bc"},"redir":{"value":1}}
	]}}`
	ps, err := List("doc", []byte(content), []hashtree.Entry{{Name: "doc/new.rm", Hash: "hn"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 3 || *ps[0].PDFPage != 1 || ps[1].PDFPage != nil || *ps[2].PDFPage != 2 {
		t.Fatalf("pages %+v", ps)
	}
	if ps[1].RMHash == nil || *ps[1].RMHash != "hn" {
		t.Fatalf("inserted page has no rm hash: %+v", ps[1])
	}
}

func TestOldFormat(t *testing.T) {
	ps, err := List("doc", []byte(`{"fileType":"notebook","pages":["a","b"]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !eq(ids(ps), []string{"a", "b"}) || ps[1].Number != 2 || ps[0].PDFPage != nil {
		t.Fatalf("pages %+v", ps)
	}
	ps, err = List("doc", []byte(`{"fileType":"pdf","pages":["a","b","c"],"redirectionPageMap":[0,-1,1]}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if *ps[0].PDFPage != 1 || ps[1].PDFPage != nil || *ps[2].PDFPage != 2 {
		t.Fatalf("pdf pages %+v", ps)
	}
	// A notebook has no PDF page, also with a redirection map.
	ps, _ = List("doc", []byte(`{"fileType":"notebook","pages":["a"],"redirectionPageMap":[0]}`), nil)
	if ps[0].PDFPage != nil {
		t.Fatalf("notebook page %+v", ps[0])
	}
}

func TestBadContent(t *testing.T) {
	if _, err := List("doc", []byte(`{"cPages":`), nil); err == nil {
		t.Fatal("broken JSON gives no error")
	}
	ps, err := List("doc", nil, nil)
	if err != nil || len(ps) != 0 {
		t.Fatalf("no content: %v %v", ps, err)
	}
}
