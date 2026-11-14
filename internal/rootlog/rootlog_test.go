package rootlog

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/s3conn"
	"github.com/brianmcgee/rmhub/internal/s3conn/s3test"
	"github.com/brianmcgee/rmhub/internal/state"
	states3 "github.com/brianmcgee/rmhub/internal/state/s3"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

// backends runs the test on each state backend. The S3 backend runs
// when RMHUB_TEST_S3_ENDPOINT is set.
func backends(t *testing.T, fn func(t *testing.T, st state.Store)) {
	t.Run("sqlite", func(t *testing.T) {
		st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		fn(t, st)
	})
	t.Run("s3", func(t *testing.T) {
		conn, err := s3conn.New(context.Background(), s3test.Settings(t))
		if err != nil {
			t.Fatal(err)
		}
		fn(t, states3.New(conn))
	})
}

func hashOf(s string) string {
	return strings.Repeat(s, 64)[:64]
}

var tablet = Source{Type: SourceDevice, ID: "dev-1", Desc: "remarkable"}

func TestKeyOrder(t *testing.T) {
	acct := ids.NewAccountID()
	gens := []uint64{100, 9, 10}
	keys := make([]string, len(gens))
	for i, g := range gens {
		keys[i] = RootKey(acct, g)
	}
	sort.Strings(keys)
	want := []string{RootKey(acct, 9), RootKey(acct, 10), RootKey(acct, 100)}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("byte order %v, want %v", keys, want)
		}
	}
	if got := RootKey(acct, 9); !strings.HasSuffix(got, "/roots/00000000000000000009") {
		t.Fatalf("key %q", got)
	}
}

func TestCommitAndRead(t *testing.T) {
	backends(t, func(t *testing.T, st state.Store) {
		ctx := context.Background()
		clock := time.Date(2026, 11, 14, 12, 0, 0, 500, time.UTC)
		l := New(st, WithClock(func() time.Time { return clock }))
		acct := ids.NewAccountID()

		// New account.
		cur, err := l.Current(ctx, acct)
		if err != nil || cur.Generation != 0 || cur.Hash != "" {
			t.Fatalf("new account: %+v, %v", cur, err)
		}

		// First commit.
		e1, err := l.Commit(ctx, acct, 0, hashOf("a"), tablet, true)
		if err != nil {
			t.Fatal(err)
		}
		if e1.Generation != 1 || e1.Hash != hashOf("a") || e1.Source != tablet || !e1.Time.Equal(clock.Truncate(time.Second)) || len(e1.WriteID) != 32 {
			t.Fatalf("first commit: %+v", e1)
		}

		// Commit from a token.
		tok := Source{Type: SourceToken, ID: "tok1"}
		e2, err := l.Commit(ctx, acct, 1, hashOf("b"), tok, false)
		if err != nil || e2.Generation != 2 {
			t.Fatalf("second commit: %+v, %v", e2, err)
		}

		// Old generation.
		if _, err := l.Commit(ctx, acct, 1, hashOf("c"), tablet, true); !errors.Is(err, ErrStale) {
			t.Fatalf("old generation: %v", err)
		}
		// Future generation.
		if _, err := l.Commit(ctx, acct, 7, hashOf("c"), tablet, true); !errors.Is(err, ErrStale) {
			t.Fatalf("future generation: %v", err)
		}

		cur, err = New(st).Current(ctx, acct)
		if err != nil || cur.Generation != 2 || cur.Hash != hashOf("b") || cur.Source != tok {
			t.Fatalf("current: %+v, %v", cur, err)
		}
		// Read an old generation.
		old, err := l.Get(ctx, acct, 1)
		if err != nil || old != e1 {
			t.Fatalf("generation 1: %+v, %v (want %+v)", old, err, e1)
		}
		// Read a generation that does not exist.
		if _, err := l.Get(ctx, acct, 3); !errors.Is(err, ErrNotFound) {
			t.Fatalf("generation 3: %v", err)
		}
		if _, err := l.Get(ctx, acct, 0); !errors.Is(err, ErrNotFound) {
			t.Fatalf("generation 0: %v", err)
		}
	})
}

