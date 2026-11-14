package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/brianmcgee/rmhub/internal/state"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
	"github.com/brianmcgee/rmhub/internal/state/statetest"
)

func open(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(filepath.Join(t.TempDir(), "state.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestContract(t *testing.T) {
	statetest.Run(t, func(t *testing.T) state.Store { return open(t) })
}

func TestFailureIsNotConflict(t *testing.T) {
	s := open(t)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := s.PutIfAbsent(context.Background(), "k", []byte("v"))
	if err == nil || errors.Is(err, state.ErrConflict) {
		t.Fatalf("err = %v, want a failure that is not a conflict", err)
	}
}
