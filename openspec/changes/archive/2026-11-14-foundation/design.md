# Design

## Context

The repository has a Go module, an empty `cmd/rmhub` and a flake with a
lint check. The package builds with `CGO_ENABLED=0`. The specs of this
change give the contracts. This document gives the structure, the
libraries and the test setup.

All later changes depend on one fact: the state store gives single-key
conditional writes and nothing more. The root log, the accounts and the
tokens must each fit that model.

## Goals / Non-Goals

**Goals:**

- Interfaces for the blob store and the state store, with two backends
  each.
- One contract test suite per store that runs against every backend.
- `rmhub serve` with configuration, startup checks, two listeners and a
  health endpoint.

**Non-Goals:**

- Blob delete and blob listing. Garbage collection needs them. It comes
  in a later change.
- A PostgreSQL backend. The state interface allows one later.
- Any reMarkable protocol endpoint or rmhub API endpoint.
- A NixOS module.

## Decisions

### Package layout

```
cmd/rmhub/            main, subcommand dispatch
internal/blob/        Store interface, errors
internal/blob/fs/     file system backend
internal/blob/s3/     S3 backend
internal/blob/blobtest/   contract suite
internal/state/       Store interface, errors
internal/state/sqlite/
internal/state/s3/
internal/state/statetest/
internal/s3conn/      shared S3 client setup
internal/config/      TOML loading and defaults
internal/server/      listeners, health, shutdown, startup probe
```

The contract suites are ordinary Go packages. Each backend test calls
`blobtest.Run(t, newStore)` or `statetest.Run(t, newStore)`.

### Interfaces

```go
// blob
Put(ctx, ns Namespace, key string, r io.Reader) error   // first write wins
Get(ctx, ns Namespace, key string) (io.ReadCloser, int64, error)
Exists(ctx, ns Namespace, keys []string) (map[string]bool, error)

// state
Get(ctx, key string) (value []byte, version string, err error)
PutIfAbsent(ctx, key string, value []byte) (version string, err error)
PutIfMatch(ctx, key string, value []byte, version string) (string, error)
Delete(ctx, key string) error
List(ctx, prefix, after string, limit int) ([]string, error)
```

The errors are sentinel values: `ErrNotFound`, `ErrConflict` and
`ErrInvalidKey`. Each other error means an unknown outcome. Callers that
need to resolve an unknown outcome read the key again.

`Delete` has no condition. AWS, R2, versitygw and rustfs support
`If-Match` on DeleteObject, but no planned caller needs it. An
unconditional delete keeps the contract smaller.

### Blob namespaces

All blob keys are content hashes. In one shared namespace, a client that
knows a hash could read the blob of another account. It could also learn
that another account has a file, from `check-files` or from a successful
read. So each account has its own namespace. A blob that two accounts
upload is stored two times. Deduplication across accounts has little
value for a small number of users.

`Namespace` is a type with an unexported field. The only constructor
checks the account ID format. So each call names an account, and a
caller cannot pass a string from a request by mistake. The blob store
checks the namespace again before each storage access.

### Blob store on the file system

The layout is `<dir>/<ns>/<k[0:2]>/<k>`, so that no directory holds all
blobs of an account.
A write goes to a temporary file in `<dir>/tmp/`. Then the backend calls
`fsync` on the file and uses `link(2)` to create the final name. `link`
fails with `EEXIST` when the name exists. That gives first-write-wins
without a lock. The backend then removes the temporary file and calls
`fsync` on the directory. `rename(2)` is not correct here, because it
replaces an existing name.

### Blob store on S3

The object name is `<prefix><ns>/<key>`. A write uses `PutObject` with
`If-None-Match: *`. Status 412 means that the blob exists, so the backend
reports success. Status 409 (`ConditionalRequestConflict`) means a
parallel write. The backend retries the write once. After the retry it
reports success if the object exists. `Exists` uses `HeadObject` for
each key, with at most 16 requests in parallel.

### State store on SQLite

The library is `modernc.org/sqlite`. It needs no cgo. The schema is one
table:

```sql
CREATE TABLE kv (key TEXT PRIMARY KEY, value BLOB NOT NULL, version TEXT NOT NULL);
```

A version is the lowercase hex SHA-256 of the value. That matches the
S3 behavior, where two writes of the same bytes can give the same ETag,
so both backends act the same. `PutIfMatch` runs
`UPDATE kv SET value=?, version=? WHERE key=? AND version=?`. Zero
changed rows means conflict. `PutIfAbsent` runs `INSERT`, and a
primary-key violation means conflict. The database uses WAL mode and
`busy_timeout`.

The alternative was `ncruces/go-sqlite3`, which runs SQLite in wazero.
modernc is in wider use. Both are acceptable.

### State store on S3

