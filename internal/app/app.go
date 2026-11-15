// Package app connects the stores and the services to the two HTTP
// handlers of rmhub serve.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/api"
	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/devices"
	"github.com/brianmcgee/rmhub/internal/libops"
	"github.com/brianmcgee/rmhub/internal/library"
	"github.com/brianmcgee/rmhub/internal/pairing"
	"github.com/brianmcgee/rmhub/internal/rendercache"
	"github.com/brianmcgee/rmhub/internal/rootlog"
	"github.com/brianmcgee/rmhub/internal/server"
	"github.com/brianmcgee/rmhub/internal/tablet"
	"github.com/brianmcgee/rmhub/internal/tablet/notify"
	"github.com/brianmcgee/rmhub/internal/tablet/sync15"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// App holds the services of one server process.
type App struct {
	Accounts  *accounts.Store
	Tokens    *tokens.Store
	Devices   *devices.Registry
	Pairing   *pairing.Store
	Roots     *rootlog.Log
	Issuer    *tablet.Issuer
	Tablet    *tablet.Service
	Discovery *tablet.Discovery
	Sync      *sync15.Service
	Hub       *notify.Hub
	API       *api.Service
	Library   *api.Library
	log       *slog.Logger
}

// New makes the services on the stores. It reads the admin token file
// and the JWT signing key. The notification hub runs until ctx ends.
func New(ctx context.Context, cfg *config.Config, stores *server.Stores, log *slog.Logger) (*App, error) {
	var admin *tokens.Admin
	if cfg.Admin.TokenFile != "" {
		a, err := tokens.ReadAdminFile(cfg.Admin.TokenFile)
		if err != nil {
			return nil, fmt.Errorf("admin.token_file: %w", err)
		}
		admin = a
	}
	key, err := tablet.LoadKey(ctx, stores.State)
	if err != nil {
		return nil, fmt.Errorf("JWT signing key: %w", err)
	}
	st := stores.State
	a := &App{
		Accounts: accounts.New(st),
		Tokens:   tokens.New(st),
		Devices:  devices.New(st),
		Pairing:  pairing.New(st),
		Roots:    rootlog.New(st, rootlog.WithLogger(log)),
		Issuer:   tablet.NewIssuer(key),
		log:      log,
	}
	a.Tablet = &tablet.Service{Accounts: a.Accounts, Devices: a.Devices, Pairing: a.Pairing, Issuer: a.Issuer, Log: log}
	a.Discovery = &tablet.Discovery{Host: cfg.Tablet.AnnouncedHost}
	a.Hub = notify.New(log, a.Devices.Check)
	a.Sync = &sync15.Service{
		Blobs:      stores.Blob,
		Roots:      a.Roots,
		Accounts:   a.Accounts,
		Notifier:   a.Hub,
		Log:        log,
		TempDir:    stores.TempDir,
		CheckFiles: cfg.Tablet.CheckFiles != "none",
		MaxBlob:    cfg.Tablet.MaxBlobBytes,
	}
	cache, err := rendercache.New(cfg.Render.CacheDir, cfg.Render.CacheMaxBytes)
	if err != nil {
		return nil, fmt.Errorf("render.cache_dir: %w", err)
	}
	lib := library.New(a.Roots, stores.Blob)
	a.API = &api.Service{Accounts: a.Accounts, Tokens: a.Tokens, Devices: a.Devices, Pairing: a.Pairing, Library: lib, Admin: admin, Log: log}
	a.Library = &api.Library{
		Lib: lib,
		Ops: &libops.Ops{
			Lib: lib, Blobs: stores.Blob, Roots: a.Roots, Now: time.Now,
			Schema: func(ctx context.Context, account string) (int, error) {
				acct, err := a.Accounts.Get(ctx, account)
				return acct.SchemaVersion, err
			},
		},
		Blobs:     stores.Blob,
		Roots:     a.Roots,
		Tokens:    a.Tokens,
		Kinds:     a.API.Kinds(),
		Log:       log,
		MaxUpload: cfg.API.MaxUploadBytes,
		TempDir:   stores.TempDir,
		Stop:      ctx.Done(),
		Render:    api.NewRenderer(cache, runtime.NumCPU(), log),
	}
	go func() {
		<-ctx.Done()
		_ = a.Library.Render.Close()
	}()
	go a.Hub.Run(ctx, a.Roots)
	return a, nil
}

// TabletHandler returns the handler of the tablet listener.
func (a *App) TabletHandler() http.Handler {
	mux := http.NewServeMux()
	server.HandleHealth(mux)
	a.Tablet.Register(mux)
	a.Discovery.Register(mux, a.Tablet.RequireUser)
	a.Sync.Register(mux, a.Tablet.RequireUser)
	mux.Handle("GET /notifications/ws/json/1", a.Tablet.RequireUser(a.Hub))
	mux.Handle("/", tablet.NotFound(a.log))
	return mux
}

// APIHandler returns the handler of the API listener.
func (a *App) APIHandler() http.Handler {
	m := api.NewMux(a.API.Log)
	a.API.Register(m)
	a.Library.Register(m)
	mux := http.NewServeMux()
	server.HandleHealth(mux)
	mux.Handle("/api/", m)
	return mux
}
