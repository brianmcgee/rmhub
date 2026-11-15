package app

import (
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/hashtree"
	"github.com/brianmcgee/rmhub/internal/library"

	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/pages"
)

func TestPagesEndpoint(t *testing.T) {
	e := newEnv(t, true)
	acct, owner := e.user("ada@example.org")
	tr := e.tree(acct)
	tr.Folder("agent", "Agent", "")
	tr.Notebook("nb", "Sketch", "agent", 3)
	tr.PDF("private", "Private", "", []byte("%PDF-1.7 p"))
	putBroken(t, tr, "broken")
	tr.Commit(4)

	r := e.req("GET", "/api/v1/library/nb/pages", owner, nil)
	var v struct{ Pages []pages.Page }
	r.json(t, &v)
	if r.status != 200 || len(v.Pages) != 3 {
		t.Fatalf("notebook pages: %d %s", r.status, r.body)
	}
	for i, p := range v.Pages {
		if p.Number != i+1 || p.RMHash == nil || p.PDFPage != nil {
			t.Errorf("page %+v", p)
		}
	}
	if r := e.req("GET", "/api/v1/library/agent/pages", owner, nil); r.status != 404 {
		t.Fatalf("folder pages: %d", r.status)
	}
	if r := e.req("GET", "/api/v1/library/broken/pages", owner, nil); r.status != 500 || !strings.Contains(string(r.body), `"internal"`) {
		t.Fatalf("broken content: %d %s", r.status, r.body)
	}

	_, tok := tokenText(t, e.scoped(owner, map[string]any{"label": "a", "grants": grantsOf("agent", "read")}))
	if r := e.req("GET", "/api/v1/library/private/pages", tok, nil); r.status != 404 {
		t.Fatalf("pages outside the grants: %d", r.status)
	}
	if r := e.req("GET", "/api/v1/library/nb/pages", tok, nil); r.status != 200 {
		t.Fatalf("scoped pages: %d", r.status)
	}
}

// putBroken adds a document whose .content is not JSON.
func putBroken(t *testing.T, tr *libtest.Env, id string) {
	t.Helper()
	m, err := library.PutJSON(tr.Ctx, tr.Blobs, tr.NS, id, ".metadata", libtest.Meta("Broken", "DocumentType", ""))
	if err != nil {
		t.Fatal(err)
	}
	c, err := library.PutFile(tr.Ctx, tr.Blobs, tr.NS, id, ".content", []byte("{not json"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := library.PutDocument(tr.Ctx, tr.Blobs, tr.NS, id, []hashtree.Entry{m, c})
	if err != nil {
		t.Fatal(err)
	}
	tr.Docs[id] = d
}
