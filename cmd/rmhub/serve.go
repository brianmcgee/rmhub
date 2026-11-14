package main

import (
	"context"
	"errors"
	"log/slog"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/brianmcgee/rmhub/internal/app"
	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/server"
)

func (c *cli) serveCmd() *cobra.Command {
	var cfgPath string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the server",
		Long: `Run the rmhub server. It opens the tablet listener and the API listener,
and runs until it gets SIGTERM or SIGINT.

Without --config, the server reads $XDG_CONFIG_HOME/rmhub/config.toml,
and uses the default values when that file does not exist.

The command has no JSON output.`,
		Example: "  rmhub serve --config /etc/rmhub/config.toml",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := slog.New(slog.NewTextHandler(c.stderr, nil))
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			if err := serve(ctx, log, cfgPath); err != nil {
				log.Error("rmhub serve failed", "err", err)
				return &exitError{code: exitUnexpected, err: errors.New("the server stopped with an error")}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&cfgPath, "config", "", "path of the configuration file (default $XDG_CONFIG_HOME/rmhub/config.toml)")
	return cmd
}

func serve(ctx context.Context, log *slog.Logger, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	stores, err := server.OpenStores(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = stores.Close() }()
	if err := server.Probe(ctx, stores.State); err != nil {
		return err
	}
	a, err := app.New(ctx, cfg, stores, log)
	if err != nil {
		return err
	}
	bound, err := server.Bind(
		server.Listener{Name: "tablet", Config: cfg.Listen.Tablet, Handler: a.TabletHandler()},
		server.Listener{Name: "api", Config: cfg.Listen.API, Handler: a.APIHandler()},
	)
	if err != nil {
		return err
	}
	err = server.Serve(ctx, log, bound)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
