package app

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/pages"
	"github.com/brianmcgee/rmhub/internal/render/rendertest"
	"github.com/brianmcgee/rmhub/internal/rmfile"
	"github.com/brianmcgee/rmhub/internal/rmfile/rmtest"
)

// strokePage returns a v6 file with one stroke at the height y.
func strokePage(y float32) []byte {
	return rmtest.File([]rmtest.Stroke{{Pen: uint32(rmfile.Fineliner), Points: []rmtest.Point{
		{X: -100, Y: y, W: 4}, {X: 100, Y: y, W: 4},
	}}}, nil)
}

// putNotebook adds a notebook with n parseable pages.
func putNotebook(tr *libtest.Env, id, name, parent string, n int) {
	extra := map[string][]byte{}
	var list []any
	for i := range n {
		pid := "p" + string(rune('a'+i))
		list = append(list, map[string]any{"id": pid, "idx": map[string]any{"value": pid}})
		extra["/"+pid+".rm"] = strokePage(float32(100 * (i + 1)))
	}
	tr.Put(id, libtest.Meta(name, "DocumentType", parent), map[string]any{"fileType": "notebook", "cPages": map[string]any{"pages": list}}, extra)
}

func pngSize(t *testing.T, b []byte) (int, int) {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not a PNG: %v", err)
	}
	return img.Bounds().Dx(), img.Bounds().Dy()
}

func TestRenderEndpoint(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("agent", "Agent", "")
	putNotebook(tr, "nb", "Sketch", "agent", 3)
	tr.Put("annot", libtest.Meta("Paper", "DocumentType", ""), map[string]any{
		"fileType": "pdf",
		"cPages": map[string]any{"pages": []any{
			map[string]any{"id": "pa", "idx": map[string]any{"value": "a"}, "redir": map[string]any{"value": 0}},
			map[string]any{"id": "pb", "idx": map[string]any{"value": "b"}},
		}},
	}, map[string][]byte{".pdf": rendertest.PDF(), "/pa.rm": strokePage(1000)})
	tr.Put("book", libtest.Meta("Book", "DocumentType", ""), map[string]any{"fileType": "epub", "pageCount": float64(1)}, map[string][]byte{".epub": []byte("PK epub")})
	tr.PDF("private", "Private", "", rendertest.PDF())
	tr.PDF("plain", "Uploaded", "", rendertest.PDF())
	onePage := map[string]any{"fileType": "notebook", "cPages": map[string]any{"pages": []any{
		map[string]any{"id": "pa", "idx": map[string]any{"value": "a"}},
	}}}
	tr.Put("long", libtest.Meta("Long", "DocumentType", ""), onePage, map[string][]byte{"/pa.rm": strokePage(30000)})
	// A v5 file that gives 2^32 - 16 layers.
	bomb := append([]byte(rmfile.HeaderV5), 0xf0, 0xff, 0xff, 0xff)
	// A page file of more than 32 MiB.
	tr.Put("heavy", libtest.Meta("Heavy", "DocumentType", ""), onePage, map[string][]byte{"/pa.rm": make([]byte, 32<<20+1)})
	tr.Put("bomb", libtest.Meta("Bomb", "DocumentType", ""), onePage, map[string][]byte{"/pa.rm": bomb})
	tr.Commit(4)
	base := "/api/v1/library/"

	r := e.req("GET", base+"nb/pages/3/render", owner, nil)
	if r.status != 200 || r.header.Get("Content-Type") != "image/png" {
		t.Fatalf("page 3: %d %s", r.status, r.body)
	}
	if w, _ := pngSize(t, r.body); w != 1404 {
		t.Fatalf("page 3 width %d", w)
	}
	etag := r.header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if r := e.req("GET", base+"nb/pages/3/render", owner, nil, "If-None-Match", etag); r.status != 304 || len(r.body) != 0 {
		t.Fatalf("unchanged page: %d %q", r.status, r.body)
	}
	renders := e.app.Library.Render.Renders.Load()
	again := e.req("GET", base+"nb/pages/3/render", owner, nil)
	if !bytes.Equal(again.body, r.body) || e.app.Library.Render.Renders.Load() != renders {
		t.Fatal("a second request does not use the cache")
	}
	if r := e.req("GET", base+"nb/pages/2/render", owner, nil); r.header.Get("ETag") == etag {
		t.Fatal("page 2 has the ETag of page 3")
	}
	if r := e.req("GET", base+"nb/pages/3/render?width=200", owner, nil); r.header.Get("ETag") == etag {
		t.Fatal("another width has the same ETag")
	}

	r = e.req("GET", base+"nb/pages/1/render?format=svg&width=702", owner, nil)
	if r.status != 200 || r.header.Get("Content-Type") != "image/svg+xml" || !strings.HasPrefix(string(r.body), "<svg") {
		t.Fatalf("SVG: %d %.100s", r.status, r.body)
	}

	r = e.req("GET", base+"annot/pages/1/render?width=702", owner, nil)
	if r.status != 200 {
		t.Fatalf("PDF page: %d %s", r.status, r.body)
	}
	if w, h := pngSize(t, r.body); w != 702 || h != 908 {
		t.Fatalf("PDF page size %d × %d", w, h)
	}
	// A PDF without a page list has the pages of the PDF.
	r = e.req("GET", base+"plain/pages", owner, nil)
	var pl struct{ Pages []pages.Page }
	r.json(t, &pl)
	if len(pl.Pages) != 1 || pl.Pages[0].PDFPage == nil || *pl.Pages[0].PDFPage != 1 || pl.Pages[0].RMHash != nil {
		t.Fatalf("pages of an uploaded PDF: %s", r.body)
	}
	if r := e.req("GET", base+"plain/pages/1/render", owner, nil); r.status != 200 {
		t.Fatalf("page of an uploaded PDF: %d %s", r.status, r.body)
	}
	if r := e.req("GET", base+"annot/pages/2/render", owner, nil); r.status != 200 {
		t.Fatalf("inserted page: %d %s", r.status, r.body)
	}

	for _, tc := range []struct {
		name, path string
		status     int
	}{
		{"page 9 of 3", "nb/pages/9/render", 404},
		{"page 0", "nb/pages/0/render", 404},
		{"EPUB page", "book/pages/1/render", 404},
		{"folder", "agent/pages/1/render", 404},
		{"width too small", "nb/pages/1/render?width=99", 400},
		{"width too large", "nb/pages/1/render?width=4097", 400},
		{"unknown format", "nb/pages/1/render?format=jpeg", 400},
		{"SVG of a PDF page", "annot/pages/1/render?format=svg", 400},
		{"too many pixels", "long/pages/1/render?width=4096", 400},
		{"page file with a bad count", "bomb/pages/1/render", 500},
		{"page file that is too large", "heavy/pages/1/render", 409},
	} {
		if r := e.req("GET", base+tc.path, owner, nil); r.status != tc.status {
			t.Errorf("%s: %d %s", tc.name, r.status, r.body)
		}
	}

	_, tok := tokenText(t, e.scoped(owner, map[string]any{"label": "a", "grants": grantsOf("agent", "read")}))
	if r := e.req("GET", base+"private/pages/1/render", tok, nil); r.status != 404 {
		t.Fatalf("render outside the grants: %d", r.status)
	}
	if r := e.req("GET", base+"nb/pages/1/render", tok, nil); r.status != 200 {
		t.Fatalf("scoped render: %d", r.status)
	}
}

