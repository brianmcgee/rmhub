package config_test

import (
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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
		{"host with scheme", "[tablet]\nannounced_host = \"https://rmhub.example.org\"\n", "tablet.announced_host"},
		{"host with path", "[tablet]\nannounced_host = \"rmhub.example.org/x\"\n", "tablet.announced_host"},
		{"bad check_files", "[tablet]\ncheck_files = \"some\"\n", "tablet.check_files"},
		{"negative upload limit", "[api]\nmax_upload_bytes = -1\n", "api.max_upload_bytes"},
		{"negative blob limit", "[tablet]\nmax_blob_bytes = -1\n", "tablet.max_blob_bytes"},
		{"negative cache limit", "[render]\ncache_max_bytes = -1\n", "render.cache_max_bytes"},
		{"negative render workers", "[render]\nmax_workers = -1\n", "render.max_workers"},
		{"pairing limit 0", "[limits]\npairing_per_minute = 0\n", "limits.pairing_per_minute"},
		{"API limit -2", "[limits]\napi_per_minute = -2\n", "limits.api_per_minute"},
		{"render limit 0", "[limits]\nrender_per_minute = 0\n", "limits.render_per_minute"},
		{"proxy host name", "[listen.api]\ntrusted_proxies = [\"proxy.local\"]\n", "listen.api.trusted_proxies"},
		{"proxy bad network", "[listen.tablet]\ntrusted_proxies = [\"10.0.0.0/40\"]\n", "listen.tablet.trusted_proxies"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := config.Parse(tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want an error that names %s", err, tc.want)
			}
		})
	}
}

func TestTabletSettings(t *testing.T) {
	cfg, err := config.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Tablet.AnnouncedHost != "local.appspot.com" || cfg.Tablet.CheckFiles != "truthful" {
		t.Fatalf("defaults: %+v", cfg.Tablet)
	}
	for _, host := range []string{"rmhub.example.org", "rmhub.example.org:8443", "10.0.0.2:8080"} {
		cfg, err := config.Parse("[tablet]\nannounced_host = \"" + host + "\"\ncheck_files = \"none\"\n")
		if err != nil || cfg.Tablet.AnnouncedHost != host || cfg.Tablet.CheckFiles != "none" {
			t.Fatalf("%s: %+v %v", host, cfg, err)
		}
	}
}

func TestRenderSettings(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	cfg, err := config.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Render.CacheDir != filepath.Join(cache, "rmhub", "render") || cfg.Render.CacheMaxBytes != 1<<30 {
		t.Fatalf("defaults: %+v", cfg.Render)
	}
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "/home/u")
	if cfg, err = config.Parse(""); err != nil || cfg.Render.CacheDir != "/home/u/.cache/rmhub/render" {
		t.Fatalf("without XDG_CACHE_HOME: %+v %v", cfg.Render, err)
	}
	cfg, err = config.Parse("[render]\ncache_dir = \"/var/cache/r\"\ncache_max_bytes = 1000\n")
	if err != nil || cfg.Render.CacheDir != "/var/cache/r" || cfg.Render.CacheMaxBytes != 1000 {
		t.Fatalf("explicit: %+v %v", cfg.Render, err)
	}
	if want := min(runtime.NumCPU(), 4); cfg.Render.MaxWorkers != want {
		t.Errorf("default max_workers = %d, want %d", cfg.Render.MaxWorkers, want)
	}
	cfg, err = config.Parse("[render]\nmax_workers = 7\n")
	if err != nil || cfg.Render.MaxWorkers != 7 {
		t.Fatalf("max_workers = 7: %+v %v", cfg.Render, err)
	}
}

func TestLimits(t *testing.T) {
	cfg, err := config.Parse("")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Limits, (config.LimitsConfig{PairingPerMinute: 10, APIPerMinute: 600, RenderPerMinute: 60}); got != want {
		t.Errorf("default limits = %+v, want %+v", got, want)
	}
	cfg, err = config.Parse("[limits]\npairing_per_minute = -1\napi_per_minute = 5\n")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Limits, (config.LimitsConfig{PairingPerMinute: -1, APIPerMinute: 5, RenderPerMinute: 60}); got != want {
		t.Errorf("limits = %+v, want %+v", got, want)
	}
}

func TestTrustedProxies(t *testing.T) {
	cfg, err := config.Parse("[listen.tablet]\ntrusted_proxies = [\"127.0.0.1\", \"10.1.0.0/16\", \"::1\"]\n")
	if err != nil {
		t.Fatal(err)
	}
	got, err := cfg.Listen.Tablet.Proxies()
	if err != nil {
		t.Fatal(err)
	}
	want := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.1.0.0/16"), netip.MustParsePrefix("::1/128")}
	if !slices.Equal(got, want) {
		t.Errorf("Proxies = %v, want %v", got, want)
	}
	if p, err := cfg.Listen.API.Proxies(); err != nil || len(p) != 0 {
		t.Errorf("default Proxies = %v, %v; want none", p, err)
	}
}
