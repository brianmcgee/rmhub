# Command line

The `rmhub` command is the server (`rmhub serve`, see
[Server configuration](configuration.md)) and the client of the rmhub
API. `rmhub <command> --help` shows the options, the JSON output form
and an example of each command.

## Client settings

| Setting | Sources, in order |
|---|---|
| Server URL | `--url`, `RMHUB_URL`, `url` in `$XDG_CONFIG_HOME/rmhub/client.toml` |
| Token | `--token`, `RMHUB_TOKEN`, `token` in `$XDG_CONFIG_HOME/rmhub/client.toml` |

```toml
# ~/.config/rmhub/client.toml (mode 600)
url = "http://127.0.0.1:8081"
token = "rmh_…"
```

## Commands

| Command | Purpose |
|---|---|
| `rmhub serve [--config <file>]` | Run the server. |
| `rmhub version` | Print the version. |
| `rmhub admin user add --email <email> [--name <name>]` | Create an account (admin token). |
| `rmhub admin user list` | List the accounts (admin token). |
| `rmhub admin token issue <account> [--label <l>] [--expires <t>]` | Issue an owner token (admin token). |
| `rmhub admin token generate` | Print a new admin token. Needs no server. |
| `rmhub account` | Show the account of the token. |
| `rmhub account set-schema <3\|4>` | Set the root index schema that the server announces to the tablets. |
| `rmhub pair` | Make a pairing code. |
| `rmhub devices list`, `rmhub devices remove <id>` | List or remove paired devices. |
| `rmhub tokens list`, `rmhub tokens create [--grant <entry>:<read\|write>]`, `rmhub tokens revoke <id>` | Manage owner tokens and scoped tokens. |
| `rmhub ls [entry]`, `rmhub stat <entry>`, `rmhub find <text>` | Read the library. |
| `rmhub get <entry> [-o <file>] [--archive]` | Download a PDF, an EPUB or the archive of a document. |
| `rmhub pages <entry>` | List the pages of a document. |
| `rmhub render <entry> --page <n> [--format png\|svg] [--width <px>] [-o <file>]` | Render a page of a notebook or a PDF as an image. |
| `rmhub upload <file> [--to <entry>] [--name <name>]` | Upload a PDF or an EPUB. |
| `rmhub mkdir`, `rmhub mv`, `rmhub rename`, `rmhub trash` | Change the library. Each change gets to the tablets at their next sync. |
| `rmhub watch [--after <generation>]` | Print the changes of the library, one JSON line for each generation. |

[The guide for AI agents](../agents/SKILL.md) gives the JSON output and
an example of each library command.

## Entry arguments

An `<entry>` argument is an ID or a path. A path starts with `/`, for
example `/Work/Paper`. Two entries can have the same path. Then the
command exits with code 5 and prints the IDs. Use the ID after that.

## Output

Each command prints text by default and one JSON value with `--json`.
Errors go to standard error.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success. |
| 1 | Unexpected error. |
| 2 | Usage or setting error. |
| 3 | Status 401 or 403. |
| 4 | Status 404. |
| 5 | Status 409 or 412. |
| 6 | Status 429. See [Request limits](configuration.md#request-limits). |
