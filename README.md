# rmhub

rmhub is a self-hosted alternative to the reMarkable cloud service.
It syncs documents between reMarkable tablets and the desktop and mobile apps.
You keep your notes on a server that you control.

## Status

Early development. The server has accounts, API tokens, tablet pairing
and Sync 1.5 for tablets, with change notifications. It has a library
API with scoped tokens and a change feed for scripts and agents. It does
not render documents yet.

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
| `tablet.announced_host` | `local.appspot.com` | Host that discovery gives to the tablet. No scheme, no path. |
| `tablet.check_files` | `truthful` | Answer of `check-files`: `truthful` or `none`. |
| `api.max_upload_bytes` | `268435456` | Maximum size of a document upload through the API, in bytes. |
| `render.cache_dir` | `$XDG_CACHE_HOME/rmhub/render` | Directory of the page render cache. Without `XDG_CACHE_HOME`, `~/.cache` is used. |
| `render.cache_max_bytes` | `1073741824` | Maximum size of the render cache, in bytes. The oldest files are deleted first. |

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

   The tablet must use the tablet listener as its cloud host. See
   [Connect a tablet](#connect-a-tablet).

## Connect a tablet

The tablet talks to the reMarkable cloud over HTTPS. To send it to rmhub,
install [rmfakecloud-proxy](https://github.com/ddvk/rmfakecloud-proxy) on
the tablet. The installer of the proxy adds a CA certificate and host
entries, so that the cloud host names go to the proxy on the tablet. Set
the upstream URL of the proxy (the installer asks for it, or the
`upstream` key of its configuration file) to the tablet listener of
rmhub, for example:

```yaml
# configuration file of rmfakecloud-proxy on the tablet
upstream: https://rmhub.example.org:8443
```

Put the tablet listener behind TLS (`listen.tablet.tls_cert_file`) or a
TLS reverse proxy when the network between the tablet and the server is
not trusted.

The discovery endpoints tell the tablet which host to use for sync and
notifications. `tablet.announced_host` sets that host. The default is
`local.appspot.com`. The host must resolve to the proxy on the tablet;
check the host entries that the proxy installer adds, and set this key
if your installer uses a different name. The value is a host name with
an optional port, with no scheme and no path.

```toml
[tablet]
announced_host = "local.appspot.com"
check_files = "truthful"
```

`tablet.check_files` controls the answer to `POST /sync/v3/check-files`:
`truthful` names the blobs that the server does not have, `none` gives
`{"missingFiles":null}` like the reMarkable cloud. Use `none` only if a
tablet behaves badly with the truthful answer.

The server announces a schema version for root indexes to the tablet.
The default is 3. Set it for each account:

```console
$ rmhub account set-schema 4
```

The server accepts root indexes of schema 3 and 4 for each setting.

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
| `rmhub tokens list`, `rmhub tokens create [--grant <entry>:<read\|write>]`, `rmhub tokens revoke <id>` | Manage owner tokens and scoped tokens. |
| `rmhub account set-schema <3\|4>` | Set the root index schema that the server announces to the tablets. |
| `rmhub ls [entry]`, `rmhub stat <entry>`, `rmhub find <text>` | Read the library. |
| `rmhub get <entry> [-o <file>] [--archive]` | Download a PDF, an EPUB or the archive of a document. |
| `rmhub upload <file> [--to <entry>] [--name <name>]` | Upload a PDF or an EPUB. |
| `rmhub mkdir`, `rmhub mv`, `rmhub rename`, `rmhub trash` | Change the library. Each change gets to the tablets at their next sync. |
| `rmhub watch [--after <generation>]` | Print the changes of the library, one JSON line for each generation. |

Each command prints text by default and one JSON value with `--json`.
Errors go to standard error. The exit codes are: 0 success, 1 unexpected
error, 2 usage or setting error, 3 status 401 or 403, 4 status 404, 5
status 409 or 412. `rmhub <command> --help` shows the options, the JSON
output form and an example.

### Library and agents

An `<entry>` argument is an ID or a path. A path starts with `/`, for
example `/Work/Paper`. Two entries can have the same path; then the
command exits with code 5 and prints the IDs. Use the ID after that.

A scoped token gives a program or an AI agent access to some folders
only:

```sh
rmhub tokens create --label agent --grant /Agent:write --grant /Reference:read --expires 720h
```

The token sees only the grant folders and their descendants. It cannot
pair tablets, remove devices or change the account. It can create
tokens with the same or fewer rights, and revoke them. Revocation of a
token also revokes each token that it created.

`docs/agents/SKILL.md` is a guide for AI agents. Give it to an agent
together with a scoped token.

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
