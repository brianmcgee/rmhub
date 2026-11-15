package stale_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/stale"
)

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-25 * time.Hour)
	for name, mtime := range map[string]time.Time{
		"put-old":    old,
		"put-new":    time.Now(),
		"upload-old": old,
		"result-old": old,
	} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "put-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(dir, "put-dir"), old, old); err != nil {
		t.Fatal(err)
	}

	if n := stale.Remove(dir, 24*time.Hour, "put-", "upload-"); n != 2 {
		t.Errorf("%d files removed, want 2", n)
	}
	for name, want := range map[string]bool{"put-old": false, "upload-old": false, "put-new": true, "result-old": true, "put-dir": true} {
		_, err := os.Lstat(filepath.Join(dir, name))
		if (err == nil) != want {
			t.Errorf("%s: exists = %v, want %v", name, err == nil, want)
		}
	}
	if n := stale.Remove(filepath.Join(dir, "none"), time.Hour, "put-"); n != 0 {
		t.Errorf("a directory that does not exist gives %d", n)
	}
}
