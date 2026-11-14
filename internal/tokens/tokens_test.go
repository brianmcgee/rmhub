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

func newStore(t *testing.T) *Store {
	t.Helper()
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st)
}

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
	text, tok, err := s.Issue(ctx, acct, "laptop", nil)
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
	gotAcct, gotID, err := s.Check(ctx, text)
	if err != nil || gotAcct != acct || gotID != tok.ID {
		t.Fatalf("check: %v %v %v", gotAcct, gotID, err)
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
	if err != nil || len(list) != 1 || list[0] != tok {
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
	text, _, err := s.Issue(ctx, ids.NewAccountID(), "", &exp)
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
