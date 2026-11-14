package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/brianmcgee/rmhub/internal/app"
	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/server"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

type result struct {
	code           int
	stdout, stderr string
}

// rmhub runs the command in the process with the environment env.
func rmhub(env map[string]string, args ...string) result {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, func(k string) string { return env[k] })
	return result{code: code, stdout: out.String(), stderr: errb.String()}
}

// testServer starts the API handler of a new server and returns its URL
// and admin token.
func testServer(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Parse("data_dir = \"" + dir + "\"")
	if err != nil {
		t.Fatal(err)
	}
	admin := tokens.GenerateAdmin()
	cfg.Admin.TokenFile = filepath.Join(dir, "admin")
	if err := os.WriteFile(cfg.Admin.TokenFile, []byte(admin), 0o600); err != nil {
		t.Fatal(err)
	}
	st, err := sqlite.Open(cfg.State.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	a, err := app.New(context.Background(), cfg, &server.Stores{State: st}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.APIHandler())
	t.Cleanup(srv.Close)
	return srv.URL, admin
}

func isolate(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return dir
}

func writeClientFile(t *testing.T, dir, text string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "rmhub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rmhub", "client.toml"), []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsOrder(t *testing.T) {
	dir := isolate(t)
	writeClientFile(t, dir, "url = \"http://file\"\ntoken = \"file-token\"\n", 0o600)
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	var stderr bytes.Buffer

	s, err := resolveSettings("", "", getenv, &stderr)
	if err != nil || s.URL != "http://file" || s.Token != "file-token" {
		t.Fatalf("file: %+v %v", s, err)
	}
	env["RMHUB_TOKEN"] = "env-token"
	env["RMHUB_URL"] = "http://env/"
	s, _ = resolveSettings("", "", getenv, &stderr)
	if s.URL != "http://env" || s.Token != "env-token" {
		t.Fatalf("environment: %+v", s)
	}
	s, _ = resolveSettings("http://flag", "flag-token", getenv, &stderr)
	if s.URL != "http://flag" || s.Token != "flag-token" {
		t.Fatalf("options: %+v", s)
	}
	// Each value has its own source.
	delete(env, "RMHUB_URL")
	s, _ = resolveSettings("", "", getenv, &stderr)
	if s.URL != "http://file" || s.Token != "env-token" {
		t.Fatalf("mixed: %+v", s)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected warning: %s", stderr.String())
	}
}

func TestClientFileWarningAndErrors(t *testing.T) {
	dir := isolate(t)
	writeClientFile(t, dir, "url = \"http://file\"\ntoken = \"t\"\n", 0o644)
	var stderr bytes.Buffer
	if _, err := resolveSettings("", "", func(string) string { return "" }, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "other users can read") {
		t.Fatalf("no warning: %q", stderr.String())
	}
	writeClientFile(t, dir, "urll = \"x\"\n", 0o600)
	_, err := resolveSettings("", "", func(string) string { return "" }, &stderr)
	if exitCode(err) != exitUsage || !strings.Contains(err.Error(), "urll") {
		t.Fatalf("unknown key: %v", err)
	}
}

func TestNoURL(t *testing.T) {
	isolate(t)
	r := rmhub(map[string]string{"RMHUB_TOKEN": "x"}, "devices", "list")
	if r.code != exitUsage || !strings.Contains(r.stderr, "RMHUB_URL") {
		t.Fatalf("%+v", r)
	}
}

func TestEndToEnd(t *testing.T) {
	isolate(t)
	url, admin := testServer(t)
	adminEnv := map[string]string{"RMHUB_URL": url, "RMHUB_TOKEN": admin}

	r := rmhub(adminEnv, "admin", "user", "add", "--email", "ada@example.org", "--name", "Ada", "--json")
	var u userView
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &u) != nil || len(u.ID) != 26 {
		t.Fatalf("user add: %+v", r)
	}
	// Exit code 5: email in use.
	if r := rmhub(adminEnv, "admin", "user", "add", "--email", "ADA@example.org"); r.code != exitConflict {
		t.Fatalf("email in use: %+v", r)
	}
	r = rmhub(adminEnv, "admin", "user", "list")
	if r.code != 0 || !strings.Contains(r.stdout, u.ID) || !strings.Contains(r.stdout, "ada@example.org") {
		t.Fatalf("user list: %+v", r)
	}

	r = rmhub(adminEnv, "admin", "token", "issue", u.ID, "--label", "laptop")
	owner := strings.TrimSpace(r.stdout)
	if r.code != 0 || !strings.HasPrefix(owner, "rmh_"+u.ID+"_") || !strings.Contains(r.stderr, "Store it now") {
		t.Fatalf("token issue: %+v", r)
	}
	ownerEnv := map[string]string{"RMHUB_URL": url, "RMHUB_TOKEN": owner}

	r = rmhub(ownerEnv, "pair")
	fields := strings.Fields(r.stdout)
	if r.code != 0 || len(fields) < 1 || len(fields[0]) != 8 || !strings.Contains(r.stdout, "expires") {
		t.Fatalf("pair: %+v", r)
	}

	r = rmhub(ownerEnv, "account", "--json")
	if r.code != 0 || !strings.Contains(r.stdout, u.ID) {
		t.Fatalf("account: %+v", r)
	}

	// JSON output: one JSON array.
	r = rmhub(ownerEnv, "devices", "list", "--json")
	var arr []any
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &arr) != nil {
		t.Fatalf("devices list --json: %+v", r)
	}

	// Exit code 4 and a JSON error.
	r = rmhub(ownerEnv, "devices", "remove", "X", "--json")
	if r.code != exitNotFound || !strings.Contains(r.stderr, `{"error":{"code":"not_found"`) || r.stdout != "" {
		t.Fatalf("remove unknown device: %+v", r)
	}

	r = rmhub(ownerEnv, "tokens", "create", "--label", "script", "--expires", "720h", "--json")
	var created issuedView
	if r.code != 0 || json.Unmarshal([]byte(r.stdout), &created) != nil || created.ExpiresAt == nil {
		t.Fatalf("tokens create: %+v", r)
	}
	r = rmhub(ownerEnv, "tokens", "list")
	if r.code != 0 || !strings.Contains(r.stdout, "laptop") || !strings.Contains(r.stdout, "script") || strings.Contains(r.stdout, created.Token) {
		t.Fatalf("tokens list: %+v", r)
	}
	if r := rmhub(ownerEnv, "tokens", "revoke", created.ID); r.code != 0 {
		t.Fatalf("revoke: %+v", r)
	}
	// Exit code 3: revoked token.
	r = rmhub(map[string]string{"RMHUB_URL": url, "RMHUB_TOKEN": created.Token}, "account")
	if r.code != exitAuth {
		t.Fatalf("revoked token: %+v", r)
	}
	// Exit code 3: owner token on an admin endpoint.
	if r := rmhub(ownerEnv, "admin", "user", "list"); r.code != exitAuth {
		t.Fatalf("owner on admin: %+v", r)
	}
	// The option overrides the environment.
	if r := rmhub(map[string]string{"RMHUB_URL": url, "RMHUB_TOKEN": created.Token}, "account", "--token", owner); r.code != 0 {
		t.Fatalf("--token: %+v", r)
	}
}