// TestPDFPageCountIsCached makes sure that the page list of a PDF
// without a page list starts PDFium one time for each PDF.
func TestPDFPageCountIsCached(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.PDF("plain", "Uploaded", "", rendertest.PDF())
	tr.Commit(4)

	rn := e.app.Library.Render
	first := e.req("GET", "/api/v1/library/plain/pages", owner, nil)
	if first.status != 200 || rn.Renders.Load() != 1 {
		t.Fatalf("first page list: status %d, %d PDFium runs, want 200 and 1", first.status, rn.Renders.Load())
	}
	again := e.req("GET", "/api/v1/library/plain/pages", owner, nil)
	if again.status != 200 || !bytes.Equal(again.body, first.body) || rn.Renders.Load() != 1 {
		t.Fatalf("second page list: status %d, %d PDFium runs, want 200 and 1\n%s", again.status, rn.Renders.Load(), again.body)
	}
}

// TestRenderAfterClose makes sure that a PDF request after the renderer
// stops gives an error and no panic.
func TestRenderAfterClose(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.PDF("plain", "Uploaded", "", rendertest.PDF())
	tr.Commit(4)

	if err := e.app.Library.Render.Close(); err != nil {
		t.Fatal(err)
	}
	if r := e.req("GET", "/api/v1/library/plain/pages", owner, nil); r.status != 500 {
		t.Fatalf("page list after close: %d %s", r.status, r.body)
	}
}
