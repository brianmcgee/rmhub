# Design

## Context

The changes `foundation`, `root-log` and `accounts` give storage, the
root compare-and-swap, tablet tokens and the device registry. This change
adds the endpoints that a paired tablet calls during a sync.

Sources for the wire behavior:

- rmfakecloud at commit `962a73b`: `internal/app/routes.go`,
  `handlers.go`, `internal/hub/hub.go`, `internal/storage/models/`. It
  works with OS 3.27.1. It announces schema 3.
- ddvk/rmapi 0.0.35: `api/sync15/tree.go`, `blobdoc.go`,
  `blobstorage.go`, `transport/transport.go`. It talks to the reMarkable
  cloud as of August 2026. It writes schema 4 roots, hashes them as
  SHA-256 of the bytes, and sorts entries because the cloud refuses
  unsorted roots.

The user chose not to record tablet traffic. These questions stay open
until the first sync with the tablet:

1. Which schema does OS 3.27.1 write when rmhub announces schema 4?
2. What does `check-files` mean to the tablet? rmfakecloud always
   answers that nothing is missing.
3. Does the tablet check `x-goog-hash` on reads?
4. Does the tablet read the root through v3 or v4?

rmhub implements both sides of questions 1 and 4, sends a correct CRC32C
for question 3, and has a fallback setting for question 2.

## Goals / Non-Goals

**Goals:**

- A tablet syncs with rmhub as it does with rmfakecloud.
- No client can store a blob that does not match its key, or a root that
  names a missing blob.

**Non-Goals:**

- Screenshare, passcode reset, Sync 1.0 (`/document-storage/`), email,
  handwriting recognition, document upload (`/doc/v*/files`),
  integrations, signed URLs and MQTT. The tablet listener logs requests
  for these paths. A later change can add one when a log shows that the
  tablet needs it.
- Retention and garbage collection of blobs.

## Decisions

### Package layout

```
internal/hashtree/        parse, hash, serialize (no I/O)
internal/tablet/          tablet mux, discovery, telemetry
internal/tablet/sync/     root and blob endpoints, tree checks
internal/tablet/notify/   websocket hub
```

`internal/hashtree` has no dependency on storage, so the change
`scoped-api` can use it to read and write trees.

### Blob write: verify before store

The server streams the body into a temporary file in the data directory.
While it streams, it computes the SHA-256 and the CRC32C. If the SHA-256
equals the key, the blob is a file. If not, and the body is smaller than
4 MiB, the server parses it as an index and computes the index hash. A
document index uses the entry list hash. A root index uses the hash rule
of its schema. Only a match leads to a write into the blob store. The
4 MiB limit keeps a large file from a parse attempt. A root index for
10,000 documents is about 1 MiB.

The server does not trust `rm-filename`. The verification rule does not
depend on the header, so a client that omits the header or sends a wrong
one gets the correct result.

### Root write: check only what changed

A full walk of the tree for each root write costs one read for each
document. The server parses the new root index and the current root
index. It checks only the entries whose hash is not in the current root.
For each such entry it reads the document index, parses it, and calls
`Exists` for its files. A typical sync changes one document, so the
check reads one index and checks a few files.

The blob checks at write time make sure that each stored blob matches
its key. So the root check needs existence and parse, not a new hash.

A parsed root index is kept in an LRU cache by hash. The next root write
needs the current root again.

### Schema setting

The announced schema is a field `schemaVersion` in the account record of
the change `accounts`. `PATCH /api/v1/account` changes it with
put-if-match. The server accepts schema 3 and schema 4 roots for each
value. The setting changes only what `GET /sync/v4/root` announces.

### Notifications

The websocket hub subscribes to the commit events of the root log. Root
writes with `broadcast` `true` and `sync-complete` requests both lead to
one notification. The source device is in the event, so the hub skips
the websockets of that device. The library is `gorilla/websocket`. The
upgrade accepts any origin, because the tablet sends no browser origin.
Each websocket has a send buffer of 64 messages. A full buffer closes the
websocket. The tablet then connects again and reads the root. The device
removal endpoint of the change `accounts` tells the hub to close the
websockets of the device.

