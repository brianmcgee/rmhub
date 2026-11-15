package main

import (
	"bytes"
	"image/png"
	"os"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/rmfile"
	"github.com/brianmcgee/rmhub/internal/rmfile/rmtest"
)

func TestPageCommands(t *testing.T) {
	isolate(t)
	t.Chdir(t.TempDir())
	ts := newTestServer(t)
	acct, env := ts.account(t)
	tr := ts.tree(t, acct)
	page := rmtest.File([]rmtest.Stroke{{Pen: uint32(rmfile.Fineliner), Points: []rmtest.Point{{X: -50, Y: 100, W: 4}, {X: 50, Y: 100, W: 4}}}}, nil)
	tr.Put("nb", libtest.Meta("Notes", "DocumentType", ""), map[string]any{
		"fileType": "notebook",
		"cPages": map[string]any{"pages": []any{
			map[string]any{"id": "a", "idx": map[string]any{"value": "a"}},
			map[string]any{"id": "b", "idx": map[string]any{"value": "b"}},
		}},
	}, map[string][]byte{"/a.rm": page})
	tr.Commit(4)

	var ps []pageView
	jsonOut(t, rmhub(env, "pages", "/Notes", "--json"), &ps)
	if len(ps) != 2 || ps[0].RMHash == nil || ps[1].RMHash != nil {
		t.Fatalf("pages: %+v", ps)
	}
	if r := rmhub(env, "pages", "nb"); r.code != 0 || !strings.Contains(strings.Join(strings.Fields(r.stdout), " "), "1 a - yes 2 b - no") {
		t.Fatalf("pages text: %+v", r)
	}

	r := rmhub(env, "render", "/Notes", "--page", "1")
	if r.code != 0 || strings.TrimSpace(r.stdout) != "Notes-p1.png" {
		t.Fatalf("render: %+v", r)
	}
	b, err := os.ReadFile("Notes-p1.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil || img.Bounds().Dx() != 1404 {
		t.Fatalf("Notes-p1.png: %v %v", err, img)
	}
	r = rmhub(env, "render", "/Notes", "--page", "1")
	if r.code != 1 || !strings.Contains(r.stderr, "exists") {
		t.Fatalf("render over a file: %+v", r)
	}
	if after, _ := os.ReadFile("Notes-p1.png"); !bytes.Equal(after, b) {
		t.Fatal("render changed the existing file")
	}

	r = rmhub(env, "render", "nb", "--page", "1", "--width", "702", "-o", "-")
	if r.code != 0 {
		t.Fatalf("render to stdout: %+v", r)
	}
	if img, err := png.Decode(strings.NewReader(r.stdout)); err != nil || img.Bounds().Dx() != 702 {
		t.Fatalf("stdout image: %v", err)
	}
	r = rmhub(env, "render", "nb", "--page", "2", "--format", "svg", "--json")
	if r.code != 0 || !strings.Contains(r.stdout, `"Notes-p2.svg"`) {
		t.Fatalf("render SVG: %+v", r)
	}
	if b, err := os.ReadFile("Notes-p2.svg"); err != nil || !bytes.HasPrefix(b, []byte("<svg")) {
		t.Fatalf("Notes-p2.svg: %v %.50s", err, b)
	}

	if r := rmhub(env, "render", "nb", "--page", "9", "-o", "x.png"); r.code != exitNotFound {
		t.Fatalf("page 9: %+v", r)
	}
	if _, err := os.Stat("x.png"); err == nil {
		t.Fatal("a failed render wrote a file")
	}
	if r := rmhub(env, "render", "nb", "--page", "0"); r.code != exitUsage {
		t.Fatalf("page 0: %+v", r)
	}
	if r := rmhub(env, "render", "nb", "--page", "1", "--width", "50", "-o", "y.png"); r.code != 1 || !strings.Contains(r.stderr, "width") {
		t.Fatalf("width 50: %+v", r)
	}
}
