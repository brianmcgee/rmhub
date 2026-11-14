// Package fs is the file system backend of the blob store.
//
// The layout is <dir>/<namespace>/<key[0:2]>/<key>. A write goes to a
// temporary file in <dir>/tmp, and link(2) gives it the final name. link
// fails when the name exists, so the first write wins without a lock.
package fs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/brianmcgee/rmhub/internal/blob"
)

// Store keeps blobs in a directory.
type Store struct {
	dir string
}

var _ blob.Store = (*Store)(nil)

// New returns a store in dir. It makes the directory if necessary.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dir, "tmp"), 0o700); err != nil {
		return nil, fmt.Errorf("blob store: %w", err)
	}
	return &Store{dir: dir}, nil
}

// TempDir returns the directory for temporary files. A file in it can
// become a blob with PutFile without a copy.
func (s *Store) TempDir() string { return filepath.Join(s.dir, "tmp") }

func (s *Store) path(ns blob.Namespace, key string) string {
	return filepath.Join(s.dir, ns.String(), key[:2], key)
}

// Put writes the blob through a temporary file.
func (s *Store) Put(ctx context.Context, ns blob.Namespace, key string, r io.Reader) error {
	if err := blob.Check(ns, key); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.TempDir(), "put-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }()
	if _, err := io.Copy(f, readerWithContext{ctx, r}); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return s.link(ns, key, tmp)
}

// PutFile makes the file at path the blob of the key. The file must be
// in TempDir and must not change after the call. The caller keeps the
// file and removes it after the call.
func (s *Store) PutFile(ns blob.Namespace, key, path string) error {
	if err := blob.Check(ns, key); err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	err = f.Sync()
	_ = f.Close()
	if err != nil {
		return err
	}
	return s.link(ns, key, path)
}

func (s *Store) link(ns blob.Namespace, key, tmp string) error {
	final := s.path(ns, key)
	dir := filepath.Dir(final)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Link(tmp, final); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil
		}
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

// Get opens the blob.
func (s *Store) Get(_ context.Context, ns blob.Namespace, key string) (io.ReadCloser, int64, error) {
	if err := blob.Check(ns, key); err != nil {
		return nil, 0, err
	}
	f, err := os.Open(s.path(ns, key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, 0, blob.ErrNotFound
		}
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, err
	}
	return f, st.Size(), nil
}

// Exists checks the keys with stat(2).
func (s *Store) Exists(_ context.Context, ns blob.Namespace, keys []string) (map[string]bool, error) {
	for _, k := range keys {
		if err := blob.Check(ns, k); err != nil {
			return nil, err
		}
	}
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		_, err := os.Stat(s.path(ns, k))
		switch {
		case err == nil:
			out[k] = true
		case errors.Is(err, fs.ErrNotExist):
			out[k] = false
		default:
			return nil, err
		}
	}
	return out, nil
}

type readerWithContext struct {
	ctx context.Context
	r   io.Reader
}

func (r readerWithContext) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
