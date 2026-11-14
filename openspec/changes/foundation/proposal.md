# Proposal

## Why

Each later feature of rmhub keeps documents and account state in storage.
The storage must work on a local disk and on S3-compatible object stores
such as AWS S3 and Cloudflare R2. rmhub must stay correct when two devices
write at the same time. This change sets the storage contracts and the
server process that all later changes use.

## What Changes

- Add a blob store. It keeps immutable blobs under a SHA-256 key. It has a
  file system backend and an S3 backend.
- Add a state store. It keeps small mutable values under string keys. Each
  write is conditional: put-if-absent or put-if-match. It has a SQLite
  backend and an S3 backend. It offers no multi-key transactions, because
  S3 cannot give them.
- Add one contract test suite that each backend must pass. The suite runs
  against a local S3 server in CI. Tests with versitygw showed correct
  conditional writes under 32 parallel writers.
- At startup, check that the state backend refuses a stale conditional
  write. If it accepts the write, stop with an error.
- Add the `rmhub serve` command. It reads a TOML configuration file,
  opens the configured stores, and serves two HTTP listeners: one for the
  tablet protocol and one for the rmhub API. A separate tablet listener
  lets a later change add mutual TLS.

## Capabilities

### New Capabilities

- `blob-store`: immutable, content-addressed blobs with atomic writes, on a
  file system or S3.
- `state-store`: small mutable values with versioned, conditional writes
  and prefix listing, on SQLite or S3.
- `server-configuration`: the configuration file, the choice of backends,
  the startup checks and the two listeners of `rmhub serve`.

### Modified Capabilities

None.

## Impact

- New Go packages for the stores, their backends and the server command.
- New dependencies: `aws-sdk-go-v2` (Apache-2.0), `modernc.org/sqlite`
  (BSD-3-Clause, no cgo), `BurntSushi/toml` (MIT).
- The flake gets a check that runs the contract suite against versitygw
  from nixpkgs. MinIO is not an option: nixpkgs removed it as abandoned.
- The package `vendorHash` changes from `null` to a real hash.
- No machine or clan service is affected. Deployment comes in a later
  change.
