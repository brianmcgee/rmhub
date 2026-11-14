package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/server"
	"github.com/brianmcgee/rmhub/internal/state"
	"github.com/brianmcgee/rmhub/internal/state/sqlite"
)

func openSQLite(t *testing.T) *sqlite.Store {
	t.Helper()
	s, err := sqlite.Open(filepath.Join(t.TempDir(), "s.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestProbeAcceptsSQLite(t *testing.T) {
	s := openSQLite(t)
	if err := server.Probe(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	keys, err := s.List(context.Background(), "server/probe/", "", 0)
	if err != nil || len(keys) != 0 {
		t.Fatalf("probe keys left: %v, %v", keys, err)
	}
}

// careless is a state store that ignores all conditions.
type careless struct{ state.Store }

func (c careless) PutIfAbsent(ctx context.Context, k string, v []byte) (string, error) {
	_ = c.Delete(ctx, k)
	return c.Store.PutIfAbsent(ctx, k, v)
}

func (c careless) PutIfMatch(ctx context.Context, k string, v []byte, _ string) (string, error) {
	_ = c.Delete(ctx, k)
	return c.Store.PutIfAbsent(ctx, k, v)
}

func TestProbeRefusesCarelessBackend(t *testing.T) {
	s := openSQLite(t)
	err := server.Probe(context.Background(), careless{s})
	if !errors.Is(err, server.ErrNoConditionalWrites) {
		t.Fatalf("err = %v, want ErrNoConditionalWrites", err)
	}
	if !strings.Contains(err.Error(), "does not enforce conditional writes") {
		t.Fatalf("message %q", err)
	}
	keys, _ := s.List(context.Background(), "server/probe/", "", 0)
	if len(keys) != 0 {
		t.Fatalf("probe keys left: %v", keys)
	}
}

func TestAddressInUse(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()
	_, err = server.Bind(
		server.Listener{Name: "api", Config: config.Listener{Address: "127.0.0.1:0"}, Handler: http.NewServeMux()},
		server.Listener{Name: "tablet", Config: config.Listener{Address: addr}, Handler: http.NewServeMux()},
	)
	if err == nil || !strings.Contains(err.Error(), addr) {
		t.Fatalf("err = %v, want an error that names %s", err, addr)
	}
}

func TestShutdownWaitsForRequest(t *testing.T) {
	started := make(chan struct{})
	mux := http.NewServeMux()
	server.HandleHealth(mux)
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("done"))
	})
	bs, err := server.Bind(server.Listener{Name: "api", Config: config.Listener{Address: "127.0.0.1:0"}, Handler: mux})
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + bs[0].Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- server.Serve(ctx, slog.New(slog.DiscardHandler), bs) }()

	resp, err := http.Get(url + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", resp, err)
	}
	_ = resp.Body.Close()

	got := make(chan string, 1)
	go func() {
		resp, err := http.Get(url + "/slow")
		if err != nil {
			got <- "error: " + err.Error()
			return
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		got <- string(b)
	}()
	<-started
	cancel()
	if body := <-got; body != "done" {
		t.Fatalf("request in progress got %q", body)
	}
	if err := <-served; err != nil {
		t.Fatalf("serve: %v", err)
	}
	if _, err := http.Get(url + "/healthz"); err == nil {
		t.Fatal("server accepts requests after shutdown")
	}
}