func TestUsageErrors(t *testing.T) {
	isolate(t)
	env := map[string]string{"RMHUB_URL": "http://127.0.0.1:1", "RMHUB_TOKEN": "x"}
	for _, args := range [][]string{
		{"nonsense"},
		{"devices", "list", "--nonsense"},
		{"devices", "remove"},
		{"admin", "user", "add"},
		{"tokens", "create", "--expires", "soon"},
	} {
		if r := rmhub(env, args...); r.code != exitUsage {
			t.Errorf("%v: exit %d, want 2 (%s)", args, r.code, r.stderr)
		}
	}
	// A connection error is an unexpected error.
	if r := rmhub(env, "devices", "list"); r.code != exitUnexpected {
		t.Errorf("connection error: exit %d", r.code)
	}
}

func TestGenerate(t *testing.T) {
	isolate(t)
	r := rmhub(nil, "admin", "token", "generate")
	tok := strings.TrimSpace(r.stdout)
	if r.code != 0 || tokens.CheckAdmin(tok) != nil {
		t.Fatalf("%+v", r)
	}
}

func TestHelp(t *testing.T) {
	r := rmhub(nil, "devices", "remove", "--help")
	for _, want := range []string{"Remove the device", "<id>", "--json", "JSON output", "Examples:", "rmhub devices remove RM110"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help has no %q:\n%s", want, r.stdout)
		}
	}
}

// TestEachCommandHasHelp checks that each command has a purpose, an
// example and a JSON output form.
func TestEachCommandHasHelp(t *testing.T) {
	root := (&cli{}).rootCmd()
	for _, cmd := range allCommands(root) {
		if !cmd.Runnable() || cmd.Name() == "help" || cmd.Name() == "completion" || cmd.HasParent() && cmd.Parent().Name() == "completion" {
			continue
		}
		if cmd.Short == "" || cmd.Long == "" || cmd.Example == "" {
			t.Errorf("%s: help text is not complete", cmd.CommandPath())
		}
		if !strings.Contains(cmd.Long, "JSON output") {
			t.Errorf("%s: help text does not give the JSON output", cmd.CommandPath())
		}
	}
}

func allCommands(c *cobra.Command) []*cobra.Command {
	out := []*cobra.Command{c}
	for _, sub := range c.Commands() {
		out = append(out, allCommands(sub)...)
	}
	return out
}
