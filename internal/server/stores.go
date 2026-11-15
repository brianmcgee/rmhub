package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/brianmcgee/rmhub/internal/blob"
	blobfs "github.com/brianmcgee/rmhub/internal/blob/fs"
	blobs3 "github.com/brianmcgee/rmhub/internal/blob/s3"
	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/s3conn"
	"github.com/brianmcgee/rmhub/internal/stale"
	"github.com/brianmcgee/rmhub/internal/state"
	states3 "github.com/brianmcgee/rmhub/internal/state/s3"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

// Stores are the open storage backends.
type Stores struct {
	Blob    blob.Store
	State   state.Store
	TempDir string
	close   func() error
}

// Close closes the backends.
func (s *Stores) Close() error {
	if s.close != nil {
		return s.close()
	}
	return nil
}

// OpenStores opens the backends of the configuration.
func OpenStores(ctx context.Context, cfg *config.Config) (*Stores, error) {
	tmp := filepath.Join(cfg.DataDir, "tmp")
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return nil, fmt.Errorf("data_dir: %w", err)
	}
	st := &Stores{TempDir: tmp}

	switch cfg.Blob.Backend {
	case "fs":
		b, err := blobfs.New(cfg.Blob.Dir)
		if err != nil {
			return nil, fmt.Errorf("blob.dir: %w", err)
		}
		st.Blob = b
		st.TempDir = b.TempDir()
	case "s3":
		c, err := s3conn.New(ctx, cfg.Blob.S3)
		if err != nil {
			return nil, fmt.Errorf("blob.s3: %w", err)
		}
		st.Blob = blobs3.New(c, tmp)
	}
	// A process that stopped during a request can leave temporary files.
	for _, dir := range []string{tmp, st.TempDir} {
		stale.Remove(dir, stale.Age, "get-", "put-", "upload-", "rmhub-blob-")
	}

	switch cfg.State.Backend {
	case "sqlite":
		if err := os.MkdirAll(filepath.Dir(cfg.State.Path), 0o700); err != nil {
			return nil, fmt.Errorf("state.path: %w", err)
		}
		s, err := sqlite.Open(cfg.State.Path)
		if err != nil {
			return nil, fmt.Errorf("state.path: %w", err)
		}
		st.State = s
		st.close = s.Close
	case "s3":
		c, err := s3conn.New(ctx, cfg.State.S3)
		if err != nil {
			return nil, fmt.Errorf("state.s3: %w", err)
		}
		st.State = states3.New(c)
	}
	return st, nil
}
