// Package rootlog keeps the generations of the library root of each
// account.
//
// Each generation is an immutable entry accounts/<id>/roots/<generation>.
// A put-if-absent of the entry for generation g+1 is the compare-and-swap:
// only one writer can create it. The head hint accounts/<id>/head makes a
// read fast. A reader walks forward from the hint, so a stale hint gives
// no wrong result.
package rootlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
)

var (
	// ErrStale means that the expected generation is not the current
	// generation. The root did not change.
	ErrStale = errors.New("rootlog: stale generation")
	// ErrNotFound means that the generation does not exist.
	ErrNotFound = errors.New("rootlog: generation not found")
)

// Source types.
const (
	SourceDevice = "device"
	SourceToken  = "token"
)

// Source is the origin of a commit: a device or an rmhub API token.
type Source struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	// Desc is the device description. It is empty for a token.
	Desc string `json:"desc,omitempty"`
}

// Entry is one generation of the root.
type Entry struct {
	Generation uint64    `json:"generation"`
	Hash       string    `json:"hash"`
	Time       time.Time `json:"time"`
	WriteID    string    `json:"writeId"`
	Source     Source    `json:"source"`
}

type head struct {
	Generation uint64 `json:"generation"`
}

// RootKey returns the state key of the entry of a generation. The
// generation has 20 digits with leading zeros, so byte order of the keys
// is number order.
func RootKey(account string, generation uint64) string {
	return fmt.Sprintf("accounts/%s/roots/%020d", account, generation)
}

// HeadKey returns the state key of the head hint.
func HeadKey(account string) string { return "accounts/" + account + "/head" }

// Log is the root log on a state store.
type Log struct {
	st     state.Store
	broker *Broker
	log    *slog.Logger
	now    func() time.Time

	mu    sync.Mutex
	cache map[string]Entry // last known entry per account
}

// Option changes a Log.
type Option func(*Log)

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option { return func(r *Log) { r.log = l } }

// WithClock sets the clock.
func WithClock(now func() time.Time) Option { return func(r *Log) { r.now = now } }

