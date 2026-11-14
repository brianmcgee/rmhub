package accounts

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

func open(t *testing.T) state.Store {
	t.Helper()
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	s := New(st)

	a, err := s.Create(ctx, "Ada@Example.org", "Ada")
	if err != nil {
		t.Fatal(err)
	}
	if !ids.IsAccountID(a.ID) || a.Email != "Ada@Example.org" || a.Name != "Ada" {
		t.Fatalf("account %+v", a)
	}
	got, err := s.Get(ctx, a.ID)
	if err != nil || got != a {
		t.Fatalf("get: %+v, %v", got, err)
	}
	// The stored record has no password field.
	raw, _, err := st.Get(ctx, recordKey(a.ID))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for k := range fields {
		if k != "id" && k != "email" && k != "name" && k != "createdAt" {
			t.Fatalf("unexpected field %q in %s", k, raw)
		}
	}

	if _, err := s.Create(ctx, "ADA@example.org", ""); !errors.Is(err, ErrEmailInUse) {
		t.Fatalf("email in use: %v", err)
	}
	for _, bad := range []string{"", "  ", "ada"} {
		if _, err := s.Create(ctx, bad, ""); !errors.Is(err, ErrInvalidEmail) {
			t.Fatalf("email %q: %v", bad, err)
		}
	}
	if _, err := s.Get(ctx, ids.NewAccountID()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing account: %v", err)
	}
	if _, err := s.Get(ctx, "../x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bad id: %v", err)
	}
}

func TestOrphanClaim(t *testing.T) {
	ctx := context.Background()
	st := open(t)
	s := New(st)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	s.SetClock(func() time.Time { return now })

	// A create that stopped after the claim.
	cv, _ := json.Marshal(claim{Account: ids.NewAccountID(), CreatedAt: now})
	if _, err := st.PutIfAbsent(ctx, claimKey("ada@example.org"), cv); err != nil {
		t.Fatal(err)
	}
	// A young orphan can be a create in progress.
	if _, err := s.Create(ctx, "ada@example.org", ""); !errors.Is(err, ErrEmailInUse) {
		t.Fatalf("young orphan: %v", err)
	}
	now = now.Add(OrphanAge + time.Second)
	a, err := s.Create(ctx, "ada@example.org", "")
	if err != nil {
		t.Fatalf("old orphan: %v", err)
	}
	list, err := s.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("list %+v, %v", list, err)
	}
}

func TestParallelCreates(t *testing.T) {
	ctx := context.Background()
	s := New(open(t))
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { _, errs[i] = s.Create(ctx, "ada@example.org", "") })
	}
	wg.Wait()
	ok, inUse := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, ErrEmailInUse):
			inUse++
		default:
			t.Fatal(err)
		}
	}
	if ok != 1 || inUse != 1 {
		t.Fatalf("%d created, %d in use", ok, inUse)
	}
	list, _ := s.List(ctx)
	if len(list) != 1 {
		t.Fatalf("%d accounts", len(list))
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	s := New(open(t))
	a, _ := s.Create(ctx, "a@example.org", "")
	b, _ := s.Create(ctx, "b@example.org", "")
	list, err := s.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list %+v, %v", list, err)
	}
	if list[0].ID > list[1].ID {
		t.Fatalf("not ordered by ID: %+v", list)
	}
	if (list[0] != a && list[0] != b) || list[0] == list[1] {
		t.Fatalf("list %+v", list)
	}
}
