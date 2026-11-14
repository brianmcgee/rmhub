package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestMain runs the command when the test binary runs as a child.
func TestMain(m *testing.M) {
	if os.Getenv("RMHUB_TEST_RUN_MAIN") == "1" {
		os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
	}
	os.Exit(m.Run())
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().String()
}

// startServe starts the command in a child process.
func startServe(t *testing.T, env []string, args ...string) (*exec.Cmd, *bytes.Buffer) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), "RMHUB_TEST_RUN_MAIN=1")
	cmd.Env = append(cmd.Env, env...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd, &out
}

func waitHealthy(t *testing.T, url string, out *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server did not become healthy:\n%s", out)
}

func xdgEnv(t *testing.T) []string {
	home := t.TempDir()
	return []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
	}
}

// TestServeDefaults starts rmhub serve with no configuration file. It
// uses the default addresses, so it skips when they are in use.
func TestServeDefaults(t *testing.T) {
	for _, a := range []string{"127.0.0.1:8080", "127.0.0.1:8081"} {
		l, err := net.Listen("tcp", a)
		if err != nil {
			t.Skipf("default address %s is in use", a)
		}
		_ = l.Close()
	}
	env := xdgEnv(t)
	cmd, out := startServe(t, env, "serve")
	waitHealthy(t, "http://127.0.0.1:8081", out)
	waitHealthy(t, "http://127.0.0.1:8080", out)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit after SIGTERM: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(env[2][len("XDG_DATA_HOME="):], "rmhub", "state.sqlite")); err != nil {
		t.Fatalf("state file: %v", err)
	}
}

func TestServeConfigFile(t *testing.T) {
	dir := t.TempDir()
	tablet, api := freeAddr(t), freeAddr(t)
	cfg := filepath.Join(dir, "config.toml")
	text := fmt.Sprintf("data_dir = %q\n[listen.tablet]\naddress = %q\n[listen.api]\naddress = %q\n", dir, tablet, api)
	if err := os.WriteFile(cfg, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd, out := startServe(t, xdgEnv(t), "serve", "--config", cfg)
	waitHealthy(t, "http://"+api, out)
	waitHealthy(t, "http://"+tablet, out)
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("exit after SIGINT: %v\n%s", err, out)
	}
}

func TestServeConfigError(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("[blob]\ndirr = \"/x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd, out := startServe(t, xdgEnv(t), "serve", "--config", cfg)
	err := cmd.Wait()
	if err == nil || !bytes.Contains(out.Bytes(), []byte("blob.dirr")) {
		t.Fatalf("exit %v, output:\n%s", err, out)
	}
}