// New returns a root log on the store.
func New(st state.Store, opts ...Option) *Log {
	l := &Log{
		st:     st,
		broker: NewBroker(),
		log:    slog.New(slog.DiscardHandler),
		now:    time.Now,
		cache:  map[string]Entry{},
	}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Subscribe returns a listener for the commits of the account. The
// account "" gives the commits of all accounts.
func (l *Log) Subscribe(account string) *Listener { return l.broker.Subscribe(account) }

func checkAccount(account string) error {
	if !ids.IsAccountID(account) {
		return fmt.Errorf("rootlog: invalid account %q", account)
	}
	return nil
}

// Get returns the entry of one generation.
func (l *Log) Get(ctx context.Context, account string, generation uint64) (Entry, error) {
	if err := checkAccount(account); err != nil {
		return Entry{}, err
	}
	if generation == 0 {
		return Entry{}, ErrNotFound
	}
	return l.get(ctx, account, generation)
}

func (l *Log) get(ctx context.Context, account string, generation uint64) (Entry, error) {
	b, _, err := l.st.Get(ctx, RootKey(account, generation))
	if errors.Is(err, state.ErrNotFound) {
		return Entry{}, ErrNotFound
	}
	if err != nil {
		return Entry{}, err
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return Entry{}, fmt.Errorf("rootlog: entry %d of %s: %w", generation, account, err)
	}
	return e, nil
}

// Current returns the newest entry. An account with no commit gives the
// generation 0 and no hash.
func (l *Log) Current(ctx context.Context, account string) (Entry, error) {
	if err := checkAccount(account); err != nil {
		return Entry{}, err
	}
	var h uint64
	b, _, err := l.st.Get(ctx, HeadKey(account))
	switch {
	case err == nil:
		var hd head
		if jerr := json.Unmarshal(b, &hd); jerr == nil {
			h = hd.Generation
		} else {
			l.log.Warn("ignore a broken head hint", "account", account, "err", jerr)
		}
	case errors.Is(err, state.ErrNotFound):
	default:
		return Entry{}, err
	}

	cached, ok := l.cached(account)
	if ok && cached.Generation > h {
		h = cached.Generation
	}
	var last *Entry
	for {
		e, err := l.get(ctx, account, h+1)
		if errors.Is(err, ErrNotFound) {
			break
		}
		if err != nil {
			return Entry{}, err
		}
		h++
		last = &e
	}
	switch {
	case last != nil:
		l.remember(account, *last)
		return *last, nil
	case h == 0:
		return Entry{}, nil
	case ok && cached.Generation == h:
		return cached, nil
	}
	e, err := l.get(ctx, account, h)
	if err != nil {
		return Entry{}, fmt.Errorf("rootlog: head hint names generation %d of %s: %w", h, account, err)
	}
	l.remember(account, e)
	return e, nil
}

func (l *Log) cached(account string) (Entry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.cache[account]
	return e, ok
}

func (l *Log) remember(account string, e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if old, ok := l.cache[account]; !ok || e.Generation > old.Generation {
		l.cache[account] = e
	}
}

// Commit replaces the root if expected is the current generation. It
// returns the new entry, or ErrStale.
func (l *Log) Commit(ctx context.Context, account string, expected uint64, hash string, src Source, broadcast bool) (Entry, error) {
	if err := checkAccount(account); err != nil {
		return Entry{}, err
	}
	if !ids.IsHash(hash) {
		return Entry{}, fmt.Errorf("rootlog: invalid hash %q", hash)
	}
	if expected > 0 {
		// A writer that is ahead of the server names a generation that
		// does not exist. Entries never disappear, so this test has no race.
		if _, err := l.get(ctx, account, expected); errors.Is(err, ErrNotFound) {
			return Entry{}, ErrStale
		} else if err != nil {
			return Entry{}, err
		}
	}
	e := Entry{
		Generation: expected + 1,
		Hash:       hash,
		Time:       l.now().UTC().Truncate(time.Second),
		WriteID:    ids.RandomHex(16),
		Source:     src,
	}
	value, err := json.Marshal(e)
	if err != nil {
		return Entry{}, err
	}
	key := RootKey(account, e.Generation)
	_, err = l.st.PutIfAbsent(ctx, key, value)
	switch {
	case err == nil:
	case errors.Is(err, state.ErrConflict):
		return Entry{}, ErrStale
	default:
		if err = l.resolve(ctx, account, key, e, value, err); err != nil {
			return Entry{}, err
		}
	}
	l.remember(account, e)
	l.advanceHead(ctx, account, e.Generation)
	l.broker.Publish(Event{Account: account, Generation: e.Generation, Hash: e.Hash, Source: src, Broadcast: broadcast})
	return e, nil
}

// resolve finds the outcome of a write that failed. It reads the entry:
// the write ID of the commit means success, another write ID means stale.
// If the entry does not exist, it tries the write once more.
func (l *Log) resolve(ctx context.Context, account, key string, e Entry, value []byte, writeErr error) error {
	retried := false
	for {
		got, err := l.get(ctx, account, e.Generation)
		switch {
		case err == nil && got.WriteID == e.WriteID:
			return nil
		case err == nil:
			return ErrStale
		case !errors.Is(err, ErrNotFound):
			return fmt.Errorf("rootlog: outcome of the commit is not known: %w (read: %v)", writeErr, err)
		case retried:
			return fmt.Errorf("rootlog: commit failed: %w", writeErr)
		}
		retried = true
		_, err = l.st.PutIfAbsent(ctx, key, value)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, state.ErrConflict):
			// Another writer, or the first write that arrived late.
			continue
		default:
			writeErr = err
		}
	}
}

// advanceHead moves the head hint forward to the generation. A failure
// is logged and ignored: readers walk forward from an old hint.
func (l *Log) advanceHead(ctx context.Context, account string, generation uint64) {
	key := HeadKey(account)
	value, _ := json.Marshal(head{Generation: generation})
	for range 10 {
		b, ver, err := l.st.Get(ctx, key)
		switch {
		case errors.Is(err, state.ErrNotFound):
			_, err = l.st.PutIfAbsent(ctx, key, value)
		case err != nil:
		default:
			var hd head
			if jerr := json.Unmarshal(b, &hd); jerr == nil && hd.Generation >= generation {
				return
			}
			_, err = l.st.PutIfMatch(ctx, key, value, ver)
		}
		switch {
		case err == nil:
			return
		case errors.Is(err, state.ErrConflict):
			continue
		default:
			l.log.Warn("head hint update failed", "account", account, "generation", generation, "err", err)
			return
		}
	}
	l.log.Warn("head hint update gave up after conflicts", "account", account, "generation", generation)
}
