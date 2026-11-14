# Tasks

## 1. Library read model

- [ ] 1.1 Write the snapshot builder in `internal/library` with the LRU cache of parsed metadata and reuse of unchanged entries. Verify with a test that a second snapshot after a one-document change reads only the blobs of that document.
- [ ] 1.2 Write the entry model with type, parent, trashed and path. Hide documents with `"deleted":true`. Verify with tests for each scenario of the `library` spec that concerns the entry model.
- [ ] 1.3 Write the list, read, resolve and search endpoints with the `ETag` and `RMHub-Generation` headers. Verify with tests for the top level, the trash, an unknown folder and two entries with one name.
- [ ] 1.4 Write the content and archive endpoints. Verify with tests that a downloaded PDF equals the stored file and that a notebook archive has each file of its index.

## 2. Write operations

- [ ] 2.1 Write the operation loop in `internal/libops` with 5 attempts and the commit with the token source and the broadcast flag. Verify with a test that injects one tablet commit before each of the first two attempts. The final root must have both changes.
- [ ] 2.2 Write the metadata editor that keeps unknown fields. Verify with a test that a rename keeps `"pinned":true` and an unknown field, increments `version` and sets `metadatamodified`.
- [ ] 2.3 Write create folder, rename, move and trash with their checks. Verify with tests for a parent that is a document, the parent `trash` and a move into a descendant.
- [ ] 2.4 Write the document upload with the PDF and EPUB checks and `api.max_upload_bytes`. Verify with tests for a valid PDF, a valid EPUB, a text file and a body above the limit.
- [ ] 2.5 Write the `If-Match` check, also after a lost race. Verify with a test that a tablet change between the read and the write gives status 412.
- [ ] 2.6 Verify with a test that an API write sends a `SyncComplete` message to an open tablet websocket. Verify that rmapi from nixpkgs lists the new document.

## 3. Scoped tokens

- [ ] 3.1 Add the fields `parent` and `grants` to the token record. Accept grants in `POST /api/v1/account/tokens`. Verify with tests for a grant on a folder, on a document and on a trashed folder.
- [ ] 3.2 Write the narrowing rules for child tokens: folder, access, expiry and chain length. Verify with tests for a wider grant, a narrower grant, a later expiry and a fifth scoped token.
- [ ] 3.3 Write the chain check and the visibility walk in `internal/grants`. Verify with tests for a revoked parent and a child folder that moves out of the parent grant.
- [ ] 3.4 Apply the visibility and write rules to each library endpoint. Verify that an entry outside the grants gives 404. Verify that a rename of the grant folder and a move into a read-only folder each give 403.
- [ ] 3.5 Limit token management for scoped tokens and add revocation of descendants. Verify with tests that a scoped token gets 403 on devices and pairing codes and that a revoked parent hides its children from the list.

## 4. Change feed

- [ ] 4.1 Write the diff of two snapshots with the five change kinds in `internal/feed`. Verify with a test for each kind.
- [ ] 4.2 Write the SSE endpoint with `after`, `Last-Event-ID`, the keepalive comment and the end at revocation. Verify with tests for a reconnect and for a value larger than the current generation.
- [ ] 4.3 Write the scoped filter. Verify with a test that a document that moves out of the grant gives `removed`, and that a generation with no visible change gives no event.
- [ ] 4.4 Verify with a test that the feed sends each generation when the broker sets the lost-events flag.

## 5. CLI and agent guide

- [ ] 5.1 Write the entry argument rule and the commands `ls`, `stat`, `find` and `get`. Verify with an end-to-end test against a server for each command, with and without `--json`.
- [ ] 5.2 Write the commands `upload`, `mkdir`, `mv`, `rename` and `trash` with `--if-match`. Verify with an end-to-end test for each command and for exit code 5 on a stale hash.
- [ ] 5.3 Write `rmhub watch` with reconnect. Verify with a test that stops the server during a watch and checks that no generation is lost after the restart.
- [ ] 5.4 Add `--grant` and `--expires` to `rmhub tokens create`. Verify with an end-to-end test that a scoped token sees only its folder.
- [ ] 5.5 Write `docs/agents/SKILL.md`. Verify that each command of the guide runs against a local server with a scoped token.

## 6. Integration

- [ ] 6.1 Run `nix fmt` and verify that no file changes.
- [ ] 6.2 Run `go test ./...` and verify that all tests pass.
- [ ] 6.3 Run `nix flake check` and verify that all checks pass.
