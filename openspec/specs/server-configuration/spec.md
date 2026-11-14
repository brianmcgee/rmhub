# server-configuration Specification

## Purpose
`rmhub serve` runs the server. Its configuration selects the storage
backends and the listeners, and the server checks the configuration and
the storage before it accepts requests.

## Requirements

### Requirement: Configuration file
`rmhub serve` MUST read a TOML configuration file. The `--config` option
MUST give the path. Without `--config`, the server MUST read
`$XDG_CONFIG_HOME/rmhub/config.toml`, with `~/.config` when
`XDG_CONFIG_HOME` is not set. If that default file does not exist, the
server MUST use the default values.

#### Scenario: Explicit path
- **WHEN** an operator starts `rmhub serve --config /etc/rmhub/config.toml`
- **THEN** the server reads `/etc/rmhub/config.toml`

#### Scenario: Explicit path is missing
- **WHEN** an operator starts `rmhub serve --config /missing.toml`
- **THEN** the server stops with an error that names `/missing.toml`

#### Scenario: No configuration file
- **WHEN** an operator starts `rmhub serve` and no default file exists
- **THEN** the server uses the default values

### Requirement: Configuration errors
The server MUST stop before it opens a listener if the configuration has
an error. These are errors: an unknown key, a value of the wrong type and
an unknown backend name. The error message MUST name the key.

#### Scenario: Unknown key
- **WHEN** the configuration contains the key `blob.dirr`
- **THEN** the server stops with an error that names `blob.dirr`

### Requirement: Backend selection
The configuration MUST select the blob backend and the state backend
separately. The blob backend MUST be `fs` or `s3`. The state backend MUST
be `sqlite` or `s3`. The default MUST be `fs` and `sqlite` under the data
directory. The data directory MUST default to `$XDG_DATA_HOME/rmhub`,
with `~/.local/share` when `XDG_DATA_HOME` is not set.

#### Scenario: Default backends
- **WHEN** the configuration does not select a backend
- **THEN** the server keeps blobs in `$XDG_DATA_HOME/rmhub/blobs`
- **AND** the server keeps state in `$XDG_DATA_HOME/rmhub/state.sqlite`

#### Scenario: Blobs on S3 and state in SQLite
- **WHEN** the configuration selects `s3` for blobs and `sqlite` for state
- **THEN** the server writes blobs to the bucket and state to the SQLite file

### Requirement: S3 settings
An S3 backend MUST take an endpoint URL, a region, a bucket and a key
prefix. The blob store and the state store MUST be able to share one
bucket with different prefixes. The secret access key MUST come from a
file that the configuration names, or from the standard AWS credential
sources. The configuration file MUST NOT contain the secret itself.

#### Scenario: Secret from a file
- **WHEN** the configuration sets `secret_access_key_file = "/run/credentials/rmhub/s3-secret"`
- **THEN** the server reads the secret from that file

#### Scenario: Secret in the configuration
- **WHEN** the configuration contains a key `secret_access_key`
- **THEN** the server stops with an error that names `secret_access_key`

### Requirement: Conditional write check at startup
At startup, the server MUST check that the state backend refuses a stale
conditional write. The check MUST use a key under `server/probe/` and
MUST delete the key after the check. If the backend accepts a stale write,
the server MUST stop before it opens a listener.

#### Scenario: Backend enforces conditional writes
- **WHEN** the server starts with a backend that refuses stale writes
- **THEN** the server opens its listeners
- **AND** no key under `server/probe/` remains

#### Scenario: Backend ignores conditions
- **WHEN** the server starts with an S3 endpoint that accepts a put-if-absent on an existing key
- **THEN** the server stops with an error that says that the backend does not enforce conditional writes

### Requirement: Two listeners
The server MUST open a tablet listener and an API listener on separate
addresses. Each listener MUST accept a TLS certificate and key, or serve
plain HTTP when none is set. The defaults MUST be `127.0.0.1:8080` for the
tablet listener and `127.0.0.1:8081` for the API listener.

#### Scenario: Default addresses
- **WHEN** the configuration does not set a listener address
- **THEN** the server listens on `127.0.0.1:8080` and `127.0.0.1:8081`

#### Scenario: Address in use
- **WHEN** the tablet listener address is in use by another process
- **THEN** the server stops with an error that names the address

### Requirement: Health endpoint
Each listener MUST answer `GET /healthz` with status 200 when the server
accepts requests.

#### Scenario: Server is ready
- **WHEN** a client sends `GET /healthz` to the API listener after startup
- **THEN** the server answers with status 200

### Requirement: Graceful shutdown
On SIGTERM or SIGINT, the server MUST stop accepting new connections. It
MUST let requests in progress finish for up to 10 seconds, and then exit
with status 0.

#### Scenario: Shutdown during a request
- **WHEN** the server gets SIGTERM while a request is in progress
- **THEN** the request completes
- **AND** the server exits with status 0
