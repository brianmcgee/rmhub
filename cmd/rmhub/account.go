package main

import (
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/spf13/cobra"

	"github.com/brianmcgee/rmhub/internal/tokens"
)

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type tokenView struct {
	ID        string     `json:"id"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type issuedView struct {
	ID        string     `json:"id"`
	Token     string     `json:"token"`
	Label     string     `json:"label"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

type deviceView struct {
	DeviceID string    `json:"deviceId"`
	Desc     string    `json:"desc"`
	PairedAt time.Time `json:"pairedAt"`
}

func expiresText(t *time.Time) string {
	if t == nil {
		return "never"
	}
	return t.UTC().Format(time.RFC3339)
}

func (c *cli) adminCmd() *cobra.Command {
	admin := &cobra.Command{
		Use:   "admin",
		Short: "Manage accounts with the admin token",
		Long:  "Manage accounts. Each command except \"admin token generate\" needs the admin token.",
	}
	user := &cobra.Command{Use: "user", Short: "Manage user accounts"}
	token := &cobra.Command{Use: "token", Short: "Generate the admin token and issue owner tokens"}
	admin.AddCommand(user, token)

	var email, name string
	add := &cobra.Command{
		Use:   "add --email <email> [--name <name>]",
		Short: "Create a user account",
		Long: `Create a user account. The email address must be unique without regard
to case.

JSON output: {"id":"…","email":"…","name":"…"}`,
		Example: "  rmhub admin user add --email ada@example.org --name Ada",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" {
				return usageErr("the option --email is necessary")
			}
			cl, err := c.client()
			if err != nil {
				return err
			}
			var u userView
			if _, err := cl.do(cmd.Context(), "POST", "/api/v1/admin/users", map[string]string{"email": email, "name": name}, &u); err != nil {
				return err
			}
			return c.output(u, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "Created account %s for %s\n", u.ID, u.Email)
			})
		},
	}
	add.Flags().StringVar(&email, "email", "", "email address of the user (necessary)")
	add.Flags().StringVar(&name, "name", "", "display name of the user")

	list := &cobra.Command{
		Use:   "list",
		Short: "List the user accounts",
		Long: `List all user accounts, ordered by ID.

JSON output: [{"id":"…","email":"…","name":"…"}, …]`,
		Example: "  rmhub admin user list --json",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			var us []userView
			if _, err := cl.do(cmd.Context(), "GET", "/api/v1/admin/users", nil, &us); err != nil {
				return err
			}
			return c.output(us, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, "ID\tEMAIL\tNAME")
				for _, u := range us {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", u.ID, u.Email, u.Name)
				}
			})
		},
	}
	user.AddCommand(add, list)

	var label, expires string
	issue := &cobra.Command{
		Use:   "issue <account> [--label <label>] [--expires <time>]",
		Short: "Issue an owner token for an account",
		Long: `Issue an owner token for the account with the ID <account>. An owner
token has full access to the account. The command prints the token on
standard output. The server does not keep the token text, so store it
now.

--expires takes an RFC 3339 time or a duration from now, such as 720h.
Without --expires, the token does not expire.

JSON output: {"id":"…","token":"…","label":"…","expiresAt":…}`,
		Example: "  rmhub admin token issue 3x7k2m4q5r6s7t2u3v4w5x6y7z --label laptop",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			exp, err := parseExpires(expires, time.Now())
			if err != nil {
				return err
			}
			cl, err := c.client()
			if err != nil {
				return err
			}
			var t issuedView
			path := "/api/v1/admin/users/" + url.PathEscape(args[0]) + "/tokens"
			if _, err := cl.do(cmd.Context(), "POST", path, map[string]any{"label": label, "expiresAt": exp}, &t); err != nil {
				return err
			}
			return c.printIssued(t)
		},
	}
	issue.Flags().StringVar(&label, "label", "", "label that names the token")
	issue.Flags().StringVar(&expires, "expires", "", "expiry time (RFC 3339) or duration from now")

	generate := &cobra.Command{
		Use:   "generate",
		Short: "Print a new random admin token",
		Long: `Print a new random admin token. The command does not need a server.
Put the token in the file that admin.token_file names, and restart the
server.

JSON output: {"token":"…"}`,
		Example: "  rmhub admin token generate > /run/secrets/rmhub-admin-token",
		Args:    noArgs,
		RunE: func(*cobra.Command, []string) error {
			tok := tokens.GenerateAdmin()
			return c.output(map[string]string{"token": tok}, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, tok)
			})
		},
	}
	token.AddCommand(issue, generate)
	return admin
}

func (c *cli) printIssued(t issuedView) error {
	if !c.json {
		_, _ = fmt.Fprintf(c.stderr, "Token %s (label %q, expires %s). Store it now: the server does not keep it.\n", t.ID, t.Label, expiresText(t.ExpiresAt))
	}
	return c.output(t, func(w io.Writer) { _, _ = fmt.Fprintln(w, t.Token) })
}

