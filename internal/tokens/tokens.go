// Package tokens makes and checks the API tokens of rmhub.
//
// An owner token has the form rmh_<account>_<id>_<secret>. The server
// keeps the record accounts/<account>/tokens/<id> with the SHA-256 hash
// of the secret. The admin token comes from a file and has the form
// rmh_admin_<text>.
package tokens

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/brianmcgee/rmhub/internal/ids"
	"github.com/brianmcgee/rmhub/internal/state"
)

const (
	// Prefix starts each rmhub token.
	Prefix = "rmh_"
	// AdminPrefix starts the admin token.
	AdminPrefix = "rmh_admin_"
	// AdminMinLen is the minimum number of characters after AdminPrefix.
	AdminMinLen = 32
	// IDLen is the length of a token ID.
	IDLen = 16
	// SecretLen is the length of a token secret.
	SecretLen = 52
)

var (
	// ErrInvalid means that the token is malformed, unknown, revoked or
	// expired.
	ErrInvalid = errors.New("token is not valid")
	// ErrNotFound means that the token ID does not exist in the account.
	ErrNotFound = errors.New("token not found")
)

// GenerateAdmin returns a new random admin token.
func GenerateAdmin() string { return AdminPrefix + ids.Random(SecretLen) }

// CheckAdmin checks the form of an admin token.
func CheckAdmin(tok string) error {
	rest, ok := strings.CutPrefix(tok, AdminPrefix)
	if !ok {
		return fmt.Errorf("the admin token does not start with %s", AdminPrefix)
	}
	if len(rest) < AdminMinLen {
		return fmt.Errorf("the admin token has %d characters after %s; it must have at least %d", len(rest), AdminPrefix, AdminMinLen)
	}
	for i := range len(rest) {
		if c := rest[i]; c <= ' ' || c >= 0x7f {
			return errors.New("the admin token contains a character that is not printable ASCII")
		}
	}
	return nil
}

// Admin checks the admin token.
type Admin struct{ hash [32]byte }

// ReadAdminFile reads and checks the admin token in the file.
func ReadAdminFile(path string) (*Admin, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	tok := strings.TrimSpace(string(b))
	if err := CheckAdmin(tok); err != nil {
		return nil, err
	}
	return &Admin{hash: sha256.Sum256([]byte(tok))}, nil
}

// IsAdmin tells if tok has the admin prefix.
func IsAdmin(tok string) bool { return strings.HasPrefix(tok, AdminPrefix) }

// Check compares tok with the admin token in constant time.
func (a *Admin) Check(tok string) bool {
	h := sha256.Sum256([]byte(tok))
	return subtle.ConstantTimeCompare(h[:], a.hash[:]) == 1
}

// Parse splits an owner token. It checks the form only.
func Parse(tok string) (account, id, secret string, ok bool) {
	rest, found := strings.CutPrefix(tok, Prefix)
	if !found {
		return "", "", "", false
	}
	parts := strings.Split(rest, "_")
	if len(parts) != 3 || !ids.IsAccountID(parts[0]) || !ids.IsBase32(parts[1], IDLen) || !ids.IsBase32(parts[2], SecretLen) {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// Record is the stored form of an owner token.
type Record struct {
	SecretHash string     `json:"secretHash"`
	Label      string     `json:"label"`
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  *time.Time `json:"expiresAt"`
}

// Token describes a token without its secret.
type Token struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// Store keeps owner tokens in a state store.
type Store struct {
	st  state.Store
	now func() time.Time
}

// New returns a token store.
func New(st state.Store) *Store { return &Store{st: st, now: time.Now} }

// SetClock sets the clock. Tests use it.
func (s *Store) SetClock(now func() time.Time) { s.now = now }

func recordKey(account, id string) string { return "accounts/" + account + "/tokens/" + id }

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// Issue makes an owner token for the account. The caller checks that the
// account exists. It returns the token text, which the server does not
// keep.
func (s *Store) Issue(ctx context.Context, account, label string, expiresAt *time.Time) (string, Token, error) {
	if !ids.IsAccountID(account) {
		return "", Token{}, fmt.Errorf("invalid account %q", account)
	}
	secret := ids.Random(SecretLen)
	rec := Record{
		SecretHash: hashSecret(secret),
		Label:      label,
		CreatedAt:  s.now().UTC().Truncate(time.Second),
		ExpiresAt:  expiresAt,
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return "", Token{}, err
	}
	for range 3 {
		id := ids.Random(IDLen)
		_, err := s.st.PutIfAbsent(ctx, recordKey(account, id), b)
		if errors.Is(err, state.ErrConflict) {
			continue
		}
		if err != nil {
			return "", Token{}, err
		}
		text := Prefix + account + "_" + id + "_" + secret
		return text, Token{ID: id, Label: label, CreatedAt: rec.CreatedAt, ExpiresAt: expiresAt}, nil
	}
	return "", Token{}, errors.New("token ID collision three times")
}

// Check returns the account and the token ID of a valid owner token.
func (s *Store) Check(ctx context.Context, tok string) (account, id string, err error) {
	account, id, secret, ok := Parse(tok)
	if !ok {
		return "", "", ErrInvalid
	}
	b, _, err := s.st.Get(ctx, recordKey(account, id))
	if errors.Is(err, state.ErrNotFound) {
		return "", "", ErrInvalid
	}
	if err != nil {
		return "", "", err
	}
	var rec Record
	if err := json.Unmarshal(b, &rec); err != nil {
		return "", "", fmt.Errorf("token record %s: %w", id, err)
	}
	if subtle.ConstantTimeCompare([]byte(hashSecret(secret)), []byte(rec.SecretHash)) != 1 {
		return "", "", ErrInvalid
	}
	if rec.ExpiresAt != nil && !s.now().Before(*rec.ExpiresAt) {
		return "", "", ErrInvalid
	}
	return account, id, nil
}

// List returns the tokens of the account, ordered by ID.
func (s *Store) List(ctx context.Context, account string) ([]Token, error) {
	prefix := "accounts/" + account + "/tokens/"
	keys, err := s.st.List(ctx, prefix, "", 0)
	if err != nil {
		return nil, err
	}
	out := []Token{}
	for _, k := range keys {
		b, _, err := s.st.Get(ctx, k)
		if errors.Is(err, state.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var rec Record
		if err := json.Unmarshal(b, &rec); err != nil {
			return nil, fmt.Errorf("token record %s: %w", k, err)
		}
		out = append(out, Token{ID: strings.TrimPrefix(k, prefix), Label: rec.Label, CreatedAt: rec.CreatedAt, ExpiresAt: rec.ExpiresAt})
	}
	slices.SortFunc(out, func(a, b Token) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// Revoke deletes a token of the account.
func (s *Store) Revoke(ctx context.Context, account, id string) error {
	if !ids.IsAccountID(account) || !ids.IsBase32(id, IDLen) {
		return ErrNotFound
	}
	key := recordKey(account, id)
	if _, _, err := s.st.Get(ctx, key); errors.Is(err, state.ErrNotFound) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return s.st.Delete(ctx, key)
}
