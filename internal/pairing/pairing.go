// Package pairing makes and uses the one-time pairing codes.
package pairing

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/state"
)

const (
	// CodeLen is the number of letters in a code.
	CodeLen = 8
	// Lifetime is the time for which a code is valid.
	Lifetime = 5 * time.Minute
)

// ErrInvalidCode means that the code is unknown, expired or used.
var ErrInvalidCode = errors.New("pairing code is not valid")

// Code is a new pairing code.
type Code struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type record struct {
	Account   string    `json:"account"`
	ExpiresAt time.Time `json:"expiresAt"`
	UsedBy    string    `json:"usedBy"`
}

// Store keeps the codes in a state store.
type Store struct {
	st  state.Store
	now func() time.Time
}

// New returns a code store.
func New(st state.Store) *Store { return &Store{st: st, now: time.Now} }

// SetClock sets the clock. Tests use it.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

func key(code string) string { return "pairing/" + code }

func randomCode() string {
	var b strings.Builder
	for range CodeLen {
		n, err := rand.Int(rand.Reader, big.NewInt(26))
		if err != nil {
			panic("pairing: crypto/rand failed: " + err.Error())
		}
		b.WriteByte(byte('a' + n.Int64()))
	}
	return b.String()
}

// Create makes a code for the account.
func (s *Store) Create(ctx context.Context, account string) (Code, error) {
	exp := s.now().UTC().Add(Lifetime).Truncate(time.Second)
	b, err := json.Marshal(record{Account: account, ExpiresAt: exp})
	if err != nil {
		return Code{}, err
	}
	for range 5 {
		c := randomCode()
		_, err := s.st.PutIfAbsent(ctx, key(c), b)
		if errors.Is(err, state.ErrConflict) {
			continue
		}
		if err != nil {
			return Code{}, err
		}
		return Code{Code: c, ExpiresAt: exp}, nil
	}
	return Code{}, errors.New("pairing code collision five times")
}

// Normalize returns the code in lowercase, or false if it has the wrong
// form.
func Normalize(code string) (string, bool) {
	code = strings.ToLower(strings.TrimSpace(code))
	if len(code) != CodeLen {
		return "", false
	}
	for i := range len(code) {
		if code[i] < 'a' || code[i] > 'z' {
			return "", false
		}
	}
	return code, true
}

// Use marks the code as used by the device and returns its account. Only
// one use of a code can succeed.
func (s *Store) Use(ctx context.Context, code, deviceID string) (string, error) {
	code, ok := Normalize(code)
	if !ok {
		return "", ErrInvalidCode
	}
	b, ver, err := s.st.Get(ctx, key(code))
	if errors.Is(err, state.ErrNotFound) {
		return "", ErrInvalidCode
	}
	if err != nil {
		return "", err
	}
	var rec record
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", fmt.Errorf("pairing record: %w", err)
	}
	if !s.now().Before(rec.ExpiresAt) {
		_ = s.st.Delete(ctx, key(code))
		return "", ErrInvalidCode
	}
	if rec.UsedBy != "" {
		return "", ErrInvalidCode
	}
	rec.UsedBy = deviceID
	nb, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	if _, err := s.st.PutIfMatch(ctx, key(code), nb, ver); errors.Is(err, state.ErrConflict) {
		return "", ErrInvalidCode
	} else if err != nil {
		return "", err
	}
	return rec.Account, nil
}
