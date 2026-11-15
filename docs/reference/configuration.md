# Server configuration

## `rmhub serve`

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

## Configuration keys

| Key | Default | Meaning |
|---|---|---|
| `data_dir` | `$XDG_DATA_HOME/rmhub` | Directory for local data and temporary files. |
| `blob.backend` | `fs` | Blob backend: `fs` or `s3`. |
| `blob.dir` | `<data_dir>/blobs` | Directory of the `fs` blob backend. |
| `blob.s3.*` | | S3 settings of the blob backend (see [S3 settings](#s3-settings)). |
| `state.backend` | `sqlite` | State backend: `sqlite` or `s3`. |
| `state.path` | `<data_dir>/state.sqlite` | File of the `sqlite` state backend. |
| `state.s3.*` | | S3 settings of the state backend (see [S3 settings](#s3-settings)). |
| `listen.tablet.address` | `127.0.0.1:8080` | Address of the tablet listener. |
| `listen.api.address` | `127.0.0.1:8081` | Address of the API listener. |
| `listen.<name>.tls_cert_file` | | TLS certificate. Set it with `tls_key_file`, or set neither for plain HTTP. |
| `listen.<name>.tls_key_file` | | TLS private key. |
| `listen.<name>.trusted_proxies` | `[]` | Addresses and CIDR networks of the reverse proxies in front of the listener. The server reads the client address from `X-Forwarded-For` only if the connection comes from one of them. |
| `admin.token_file` | | File with the admin token. Without it, each admin request gets status 401. |
| `tablet.announced_host` | `local.appspot.com` | Host that discovery gives to the tablet. A host name with an optional port, with no scheme and no path. |
| `tablet.check_files` | `truthful` | Answer of `POST /sync/v3/check-files`. `truthful` names the blobs that the server does not have. `none` gives `{"missingFiles":null}`, like the reMarkable cloud. |
| `tablet.max_blob_bytes` | `268435456` | Maximum size of one file that a tablet syncs, in bytes. The tablet cannot sync a larger document. |
| `api.max_upload_bytes` | `268435456` | Maximum size of a document upload through the API, in bytes. |
| `render.cache_dir` | `$XDG_CACHE_HOME/rmhub/render` | Directory of the page render cache. Without `XDG_CACHE_HOME`, `~/.cache` is used. |
| `render.cache_max_bytes` | `1073741824` | Maximum size of the render cache, in bytes. The oldest files are deleted first. |
| `render.max_workers` | CPU threads, at most `4` | Maximum number of page renders at the same time. More requests wait. Each worker can use up to 1 GiB of memory for a PDF page. Thus this value multiplied by 1 GiB is the memory limit of the PDF renders. A PDF that PDFium cannot draw in 60 seconds gets status 409. A page whose strokes need too much drawing work gets status 400. |
| `limits.pairing_per_minute` | `10` | See [Request limits](#request-limits). |
| `limits.api_per_minute` | `600` | See [Request limits](#request-limits). |
| `limits.render_per_minute` | `60` | See [Request limits](#request-limits). |

## S3 settings

The keys below are under `blob.s3` and under `state.s3`.

| Key | Default | Meaning |
|---|---|---|
| `endpoint` | | URL of the S3 service. |
| `region` | `us-east-1` | Region of the bucket. |
| `bucket` | | Name of the bucket. |
| `prefix` | | Prefix of each object key. |
| `path_style` | | Use path-style addresses. |
| `access_key_id` | | Access key ID. Without it, the server uses the standard AWS credential sources. |
| `secret_access_key_file` | | File with the secret access key. |

The secret comes from the file. The configuration must not contain the
secret: the key `secret_access_key` is an error. The blob store and the
state store can share one bucket with different prefixes.

## Request limits

Each limit is a number of requests in one minute. A client can use the
full number at one time. After that, the number fills again at a
constant rate.

| Key | Default | Limit |
|---|---|---|
| `limits.pairing_per_minute` | `10` | Pairing attempts of one client address. Each attempt counts, with a valid code or not. All addresses of one IPv6 /64 network count as one address. |
| `limits.api_per_minute` | `600` | API requests of one token. A scoped token and its parent token have separate limits. |
| `limits.render_per_minute` | `60` | Page renders of one account, for all its tokens together. A page that is in the render cache does not count. |

The value `-1` turns a limit off. The value `0` is an error.

Above a limit, the server answers with status 429 and a `Retry-After`
header with the wait time in seconds. The API gives the error code
`rate_limited`, and the client exits with code 6. The request did
nothing.

[Request limits](../explanation/request-limits.md) explains what the
limits do not cover.
