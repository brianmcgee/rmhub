# Tasks

## 1. Hash tree

- [ ] 1.1 Write the index parser in `internal/hashtree` for schema 3 and schema 4 with the lenient rules. Verify with tests for each scenario of the `hash-tree` spec, including the entry with four fields and the wrong summary count.
- [ ] 1.2 Write the file hash, the entry list hash and the root index hash. Verify with a test that the SHA-256 of `test` is `9f86d081…0a08` and that entry order does not change the entry list hash.
- [ ] 1.3 Write the serializer. Verify the schema 4 example of the spec byte for byte. Verify with a round-trip test that parse, write and parse give the same entries and hash.
- [ ] 1.4 Add test data from ddvk/rmapi 0.0.35: a schema 4 root index and a schema 3 document index that rmapi writes. Verify that the parser accepts them and that the computed hashes equal the rmapi hashes.

## 2. Discovery and telemetry

- [ ] 2.1 Add `tablet.announced_host` to the configuration. Refuse a value with a scheme or a path. Verify with configuration tests.
- [ ] 2.2 Write the discovery endpoints and the service locator in `internal/tablet`. Verify each response body of the `tablet-discovery` spec with tests.
- [ ] 2.3 Write the telemetry, report, sync report and beta endpoints with the 1 MiB read limit. Verify each status code and body of the `tablet-telemetry` spec with tests.
- [ ] 2.4 Write the 404 handler that logs the method and the path. Verify with a test that the log has no query, no body and no `Authorization` value.

## 3. Root endpoints

- [ ] 3.1 Add the field `schemaVersion` to the account record with the default 3. Add `PATCH /api/v1/account` and `rmhub account set-schema`. Verify with tests for the values 3, 4 and 5.
- [ ] 3.2 Write `GET /sync/v4/root` and `GET /sync/v3/root` with the `x-goog-hash` header. Verify with tests for a new account and an account with a root, and verify the CRC32C value with a known body.
- [ ] 3.3 Write `PUT /sync/v3/root` with the commit through the root log and the device as the source. Verify with tests for a successful write, a body that is not JSON and status 412 with an empty body.
- [ ] 3.4 Write the tree check for changed entries with an LRU cache of parsed root indexes. Verify with tests for a missing document index, a missing file and a root hash that names a file blob.

## 4. Blob endpoints

- [ ] 4.1 Write `GET /sync/v3/files/{hash}` with the media type and the `x-goog-hash` header. Verify with tests for an existing blob, a missing blob and an invalid hash.
- [ ] 4.2 Write `PUT /sync/v3/files/{hash}` with a temporary file, SHA-256 and CRC32C. Accept an index body below 4 MiB by its index hash. Verify with tests for a file, a document index, a schema 3 root, a schema 4 root and a mismatch.
- [ ] 4.3 Verify with a test that a CRC32C mismatch gets status 400 and that the blob is not stored. Verify that a write without `x-goog-hash` succeeds.
- [ ] 4.4 Add `tablet.check_files` with the values `truthful` and `none`. Write `POST /sync/v3/check-files`, `GET /sync/v3/missing` and `POST /sync/v2/sync-complete`. Verify with tests for both values of the setting.
- [ ] 4.5 Write a test with two accounts that both have a paired device. Verify that the account B gets status 404 for a blob of the account A and gets that hash in `missingFiles`. Verify that a root of the account B that names it gets status 400.

## 5. Notifications

- [ ] 5.1 Add `gorilla/websocket`. Write the hub in `internal/tablet/notify` that subscribes to root log events and skips events without broadcast. Verify with a test that two devices of one account get the right messages and that a device of another account gets none.
- [ ] 5.2 Verify the JSON of the `SyncComplete` message field by field with a test. Verify that message IDs are unique.
- [ ] 5.3 Write the send buffer of 64 messages, the ping every 30 seconds and the close at device removal. Verify with tests that a stalled websocket is closed and that root writes still complete.

## 6. Conformance check with rmapi

- [ ] 6.1 Write `checks/rmapi-conformance.nix` with rmapi from nixpkgs. Pair rmapi through the CLI and a pairing code on standard input. Verify that the check passes for `mkdir`, `put`, `mv`, `ls`, `get` and `rm`.
- [ ] 6.2 Add the run with `RMAPI_FORCE_SCHEMA_VERSION=3`. Verify that the check passes and that the stored root is schema 3.
- [ ] 6.3 Add the run with two rmapi instances at the same time. Verify that both changes are in the final tree and that the root log has one entry for each change.
- [ ] 6.4 Document in `README.md` how to point rmfakecloud-proxy at the tablet listener, the setting `tablet.announced_host`, the setting `tablet.check_files` and the schema setting. Verify that the documented configuration starts the server.

## 7. Integration

- [ ] 7.1 Run `nix fmt` and verify that no file changes.
- [ ] 7.2 Run `go test ./...` and verify that all tests pass.
- [ ] 7.3 Run `nix flake check` and verify that all checks pass, including `rmapi-conformance`.
