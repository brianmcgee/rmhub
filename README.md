# rmhub

rmhub is a self-hosted alternative to the reMarkable cloud service.
It syncs documents between reMarkable tablets and the desktop and mobile apps.
You keep your notes on a server that you control.

## Status

Early development. The server has accounts, API tokens and tablet
pairing, but it does not sync documents yet.

## Run the server

```console
rmhub serve [--config <file>]
```

`rmhub serve` reads a TOML configuration file. Without `--config`, it
reads `$XDG_CONFIG_HOME/rmhub/config.toml` (`~/.config` when
`XDG_CONFIG_HOME` is not set). If that file does not exist, the server
uses the default values. An unknown key, a value of the wrong type or an
unknown backend stops the server with an error that names the key.

The server opens two listeners: one for the tablet protocol and one for
the rmhub API. Each listener answers `GET /healthz` with status 200. On
SIGTERM or SIGINT, the server lets requests in progress finish for up to
10 seconds and then exits with status 0.

At startup, the server checks that the state backend refuses a stale
conditional write. A backend that accepts the write stops the server.

### Configuration keys

| Key | Default | Meaning |
|---|---|---|
| `data_dir` | `$XDG_DATA_HOME/rmhub` | Directory for local data and temporary files. |
| `blob.backend` | `fs` | Blob backend: `fs` or `s3`. |
| `blob.dir` | `<data_dir>/blobs` | Directory of the `fs` blob backend. |
| `blob.s3.*` | | S3 settings of the blob backend (see below). |
| `state.backend` | `sqlite` | State backend: `sqlite` or `s3`. |
| `state.path` | `<data_dir>/state.sqlite` | File of the `sqlite` state backend. |
| `state.s3.*` | | S3 settings of the state backend (see below). |
| `listen.tablet.address` | `127.0.0.1:8080` | Address of the tablet listener. |
| `listen.api.address` | `127.0.0.1:8081` | Address of the API listener. |
| `listen.<name>.tls_cert_file` | | TLS certificate. Set it with `tls_key_file`, or set neither for plain HTTP. |
| `listen.<name>.tls_key_file` | | TLS private key. |
| `admin.token_file` | | File with the admin token. Without it, each admin request gets status 401. |

The S3 settings are `endpoint`, `region` (default `us-east-1`),
`bucket`, `prefix`, `path_style`, `access_key_id` and
`secret_access_key_file`. The secret comes from the file. The
configuration must not contain the secret: the key `secret_access_key`
is an error. Without `access_key_id`, the server uses the standard AWS
credential sources. The blob store and the state store can share one
bucket with different prefixes.

Example with blobs on Cloudflare R2 and state in SQLite:

```toml
data_dir = "/var/lib/rmhub"

[blob]
backend = "s3"
s3.endpoint = "https://<account>.r2.cloudflarestorage.com"
s3.region = "auto"
s3.bucket = "rmhub"
s3.prefix = "blobs/"
s3.path_style = true
s3.access_key_id = "<key id>"
s3.secret_access_key_file = "/run/credentials/rmhub.service/s3"

[state]
backend = "sqlite"

[listen.tablet]
address = "127.0.0.1:8080"

[listen.api]
address = "127.0.0.1:8081"
```

## First run

1. Generate the admin token and put it in a file that only the server
   can read:

   ```console
   $ install -d -m 700 ~/.config/rmhub
   $ rmhub admin token generate > ~/.config/rmhub/admin-token
   $ chmod 600 ~/.config/rmhub/admin-token
   ```

2. Name the file in the configuration and start the server:

   ```toml
   # ~/.config/rmhub/config.toml
   [admin]
   token_file = "/home/ada/.config/rmhub/admin-token"
   ```

   ```console
   $ rmhub serve
   ```

3. Add a user and issue an owner token for the new account:

   ```console
   $ export RMHUB_URL=http://127.0.0.1:8081
   $ export RMHUB_TOKEN=$(cat ~/.config/rmhub/admin-token)
   $ rmhub admin user add --email ada@example.org --name Ada
   Created account 3x7k2m4q5r6s7t2u3v4w5x6y7z for ada@example.org
   $ rmhub admin token issue 3x7k2m4q5r6s7t2u3v4w5x6y7z --label laptop
   rmh_3x7k2m4q5r6s7t2u3v4w5x6y7z_…
   ```

   The server does not keep the token text. Store it now.

4. Use the owner token to make a pairing code, and type the code on the
   tablet within 5 minutes:

   ```console
   $ export RMHUB_TOKEN=rmh_3x7k2m4q5r6s7t2u3v4w5x6y7z_…
   $ rmhub pair
   abcdefgh  (expires 2026-11-14T12:05:00Z)
   ```

   The tablet must use the tablet listener as its cloud host.

## Client

The `rmhub` command is also the client of the rmhub API. It reads the
server URL from `--url`, `RMHUB_URL` or `url` in
`$XDG_CONFIG_HOME/rmhub/client.toml`, in that order, and the token from
`--token`, `RMHUB_TOKEN` or `token` in `client.toml`:

```toml
# ~/.config/rmhub/client.toml (mode 600)
url = "http://127.0.0.1:8081"
token = "rmh_…"
```

| Command | Purpose |
|---|---|
| `rmhub admin user add --email <email> [--name <name>]` | Create an account (admin token). |
| `rmhub admin user list` | List the accounts (admin token). |
| `rmhub admin token issue <account> [--label <l>] [--expires <t>]` | Issue an owner token (admin token). |
| `rmhub admin token generate` | Print a new admin token. Needs no server. |
| `rmhub account` | Show the account of the token. |
| `rmhub pair` | Make a pairing code. |
| `rmhub devices list`, `rmhub devices remove <id>` | List or remove paired devices. |
| `rmhub tokens list`, `rmhub tokens create`, `rmhub tokens revoke <id>` | Manage the owner tokens. |

Each command prints text by default and one JSON value with `--json`.
Errors go to standard error. The exit codes are: 0 success, 1 unexpected
error, 2 usage or setting error, 3 status 401 or 403, 4 status 404, 5
status 409 or 412. `rmhub <command> --help` shows the options, the JSON
output form and an example.

## Development

The repository is a Nix flake that uses
[blueprint](https://github.com/numtide/blueprint) and
[treefmt-nix](https://github.com/numtide/treefmt-nix).

```console
nix develop          # Go toolchain, gopls, golangci-lint
nix build .#rmhub    # build the server
nix fmt              # format Nix and Go code
nix flake check      # run the formatter, lint and S3 contract checks
```

The S3 contract tests run when `RMHUB_TEST_S3_ENDPOINT` is set. The
check `s3-contract` runs them against versitygw in the build sandbox.
Other variables: `RMHUB_TEST_S3_BUCKET`, `RMHUB_TEST_S3_ACCESS_KEY_ID`,
`RMHUB_TEST_S3_SECRET_ACCESS_KEY` and `RMHUB_TEST_S3_REGION`.

```console
go test ./...
```

With direnv, run `direnv allow` once to load the devshell.

## Specifications

Changes go through [OpenSpec](https://github.com/Fission-AI/OpenSpec).
The specs are in `openspec/specs/`. The proposed changes are in
`openspec/changes/`.

## Licence

rmhub is free software under the GNU Affero General Public License,
version 3 or later. See `LICENSE`.
