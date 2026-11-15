package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/feed"
	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// FeedTimers has the intervals of the change feed. A zero value means 30
// seconds.
type FeedTimers struct {
	// Poll is the time between two reads of the root log without a
	// commit event.
	Poll time.Duration
	// Keepalive is the time between two SSE comment lines.
	Keepalive time.Duration
	// TokenCheck is the time between two checks of the token chain.
	TokenCheck time.Duration
}

func orDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return 30 * time.Second
	}
	return d
}

// startAfter reads the start generation from the query after or the
// header Last-Event-ID. Without both, the feed starts after current.
func startAfter(r *http.Request, current uint64) (uint64, error) {
	v := r.URL.Query().Get("after")
	name := "the query after"
	if v == "" {
		v = r.Header.Get("Last-Event-ID")
		name = "the header Last-Event-ID"
	}
	if v == "" {
		return current, nil
	}
	g, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
	if err != nil {
		return 0, Errorf(CodeBadRequest, "%s must be a generation number", name)
	}
	if g > current {
		return 0, Errorf(CodeBadRequest, "%s is %d, but the current generation is %d", name, g, current)
	}
	return g, nil
}

// changes streams the change feed. The commit events only wake the feed.
// The feed reads each generation from the root log, so a lost event
// causes no gap.
func (l *Library) changes(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	p := PrincipalFrom(ctx)
	sub := l.Roots.Subscribe(p.Account)
	defer sub.Close()
	cur, err := l.Roots.Current(ctx, p.Account)
	if err != nil {
		return err
	}
	last, err := startAfter(r, cur.Generation)
	if err != nil {
		return err
	}
	prev, err := l.Lib.At(ctx, p.Account, last, nil)
	if err != nil {
		return err
	}
	sc := grants.FromChain(p.Chain)
	// Auth checked the form "Bearer <token>".
	tok := strings.TrimSpace(r.Header.Get("Authorization")[7:])

	flusher, _ := w.(http.Flusher)
	flush := func() {
		if flusher != nil {
			flusher.Flush()
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flush()

	poll := time.NewTicker(orDefault(l.Feed.Poll))
	defer poll.Stop()
	keep := time.NewTicker(orDefault(l.Feed.Keepalive))
	defer keep.Stop()
	check := time.NewTicker(orDefault(l.Feed.TokenCheck))
	defer check.Stop()

	// send writes the events of each generation after last.
	send := func() error {
		cur, err := l.Roots.Current(ctx, p.Account)
		if err != nil {
			return err
		}
		for g := last + 1; g <= cur.Generation; g++ {
			next, err := l.Lib.At(ctx, p.Account, g, prev)
			if err != nil {
				return err
			}
			if cs := feed.Diff(prev, next, sc); len(cs) > 0 {
				if err := writeEvent(w, next, cs); err != nil {
					return err
				}
				flush()
			}
			prev, last = next, g
		}
		return nil
	}
	if err := send(); err != nil {
		return l.endFeed(ctx, p.Account, err)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-l.Stop:
			return nil
		case <-sub.C:
		case <-poll.C:
		case <-keep.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return nil
			}
			flush()
			continue
		case <-check.C:
			if _, _, err := l.Tokens.Check(ctx, tok); err != nil {
				if !errors.Is(err, tokens.ErrInvalid) {
					l.Log.Warn("change feed: token check failed", "account", p.Account, "err", err)
				}
				return nil
			}
			continue
		}
		sub.Lost()
		if err := send(); err != nil {
			return l.endFeed(ctx, p.Account, err)
		}
	}
}

// endFeed logs an error after the start of the stream. The status line is
// sent, so the client sees only the end of the stream.
func (l *Library) endFeed(ctx context.Context, account string, err error) error {
	if ctx.Err() != nil {
		return nil
	}
	l.Log.Warn("change feed stopped", "account", account, "err", err)
	return nil
}

func writeEvent(w http.ResponseWriter, s *library.Snapshot, cs []feed.Change) error {
	data, err := json.Marshal(feed.Event{Generation: s.Generation, Source: feed.SourceOf(s.Source), Changes: cs})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: change\nid: %d\ndata: %s\n\n", s.Generation, data)
	return err
}
