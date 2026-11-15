package tokens

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

func newState(t *testing.T) *sqlite.Store {
	t.Helper()
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func newStore(t *testing.T) *Store { return New(newState(t)) }

func TestAdminFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, text string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tok := GenerateAdmin()
	if !strings.HasPrefix(tok, AdminPrefix) || len(tok) != len(AdminPrefix)+SecretLen {
		t.Fatalf("generated %q", tok)
	}
	a, err := ReadAdminFile(write("valid", tok+"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !a.Check(tok) || a.Check(tok+"x") || a.Check(GenerateAdmin()) {
		t.Fatal("admin check gives a wrong result")
	}
	if _, err := ReadAdminFile(filepath.Join(dir, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	if _, err := ReadAdminFile(write("short", "rmh_admin_abc")); err == nil {
		t.Fatal("short token accepted")
	}
	if _, err := ReadAdminFile(write("prefix", "rmh_x_"+strings.Repeat("a", 40))); err == nil {
		t.Fatal("token without prefix accepted")
	}
}

func TestIssueAndCheck(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	acct := ids.NewAccountID()
	text, tok, err := s.Issue(ctx, acct, Spec{Label: "laptop"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(text, "rmh_"+acct+"_") || tok.Label != "laptop" || tok.ExpiresAt != nil {
		t.Fatalf("token %q %+v", text, tok)
	}
	a, id, s2, ok := Parse(text)
	if !ok || a != acct || id != tok.ID || len(s2) != SecretLen || len(id) != IDLen {
		t.Fatalf("parse %q: %v %v %v %v", text, a, id, s2, ok)
	}
	gotAcct, chain, err := s.Check(ctx, text)
	if err != nil || gotAcct != acct || len(chain) != 1 || chain[0].ID != tok.ID || chain[0].Scoped() {
		t.Fatalf("check: %v %+v %v", gotAcct, chain, err)
	}

	// Wrong secret.
	wrong := text[:len(text)-1] + map[bool]string{true: "b", false: "a"}[strings.HasSuffix(text, "a")]
	if _, _, err := s.Check(ctx, wrong); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong secret: %v", err)
	}
	// Malformed and unknown.
	for _, bad := range []string{"", "x", "rmh_" + acct + "_abc_def", "rmh_" + acct + "_" + ids.Random(16) + "_" + ids.Random(52)} {
		if _, _, err := s.Check(ctx, bad); !errors.Is(err, ErrInvalid) {
			t.Fatalf("token %q: %v", bad, err)
		}
	}

	list, err := s.List(ctx, acct)
	if err != nil || len(list) != 1 || list[0].ID != tok.ID || list[0].Label != "laptop" {
		t.Fatalf("list %+v, %v", list, err)
	}

	// Revoke: another account gets not-found and the token stays valid.
	if err := s.Revoke(ctx, ids.NewAccountID(), tok.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoke from another account: %v", err)
	}
	if _, _, err := s.Check(ctx, text); err != nil {
		t.Fatalf("token after foreign revoke: %v", err)
	}
	if err := s.Revoke(ctx, acct, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Check(ctx, text); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revoked token: %v", err)
	}
	if err := s.Revoke(ctx, acct, tok.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second revoke: %v", err)
	}
}

func TestExpiry(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	exp := now.Add(time.Hour)
	text, _, err := s.Issue(ctx, ids.NewAccountID(), Spec{ExpiresAt: &exp})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Check(ctx, text); err != nil {
		t.Fatalf("before expiry: %v", err)
	}
	now = exp.Add(time.Minute)
	if _, _, err := s.Check(ctx, text); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expired token: %v", err)
	}
}

func TestChain(t *testing.T) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })
	acct := ids.NewAccountID()
	_, owner, err := s.Issue(ctx, acct, Spec{Label: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	g := []Grant{{Folder: "f", Access: AccessWrite}}
	exp := now.Add(time.Hour)
	_, parent, err := s.Issue(ctx, acct, Spec{Label: "agent", Parent: owner.ID, Grants: g, ExpiresAt: &exp})
	if err != nil {
		t.Fatal(err)
	}
	childText, child, err := s.Issue(ctx, acct, Spec{Label: "sub", Parent: parent.ID, Grants: g})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Issue(ctx, acct, Spec{Parent: parent.ID}); err == nil {
		t.Fatal("scoped token without grants accepted")
	}

	_, chain, err := s.Check(ctx, childText)
	if err != nil || len(chain) != 3 || chain[0].ID != child.ID || chain[1].ID != parent.ID || chain[2].ID != owner.ID || !chain[0].Scoped() {
		t.Fatalf("chain %+v %v", chain, err)
	}
	// An expired parent stops the child.
	now = exp.Add(time.Second)
	if _, _, err := s.Check(ctx, childText); !errors.Is(err, ErrInvalid) {
		t.Fatalf("child of expired parent: %v", err)
	}
	now = exp.Add(-time.Minute)

	list, _ := s.List(ctx, acct)
	if d := Descendants(list, owner.ID); len(d) != 2 {
		t.Fatalf("descendants of owner: %+v", d)
	}
	if d := Descendants(list, parent.ID); len(d) != 1 || d[0].ID != child.ID {
		t.Fatalf("descendants of parent: %+v", d)
	}
	// Revocation of the parent removes the child too.
	if err := s.Revoke(ctx, acct, parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Check(ctx, childText); !errors.Is(err, ErrInvalid) {
		t.Fatalf("child of revoked parent: %v", err)
	}
	list, _ = s.List(ctx, acct)
	if len(list) != 1 || list[0].ID != owner.ID {
		t.Fatalf("list after revoke: %+v", list)
	}
}

// TestMissingParent checks the state after a crash between the two
// deletes of a revocation: the child stays, but its parent is gone.
func TestMissingParent(t *testing.T) {
	ctx := context.Background()
	st := newState(t)
	s := New(st)
	acct := ids.NewAccountID()
	_, owner, _ := s.Issue(ctx, acct, Spec{})
	_, parent, _ := s.Issue(ctx, acct, Spec{Parent: owner.ID, Grants: []Grant{{Folder: "f", Access: AccessRead}}})
	text, _, _ := s.Issue(ctx, acct, Spec{Parent: parent.ID, Grants: []Grant{{Folder: "f", Access: AccessRead}}})
	if err := st.Delete(ctx, recordKey(acct, parent.ID)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Check(ctx, text); !errors.Is(err, ErrInvalid) {
		t.Fatalf("child with missing parent: %v", err)
	}
}
