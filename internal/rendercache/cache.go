// Package rendercache keeps page render results in a directory with a
// size limit.
package rendercache

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/brianmcgee/rmhub/internal/stale"
)

// tmpPrefix starts the name of a file that is not complete.
const tmpPrefix = ".tmp-"

// Cache is a directory of render results. The name of a file is its key.
type Cache struct {
	dir string
	max int64
	// mu serializes the eviction.
	mu sync.Mutex
}

// New opens the cache in dir and makes the directory if necessary. max is
// the size limit in bytes.
func New(dir string, max int64) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("make the render cache directory: %w", err)
	}
	// A process that stopped during a write can leave files that are not
	// complete.
	stale.Remove(dir, stale.Age, tmpPrefix)
	return &Cache{dir: dir, max: max}, nil
}

// Key returns the hex SHA-256 of the parts. A zero byte separates the
// parts, so two different lists give two different keys.
func Key(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *Cache) path(key string) string { return filepath.Join(c.dir, key) }

// Get returns the result with the key. A hit sets the modification time
// to now, so that the eviction keeps files in use.
func (c *Cache) Get(key string) ([]byte, bool) {
	b, err := os.ReadFile(c.path(key))
	if err != nil {
		return nil, false
	}
	now := time.Now()
	_ = os.Chtimes(c.path(key), now, now)
	return b, true
}

// Put adds the result with the key. The write is atomic: a reader sees
// the full file or no file. Then Put deletes the oldest files until the
// cache is at or below its limit.
func (c *Cache) Put(key string, data []byte) error {
	f, err := os.CreateTemp(c.dir, tmpPrefix+"*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), c.path(key))
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return fmt.Errorf("write the render cache: %w", err)
	}
	return c.evict()
}

type file struct {
	name  string
	size  int64
	mtime time.Time
}

// evict deletes the files with the oldest modification time until the
// cache is at or below its limit.
func (c *Cache) evict() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ents, err := os.ReadDir(c.dir)
	if err != nil {
		return err
	}
	var files []file
	var total int64
	for _, e := range ents {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), tmpPrefix) {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		files = append(files, file{e.Name(), info.Size(), info.ModTime()})
		total += info.Size()
	}
	if total <= c.max {
		return nil
	}
	slices.SortFunc(files, func(a, b file) int {
		if d := a.mtime.Compare(b.mtime); d != 0 {
			return d
		}
		return strings.Compare(a.name, b.name)
	})
	for _, f := range files {
		if total <= c.max {
			break
		}
		if err := os.Remove(c.path(f.name)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		total -= f.size
	}
	return nil
}
