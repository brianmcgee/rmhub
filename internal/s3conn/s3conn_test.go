package s3conn_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/s3conn"
)

func TestReadSecretFromFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s3-secret")
	if err := os.WriteFile(p, []byte("s3cr3t\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s3conn.ReadSecret(p)
	if err != nil || got != "s3cr3t" {
		t.Fatalf("ReadSecret = %q, %v", got, err)
	}
	c, err := s3conn.New(context.Background(), s3conn.Settings{
		Endpoint: "http://127.0.0.1:1", Bucket: "b", AccessKeyID: "id", SecretAccessKeyFile: p,
	})
	if err != nil {
		t.Fatal(err)
	}
	creds, err := c.Client.Options().Credentials.Retrieve(context.Background())
	if err != nil || creds.SecretAccessKey != "s3cr3t" || creds.AccessKeyID != "id" {
		t.Fatalf("credentials = %+v, %v", creds, err)
	}
}

func TestMissingSecretFile(t *testing.T) {
	_, err := s3conn.New(context.Background(), s3conn.Settings{
		Bucket: "b", AccessKeyID: "id", SecretAccessKeyFile: "/missing/secret",
	})
	if err == nil || !strings.Contains(err.Error(), "secret_access_key_file") {
		t.Fatalf("err = %v", err)
	}
}
