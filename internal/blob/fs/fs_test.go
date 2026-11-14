package fs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/brianmcgee/rmhub/internal/blob"
	"github.com/brianmcgee/rmhub/internal/blob/blobtest"
	"github.com/brianmcgee/rmhub/internal/blob/fs"
)

func TestContract(t *testing.T) {
	blobtest.Run(t, func(t *testing.T) blob.Store {
		s, err := fs.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func TestStorageFailureIsNotNotFound(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores file modes")
	}
	dir := t.TempDir()
	s, err := fs.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ns := blobtest.NewNamespace(t)
	key := blobtest.HashOf([]byte("x"))
	if err := os.MkdirAll(filepath.Join(dir, ns.String()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(dir, ns.String()), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(dir, ns.String()), 0o700) })
	_, _, err = s.Get(context.Background(), ns, key)
	if err == nil || errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("err = %v, want a storage failure", err)
	}
}

func TestInvalidKeyWritesNothing(t *testing.T) {
	dir := t.TempDir()
	s, err := fs.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	ns := blobtest.NewNamespace(t)
	_ = s.Put(context.Background(), ns, "../accounts/x", nil)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "tmp" {
		t.Fatalf("directory has %v, want only tmp", entries)
	}
}
