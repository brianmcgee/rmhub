// Package state defines the store for small mutable values.
//
// Each write is conditional on one key. The store gives no multi-key
// transactions.
package state

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrNotFound means that the key does not exist.
	ErrNotFound = errors.New("state: not found")
	// ErrConflict means that the condition of a write was false. The
	// storage did not change.
	ErrConflict = errors.New("state: conflict")
	// ErrInvalidKey means that the key has a wrong form.
	ErrInvalidKey = errors.New("state: invalid key")
)

// MaxKeyLen is the maximum length of a key in bytes.
const MaxKeyLen = 512

// Store keeps small values under string keys.
//
// Each error other than ErrNotFound, ErrConflict and ErrInvalidKey is a
// failure. A failure of a write means that the outcome is not known.
type Store interface {
	// Get returns the value and its version.
	Get(ctx context.Context, key string) (value []byte, version string, err error)
	// PutIfAbsent writes the value only if the key does not exist.
	PutIfAbsent(ctx context.Context, key string, value []byte) (version string, err error)
	// PutIfMatch writes the value only if the current version is version.
	// A missing key gives ErrConflict.
	PutIfMatch(ctx context.Context, key string, value []byte, version string) (string, error)
	// Delete removes the key. A missing key is not an error.
	Delete(ctx context.Context, key string) error
	// List returns up to limit keys that start with prefix and sort after
	// the key after, in ascending byte order. A limit of 0 or less means
	// no limit.
	List(ctx context.Context, prefix, after string, limit int) ([]string, error)
}

// CheckKey returns ErrInvalidKey for a key with a wrong form.
func CheckKey(key string) error {
	if err := checkKey(key); err != "" {
		return fmt.Errorf("%w: %q: %s", ErrInvalidKey, key, err)
	}
	return nil
}

func checkKey(key string) string {
	if len(key) == 0 || len(key) > MaxKeyLen {
		return "length must be 1 to 512 bytes"
	}
	for i := range len(key) {
		c := key[i]
		ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-' || c == '/'
		if !ok {
			return "character not allowed"
		}
	}
	if key[0] == '/' || key[len(key)-1] == '/' {
		return "starts or ends with /"
	}
	for seg := range strings.SplitSeq(key, "/") {
		switch seg {
		case "":
			return "contains //"
		case ".", "..":
			return "contains a . or .. segment"
		}
	}
	return ""
}

// CheckPrefix checks a list prefix. An empty prefix is valid. Otherwise
// the prefix must be a valid key, optionally with a trailing /.
func CheckPrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	return CheckKey(strings.TrimSuffix(prefix, "/"))
}
