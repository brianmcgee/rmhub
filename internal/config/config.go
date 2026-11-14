// Package config loads the TOML configuration of rmhub serve.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/brianmcgee/rmhub/internal/s3conn"
	"github.com/brianmcgee/rmhub/internal/xdg"
)

// Config is the server configuration.
type Config struct {
	DataDir string       `toml:"data_dir"`
	Blob    BlobConfig   `toml:"blob"`
	State   StateConfig  `toml:"state"`
	Listen  ListenConfig `toml:"listen"`
	Admin   AdminConfig  `toml:"admin"`
	Tablet  TabletConfig `toml:"tablet"`
}

// TabletConfig has the settings of the tablet protocol.
type TabletConfig struct {
	// AnnouncedHost is the host that the discovery endpoints return: a
	// host name with an optional port.
	AnnouncedHost string `toml:"announced_host"`
	// CheckFiles is "truthful" or "none". With "none", check-files
	// always tells that no file is missing.
	CheckFiles string `toml:"check_files"`
}

// AdminConfig names the file of the admin token. Without a file, each
// admin request gets status 401.
type AdminConfig struct {
	TokenFile string `toml:"token_file"`
}

// BlobConfig selects the blob backend.
type BlobConfig struct {
	Backend string          `toml:"backend"`
	Dir     string          `toml:"dir"`
	S3      s3conn.Settings `toml:"s3"`
}

// StateConfig selects the state backend.
type StateConfig struct {
	Backend string          `toml:"backend"`
	Path    string          `toml:"path"`
	S3      s3conn.Settings `toml:"s3"`
}

// ListenConfig has the two listeners.
type ListenConfig struct {
	Tablet Listener `toml:"tablet"`
	API    Listener `toml:"api"`
}

// Listener is one HTTP listener. Without a certificate and a key it
// serves plain HTTP.
type Listener struct {
	Address     string `toml:"address"`
	TLSCertFile string `toml:"tls_cert_file"`
	TLSKeyFile  string `toml:"tls_key_file"`
}

// DefaultPath returns $XDG_CONFIG_HOME/rmhub/config.toml.
func DefaultPath() string {
	return filepath.Join(xdg.ConfigHome(), "rmhub", "config.toml")
}

// Load reads the configuration. With an empty path it reads the default
// file, and uses the default values when that file does not exist.
func Load(path string) (*Config, error) {
	explicit := path != ""
	if !explicit {
		path = DefaultPath()
	}
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
	case !explicit && errors.Is(err, fs.ErrNotExist):
		data = nil
	default:
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	cfg, err := Parse(string(data))
	if err != nil {
		if len(data) > 0 {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
		return nil, err
	}
	return cfg, nil
}

// Parse decodes the TOML text, sets the default values and checks the
// result.
func Parse(text string) (*Config, error) {
	var cfg Config
	md, err := toml.Decode(text, &cfg)
	if err != nil {
		return nil, err
	}
	if err := checkUndecoded(md.Undecoded()); err != nil {
		return nil, err
	}
	cfg.setDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func checkUndecoded(keys []toml.Key) error {
	if len(keys) == 0 {
		return nil
	}
	for _, k := range keys {
		if k[len(k)-1] == "secret_access_key" {
			return fmt.Errorf("%s: the configuration must not contain a secret; use secret_access_key_file", k)
		}
	}
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.String()
	}
	return fmt.Errorf("unknown configuration key %s", strings.Join(names, ", "))
}

func (c *Config) setDefaults() {
	if c.DataDir == "" {
		c.DataDir = filepath.Join(xdg.DataHome(), "rmhub")
	}
	if c.Blob.Backend == "" {
		c.Blob.Backend = "fs"
	}
	if c.Blob.Dir == "" {
		c.Blob.Dir = filepath.Join(c.DataDir, "blobs")
	}
	if c.State.Backend == "" {
		c.State.Backend = "sqlite"
	}
	if c.State.Path == "" {
		c.State.Path = filepath.Join(c.DataDir, "state.sqlite")
	}
	if c.Listen.Tablet.Address == "" {
		c.Listen.Tablet.Address = "127.0.0.1:8080"
	}
	if c.Listen.API.Address == "" {
		c.Listen.API.Address = "127.0.0.1:8081"
	}
	if c.Tablet.AnnouncedHost == "" {
		c.Tablet.AnnouncedHost = "local.appspot.com"
	}
	if c.Tablet.CheckFiles == "" {
		c.Tablet.CheckFiles = "truthful"
	}
}

func (c *Config) validate() error {
	switch c.Blob.Backend {
	case "fs":
	case "s3":
		if err := checkS3("blob.s3", c.Blob.S3); err != nil {
			return err
		}
	default:
		return fmt.Errorf("blob.backend: unknown backend %q (use fs or s3)", c.Blob.Backend)
	}
	switch c.State.Backend {
	case "sqlite":
	case "s3":
		if err := checkS3("state.s3", c.State.S3); err != nil {
			return err
		}
	default:
		return fmt.Errorf("state.backend: unknown backend %q (use sqlite or s3)", c.State.Backend)
	}
	if c.Blob.Backend == "s3" && c.State.Backend == "s3" &&
		c.Blob.S3.Endpoint == c.State.S3.Endpoint && c.Blob.S3.Bucket == c.State.S3.Bucket &&
		c.Blob.S3.Prefix == c.State.S3.Prefix {
		return errors.New("blob.s3.prefix and state.s3.prefix must differ when both use one bucket")
	}
	for name, l := range map[string]Listener{"listen.tablet": c.Listen.Tablet, "listen.api": c.Listen.API} {
		if (l.TLSCertFile == "") != (l.TLSKeyFile == "") {
			return fmt.Errorf("%s: set both tls_cert_file and tls_key_file, or neither", name)
		}
	}
	if c.Listen.Tablet.Address == c.Listen.API.Address {
		return errors.New("listen.tablet.address and listen.api.address must differ")
	}
	if err := checkHost(c.Tablet.AnnouncedHost); err != nil {
		return fmt.Errorf("tablet.announced_host: %w", err)
	}
	if c.Tablet.CheckFiles != "truthful" && c.Tablet.CheckFiles != "none" {
		return fmt.Errorf("tablet.check_files: unknown value %q (use truthful or none)", c.Tablet.CheckFiles)
	}
	return nil
}

// checkHost accepts a host name or address with an optional port.
func checkHost(h string) error {
	if strings.ContainsAny(h, "/?#@ ") || strings.Contains(h, "://") {
		return fmt.Errorf("value %q must be a host name with an optional port, without a scheme or a path", h)
	}
	u, err := url.Parse("http://" + h)
	if err != nil || u.Host != h || u.Hostname() == "" {
		return fmt.Errorf("value %q must be a host name with an optional port, without a scheme or a path", h)
	}
	return nil
}

func checkS3(name string, s s3conn.Settings) error {
	if s.Bucket == "" {
		return fmt.Errorf("%s.bucket: not set", name)
	}
	if s.AccessKeyID != "" && s.SecretAccessKeyFile == "" {
		return fmt.Errorf("%s.secret_access_key_file: not set, but %s.access_key_id is set", name, name)
	}
	return nil
}
