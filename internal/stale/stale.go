// Package stale removes temporary files that a stopped process left.
package stale

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Age is the age after which a temporary file is stale. No request
// writes one file for this time, so a file of a process that runs is
// safe.
const Age = 24 * time.Hour

// Remove deletes each regular file in dir whose name starts with one of
// the prefixes and whose modification time is older than age. It returns
// the number of deleted files. It ignores errors: a file that stays is
// not a failure of the server.
func Remove(dir string, age time.Duration, prefixes ...string) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	limit := time.Now().Add(-age)
	n := 0
	for _, e := range ents {
		if !e.Type().IsRegular() || !hasPrefix(e.Name(), prefixes) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(limit) {
			continue
		}
		if os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n
}

func hasPrefix(name string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