The server sends a ping every 30 seconds. A conforming websocket client
answers a ping with a pong automatically. [INFERENCE] The Qt websocket
stack on the tablet does this.

The root log sends events for each commit. A root write without
`broadcast` also gives an event, and the hub ignores it. The event has a
`broadcast` flag for that purpose.

### Conformance check with rmapi

`checks/rmapi-conformance.nix` starts rmhub with SQLite and the file
system backends in the build sandbox. It creates a user, an owner token
and a pairing code with the CLI. It gives the code to rmapi on standard
input with `RMAPI_HOST=http://127.0.0.1:<port>` and `RMAPI_CONFIG` in the
build directory. Then it runs rmapi commands: `mkdir`, `put` of a PDF,
`mv`, `ls`, `get`, `rm`. After each command it compares the result with
the expected tree. Then it runs the same steps with
`RMAPI_FORCE_SCHEMA_VERSION=3`. A third step runs two rmapi instances at
the same time and checks that both changes are in the final tree.

rmapi uses `/token/json/2/device/new`, `/token/json/2/user/new`,
`/sync/v4/root`, `PUT /sync/v3/root` and `/sync/v3/files/`. It selects
Sync 1.5 from the `scopes` claim of the user token. So the check also
covers the tokens of the change `accounts`.

## Risks / Trade-offs

- [The tablet does not accept a true `check-files` answer] →
  `tablet.check_files = "none"` gives the rmfakecloud answer.
- [The tablet uses an endpoint that rmhub does not have] → The warning
  log names the path. The user reports it, and a small change adds it.
- [Status 400 on a root write that names a missing blob] → The response
  of the tablet to this status is not known. A correct tablet uploads
  each blob before the root, so this status only shows an existing
  fault.
- [rmapi is not the tablet] → The check proves the protocol for one
  client that the reMarkable cloud accepts. The first sync with the
  tablet is the final test. The user can keep rmfakecloud as a fallback
  for that test.
- [Temporary files for each blob write] → The temporary directory is in
  the data directory, so the blob store on the file system can link the
  file without a copy. For S3 the server streams the file to the bucket.

## Implementation notes

These decisions were made during the implementation without the user.
Review them.

- The sync package is `internal/tablet/sync15`, not
  `internal/tablet/sync`. The name `sync` hides the standard library
  package.
- The index parser accepts each entry type, not only `0` and
  `80000000`. rmapi and rmfakecloud do not check the type. Hashes must
  be lowercase hex.
- A schema 3 root index also matches the SHA-256 of its bytes. rmapi
  with `RMAPI_FORCE_SCHEMA_VERSION=3` uses this hash for a schema 3
  root, and task 6.2 needs it.
- Blob write rule: the SHA-256 of the body is the key, or the body is
  below 4 MiB, parses as an index, and its entry list hash (or root
  hash) is the key.
- A blob read above 8 MiB goes to a temporary file first, because the
  `x-goog-hash` header must come before the body. Smaller blobs stay
  in memory.
- In `check-files`, an invalid hash counts as missing. An empty result
  is `[]`, not `null`.
- The websocket of a removed device closes at the next ping tick. At
  each tick the writer checks the device record through the 30 second
  device cache, so the close comes within 60 seconds. This works across
  processes, so there is no direct hook from the device removal.
- The pong deadline is 90 seconds after the last pong.
- `POST /sync/v2/sync-complete` sends its notification through the hub
  directly, not through the root log, because it does not change the
  root.
- The analytics, report and beta endpoints need no token. Only
  `/sync/reports/v1` needs a user token.
- The account view of the API and the CLI shows `schemaVersion`.
  `PATCH /api/v1/account` takes only `schemaVersion`.
- The README example for rmfakecloud-proxy uses the `upstream` key from
  the proxy README. The installer of the proxy was not tested here.
