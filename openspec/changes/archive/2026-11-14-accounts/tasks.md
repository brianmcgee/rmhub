# Tasks

## 1. API base

- [x] 1.1 Write `internal/api` with the `/api/v1/` mux on the API listener, the JSON body decoder and the error format. Verify with tests that a body that is not JSON gets status 400 and that each error code has its status.
- [x] 1.2 Write the bearer token middleware with an interface for token kinds. Verify with tests that a missing, malformed or unknown token gets status 401.
- [x] 1.3 Verify with a test that `GET /api/v1/account/devices` on the tablet listener gets status 404.

## 2. Admin token and accounts

- [x] 2.1 Add `admin.token_file` to the configuration. Read and check the token at startup. Verify with tests for a valid token, a missing file and a short token.
- [x] 2.2 Write `internal/accounts` with the email claim and the account record. Verify with tests for a new user, an email in use without regard to case, and a claim whose account does not exist.
- [x] 2.3 Write a test with two parallel creates for one email. Verify that exactly one create succeeds.
- [x] 2.4 Add the admin endpoints for users. Verify with tests that an owner token gets status 403 and that a missing user gets status 404.

## 3. Owner tokens

- [x] 3.1 Write `internal/tokens` with the token format, SHA-256 secret hashes, constant-time comparison and expiry. Verify with tests for an expired token and for a token with a wrong secret.
- [x] 3.2 Add the endpoints to issue, create, list and revoke tokens, and `GET /api/v1/account`. Verify with tests that a list contains no secret and that a revoked token gets status 401. Verify that a token ID of another account gets status 404.

## 4. Pairing and tablet tokens

- [x] 4.1 Write the JWT signing key setup with put-if-absent. Verify with a test that two server instances on one state store get the same key.
- [x] 4.2 Write the pairing code endpoint and the code use with put-if-match. Verify with tests for a used code, an expired code and two parallel uses of one code.
- [x] 4.3 Write `POST /token/json/2/device/new` with the device ID check. Verify with tests that a code in uppercase works, that the device ID `../x` gets status 400 and that the code stays unused.
- [x] 4.4 Write `POST /token/json/2/user/new` with the claims of the `device-pairing` spec. Verify with a test that decodes the token and compares each claim.
- [x] 4.5 Write the tablet authentication middleware. Verify with tests that a device token, an expired user token and a token without `version` 10 get status 401 with the body `Unauthorized`.

## 5. Device registry

- [x] 5.1 Add the device list and the device remove endpoints. Verify with a test that the list shows the paired device.
- [x] 5.2 Add the device check with a 30-second cache to the tablet middleware. Verify with a test that a removed device gets status 401 after the cache time.
- [x] 5.3 Add `POST /token/json/2/device/delete` and `POST /token/json/3/device/delete`. Verify with a test that the device is no longer in the list.

## 6. CLI

- [x] 6.1 Add `spf13/cobra`. Move `serve` into a cobra command. Write the client settings from options, environment and `client.toml`. Verify with tests for the order of the settings sources.
- [x] 6.2 Write the output layer for text and `--json`, and the map from API errors to exit codes. Verify with tests for exit codes 2, 3, 4 and 5.
- [x] 6.3 Add the admin commands and the account commands of the `cli` spec, each with help text and an example. Verify with an end-to-end test that starts a server, adds a user, issues a token and creates a pairing code through the CLI.
- [x] 6.4 Document the first-run steps in `README.md`: generate the admin token, add a user, issue a token, pair a tablet. Verify that the documented commands run against a local server.

## 7. Integration

- [x] 7.1 Run `nix fmt` and verify that no file changes.
- [x] 7.2 Run `go test ./...` and verify that all tests pass.
- [x] 7.3 Run `nix flake check` and verify that all checks pass.
