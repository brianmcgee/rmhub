package state_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/state"
)

func TestCheckKey(t *testing.T) {
	for _, k := range []string{
		"accounts/u1/roots/00000000000000000001",
		"a",
		"server/jwt-key",
		"x/y.z_w-1",
		strings.Repeat("a", 512),
	} {
		if err := state.CheckKey(k); err != nil {
			t.Errorf("CheckKey(%q) = %v, want nil", k, err)
		}
	}
	for _, k := range []string{
		"",
		strings.Repeat("a", 513),
		"Accounts/u1",
		"a b",
		"a:b",
		"/a",
		"a/",
		"a//b",
		"accounts/../server/keys",
		"./a",
		"a/.",
		"..",
		"a\x00b",
		"ä",
	} {
		if err := state.CheckKey(k); !errors.Is(err, state.ErrInvalidKey) {
			t.Errorf("CheckKey(%q) = %v, want ErrInvalidKey", k, err)
		}
	}
}

func TestCheckPrefix(t *testing.T) {
	for _, p := range []string{"", "a/", "accounts/x/roots/"} {
		if err := state.CheckPrefix(p); err != nil {
			t.Errorf("CheckPrefix(%q) = %v", p, err)
		}
	}
	for _, p := range []string{"/", "../", "A/"} {
		if err := state.CheckPrefix(p); err == nil {
			t.Errorf("CheckPrefix(%q) succeeded", p)
		}
	}
}
