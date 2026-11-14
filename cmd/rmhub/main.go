// Command rmhub is a self-hosted sync server for reMarkable tablets, and
// the client of its API.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// cli holds the state of one run of the command.
type cli struct {
	stdout, stderr io.Writer
	getenv         func(string) string

	url, token string
	json       bool
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	c := &cli{stdout: stdout, stderr: stderr, getenv: getenv}
	root := c.rootCmd()
	root.SetArgs(args)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.ExecuteContext(context.Background())
	if err == nil {
		return exitOK
	}
	if strings.HasPrefix(err.Error(), "unknown command") || strings.HasPrefix(err.Error(), "unknown flag") ||
		strings.HasPrefix(err.Error(), "unknown shorthand flag") {
		err = &exitError{code: exitUsage, err: err}
	}
	c.printError(err)
	return exitCode(err)
}

func (c *cli) printError(err error) {
	var ae *apiError
	if c.json && errors.As(err, &ae) && len(ae.body) > 0 {
		_, _ = fmt.Fprintf(c.stderr, "%s\n", ae.body)
		return
	}
	if c.json {
		b, _ := json.Marshal(map[string]any{"error": map[string]string{"code": "client", "message": err.Error()}})
		_, _ = fmt.Fprintf(c.stderr, "%s\n", b)
		return
	}
	_, _ = fmt.Fprintf(c.stderr, "rmhub: %v\n", err)
}

func (c *cli) rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "rmhub",
		Short: "Self-hosted sync server for reMarkable tablets, and its client",
		Long: `rmhub is a sync server for reMarkable tablets. The same program is the
client of the rmhub API.

The client reads the server URL from --url, RMHUB_URL or url in
$XDG_CONFIG_HOME/rmhub/client.toml, in that order. It reads the token
from --token, RMHUB_TOKEN or token in client.toml, in that order.

Exit codes: 0 success, 1 unexpected error, 2 usage or setting error,
3 status 401 or 403, 4 status 404, 5 status 409 or 412.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&c.url, "url", "", "server URL of the rmhub API (overrides RMHUB_URL)")
	root.PersistentFlags().StringVar(&c.token, "token", "", "API token (overrides RMHUB_TOKEN)")
	root.PersistentFlags().BoolVar(&c.json, "json", false, "print one JSON value to standard output")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &exitError{code: exitUsage, err: err}
	})
	root.AddCommand(c.serveCmd(), c.versionCmd(), c.adminCmd(), c.accountCmd(), c.pairCmd(), c.devicesCmd(), c.tokensCmd())
	return root
}

func (c *cli) versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the version",
		Long:    "Print the version of rmhub.\n\nJSON output: {\"version\":\"…\"}",
		Example: "  rmhub version",
		Args:    noArgs,
		RunE: func(*cobra.Command, []string) error {
			return c.output(map[string]string{"version": version}, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, "rmhub", version)
			})
		},
	}
}

// client returns an API client with the resolved settings.
func (c *cli) client() (*client, error) {
	s, err := resolveSettings(c.url, c.token, c.getenv, c.stderr)
	if err != nil {
		return nil, err
	}
	return newClient(s), nil
}

// output prints v as JSON with --json, or calls text.
func (c *cli) output(v any, text func(w io.Writer)) error {
	if c.json {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(c.stdout, "%s\n", b)
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	text(tw)
	return tw.Flush()
}

func wrapArgs(fn cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if err := fn(cmd, args); err != nil {
			return &exitError{code: exitUsage, err: err}
		}
		return nil
	}
}

var noArgs = wrapArgs(cobra.NoArgs)

func exactArgs(n int) cobra.PositionalArgs { return wrapArgs(cobra.ExactArgs(n)) }
