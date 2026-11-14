# Tasks

## 1. Blob store and file system backend

- [ ] 1.1 Write the `internal/blob` interface with `ErrNotFound`, `ErrInvalidKey` and the `Namespace` type. Make it refuse invalid keys and invalid namespaces. Verify with a unit test that refuses `../accounts/x`, uppercase keys and the namespace `../tmp`.
- [ ] 1.2 Write the `internal/blob/blobtest` contract suite. Cover each scenario of the `blob-store` spec, including 32 parallel writes of one key, a source that fails part way and one key in two namespaces.
- [ ] 1.3 Write the `internal/blob/fs` backend with a temporary file, `fsync` and `link(2)`. Verify that `go test ./internal/blob/...` passes the contract suite.

## 2. State store and SQLite backend

- [ ] 2.1 Write the `internal/state` interface with `ErrNotFound`, `ErrConflict` and `ErrInvalidKey`. Make it refuse invalid keys. Verify with a unit test for each invalid key form in the `state-store` spec.
- [ ] 2.2 Write the `internal/state/statetest` contract suite. Cover each scenario of the `state-store` spec, including 32 parallel put-if-absent and put-if-match writes.
- [ ] 2.3 Add `modernc.org/sqlite`. Write the `internal/state/sqlite` backend with SHA-256 versions. Verify that `go test ./internal/state/...` passes the contract suite.
- [ ] 2.4 Set the real `vendorHash` in `packages/rmhub/default.nix`. Verify that `nix build .#rmhub` succeeds.

## 3. S3 backends

- [ ] 3.1 Add `aws-sdk-go-v2`. Write `internal/s3conn` with endpoint, region, path-style addressing, an access key ID and a secret from a file or the default credential chain. Verify with a unit test that reads the secret from a file.
- [ ] 3.2 Write the `internal/blob/s3` backend with `If-None-Match: *` and the 409 retry. Make its test run `blobtest` when `RMHUB_TEST_S3_ENDPOINT` is set and skip when it is not set.
- [ ] 3.3 Write the `internal/state/s3` backend with `If-None-Match`, `If-Match` and `StartAfter`. Map status 412 and status 404 on `If-Match` to `ErrConflict`. Make its test run `statetest` when `RMHUB_TEST_S3_ENDPOINT` is set.
- [ ] 3.4 Add `checks/s3-contract.nix`. It starts versitygw on a loopback port in the sandbox and runs the S3 contract tests. Verify that `nix build .#checks.x86_64-linux.s3-contract` succeeds.
- [ ] 3.5 Make the check fail when a backend ignores conditions. Run it once with the `If-None-Match` header removed and verify that the check fails. Then restore the header.

## 4. Configuration

- [ ] 4.1 Add `BurntSushi/toml`. Write `internal/config` with the defaults, the XDG paths, the backend choice, the S3 settings and the two listeners. Verify with unit tests for the default values and for an explicit `--config` path.
- [ ] 4.2 Refuse unknown keys, wrong types, unknown backends and the key `secret_access_key`. Verify with a unit test for each error that the error text names the key.

## 5. Server

- [ ] 5.1 Write the startup probe in `internal/server`. Verify with a unit test that it accepts the SQLite backend and refuses a fake backend that accepts a stale write. Verify that no `server/probe/` key remains.
- [ ] 5.2 Write the two listeners with optional TLS, `GET /healthz` and the graceful shutdown. Verify with a test that a request in progress completes after SIGTERM and that the exit status is 0.
- [ ] 5.3 Add the `serve` subcommand to `cmd/rmhub`. Verify that `rmhub serve` with no configuration file answers `GET /healthz` on `127.0.0.1:8081` with status 200.
- [ ] 5.4 Document `rmhub serve` and the configuration keys in `README.md`. Verify that the documented example configuration starts the server.

## 6. Integration

- [ ] 6.1 Run `nix fmt` and verify that no file changes.
- [ ] 6.2 Run `go test ./...` and verify that all tests pass.
- [ ] 6.3 Run `nix flake check` and verify that all checks pass, including `s3-contract`.
