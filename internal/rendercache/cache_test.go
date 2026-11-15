package rendercache_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/rendercache"
)

func size(t *testing.T, dir string) (n int64, names []string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		info, _ := e.Info()
		n += info.Size()
		names = append(names, e.Name())
	}
	return n, names
}

func TestEviction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "render")
	c, err := rendercache.New(dir, 250)
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("x"), 100)
	old := time.Now().Add(-time.Hour)
	for i, k := range []string{"a", "b"} {
		if err := c.Put(k, data); err != nil {
			t.Fatal(err)
		}
		at := old.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(filepath.Join(dir, k), at, at); err != nil {
			t.Fatal(err)
		}
	}
	// A hit makes "a" the newest file, so "b" is now the oldest.
	if got, ok := c.Get("a"); !ok || !bytes.Equal(got, data) {
		t.Fatalf("get a: %q %v", got, ok)
	}
	if err := c.Put("c", data); err != nil {
		t.Fatal(err)
	}
	n, names := size(t, dir)
	if n > 250 {
		t.Fatalf("cache size %d is above the limit", n)
	}
	if _, ok := c.Get("b"); ok {
		t.Fatalf("the oldest file is still in the cache: %v", names)
	}
	for _, k := range []string{"a", "c"} {
		if _, ok := c.Get(k); !ok {
			t.Fatalf("%s is not in the cache: %v", k, names)
		}
	}

	// A file larger than the limit does not stay.
	if err := c.Put("d", bytes.Repeat([]byte("y"), 300)); err != nil {
		t.Fatal(err)
	}
	if n, names := size(t, dir); n > 250 {
		t.Fatalf("cache size %d: %v", n, names)
	}
}

func TestKey(t *testing.T) {
	if rendercache.Key("ab", "c") == rendercache.Key("a", "bc") {
		t.Fatal("two different part lists give the same key")
	}
}

// TestNewRemovesStaleFiles makes sure that New removes an old file that
// is not complete, and keeps the results.
func TestNewRemovesStaleFiles(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	for _, name := range []string{".tmp-123", "result"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := rendercache.New(dir, 1000); err != nil {
		t.Fatal(err)
	}
	if _, names := size(t, dir); len(names) != 1 || names[0] != "result" {
		t.Fatalf("files after New: %v", names)
	}
}
