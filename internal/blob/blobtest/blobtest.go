// Package blobtest is the contract test suite for blob.Store backends.
package blobtest

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/ids"
)

// Factory returns a new, empty store for one test.
type Factory func(t *testing.T) blob.Store

// HashOf returns the blob key of the bytes.
func HashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// NewNamespace returns a random valid namespace.
func NewNamespace(t *testing.T) blob.Namespace {
	t.Helper()
	ns, err := blob.NewNamespace(ids.NewAccountID())
	if err != nil {
		t.Fatal(err)
	}
	return ns
}

func readAll(t *testing.T, s blob.Store, ns blob.Namespace, key string) ([]byte, int64, error) {
	t.Helper()
	rc, n, err := s.Get(context.Background(), ns, key)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return b, n, nil
}

type failingReader struct {
	data []byte
	err  error
}

func (f *failingReader) Read(p []byte) (int, error) {
	if len(f.data) == 0 {
		return 0, f.err
	}
	n := copy(p, f.data)
	f.data = f.data[n:]
	return n, nil
}

// Run runs each contract test against stores from newStore.
func Run(t *testing.T, newStore Factory) {
	ctx := context.Background()

	t.Run("ValidKey", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		const key = "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
		if err := s.Put(ctx, ns, key, bytes.NewReader([]byte("test"))); err != nil {
			t.Fatalf("put: %v", err)
		}
		b, _, err := readAll(t, s, ns, key)
		if err != nil || string(b) != "test" {
			t.Fatalf("get = %q, %v", b, err)
		}
	})

	t.Run("InvalidKeys", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		upper := "9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08"
		for _, key := range []string{"../accounts/x", upper, "", "abc"} {
			if err := s.Put(ctx, ns, key, bytes.NewReader([]byte("x"))); !errors.Is(err, blob.ErrInvalidKey) {
				t.Errorf("put %q: err = %v, want ErrInvalidKey", key, err)
			}
			if _, _, err := s.Get(ctx, ns, key); !errors.Is(err, blob.ErrInvalidKey) {
				t.Errorf("get %q: err = %v, want ErrInvalidKey", key, err)
			}
			if _, err := s.Exists(ctx, ns, []string{key}); !errors.Is(err, blob.ErrInvalidKey) {
				t.Errorf("exists %q: err = %v, want ErrInvalidKey", key, err)
			}
		}
	})

	t.Run("InvalidNamespace", func(t *testing.T) {
		s := newStore(t)
		key := HashOf([]byte("x"))
		var zero blob.Namespace
		if err := s.Put(ctx, zero, key, bytes.NewReader([]byte("x"))); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("put in zero namespace: err = %v", err)
		}
		if _, _, err := s.Get(ctx, zero, key); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("get in zero namespace: err = %v", err)
		}
		if _, err := blob.NewNamespace("../tmp"); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("NewNamespace(../tmp): err = %v", err)
		}
	})

	t.Run("NamespacesAreSeparate", func(t *testing.T) {
		s := newStore(t)
		a, b := NewNamespace(t), NewNamespace(t)
		data := []byte("shared bytes")
		key := HashOf(data)
		if err := s.Put(ctx, a, key, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Get(ctx, b, key); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("get in other namespace: err = %v, want ErrNotFound", err)
		}
		ex, err := s.Exists(ctx, b, []string{key})
		if err != nil || ex[key] {
			t.Fatalf("exists in other namespace = %v, %v", ex, err)
		}
		if err := s.Put(ctx, b, key, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		got, _, err := readAll(t, s, b, key)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("get in second namespace = %q, %v", got, err)
		}
	})

	t.Run("FailedSource", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		data := bytes.Repeat([]byte("a"), 100000)
		key := HashOf(data)
		r := &failingReader{data: data[:5000], err: errors.New("source broke")}
		if err := s.Put(ctx, ns, key, r); err == nil {
			t.Fatal("put with failing source succeeded")
		}
		if _, _, err := s.Get(ctx, ns, key); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("get after failed put: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("FirstWriteWins", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		key := HashOf([]byte("A"))
		if err := s.Put(ctx, ns, key, bytes.NewReader([]byte("A"))); err != nil {
			t.Fatal(err)
		}
		if err := s.Put(ctx, ns, key, bytes.NewReader([]byte("B"))); err != nil {
			t.Fatalf("second put: %v", err)
		}
		got, _, err := readAll(t, s, ns, key)
		if err != nil || string(got) != "A" {
			t.Fatalf("get = %q, %v, want A", got, err)
		}
	})

	t.Run("ParallelWrites", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		key := HashOf([]byte("parallel"))
		const n = 32
		bodies := make([][]byte, n)
		for i := range bodies {
			bodies[i] = bytes.Repeat([]byte{byte('a' + i%26), byte(i)}, 4096)
		}
		var wg sync.WaitGroup
		errs := make([]error, n)
		for i := range n {
			wg.Go(func() {
				errs[i] = s.Put(ctx, ns, key, bytes.NewReader(bodies[i]))
			})
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Errorf("writer %d: %v", i, err)
			}
		}
		got, _, err := readAll(t, s, ns, key)
		if err != nil {
			t.Fatal(err)
		}
		matches := 0
		for _, b := range bodies {
			if bytes.Equal(got, b) {
				matches++
			}
		}
		if matches != 1 {
			t.Fatalf("blob matches %d writers, want exactly 1", matches)
		}
	})

	t.Run("ReadDuringWrite", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		data := make([]byte, 1<<20)
		_, _ = rand.Read(data)
		key := HashOf(data)
		done := make(chan error, 1)
		go func() { done <- s.Put(ctx, ns, key, bytes.NewReader(data)) }()
		for {
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
				return
			default:
			}
			got, _, err := readAll(t, s, ns, key)
			switch {
			case errors.Is(err, blob.ErrNotFound):
			case err != nil:
				t.Fatal(err)
			case !bytes.Equal(got, data):
				t.Fatalf("read %d bytes of a partial blob", len(got))
			}
		}
	})

	t.Run("ReadExisting", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		data := make([]byte, 1048576)
		_, _ = rand.Read(data)
		key := HashOf(data)
		if err := s.Put(ctx, ns, key, bytes.NewReader(data)); err != nil {
			t.Fatal(err)
		}
		got, n, err := readAll(t, s, ns, key)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1048576 || !bytes.Equal(got, data) {
			t.Fatalf("get size %d, %d bytes, equal=%v", n, len(got), bytes.Equal(got, data))
		}
	})

	t.Run("ReadMissing", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		if _, _, err := s.Get(ctx, ns, HashOf([]byte("missing"))); !errors.Is(err, blob.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Exists", func(t *testing.T) {
		s := newStore(t)
		ns := NewNamespace(t)
		k1, k2 := HashOf([]byte("one")), HashOf([]byte("two"))
		if err := s.Put(ctx, ns, k1, bytes.NewReader([]byte("one"))); err != nil {
			t.Fatal(err)
		}
		ex, err := s.Exists(ctx, ns, []string{k1, k2})
		if err != nil {
			t.Fatal(err)
		}
		if !ex[k1] || ex[k2] {
			t.Fatalf("exists = %v, want %s only", ex, k1)
		}
		if ex, err := s.Exists(ctx, ns, nil); err != nil || len(ex) != 0 {
			t.Fatalf("exists of no keys = %v, %v", ex, err)
		}
	})
}
