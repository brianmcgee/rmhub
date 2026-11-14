// Command rmhub is a self-hosted sync server for reMarkable tablets.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/brianmcgee/rmhub/internal/config"
	"github.com/brianmcgee/rmhub/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage: rmhub <command> [options]

Commands:
  serve      Run the server.
  version    Print the version.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "serve":
		return serveCmd(args[1:], stderr)
	case "version":
		_, _ = fmt.Fprintln(stdout, "rmhub", version)
		return 0
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usage)
		return 0
	default:
		_, _ = fmt.Fprintf(stderr, "rmhub: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func serveCmd(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfgPath := fs.String("config", "", "path of the configuration file (default $XDG_CONFIG_HOME/rmhub/config.toml)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	log := slog.New(slog.NewTextHandler(stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	if err := serve(ctx, log, *cfgPath); err != nil {
		log.Error("rmhub serve failed", "err", err)
		return 1
	}
	return 0
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

	tablet := http.NewServeMux()
	server.HandleHealth(tablet)
	api := http.NewServeMux()
	server.HandleHealth(api)

	bound, err := server.Bind(
		server.Listener{Name: "tablet", Config: cfg.Listen.Tablet, Handler: tablet},
		server.Listener{Name: "api", Config: cfg.Listen.API, Handler: api},
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
