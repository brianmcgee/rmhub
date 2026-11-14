// Package blob defines the store for immutable, content-addressed blobs.
//
// A blob key is the lowercase hex SHA-256 of the blob, or the hash of an
// index. Each blob lives in the namespace of one account.
package blob

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/brianmcgee/rmhub/internal/ids"
)

var (
	// ErrNotFound means that the key has no blob in the namespace.
	ErrNotFound = errors.New("blob: not found")
	// ErrInvalidKey means that the key or the namespace has a wrong form.
	ErrInvalidKey = errors.New("blob: invalid key")
)

// Namespace is the blob namespace of one account. The zero value is not
// valid. Use NewNamespace to make one.
type Namespace struct {
	id string
}

// NewNamespace returns the namespace of the account. It refuses an
// account ID with a wrong form.
func NewNamespace(account string) (Namespace, error) {
	if !ids.IsAccountID(account) {
		return Namespace{}, fmt.Errorf("%w: namespace %q", ErrInvalidKey, account)
	}
	return Namespace{id: account}, nil
}

// String returns the account ID of the namespace.
func (n Namespace) String() string { return n.id }

// Check returns ErrInvalidKey for a namespace that NewNamespace did not
// make, and for a key with a wrong form. Each backend calls it before
// each storage access.
func Check(ns Namespace, key string) error {
	if !ids.IsAccountID(ns.id) {
		return fmt.Errorf("%w: namespace %q", ErrInvalidKey, ns.id)
	}
	return CheckKey(key)
}

// CheckKey returns ErrInvalidKey for a key that is not 64 lowercase
// hexadecimal characters.
func CheckKey(key string) error {
	if !ids.IsHash(key) {
		return fmt.Errorf("%w: key %q", ErrInvalidKey, key)
	}
	return nil
}

// Store keeps immutable blobs.
//
// Each error other than ErrNotFound and ErrInvalidKey is a storage failure.
type Store interface {
	// Put writes the blob atomically. If the key exists, Put keeps the
	// first blob and returns nil.
	Put(ctx context.Context, ns Namespace, key string, r io.Reader) error
	// Get returns the bytes and the size of the blob.
	Get(ctx context.Context, ns Namespace, key string) (io.ReadCloser, int64, error)
	// Exists tells for each key if it has a blob. It does not read the
	// blob bytes.
	Exists(ctx context.Context, ns Namespace, keys []string) (map[string]bool, error)
}
