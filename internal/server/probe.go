package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
)

// ErrNoConditionalWrites means that the state backend accepted a write
// whose condition was false.
var ErrNoConditionalWrites = errors.New("the state backend does not enforce conditional writes")

// Probe checks that the state backend refuses stale conditional writes.
// It uses a key under server/probe/ and deletes the key after the check.
func Probe(ctx context.Context, s state.Store) (err error) {
	key := "server/probe/" + ids.Random(16)
	v1, err := s.PutIfAbsent(ctx, key, []byte("probe 1"))
	if err != nil {
		return fmt.Errorf("probe: first write: %w", err)
	}
	defer func() {
		if derr := s.Delete(ctx, key); derr != nil && err == nil {
			err = fmt.Errorf("probe: delete: %w", derr)
		}
	}()
	if _, err := s.PutIfAbsent(ctx, key, []byte("probe 2")); !errors.Is(err, state.ErrConflict) {
		return probeFail("put-if-absent on an existing key", err)
	}
	v2, err := s.PutIfMatch(ctx, key, []byte("probe 3"), v1)
	if err != nil {
		return fmt.Errorf("probe: update: %w", err)
	}
	if v2 == v1 {
		return fmt.Errorf("probe: the version did not change after a write of other bytes")
	}
	if _, err := s.PutIfMatch(ctx, key, []byte("probe 4"), v1); !errors.Is(err, state.ErrConflict) {
		return probeFail("put-if-match with a stale version", err)
	}
	return nil
}

func probeFail(what string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: it accepted a %s", ErrNoConditionalWrites, what)
	}
	return fmt.Errorf("probe: %s: %w", what, err)
}
