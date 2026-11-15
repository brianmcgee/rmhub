package notify

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/brianmcgee/rmhub/internal/rootlog"
)

// TestFullBuffer checks that a websocket whose writer does not drain the
// send buffer is closed after SendBuffer messages, and that Notify does
// not block.
func TestFullBuffer(t *testing.T) {
	h := New(slog.New(slog.NewTextHandler(io.Discard, nil)), func(context.Context, string, string, string) (bool, error) { return true, nil })
	conns := make(chan *conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := h.upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		// No writer goroutine: the buffer is never drained.
		c := &conn{ws: ws, account: "a", deviceID: "D2", send: make(chan []byte, SendBuffer), done: make(chan struct{})}
		h.add(c)
		conns <- c
	}))
	defer srv.Close()
	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = client.Close() }()
	c := <-conns

	src := rootlog.Source{Type: rootlog.SourceDevice, ID: "D1"}
	start := time.Now()
	closedAt := -1
	for i := range 100 {
		h.Notify("a", src)
		select {
		case <-c.done:
			if closedAt < 0 {
				closedAt = i
			}
		default:
		}
	}
	// The 64 messages fill the buffer; the 65th (index 64) closes it.
	if closedAt != SendBuffer {
		t.Fatalf("closed at message index %d, want %d", closedAt, SendBuffer)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Notify blocked for %v", d)
	}
	// Messages of the source device itself are skipped.
	h2 := New(slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	own := &conn{account: "a", deviceID: "D1", send: make(chan []byte, 1), done: make(chan struct{})}
	h2.add(own)
	h2.Notify("a", src)
	if len(own.send) != 0 {
		t.Fatal("source device got its own message")
	}
}