The object name is `<prefix><key>`. The version is the ETag.
`PutIfAbsent` uses `If-None-Match: *`. `PutIfMatch` uses `If-Match`.
Status 412 means conflict. Status 404 on `If-Match` also means conflict:
versitygw and rustfs answer 404 for a missing key, SeaweedFS answers 412.
Status 409 is retried once. If the retry also gets 409, the backend
returns failure, not conflict, because the outcome is not known. `List`
uses `ListObjectsV2` with `StartAfter`.

The library is `aws-sdk-go-v2`. Its `PutObjectInput` has `IfMatch` and
`IfNoneMatch`. R2 needs its S3 endpoint
(`<account>.r2.cloudflarestorage.com`) and path-style addressing. The
configuration exposes both settings.

### Startup probe

The server writes `server/probe/<random>` with `PutIfAbsent`. Then it
does `PutIfAbsent` again and `PutIfMatch` with a wrong version. Both must
return conflict. Then it deletes the key. This finds an S3 service that
ignores the condition headers. A service that ignores them returns 200
for both.

### Configuration

The format is TOML, read with `BurntSushi/toml`. Unknown keys are an
error: the loader checks `MetaData.Undecoded()`. NixOS can generate TOML
with `pkgs.formats.toml`. Example:

```toml
data_dir = "/var/lib/rmhub"

[blob]
backend = "s3"
s3.endpoint = "https://<account>.r2.cloudflarestorage.com"
s3.region = "auto"
s3.bucket = "rmhub"
s3.prefix = "blobs/"
s3.access_key_id = "…"
s3.secret_access_key_file = "/run/credentials/rmhub.service/s3"

[state]
backend = "sqlite"

[listen.tablet]
address = "127.0.0.1:8080"

[listen.api]
address = "127.0.0.1:8081"
tls_cert_file = "/run/credentials/rmhub.service/api.crt"
tls_key_file = "/run/credentials/rmhub.service/api.key"
```

When `access_key_id` is not set, the S3 client uses the standard AWS
credential chain.

### Server

The HTTP routing is `net/http.ServeMux` with method patterns. No
framework is necessary. Each listener has its own mux, so that a tablet
route cannot appear on the API listener by mistake. Logs go to stderr
through `log/slog`.

### CI with versitygw

The contract tests for S3 read `RMHUB_TEST_S3_ENDPOINT`. Without it they
skip. A flake check `checks/s3-contract.nix` starts versitygw with its
posix backend on a loopback port in the build sandbox. Then it runs the
S3 contract tests.

Tests of the candidates on 2026-11-14 used curl with SigV4 and 32
parallel writers:

| Server | Conditional put/delete | 32 parallel writers |
|---|---|---|
| versitygw 1.10.0 | correct | exactly one winner, in 9 of 9 runs |
| rustfs 1.0.4 | correct | exactly one winner, in 7 of 7 runs |
| SeaweedFS 4.48 | correct in the first run | status 500 for every request in later runs |
| MinIO | not tested | removed from nixpkgs |

versitygw is a Go program with an Apache-2.0 licence and a small
footprint. rustfs is the fallback.

## Risks / Trade-offs

- [A local S3 server is not AWS or R2] → The startup probe runs against
  the real backend at each start. The design notes the known differences
  between services (404 or 412 on a missing key).
- [The 409 retry can hide a parallel write] → Only the blob store treats
  a 409 as success after the retry, and only when the object exists.
  Blobs are immutable, so any winner is correct. The state store reports
  failure.
- [SQLite versions are content hashes] → Two writes of the same bytes
  have the same version. The spec permits this. Callers that need
  distinct versions put a write ID in the value.
- [One HeadObject per key in `Exists`] → This is acceptable at personal
  scale. A later change can use a listing if a sync checks many keys.

## Implementation Notes

Decisions made during the implementation, for review:

- The S3 state backend turns off the retries of the AWS SDK for each
  conditional write. An SDK retry after a lost answer gets status 412
  from the first attempt of the same request, and the backend would
  report a conflict for a write that succeeded.
- The S3 blob backend copies a body that cannot seek into a temporary
  file in `<data_dir>/tmp` before the upload. The SigV4 signature and
  the 409 retry need a seekable body, and a source that fails part way
  never reaches the bucket.
- The configuration has three keys that the spec does not name:
  `blob.dir` (the spec example `blob.dirr` implies it), `state.path` and
  `s3.path_style` (the design says that R2 needs path-style addressing).
- The two listener addresses must differ. A listener with only one of
  `tls_cert_file` and `tls_key_file` is a configuration error.
- versitygw with its posix backend cannot keep a key `a` and a key
  `a/1` at the same time. The state key layouts of the later changes
  never use a key that is also the prefix of another key followed by
  `/`. The contract suite follows the same rule.
- The build sandbox has no extended attributes, so the check
  `s3-contract` starts versitygw with `--sidecar`.
- SQLite runs with `synchronous=FULL`, so that a commit survives a
  power loss.
- `rmhub serve` uses the standard `flag` package. The change `accounts`
  moves it into cobra.
