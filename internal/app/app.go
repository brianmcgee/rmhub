// Package app connects the stores and the services to the two HTTP
// handlers of rmhub serve.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/brianmcgee/rmhub/internal/accounts"
	"github.com/brianmcgee/rmhub/internal/api"
	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/devices"
	"github.com/brianmcgee/rmhub/internal/pairing"
	"github.com/brianmcgee/rmhub/internal/server"
	"github.com/brianmcgee/rmhub/internal/tablet"
	"github.com/brianmcgee/rmhub/internal/tokens"
)

// App holds the services of one server process.
type App struct {
	Accounts *accounts.Store
	Tokens   *tokens.Store
	Devices  *devices.Registry
	Pairing  *pairing.Store
	Issuer   *tablet.Issuer
	Tablet   *tablet.Service
	API      *api.Service
}

// New makes the services on the stores. It reads the admin token file
// and the JWT signing key.
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
		Issuer:   tablet.NewIssuer(key),
	}
	a.Tablet = &tablet.Service{Accounts: a.Accounts, Devices: a.Devices, Pairing: a.Pairing, Issuer: a.Issuer, Log: log}
	a.API = &api.Service{Accounts: a.Accounts, Tokens: a.Tokens, Devices: a.Devices, Pairing: a.Pairing, Admin: admin, Log: log}
	return a, nil
}

// TabletHandler returns the handler of the tablet listener.
func (a *App) TabletHandler() http.Handler {
	mux := http.NewServeMux()
	server.HandleHealth(mux)
	a.Tablet.Register(mux)
	return mux
}

// APIHandler returns the handler of the API listener.
func (a *App) APIHandler() http.Handler {
	m := api.NewMux(a.API.Log)
	a.API.Register(m)
	mux := http.NewServeMux()
	server.HandleHealth(mux)
	mux.Handle("/api/", m)
	return mux
}
