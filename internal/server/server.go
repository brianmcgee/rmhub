// Package server runs the HTTP listeners of rmhub serve.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/brianmcgee/rmhub/internal/config"
)

// IdleTimeout is the time that a connection without a request stays
// open. Without it, each client can keep connections open with no limit
// of time.
var IdleTimeout = 2 * time.Minute

// ShutdownTimeout is the time that requests in progress get to finish
// after a shutdown signal.
const ShutdownTimeout = 10 * time.Second

// Listener is one listener with its handler.
type Listener struct {
	Name    string
	Config  config.Listener
	Handler http.Handler
}

// HandleHealth adds GET /healthz to the mux.
func HandleHealth(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
}

// Bound is a listener with an open socket.
type Bound struct {
	Listener
	ln net.Listener
}

// Addr returns the address of the socket.
func (b *Bound) Addr() net.Addr { return b.ln.Addr() }

// Bind opens the sockets of all listeners. If one fails, it closes the
// others and returns an error that names the address.
func Bind(ls ...Listener) ([]*Bound, error) {
	var out []*Bound
	for _, l := range ls {
		var tlsCfg *tls.Config
		if l.Config.TLSCertFile != "" {
			cert, err := tls.LoadX509KeyPair(l.Config.TLSCertFile, l.Config.TLSKeyFile)
			if err != nil {
				closeAll(out)
				return nil, fmt.Errorf("listen.%s: %w", l.Name, err)
			}
			tlsCfg = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
		}
		ln, err := net.Listen("tcp", l.Config.Address)
		if err != nil {
			closeAll(out)
			return nil, fmt.Errorf("listen.%s: address %s: %w", l.Name, l.Config.Address, err)
		}
		if tlsCfg != nil {
			ln = tls.NewListener(ln, tlsCfg)
		}
		out = append(out, &Bound{Listener: l, ln: ln})
	}
	return out, nil
}

func closeAll(bs []*Bound) {
	for _, b := range bs {
		_ = b.ln.Close()
	}
}

// Serve serves the bound listeners until ctx ends. Then it stops
// accepting connections and lets requests in progress finish for up to
// ShutdownTimeout.
func Serve(ctx context.Context, log *slog.Logger, bs []*Bound) error {
	servers := make([]*http.Server, len(bs))
	errc := make(chan error, len(bs))
	for i, b := range bs {
		srv := &http.Server{
			Handler:           b.Handler,
			ReadHeaderTimeout: 30 * time.Second,
			// No ReadTimeout and no WriteTimeout: uploads, event streams
			// and websockets are long.
			IdleTimeout: IdleTimeout,
			ErrorLog:    slog.NewLogLogger(log.Handler(), slog.LevelWarn),
			BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
		}
		servers[i] = srv
		log.Info("listening", "listener", b.Name, "address", b.Addr().String(), "tls", b.Config.TLSCertFile != "")
		go func() {
			if err := srv.Serve(b.ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errc <- fmt.Errorf("listen.%s: %w", b.Name, err)
				return
			}
			errc <- nil
		}()
	}

	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errc:
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), ShutdownTimeout)
	defer cancel()
	for _, srv := range servers {
		if err := srv.Shutdown(sctx); err != nil {
			log.Warn("shutdown", "err", err)
			_ = srv.Close()
		}
	}
	return serveErr
}
