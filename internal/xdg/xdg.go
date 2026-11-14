// Package xdg gives the base directories of the XDG Base Directory
// Specification.
package xdg

import (
	"os"
	"path/filepath"
)

func dir(env, fallback string) string {
	if v := os.Getenv(env); v != "" && filepath.IsAbs(v) {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/"
	}
	return filepath.Join(home, fallback)
}

// ConfigHome returns $XDG_CONFIG_HOME, or ~/.config.
func ConfigHome() string { return dir("XDG_CONFIG_HOME", ".config") }

// DataHome returns $XDG_DATA_HOME, or ~/.local/share.
func DataHome() string { return dir("XDG_DATA_HOME", filepath.Join(".local", "share")) }

// CacheHome returns $XDG_CACHE_HOME, or ~/.cache.
func CacheHome() string { return dir("XDG_CACHE_HOME", ".cache") }
