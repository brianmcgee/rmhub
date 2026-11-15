package grants

import (
	"testing"

	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/library/libtest"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

func chain(lists ...[]tokens.Grant) []tokens.Token {
	var out []tokens.Token
	for i, gs := range lists {
		out = append(out, tokens.Token{ID: string(rune('a' + i)), Grants: gs})
	}
	return append(out, tokens.Token{ID: "owner"})
}

func snap(t *testing.T, e *libtest.Env) *library.Snapshot {
	t.Helper()
	e.Commit(4)
	s, err := library.New(e.Roots, e.Blobs).Current(e.Ctx, e.Account)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestVisibility(t *testing.T) {
	e := libtest.New(t)
	e.Folder("f", "F", "")
	e.Folder("g", "G", "f")
	e.PDF("d", "D", "g", []byte("%PDF"))
	e.Folder("r", "R", "")
	e.PDF("x", "X", "", []byte("%PDF"))
	s := snap(t, e)

	if FromChain(chain()) != nil {
		t.Fatal("owner token has a scope")
	}
	sc := FromChain(chain([]tokens.Grant{{Folder: "f", Access: "write"}, {Folder: "r", Access: "read"}}))
	for id, want := range map[string]string{"f": "write", "g": "write", "d": "write", "r": "read", "x": "", "missing": ""} {
		if got := sc.Access(s, id); got != want {
			t.Errorf("access %s = %q, want %q", id, got, want)
		}
	}
	if tops := sc.Tops(s); len(tops) != 2 || tops[0].ID != "f" || tops[1].ID != "r" {
		t.Fatalf("tops %v", tops)
	}
	if got := s.Path("d", sc.IsTop); got != "/F/G/D" {
		t.Fatalf("path %q", got)
	}

	// The child C has a read grant on G in F of its parent T.
	child := FromChain(chain([]tokens.Grant{{Folder: "g", Access: "read"}}, []tokens.Grant{{Folder: "f", Access: "write"}}))
	if child.Access(s, "d") != "read" || child.Visible(s, "f") {
		t.Fatal("child scope")
	}
	if got := s.Path("d", child.IsTop); got != "/G/D" {
		t.Fatalf("child path %q", got)
	}
	// A tablet moves F into R: the grants follow F.
	e.Folder("f", "F", "r")
	s = snap(t, e)
	if sc.Access(s, "d") != "write" {
		t.Fatal("grant did not follow the moved folder")
	}
	// A tablet moves G out of F: C can no longer read G.
	e.Folder("g", "G", "")
	s = snap(t, e)
	if child.Visible(s, "g") || child.Visible(s, "d") {
		t.Fatal("child sees a folder outside the parent grant")
	}
}

func TestCovers(t *testing.T) {
	e := libtest.New(t)
	e.Folder("f", "F", "")
	e.Folder("g", "G", "f")
	e.Folder("o", "O", "")
	s := snap(t, e)
	read := FromChain(chain([]tokens.Grant{{Folder: "f", Access: "read"}}))
	write := FromChain(chain([]tokens.Grant{{Folder: "f", Access: "write"}}))
	for _, c := range []struct {
		sc   *Scope
		g    tokens.Grant
		want bool
	}{
		{read, tokens.Grant{Folder: "f", Access: "write"}, false},
		{read, tokens.Grant{Folder: "g", Access: "read"}, true},
		{write, tokens.Grant{Folder: "g", Access: "read"}, true},
		{write, tokens.Grant{Folder: "g", Access: "write"}, true},
		{write, tokens.Grant{Folder: "o", Access: "read"}, false},
	} {
		if got := c.sc.Covers(s, c.g); got != c.want {
			t.Errorf("covers %+v = %v", c.g, got)
		}
	}
}
