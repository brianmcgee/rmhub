package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brianmcgee/rmhub/internal/config"
)

func TestDefaults(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	checks := map[string][2]string{
		"data_dir":              {cfg.DataDir, filepath.Join(data, "rmhub")},
		"blob.backend":          {cfg.Blob.Backend, "fs"},
		"blob.dir":              {cfg.Blob.Dir, filepath.Join(data, "rmhub", "blobs")},
		"state.backend":         {cfg.State.Backend, "sqlite"},
		"state.path":            {cfg.State.Path, filepath.Join(data, "rmhub", "state.sqlite")},
		"listen.tablet.address": {cfg.Listen.Tablet.Address, "127.0.0.1:8080"},
		"listen.api.address":    {cfg.Listen.API.Address, "127.0.0.1:8081"},
	}
	for k, v := range checks {
		if v[0] != v[1] {
			t.Errorf("%s = %q, want %q", k, v[0], v[1])
		}
	}
}

func TestDefaultFile(t *testing.T) {
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)
	if err := os.MkdirAll(filepath.Join(cfgHome, "rmhub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgHome, "rmhub", "config.toml"), []byte(`data_dir = "/srv/rmhub"`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != "/srv/rmhub" || cfg.Blob.Dir != "/srv/rmhub/blobs" {
		t.Fatalf("data_dir %q, blob.dir %q", cfg.DataDir, cfg.Blob.Dir)
	}
}

func TestExplicitPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	text := `
data_dir = "/var/lib/rmhub"

[blob]
backend = "s3"
s3.endpoint = "https://acct.r2.cloudflarestorage.com"
s3.region = "auto"
s3.bucket = "rmhub"
s3.prefix = "blobs/"
s3.path_style = true
s3.access_key_id = "id"
s3.secret_access_key_file = "/run/credentials/rmhub.service/s3"

[state]
backend = "sqlite"

[listen.tablet]
address = "0.0.0.0:8443"

[listen.api]
address = "127.0.0.1:9000"
tls_cert_file = "/c.crt"
tls_key_file = "/c.key"
`
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Blob.Backend != "s3" || cfg.Blob.S3.Bucket != "rmhub" || !cfg.Blob.S3.PathStyle ||
		cfg.Blob.S3.SecretAccessKeyFile != "/run/credentials/rmhub.service/s3" {
		t.Errorf("blob = %+v", cfg.Blob)
	}
	if cfg.State.Path != "/var/lib/rmhub/state.sqlite" {
		t.Errorf("state.path = %q", cfg.State.Path)
	}
	if cfg.Listen.Tablet.Address != "0.0.0.0:8443" || cfg.Listen.API.TLSCertFile != "/c.crt" {
		t.Errorf("listen = %+v", cfg.Listen)
	}
}

func TestExplicitPathMissing(t *testing.T) {
	_, err := config.Load("/missing.toml")
	if err == nil || !strings.Contains(err.Error(), "/missing.toml") {
		t.Fatalf("err = %v, want an error that names /missing.toml", err)
	}
}

func TestErrors(t *testing.T) {
	for _, tc := range []struct {
		name, text, want string
	}{
		{"unknown key", "[blob]\ndirr = \"/x\"\n", "blob.dirr"},
		{"wrong type", "[blob]\nbackend = 3\n", "blob.backend"},
		{"unknown blob backend", "[blob]\nbackend = \"ftp\"\n", "blob.backend"},
		{"unknown state backend", "[state]\nbackend = \"postgres\"\n", "state.backend"},
		{"secret in config", "[blob]\nbackend = \"s3\"\ns3.bucket = \"b\"\ns3.secret_access_key = \"x\"\n", "secret_access_key"},
		{"secret in state config", "[state.s3]\nsecret_access_key = \"x\"\n", "state.s3.secret_access_key"},
		{"s3 without bucket", "[state]\nbackend = \"s3\"\n", "state.s3.bucket"},
		{"half tls", "[listen.api]\ntls_cert_file = \"/c\"\n", "listen.api"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Parse(tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want an error that names %s", err, tc.want)
			}
		})
	}
}
