package rootlog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

func memLog(t *testing.T) *Log {
	t.Helper()
	st, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return New(st)
}

func TestTwoListeners(t *testing.T) {
	l := memLog(t)
	ctx := context.Background()
	acct := ids.NewAccountID()
	a, b := l.Subscribe(acct), l.Subscribe(acct)
	other := l.Subscribe(ids.NewAccountID())
	defer a.Close()
	defer b.Close()
	defer other.Close()

	if _, err := l.Commit(ctx, acct, 0, hashOf("a"), tablet, true); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Commit(ctx, acct, 1, hashOf("b"), Source{Type: SourceToken, ID: "t"}, false); err != nil {
		t.Fatal(err)
	}
	for _, ln := range []*Listener{a, b} {
		e1, e2 := <-ln.C, <-ln.C
		if e1.Generation != 1 || e1.Hash != hashOf("a") || !e1.Broadcast || e1.Source != tablet || e1.Account != acct {
			t.Fatalf("event 1: %+v", e1)
		}
		// Commit without broadcast.
		if e2.Generation != 2 || e2.Broadcast || e2.Source.Type != SourceToken {
			t.Fatalf("event 2: %+v", e2)
		}
		if ln.Lost() {
			t.Fatal("lost flag set")
		}
	}
	if len(other.C) != 0 {
		t.Fatal("listener of another account got an event")
	}
}

func TestFailedCommitSendsNoEvent(t *testing.T) {
	l := memLog(t)
	ctx := context.Background()
	acct := ids.NewAccountID()
	if _, err := l.Commit(ctx, acct, 0, hashOf("a"), tablet, true); err != nil {
		t.Fatal(err)
	}
	ln := l.Subscribe(acct)
	defer ln.Close()
	if _, err := l.Commit(ctx, acct, 0, hashOf("b"), tablet, true); !errors.Is(err, ErrStale) {
		t.Fatalf("commit: %v", err)
	}
	if len(ln.C) != 0 {
		t.Fatal("stale commit sent an event")
	}
}

func TestBlockedListener(t *testing.T) {
	l := memLog(t)
	ctx := context.Background()
	acct := ids.NewAccountID()
	blocked := l.Subscribe(acct) // never read
	defer blocked.Close()
	all := l.Subscribe("")
	defer all.Close()

	got := make(chan int)
	go func() {
		n := 0
		for e := range all.C {
			n++
			if e.Generation == 100 {
				break
			}
		}
		got <- n
	}()

	start := time.Now()
	for g := range uint64(100) {
		if _, err := l.Commit(ctx, acct, g, hashOf("a"), tablet, true); err != nil {
			t.Fatal(err)
		}
	}
	elapsed := time.Since(start)
	if n := <-got; n != 100 {
		t.Fatalf("other listener got %d events, want 100", n)
	}
	if !blocked.Lost() {
		t.Fatal("blocked listener has no lost-events flag")
	}
	if blocked.Lost() {
		t.Fatal("Lost did not clear the flag")
	}
	if len(blocked.C) != BufferSize {
		t.Fatalf("blocked listener holds %d events, want %d", len(blocked.C), BufferSize)
	}
	// Each commit does a few SQLite writes with fsync. A commit that waits
	// for the blocked listener would never finish.
	t.Logf("100 commits in %v", elapsed)
}

func TestClosedListenerGetsNothing(t *testing.T) {
	b := NewBroker()
	ln := b.Subscribe("x")
	ln.Close()
	ln.Close()
	b.Publish(Event{Account: "x"})
	if len(ln.C) != 0 {
		t.Fatal("closed listener got an event")
	}
}
