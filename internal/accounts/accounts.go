// Package accounts keeps the account records and the email claims.
//
// A create writes two keys: first the email claim emails/<hash> with
// put-if-absent, then the account record accounts/<id>/account. A create
// that stops halfway leaves an orphan claim. A later create takes over an
// orphan claim when it is older than OrphanAge.
package accounts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
)

var (
	// ErrNotFound means that the account does not exist.
	ErrNotFound = errors.New("account not found")
	// ErrEmailInUse means that another account has the email address.
	ErrEmailInUse = errors.New("email address is in use")
	// ErrInvalidEmail means that the email address is empty or has no @.
	ErrInvalidEmail = errors.New("email address is not valid")
	// ErrInvalidSchema means that the schema version is not 3 or 4.
	ErrInvalidSchema = errors.New("schema version must be 3 or 4")
)

// DefaultSchema is the announced schema of a new account.
const DefaultSchema = 3

// OrphanAge is the age after which a claim without an account can be
// taken over. A create that is still in progress finishes well within
// this time.
const OrphanAge = time.Minute

// Account is an account record. It has no password.
type Account struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	// SchemaVersion is the schema that GET /sync/v4/root announces: 3
	// or 4.
	SchemaVersion int `json:"schemaVersion"`
}

type claim struct {
	Account   string    `json:"account"`
	CreatedAt time.Time `json:"createdAt"`
}

// Store keeps accounts in a state store.
type Store struct {
	st  state.Store
	now func() time.Time
}

// New returns an account store.
func New(st state.Store) *Store { return &Store{st: st, now: time.Now} }

// SetClock sets the clock. Tests use it.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

func recordKey(id string) string { return "accounts/" + id + "/account" }

func claimKey(email string) string {
	h := sha256.Sum256([]byte(strings.ToLower(email)))
	return "emails/" + hex.EncodeToString(h[:])
}

// Create makes an account. The email address is unique without regard
// to case.
func (s *Store) Create(ctx context.Context, email, name string) (Account, error) {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return Account{}, ErrInvalidEmail
	}
	now := s.now().UTC().Truncate(time.Second)
	acct := Account{ID: ids.NewAccountID(), Email: email, Name: strings.TrimSpace(name), CreatedAt: now, SchemaVersion: DefaultSchema}
	ck := claimKey(email)
	cv, _ := json.Marshal(claim{Account: acct.ID, CreatedAt: now})

	if err := s.takeClaim(ctx, ck, cv); err != nil {
		return Account{}, err
	}
	rv, _ := json.Marshal(acct)
	if _, err := s.st.PutIfAbsent(ctx, recordKey(acct.ID), rv); err != nil {
		return Account{}, err
	}
	// A stalled create can lose its claim to a takeover. Then the claim
	// names another account, and this account must not stay.
	b, _, err := s.st.Get(ctx, ck)
	if err != nil {
		return Account{}, err
	}
	var c claim
	if err := json.Unmarshal(b, &c); err != nil || c.Account != acct.ID {
		_ = s.st.Delete(ctx, recordKey(acct.ID))
		return Account{}, ErrEmailInUse
	}
	return acct, nil
}

// takeClaim writes the claim, or takes over an orphan claim.
func (s *Store) takeClaim(ctx context.Context, key string, value []byte) error {
	for range 5 {
		_, err := s.st.PutIfAbsent(ctx, key, value)
		if err == nil {
			return nil
		}
		if !errors.Is(err, state.ErrConflict) {
			return err
		}
		b, ver, err := s.st.Get(ctx, key)
		if errors.Is(err, state.ErrNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		var c claim
		if err := json.Unmarshal(b, &c); err != nil {
			return fmt.Errorf("email claim %s: %w", key, err)
		}
		if _, err := s.Get(ctx, c.Account); err == nil {
			return ErrEmailInUse
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if s.now().Sub(c.CreatedAt) < OrphanAge {
			// Another create is in progress.
			return ErrEmailInUse
		}
		_, err = s.st.PutIfMatch(ctx, key, value, ver)
		if err == nil {
			return nil
		}
		if !errors.Is(err, state.ErrConflict) {
			return err
		}
	}
	return ErrEmailInUse
}

// Get returns one account.
func (s *Store) Get(ctx context.Context, id string) (Account, error) {
	a, _, err := s.get(ctx, id)
	return a, err
}

func (s *Store) get(ctx context.Context, id string) (Account, string, error) {
	if !ids.IsAccountID(id) {
		return Account{}, "", ErrNotFound
	}
	b, v, err := s.st.Get(ctx, recordKey(id))
	if errors.Is(err, state.ErrNotFound) {
		return Account{}, "", ErrNotFound
	}
	if err != nil {
		return Account{}, "", err
	}
	var a Account
	if err := json.Unmarshal(b, &a); err != nil {
		return Account{}, "", fmt.Errorf("account %s: %w", id, err)
	}
	if a.SchemaVersion == 0 {
		a.SchemaVersion = DefaultSchema
	}
	return a, v, nil
}

// SetSchema changes the announced schema of the account with a
// put-if-match.
func (s *Store) SetSchema(ctx context.Context, id string, schema int) (Account, error) {
	if schema != 3 && schema != 4 {
		return Account{}, ErrInvalidSchema
	}
	for {
		a, v, err := s.get(ctx, id)
		if err != nil {
			return Account{}, err
		}
		a.SchemaVersion = schema
		b, _ := json.Marshal(a)
		_, err = s.st.PutIfMatch(ctx, recordKey(id), b, v)
		if errors.Is(err, state.ErrConflict) {
			continue
		}
		if err != nil {
			return Account{}, err
		}
		return a, nil
	}
}

// List returns all accounts, ordered by ID. It reads the email claims,
// and ignores each claim without an account.
func (s *Store) List(ctx context.Context) ([]Account, error) {
	keys, err := s.st.List(ctx, "emails/", "", 0)
	if err != nil {
		return nil, err
	}
	out := []Account{}
	for _, k := range keys {
		b, _, err := s.st.Get(ctx, k)
		if errors.Is(err, state.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var c claim
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, fmt.Errorf("email claim %s: %w", k, err)
		}
		a, err := s.Get(ctx, c.Account)
		if errors.Is(err, ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b Account) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}
