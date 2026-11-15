package feed

import (
	"fmt"
	"testing"

	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

func ops(cs []Change) string {
	s := ""
	for _, c := range cs {
		s += fmt.Sprintf("%s:%s ", c.ID, c.Op)
		if (c.Entry == nil) != (c.Op == OpRemoved) {
			s += "(bad entry) "
		}
	}
	return s
}

func TestDiff(t *testing.T) {
	e := libtest.New(t)
	lib := library.New(e.Roots, e.Blobs)
	e.Folder("f", "F", "")
	e.PDF("move", "M", "", []byte("%PDF m"))
	e.PDF("trash", "T", "", []byte("%PDF t"))
	e.PDF("upd", "U", "", []byte("%PDF u"))
	e.PDF("rm", "R", "", []byte("%PDF r"))
	e.PDF("same", "S", "", []byte("%PDF s"))
	e.Commit(4)
	prev, err := lib.Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	e.PDF("new", "N", "f", []byte("%PDF n"))
	e.PDF("move", "M", "f", []byte("%PDF m"))
	e.PDF("trash", "T", library.Trash, []byte("%PDF t"))
	e.PDF("upd", "U2", "", []byte("%PDF u"))
	delete(e.Docs, "rm")
	e.Commit(4)
	next, err := lib.Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	want := "move:moved new:created rm:removed trash:trashed upd:updated "
	if got := ops(Diff(prev, next, nil)); got != want {
		t.Fatalf("owner diff %q, want %q", got, want)
	}

	// The token sees only F. A move into F is a create for the token.
	sc := grants.FromChain([]tokens.Token{{ID: "a", Grants: []tokens.Grant{{Folder: "f", Access: "read"}}}, {ID: "o"}})
	if got := ops(Diff(prev, next, sc)); got != "move:created new:created " {
		t.Fatalf("scoped diff %q", got)
	}
	// A move out of F is a removal for the token.
	e.PDF("move", "M", "", []byte("%PDF m"))
	e.PDF("upd", "U3", "", []byte("%PDF u"))
	e.Commit(4)
	last, err := lib.Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	if got := ops(Diff(next, last, sc)); got != "move:removed " {
		t.Fatalf("scoped removal %q", got)
	}
	// A change outside F gives no change for the token.
	e.PDF("upd", "U4", "", []byte("%PDF u"))
	e.Commit(4)
	after, err := lib.Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	if cs := Diff(last, after, sc); len(cs) != 0 {
		t.Fatalf("scoped diff outside the grant %q", ops(cs))
	}
}
