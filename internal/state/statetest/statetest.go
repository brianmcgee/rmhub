// Package statetest is the contract test suite for state.Store backends.
package statetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/brianmcgee/rmhub/internal/state"
)

// Factory returns a new, empty store for one test.
type Factory func(t *testing.T) state.Store

func mustPut(t *testing.T, s state.Store, key, value string) string {
	t.Helper()
	v, err := s.PutIfAbsent(context.Background(), key, []byte(value))
	if err != nil {
		t.Fatalf("put %s: %v", key, err)
	}
	return v
}

func mustGet(t *testing.T, s state.Store, key string) (string, string) {
	t.Helper()
	b, v, err := s.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	return string(b), v
}

// Run runs each contract test against stores from newStore.
func Run(t *testing.T, newStore Factory) {
	ctx := context.Background()

	t.Run("InvalidKey", func(t *testing.T) {
		s := newStore(t)
		const bad = "accounts/../server/keys"
		if _, err := s.PutIfAbsent(ctx, bad, []byte("x")); !errors.Is(err, state.ErrInvalidKey) {
			t.Errorf("put-if-absent: %v", err)
		}
		if _, err := s.PutIfMatch(ctx, bad, []byte("x"), "v"); !errors.Is(err, state.ErrInvalidKey) {
			t.Errorf("put-if-match: %v", err)
		}
		if _, _, err := s.Get(ctx, bad); !errors.Is(err, state.ErrInvalidKey) {
			t.Errorf("get: %v", err)
		}
		if err := s.Delete(ctx, bad); !errors.Is(err, state.ErrInvalidKey) {
			t.Errorf("delete: %v", err)
		}
		if _, err := s.List(ctx, "../", "", 0); !errors.Is(err, state.ErrInvalidKey) {
			t.Errorf("list: %v", err)
		}
		keys, err := s.List(ctx, "", "", 0)
		if err != nil || len(keys) != 0 {
			t.Fatalf("storage changed: %v, %v", keys, err)
		}
	})

	t.Run("ValidKey", func(t *testing.T) {
		s := newStore(t)
		mustPut(t, s, "accounts/u1/roots/00000000000000000001", "x")
	})

	t.Run("ReadExisting", func(t *testing.T) {
		s := newStore(t)
		v1 := mustPut(t, s, "k", "V")
		got, v := mustGet(t, s, "k")
		if got != "V" || v == "" || v != v1 {
			t.Fatalf("get = %q %q, put version %q", got, v, v1)
		}
	})

	t.Run("ReadMissing", func(t *testing.T) {
		s := newStore(t)
		if _, _, err := s.Get(ctx, "missing"); !errors.Is(err, state.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("PutIfAbsentExisting", func(t *testing.T) {
		s := newStore(t)
		mustPut(t, s, "k", "V1")
		if _, err := s.PutIfAbsent(ctx, "k", []byte("V2")); !errors.Is(err, state.ErrConflict) {
			t.Fatalf("err = %v, want ErrConflict", err)
		}
		if got, _ := mustGet(t, s, "k"); got != "V1" {
			t.Fatalf("value = %q, want V1", got)
		}
	})

	t.Run("ParallelPutIfAbsent", func(t *testing.T) {
		s := newStore(t)
		const n = 32
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				_, errs[i] = s.PutIfAbsent(ctx, "race", fmt.Appendf(nil, "writer %d", i))
			})
		}
		wg.Wait()
		countResults(t, errs)
	})

	t.Run("PutIfMatch", func(t *testing.T) {
		s := newStore(t)
		e1 := mustPut(t, s, "k", "V1")
		e2, err := s.PutIfMatch(ctx, "k", []byte("V2"), e1)
		if err != nil || e2 == "" {
			t.Fatalf("put-if-match = %q, %v", e2, err)
		}
		got, v := mustGet(t, s, "k")
		if got != "V2" || v != e2 {
			t.Fatalf("get = %q %q, want V2 %q", got, v, e2)
		}
	})

	t.Run("PutIfMatchStale", func(t *testing.T) {
		s := newStore(t)
		e1 := mustPut(t, s, "k", "V1")
		if _, err := s.PutIfMatch(ctx, "k", []byte("V2"), e1); err != nil {
			t.Fatal(err)
		}
		if _, err := s.PutIfMatch(ctx, "k", []byte("V3"), e1); !errors.Is(err, state.ErrConflict) {
			t.Fatalf("err = %v, want ErrConflict", err)
		}
		if got, _ := mustGet(t, s, "k"); got != "V2" {
			t.Fatalf("value = %q, want V2", got)
		}
	})

	t.Run("PutIfMatchMissing", func(t *testing.T) {
		s := newStore(t)
		e1 := mustPut(t, s, "other", "V1")
		if _, err := s.PutIfMatch(ctx, "k", []byte("V"), e1); !errors.Is(err, state.ErrConflict) {
			t.Fatalf("err = %v, want ErrConflict", err)
		}
		if _, _, err := s.Get(ctx, "k"); !errors.Is(err, state.ErrNotFound) {
			t.Fatalf("key exists after the write: %v", err)
		}
	})

	t.Run("ParallelPutIfMatch", func(t *testing.T) {
		s := newStore(t)
		e1 := mustPut(t, s, "race", "start")
		const n = 32
		errs := make([]error, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				_, errs[i] = s.PutIfMatch(ctx, "race", fmt.Appendf(nil, "writer %d", i), e1)
			})
		}
		wg.Wait()
		countResults(t, errs)
	})

	t.Run("IdenticalBytes", func(t *testing.T) {
		s := newStore(t)
		e1 := mustPut(t, s, "k", "V")
		e2, err := s.PutIfMatch(ctx, "k", []byte("V"), e1)
		if err != nil || e2 == "" {
			t.Fatalf("put-if-match of identical bytes = %q, %v", e2, err)
		}
		if _, v := mustGet(t, s, "k"); v != e2 {
			t.Fatalf("version %q, want %q", v, e2)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		s := newStore(t)
		mustPut(t, s, "k", "V")
		if err := s.Delete(ctx, "k"); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Get(ctx, "k"); !errors.Is(err, state.ErrNotFound) {
			t.Fatalf("get after delete: %v", err)
		}
		if err := s.Delete(ctx, "k"); err != nil {
			t.Fatalf("delete of a missing key: %v", err)
		}
		mustPut(t, s, "k", "again")
	})

	t.Run("ListPrefix", func(t *testing.T) {
		s := newStore(t)
		// No key is also a prefix of another key followed by "/". S3
		// services with a POSIX backend cannot store both.
		for _, k := range []string{"b/1", "a/3", "a/1", "a/2", "a.x", "ab/1"} {
			mustPut(t, s, k, "x")
		}
		got, err := s.List(ctx, "a/", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"a/1", "a/2", "a/3"}; !slices.Equal(got, want) {
			t.Fatalf("list = %v, want %v", got, want)
		}
		all, err := s.List(ctx, "", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"a.x", "a/1", "a/2", "a/3", "ab/1", "b/1"}; !slices.Equal(all, want) {
			t.Fatalf("list all = %v, want %v", all, want)
		}
	})

	t.Run("ListPages", func(t *testing.T) {
		s := newStore(t)
		for _, k := range []string{"a/1", "a/2", "a/3"} {
			mustPut(t, s, k, "x")
		}
		got, err := s.List(ctx, "a/", "a/1", 1)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"a/2"}; !slices.Equal(got, want) {
			t.Fatalf("list = %v, want %v", got, want)
		}
		got, err = s.List(ctx, "a/", "a/3", 10)
		if err != nil || len(got) != 0 {
			t.Fatalf("list after last = %v, %v", got, err)
		}
	})

	t.Run("ListMany", func(t *testing.T) {
		s := newStore(t)
		var want []string
		for i := range 1100 {
			k := fmt.Sprintf("m/%05d", i)
			want = append(want, k)
			mustPut(t, s, k, "x")
		}
		got, err := s.List(ctx, "m/", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Fatalf("list returned %d keys, want %d", len(got), len(want))
		}
	})
}

func countResults(t *testing.T, errs []error) {
	t.Helper()
	ok, conflict := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			ok++
		case errors.Is(err, state.ErrConflict):
			conflict++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || conflict != len(errs)-1 {
		t.Fatalf("%d successes and %d conflicts, want 1 and %d", ok, conflict, len(errs)-1)
	}
}