func TestCommitRefusesInput(t *testing.T) {
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	l := New(st)
	ctx := context.Background()
	if _, err := l.Commit(ctx, "../x", 0, hashOf("a"), tablet, true); err == nil {
		t.Fatal("bad account accepted")
	}
	if _, err := l.Commit(ctx, ids.NewAccountID(), 0, "ABC", tablet, true); err == nil {
		t.Fatal("bad hash accepted")
	}
}

func TestParallelCommits(t *testing.T) {
	backends(t, func(t *testing.T, st state.Store) {
		ctx := context.Background()
		acct := ids.NewAccountID()
		var ok, stale atomic.Int32
		var wg sync.WaitGroup
		for i := range 32 {
			wg.Go(func() {
				// Each writer has its own instance, like separate servers.
				_, err := New(st).Commit(ctx, acct, 0, hashOf(string(rune('a'+i%6))), tablet, true)
				switch {
				case err == nil:
					ok.Add(1)
				case errors.Is(err, ErrStale):
					stale.Add(1)
				default:
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if ok.Load() != 1 || stale.Load() != 31 {
			t.Fatalf("%d succeeded, %d stale; want 1 and 31", ok.Load(), stale.Load())
		}
	})
}

func setHead(t *testing.T, st state.Store, acct string, g uint64) {
	t.Helper()
	ctx := context.Background()
	b, _ := json.Marshal(head{Generation: g})
	if err := st.Delete(ctx, HeadKey(acct)); err != nil && !errors.Is(err, state.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := st.PutIfAbsent(ctx, HeadKey(acct), b); err != nil {
		t.Fatal(err)
	}
}

func headOf(t *testing.T, st state.Store, acct string) uint64 {
	t.Helper()
	b, _, err := st.Get(context.Background(), HeadKey(acct))
	if err != nil {
		t.Fatal(err)
	}
	var h head
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	return h.Generation
}

func TestStaleHeadHint(t *testing.T) {
	backends(t, func(t *testing.T, st state.Store) {
		ctx := context.Background()
		acct := ids.NewAccountID()
		l := New(st)
		for g := range uint64(3) {
			if _, err := l.Commit(ctx, acct, g, hashOf(string(rune('a'+g))), tablet, false); err != nil {
				t.Fatal(err)
			}
		}
		if headOf(t, st, acct) != 3 {
			t.Fatalf("head hint %d, want 3", headOf(t, st, acct))
		}

		// Head hint update failed.
		setHead(t, st, acct, 1)
		cur, err := New(st).Current(ctx, acct)
		if err != nil || cur.Generation != 3 || cur.Hash != hashOf("c") {
			t.Fatalf("stale hint: %+v, %v", cur, err)
		}
		// Head hint is missing.
		if err := st.Delete(ctx, HeadKey(acct)); err != nil {
			t.Fatal(err)
		}
		cur, err = New(st).Current(ctx, acct)
		if err != nil || cur.Generation != 3 {
			t.Fatalf("missing hint: %+v, %v", cur, err)
		}
		// A commit with a missing hint creates the hint again.
		if _, err := l.Commit(ctx, acct, 3, hashOf("d"), tablet, false); err != nil {
			t.Fatal(err)
		}
		if headOf(t, st, acct) != 4 {
			t.Fatalf("head hint %d, want 4", headOf(t, st, acct))
		}
	})
}

func TestHeadMovesOnlyForward(t *testing.T) {
	backends(t, func(t *testing.T, st state.Store) {
		ctx := context.Background()
		acct := ids.NewAccountID()
		l := New(st)
		l.advanceHead(ctx, acct, 20)
		l.advanceHead(ctx, acct, 19)
		if got := headOf(t, st, acct); got != 20 {
			t.Fatalf("head hint %d, want 20", got)
		}
	})
}

func TestCacheSeesOtherInstance(t *testing.T) {
	backends(t, func(t *testing.T, st state.Store) {
		ctx := context.Background()
		acct := ids.NewAccountID()
		a, b := New(st), New(st)
		if _, err := a.Commit(ctx, acct, 0, hashOf("a"), tablet, false); err != nil {
			t.Fatal(err)
		}
		if cur, err := a.Current(ctx, acct); err != nil || cur.Generation != 1 {
			t.Fatalf("a: %+v, %v", cur, err)
		}
		if _, err := b.Commit(ctx, acct, 1, hashOf("b"), tablet, false); err != nil {
			t.Fatal(err)
		}
		cur, err := a.Current(ctx, acct)
		if err != nil || cur.Generation != 2 || cur.Hash != hashOf("b") {
			t.Fatalf("a after commit by b: %+v, %v", cur, err)
		}
		// A commit by a with the old generation is stale.
		if _, err := a.Commit(ctx, acct, 1, hashOf("c"), tablet, false); !errors.Is(err, ErrStale) {
			t.Fatalf("stale commit: %v", err)
		}
	})
}

var errStorage = errors.New("storage failure")

// faulty changes the result of a put-if-absent of a root entry.
type faulty struct {
	state.Store
	// mode is "lost" (store, then fail once), "other" (another writer
	// stores an entry, then fail once) or "down" (fail, store nothing).
	mode  string
	fired atomic.Bool
}

func (f *faulty) PutIfAbsent(ctx context.Context, key string, value []byte) (string, error) {
	if !strings.Contains(key, "/roots/") {
		return f.Store.PutIfAbsent(ctx, key, value)
	}
	switch f.mode {
	case "lost":
		if !f.fired.Swap(true) {
			if _, err := f.Store.PutIfAbsent(ctx, key, value); err != nil {
				return "", err
			}
			return "", errStorage
		}
	case "other":
		if !f.fired.Swap(true) {
			other, _ := json.Marshal(Entry{Hash: hashOf("f"), WriteID: "other"})
			if _, err := f.Store.PutIfAbsent(ctx, key, other); err != nil {
				return "", err
			}
			return "", errStorage
		}
	case "down":
		return "", errStorage
	}
	return f.Store.PutIfAbsent(ctx, key, value)
}

func TestUnknownOutcome(t *testing.T) {
	ctx := context.Background()
	open := func(t *testing.T) state.Store {
		st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		return st
	}

	t.Run("lost answer", func(t *testing.T) {
		st := &faulty{Store: open(t), mode: "lost"}
		l := New(st)
		ev := l.Subscribe("")
		acct := ids.NewAccountID()
		e, err := l.Commit(ctx, acct, 0, hashOf("a"), tablet, true)
		if err != nil || e.Generation != 1 {
			t.Fatalf("commit: %+v, %v", e, err)
		}
		if cur, _ := New(st).Current(ctx, acct); cur != e {
			t.Fatalf("current %+v, want %+v", cur, e)
		}
		if len(ev.C) != 1 {
			t.Fatalf("%d events, want 1", len(ev.C))
		}
	})

	t.Run("another writer", func(t *testing.T) {
		st := &faulty{Store: open(t), mode: "other"}
		l := New(st)
		ev := l.Subscribe("")
		_, err := l.Commit(ctx, ids.NewAccountID(), 0, hashOf("a"), tablet, true)
		if !errors.Is(err, ErrStale) {
			t.Fatalf("commit: %v, want stale", err)
		}
		if len(ev.C) != 0 {
			t.Fatal("stale commit sent an event")
		}
	})

	t.Run("second failure", func(t *testing.T) {
		st := &faulty{Store: open(t), mode: "down"}
		l := New(st)
		acct := ids.NewAccountID()
		_, err := l.Commit(ctx, acct, 0, hashOf("a"), tablet, true)
		if err == nil || errors.Is(err, ErrStale) || !errors.Is(err, errStorage) {
			t.Fatalf("commit: %v, want the storage failure", err)
		}
		if cur, _ := l.Current(ctx, acct); cur.Generation != 0 {
			t.Fatalf("current %+v after failure", cur)
		}
	})
}