func (c *cli) accountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "account",
		Short: "Show the account of the token",
		Long: `Show the account that the owner token belongs to.

JSON output: {"id":"…","email":"…","name":"…"}`,
		Example: "  rmhub account",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			var u userView
			if _, err := cl.do(cmd.Context(), "GET", "/api/v1/account", nil, &u); err != nil {
				return err
			}
			return c.output(u, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "ID:\t%s\nEmail:\t%s\nName:\t%s\n", u.ID, u.Email, u.Name)
			})
		},
	}
}

func (c *cli) pairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pair",
		Short: "Create a code to pair a tablet",
		Long: `Create a one-time pairing code for the account. Type the code on the
tablet or in the reMarkable app within 5 minutes.

JSON output: {"code":"…","expiresAt":"…"}`,
		Example: "  rmhub pair",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			var p struct {
				Code      string    `json:"code"`
				ExpiresAt time.Time `json:"expiresAt"`
			}
			if _, err := cl.do(cmd.Context(), "POST", "/api/v1/account/pairing-codes", nil, &p); err != nil {
				return err
			}
			return c.output(p, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "%s\t(expires %s)\n", p.Code, p.ExpiresAt.UTC().Format(time.RFC3339))
			})
		},
	}
}

func (c *cli) devicesCmd() *cobra.Command {
	devices := &cobra.Command{Use: "devices", Short: "List and remove paired devices"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the paired devices",
		Long: `List the devices that paired with the account.

JSON output: [{"deviceId":"…","desc":"…","pairedAt":"…"}, …]`,
		Example: "  rmhub devices list",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			var ds []deviceView
			if _, err := cl.do(cmd.Context(), "GET", "/api/v1/account/devices", nil, &ds); err != nil {
				return err
			}
			return c.output(ds, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, "DEVICE ID\tDESCRIPTION\tPAIRED")
				for _, d := range ds {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", d.DeviceID, d.Desc, d.PairedAt.UTC().Format(time.RFC3339))
				}
			})
		},
	}
	remove := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a paired device",
		Long: `Remove the device with the device ID <id> from the account. Within 60
seconds, the device can no longer sync.

JSON output: {"deviceId":"…","removed":true}`,
		Example: "  rmhub devices remove RM110-313-12345",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			if _, err := cl.do(cmd.Context(), "DELETE", "/api/v1/account/devices/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			return c.output(map[string]any{"deviceId": args[0], "removed": true}, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "Removed device %s\n", args[0])
			})
		},
	}
	devices.AddCommand(list, remove)
	return devices
}

func (c *cli) tokensCmd() *cobra.Command {
	toks := &cobra.Command{Use: "tokens", Short: "Manage the API tokens of the account"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the tokens of the account",
		Long: `List the API tokens of the account. The list does not contain the token
secrets.

JSON output: [{"id":"…","label":"…","createdAt":"…","expiresAt":…}, …]`,
		Example: "  rmhub tokens list",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			var ts []tokenView
			if _, err := cl.do(cmd.Context(), "GET", "/api/v1/account/tokens", nil, &ts); err != nil {
				return err
			}
			return c.output(ts, func(w io.Writer) {
				_, _ = fmt.Fprintln(w, "ID\tLABEL\tCREATED\tEXPIRES")
				for _, t := range ts {
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", t.ID, t.Label, t.CreatedAt.UTC().Format(time.RFC3339), expiresText(t.ExpiresAt))
				}
			})
		},
	}
	var label, expires string
	create := &cobra.Command{
		Use:   "create [--label <label>] [--expires <time>]",
		Short: "Create an owner token for the account",
		Long: `Create an owner token for the account of the current token. The command
prints the token on standard output. The server does not keep the token
text, so store it now.

--expires takes an RFC 3339 time or a duration from now, such as 720h.
Without --expires, the token does not expire.

JSON output: {"id":"…","token":"…","label":"…","expiresAt":…}`,
		Example: "  rmhub tokens create --label backup-script --expires 720h",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exp, err := parseExpires(expires, time.Now())
			if err != nil {
				return err
			}
			cl, err := c.client()
			if err != nil {
				return err
			}
			var t issuedView
			if _, err := cl.do(cmd.Context(), "POST", "/api/v1/account/tokens", map[string]any{"label": label, "expiresAt": exp}, &t); err != nil {
				return err
			}
			return c.printIssued(t)
		},
	}
	create.Flags().StringVar(&label, "label", "", "label that names the token")
	create.Flags().StringVar(&expires, "expires", "", "expiry time (RFC 3339) or duration from now")
	revoke := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a token",
		Long: `Revoke the token with the ID <id>. The next request with the token gets
status 401.

JSON output: {"id":"…","revoked":true}`,
		Example: "  rmhub tokens revoke abcdefghijklmnop",
		Args:    exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cl, err := c.client()
			if err != nil {
				return err
			}
			if _, err := cl.do(cmd.Context(), "DELETE", "/api/v1/account/tokens/"+url.PathEscape(args[0]), nil, nil); err != nil {
				return err
			}
			return c.output(map[string]any{"id": args[0], "revoked": true}, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "Revoked token %s\n", args[0])
			})
		},
	}
	toks.AddCommand(list, create, revoke)
	return toks
}
