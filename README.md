# rmhub

rmhub is a self-hosted alternative to the reMarkable cloud service.
It syncs documents between reMarkable tablets and the desktop and mobile apps.
You keep your notes on a server that you control.

## Status

Early development. The server has accounts, API tokens, tablet pairing
and Sync 1.5 for tablets, with change notifications. It has a library
API with scoped tokens and a change feed for scripts and agents. It
renders notebook pages and annotated PDF pages as images. It does not
render EPUB pages.

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
| `tablet.max_blob_bytes` | `268435456` | Maximum size of one file that a tablet syncs, in bytes. The tablet cannot sync a larger document. |
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

## Deploy on NixOS

The flake exports the NixOS module `nixosModules.rmhub`. The file
[`examples/nixos.nix`](examples/nixos.nix) is a full host configuration:
TLS on the tablet listener with a certificate from Let's Encrypt, and the
admin token from a secret file. The VM test `checks/nixos-module.nix`
imports that file, so the example stays correct. Import the module and
the example (or your copy of it) in a host:

```nix
{
  inputs.rmhub.url = "github:brianmcgee/rmhub";
  # in the host configuration:
  #   imports = [ inputs.rmhub.nixosModules.rmhub ./rmhub.nix ];
}
```

| Option | Default | Meaning |
| --- | --- | --- |
| `services.rmhub.enable` | `false` | Run `rmhub serve` as `rmhub.service`. |
| `services.rmhub.package` | the flake package | The rmhub package. |
| `services.rmhub.settings` | `{ }` | The TOML configuration (see [Configuration keys](#configuration-keys)). The module sets `data_dir = "/var/lib/rmhub"` and `render.cache_dir = "/var/cache/rmhub/render"`. |
| `services.rmhub.credentials` | `{ }` | Secret files, from a name to a path on the host. Refer to a file in `settings` as `/run/credentials/rmhub.service/<name>`. |
| `services.rmhub.openFirewall` | `false` | Open the TCP ports of both listeners. |

Do not put a secret in `settings`: the settings go into the Nix store.
The secret files can be readable by root only, because systemd reads
them before the service user exists. The service runs as a dynamic user
with systemd hardening. It does not set `MemoryDenyWriteExecute`,
because the PDF renderer compiles WebAssembly to machine code.

### TLS for the tablet

rmfakecloud-proxy on the tablet checks the certificate of its upstream
against the trusted roots of the tablet. A self-signed certificate
fails, unless you add its CA to the tablet. Use one of these setups.

**A public certificate on the tablet listener.** `examples/nixos.nix`
gets a certificate through `security.acme` and gives it to rmhub as the
credentials `tls-cert` and `tls-key`. systemd copies credentials at
service start, so rmhub must restart after each renewal:

```nix
security.acme.certs."rmhub.example.org".reloadServices = [ "rmhub.service" ];
```

The tablet loses its notification connection at the restart and
connects again.

**A reverse proxy in front of a plain tablet listener.** Keep the tablet
listener on the loopback interface without TLS, and let nginx terminate
TLS. The proxy must pass websocket upgrades (the tablet gets change
notifications through a websocket) and must allow bodies of at least
256 MiB:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl;
    server_name rmhub.example.org;
    ssl_certificate     /var/lib/acme/rmhub.example.org/fullchain.pem;
    ssl_certificate_key /var/lib/acme/rmhub.example.org/key.pem;

    client_max_body_size 256m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 1h;
        proxy_request_buffering off;
    }
}
```

With this setup, the `upstream` of the proxy on the tablet is
`https://rmhub.example.org` (port 443).

## Switch from rmfakecloud

The switch changes one setting on the tablet. rmhub does not import data
from rmfakecloud: the tablet uploads its library at the first sync. Keep
rmfakecloud and its data unchanged until you are sure that you stay with
rmhub.

1. **Back up the library.** Download each document from rmfakecloud with
   rmapi, with a separate rmapi configuration:

   ```console
   $ export RMAPI_HOST=https://rmfakecloud.example.org
   $ export RMAPI_CONFIG=$HOME/.config/rmapi/rmfakecloud.conf
   $ rmapi ls /
   $ rmapi mget -o $HOME/rm-backup /
   ```

   The first `rmapi` command asks for a pairing code from rmfakecloud.

2. **Create the rmhub user** and an owner token, as in
   [First run](#first-run) step 3.

3. **Create a pairing code** with the owner token:

   ```console
   $ rmhub pair
   ```

   The code expires after 5 minutes. Do steps 4 to 6 in that time, or
   make a new code.

4. **Change the upstream of the proxy** on the tablet to rmhub. Write the
   old value down:

   ```yaml
   # configuration file of rmfakecloud-proxy on the tablet
   upstream: https://rmhub.example.org:8443
   ```

5. **Restart the proxy** on the tablet:

   ```console
   # systemctl restart proxy
   ```

6. **Pair the tablet with the code.** The old device token of the tablet
   is from rmfakecloud, and rmhub refuses it. If the tablet does not ask
   for a code, disconnect it from the cloud in its settings, then connect
   again and type the code.

7. **Check the sync.** The first sync uploads the full library from the
   tablet. This takes time for a large library. Then the library shows
   on rmhub:

   ```console
   $ rmhub ls /
   ```

### Way back

To go back to rmfakecloud, set `upstream` in the proxy configuration on
the tablet to its old value and restart the proxy. Pair again with a
code from rmfakecloud if the tablet asks.

**Warning:** changes that you make while the tablet uses rmhub are not
in rmfakecloud. They get there only when the tablet syncs with
rmfakecloud again. A change that an API client made on rmhub and that
the tablet did not sync is lost. Do not delete the rmfakecloud data
before you are sure that you stay with rmhub.

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
| `rmhub pages <entry>` | List the pages of a document. |
| `rmhub render <entry> --page <n> [--format png\|svg] [--width <px>] [-o <file>]` | Render a page of a notebook or a PDF as an image. |
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
