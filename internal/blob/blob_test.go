package blob_test

import (
	"errors"
	"testing"

	"github.com/brianmcgee/rmhub/internal/blob"
)

func TestCheckKey(t *testing.T) {
	valid := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if err := blob.CheckKey(valid); err != nil {
		t.Fatalf("valid key: %v", err)
	}
	for _, k := range []string{
		"../accounts/x",
		"9F86D081884C7D659A2FEAA0C55AD015A3BF4F1B2B0B822CD15D6C15B0F00A08",
		valid[:63],
		valid + "0",
		"",
	} {
		if err := blob.CheckKey(k); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("CheckKey(%q) = %v, want ErrInvalidKey", k, err)
		}
	}
}

func TestNamespace(t *testing.T) {
	for _, id := range []string{"../tmp", "", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "abcdefghijklmnopqrstuvwxy1"} {
		if _, err := blob.NewNamespace(id); !errors.Is(err, blob.ErrInvalidKey) {
			t.Errorf("NewNamespace(%q) = %v, want ErrInvalidKey", id, err)
		}
	}
	ns, err := blob.NewNamespace("abcdefghijklmnopqrstuvwxyz")
	if err != nil {
		t.Fatal(err)
	}
	key := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	if err := blob.Check(ns, key); err != nil {
		t.Fatal(err)
	}
	if err := blob.Check(blob.Namespace{}, key); !errors.Is(err, blob.ErrInvalidKey) {
		t.Fatalf("zero namespace: %v", err)
	}
}
