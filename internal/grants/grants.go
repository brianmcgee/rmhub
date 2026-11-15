// Package grants applies the grants of a scoped token chain to a library
// snapshot.
//
// Each token of the chain must see an entry for the entry to be visible,
// and each token must have write access for a write. For one token, the
// walk goes from the entry to the top level and stops at the first grant
// folder. The access of that grant applies.
package grants

import (
	"cmp"
	"slices"
	"strings"

	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// Scope is the grants of a scoped token and of each scoped ancestor.
type Scope struct {
	// chain holds one grant list for each scoped token, the token itself
	// first.
	chain [][]tokens.Grant
}

// FromChain returns the scope of a token chain from tokens.Store.Check.
// It returns nil for an owner token.
func FromChain(chain []tokens.Token) *Scope {
	if len(chain) == 0 || !chain[0].Scoped() {
		return nil
	}
	sc := &Scope{}
	for _, t := range chain {
		if t.Scoped() {
			sc.chain = append(sc.chain, t.Grants)
		}
	}
	return sc
}

// Grants returns the grants of the token itself.
func (sc *Scope) Grants() []tokens.Grant { return sc.chain[0] }

// Len returns the number of scoped tokens in the chain.
func (sc *Scope) Len() int { return len(sc.chain) }

// accessOf walks from the entry to the top level and returns the access
// of the first grant folder, or "".
func accessOf(s *library.Snapshot, id string, grants []tokens.Grant) string {
	cur := id
	for range s.Len() + 1 {
		for _, g := range grants {
			if g.Folder == cur {
				return g.Access
			}
		}
		d, ok := s.Doc(cur)
		if !ok {
			return ""
		}
		cur = d.Parent
	}
	return ""
}

// Access returns the access of the chain to the entry: "", read or write.
func (sc *Scope) Access(s *library.Snapshot, id string) string {
	if _, ok := s.Doc(id); !ok {
		return ""
	}
	access := tokens.AccessWrite
	for _, gs := range sc.chain {
		switch accessOf(s, id, gs) {
		case "":
			return ""
		case tokens.AccessRead:
			access = tokens.AccessRead
		}
	}
	return access
}

// Visible tells if the chain can see the entry.
func (sc *Scope) Visible(s *library.Snapshot, id string) bool { return sc.Access(s, id) != "" }

// CanWrite tells if the chain has write access to the entry.
func (sc *Scope) CanWrite(s *library.Snapshot, id string) bool {
	return sc.Access(s, id) == tokens.AccessWrite
}

// IsGrantFolder tells if the entry is the folder of a grant of a token of
// the chain.
func (sc *Scope) IsGrantFolder(id string) bool {
	for _, gs := range sc.chain {
		for _, g := range gs {
			if g.Folder == id {
				return true
			}
		}
	}
	return false
}

// IsTop tells if the entry is the folder of a grant of the token itself.
// Paths of the token start at such a folder.
func (sc *Scope) IsTop(id string) bool {
	for _, g := range sc.chain[0] {
		if g.Folder == id {
			return true
		}
	}
	return false
}

// Tops returns the visible grant folders of the token itself, sorted by
// name and then by ID.
func (sc *Scope) Tops(s *library.Snapshot) []*library.Doc {
	var out []*library.Doc
	for _, g := range sc.chain[0] {
		if d, ok := s.Doc(g.Folder); ok && sc.Visible(s, g.Folder) && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b *library.Doc) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.ID, b.ID))
	})
	return out
}

// Covers tells if the token itself can give the grant to a child token:
// the grant folder is a grant folder of the token or its descendant, and
// the access is not higher than the access of that grant.
func (sc *Scope) Covers(s *library.Snapshot, g tokens.Grant) bool {
	have := accessOf(s, g.Folder, sc.chain[0])
	switch have {
	case tokens.AccessWrite:
		return true
	case tokens.AccessRead:
		return g.Access == tokens.AccessRead
	}
	return false
}
