// Package feed computes the changes between two library snapshots for
// the change feed.
package feed

import (
	"slices"
	"strings"

	"github.com/brianmcgee/rmhub/internal/grants"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/rootlog"
)

// Change operations.
const (
	OpCreated = "created"
	OpUpdated = "updated"
	OpMoved   = "moved"
	OpTrashed = "trashed"
	OpRemoved = "removed"
)

// Change is one changed entry. Entry is nil for OpRemoved.
type Change struct {
	ID    string         `json:"id"`
	Op    string         `json:"op"`
	Entry *library.Entry `json:"entry"`
}

// Event is the data of one feed event.
type Event struct {
	Generation uint64   `json:"generation"`
	Source     Source   `json:"source"`
	Changes    []Change `json:"changes"`
}

// Source is the writer of a generation.
type Source struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// SourceOf returns the feed form of a root log source.
func SourceOf(s rootlog.Source) Source { return Source{Type: s.Type, ID: s.ID} }

// Diff returns the changes from prev to next, sorted by ID. For a scoped
// token, sc limits the changes to visible entries: an entry that becomes
// visible is created, and an entry that stops to be visible is removed.
// sc is nil for an owner token.
func Diff(prev, next *library.Snapshot, sc *grants.Scope) []Change {
	vis := func(s *library.Snapshot, id string) (*library.Doc, bool) {
		d, ok := s.Doc(id)
		if !ok || (sc != nil && !sc.Visible(s, id)) {
			return nil, false
		}
		return d, true
	}
	var out []Change
	for _, d := range next.Docs() {
		n, ok := vis(next, d.ID)
		if !ok {
			continue
		}
		p, had := vis(prev, d.ID)
		op := ""
		switch {
		case !had:
			op = OpCreated
		case n.Trashed && !p.Trashed:
			op = OpTrashed
		case n.Parent != p.Parent:
			op = OpMoved
		case n.Hash != p.Hash || n.Trashed != p.Trashed:
			op = OpUpdated
		default:
			continue
		}
		e := n.Entry
		if sc != nil && sc.IsTop(e.ID) {
			e.Parent = ""
		}
		out = append(out, Change{ID: d.ID, Op: op, Entry: &e})
	}
	for _, d := range prev.Docs() {
		if _, had := vis(prev, d.ID); !had {
			continue
		}
		if _, ok := vis(next, d.ID); !ok {
			out = append(out, Change{ID: d.ID, Op: OpRemoved})
		}
	}
	slices.SortFunc(out, func(a, b Change) int { return strings.Compare(a.ID, b.ID) })
	return out
}
