# tablet-sync Specification

## Purpose
Sync 1.5 is the protocol that reMarkable OS 3.27.1 uses to sync a library.
The tablet uploads blobs, then replaces the root with a compare-and-swap
on the generation. rmhub checks each upload, so that no client can store
a broken or false tree.

## Requirements

### Requirement: Sync endpoints need a user token
Each endpoint of this capability MUST require a user token on the tablet
listener, as the capability `device-pairing` defines.

#### Scenario: Root read without a token
- **WHEN** a client requests `GET /sync/v4/root` without a token
- **THEN** the server answers with status 401

### Requirement: Blobs of one account
Each blob read, blob write, root check and `check-files` request MUST use
the blob namespace of the account of the user token. A blob of another
account MUST be handled as a missing blob.

#### Scenario: Read a blob of another account
- **GIVEN** the account A has the blob H, and the account B does not
- **WHEN** a tablet of the account B reads the hash H
- **THEN** the server answers with status 404

#### Scenario: Check a file of another account
- **GIVEN** the account A has the blob H, and the account B does not
- **WHEN** a tablet of the account B checks the file H
- **THEN** the server answers `{"missingFiles":["H"]}`

#### Scenario: Root that names a blob of another account
- **GIVEN** the account A has the document index H, and the account B does not
- **WHEN** a tablet of the account B writes a root whose index names H
- **THEN** the server answers with status 400

### Requirement: Root read v4
`GET /sync/v4/root` MUST answer with status 200 and
`{"generation":<g>,"hash":"<h>","schemaVersion":<s>}`. The value `s` MUST
be the announced schema of the account. For an account with the generation
0, the body MUST be `{"generation":0,"schemaVersion":<s>}`.

#### Scenario: New account
- **WHEN** a tablet requests `GET /sync/v4/root` for an account with no commit
- **THEN** the server answers `{"generation":0,"schemaVersion":3}`

#### Scenario: Account with a root
- **GIVEN** the account has the generation 12 and the root hash H
- **WHEN** a tablet requests `GET /sync/v4/root`
- **THEN** the server answers `{"generation":12,"hash":"H","schemaVersion":3}`

### Requirement: Root read v3
`GET /sync/v3/root` MUST answer with status 200 and
`{"generation":<g>,"hash":"<h>"}`. For an account with the generation 0, it
MUST answer with status 404 and `{"message":"root not found"}`.

#### Scenario: New account on v3
- **WHEN** a tablet requests `GET /sync/v3/root` for an account with no commit
- **THEN** the server answers with status 404 and `{"message":"root not found"}`

### Requirement: CRC32C header on reads
Each response of a root read and a blob read MUST have the header
`x-goog-hash: crc32c=<v>`. The value `v` MUST be the base64 encoding of
the big-endian CRC32C (Castagnoli) of the response body.

#### Scenario: Header on a blob read
- **WHEN** a tablet reads a blob
- **THEN** the response has an `x-goog-hash` header with the CRC32C of the body

### Requirement: Root write
`PUT /sync/v3/root` MUST accept `{"generation":<g>,"hash":"<h>","broadcast":<b>}`.
On success it MUST answer with status 200 and
`{"generation":<g+1>,"hash":"<h>"}`. The root log MUST record the device
ID of the token as the source.

#### Scenario: Successful root write
- **GIVEN** the account has the generation 4
- **WHEN** a tablet writes a valid root with the generation 4
- **THEN** the server answers with the generation 5 and the same hash

#### Scenario: Body is not JSON
- **WHEN** a tablet sends `PUT /sync/v3/root` with a body that is not JSON
- **THEN** the server answers with status 400

### Requirement: Stale root write
A root write with a generation that is not the current generation MUST get
status 412 with an empty body. The root MUST NOT change.

#### Scenario: Another device wrote first
- **GIVEN** the account has the generation 5
- **WHEN** a tablet writes a root with the generation 4
- **THEN** the server answers with status 412 and an empty body

### Requirement: Root write checks the tree
Before the commit, the server MUST check the new root. The blob of the
root hash MUST exist and MUST parse as a root index whose hash is the root
hash. Each document index that the current root does not have MUST exist
and MUST parse. Each file that such an index names MUST exist. If a check
fails, the server MUST answer with status 400 and MUST NOT commit.

#### Scenario: Root names a missing document index
- **WHEN** a tablet writes a root whose index names a document index that the server does not have
- **THEN** the server answers with status 400
- **AND** the generation does not change

#### Scenario: Root hash names a file blob
- **WHEN** a tablet writes a root hash that names a blob that is not a root index
- **THEN** the server answers with status 400

### Requirement: Root write notification
After a successful root write with `broadcast` `true`, the server MUST send
a notification as the capability `tablet-notifications` defines. With
`broadcast` `false`, the server MUST NOT send a notification.

#### Scenario: Broadcast
- **WHEN** a tablet writes a root with `broadcast` `true`
- **THEN** the other devices of the account get a `SyncComplete` message

### Requirement: Blob read
`GET /sync/v3/files/{hash}` MUST answer with status 200, the media type
`application/octet-stream` and the blob bytes. For a blob that does not
exist, or a hash that is not 64 lowercase hexadecimal characters, it MUST
answer with status 404.

#### Scenario: Read a missing blob
- **WHEN** a tablet reads a hash that the server does not have
- **THEN** the server answers with status 404

### Requirement: Blob write
`PUT /sync/v3/files/{hash}` MUST store the body as the blob of the hash
and answer with status 200 and an empty body. The server MUST accept the
blob only if one of these conditions is true:

- The hash is the SHA-256 of the body.
- The body parses as an index, and the hash of that index is the hash.

Otherwise the server MUST answer with status 400 and MUST NOT store the
blob.

#### Scenario: Valid file
- **WHEN** a tablet writes a body whose SHA-256 is the hash
- **THEN** the server answers with status 200 and stores the blob

#### Scenario: Valid document index
- **WHEN** a tablet writes a schema 3 document index whose entry list hash is the hash
- **THEN** the server answers with status 200 and stores the blob

#### Scenario: Body does not match the hash
- **WHEN** a tablet writes a body that matches the hash under no rule
- **THEN** the server answers with status 400
- **AND** a read of the hash returns status 404

### Requirement: CRC32C check on writes
If a blob write has the header `x-goog-hash` with a `crc32c` value, the
server MUST compare it with the CRC32C of the body. A mismatch MUST get
status 400, and the server MUST NOT store the blob. A write without the
header MUST be accepted.

#### Scenario: Body changed in transit
- **WHEN** a tablet writes a blob with a `crc32c` value that does not match the body
- **THEN** the server answers with status 400

### Requirement: Check files
`POST /sync/v3/check-files` MUST accept `{"filename":"…","files":["<hash>",…],"reason":"…"}`.
It MUST answer with status 200 and `{"missingFiles":[…]}`. When
`tablet.check_files` is `truthful` (the default), the list MUST contain
each hash of `files` that the server does not have. When
`tablet.check_files` is `none`, the body MUST be `{"missingFiles":null}`.

#### Scenario: One file is missing
- **GIVEN** the server has the blob H1 and not the blob H2
- **WHEN** a tablet checks the files H1 and H2
- **THEN** the server answers `{"missingFiles":["H2"]}`

#### Scenario: Fallback setting
- **GIVEN** `tablet.check_files` is `none`
- **WHEN** a tablet checks the files H1 and H2
- **THEN** the server answers `{"missingFiles":null}`

### Requirement: Missing hashes and sync complete
`GET /sync/v3/missing` MUST answer with status 200 and `{"hashes":[]}`.
`POST /sync/v2/sync-complete` with `{"generation":<g>}` MUST send a
notification and answer with status 200 and
`{"id":"<message ID>","generation":0}`.

#### Scenario: Legacy sync complete
- **WHEN** a tablet sends `POST /sync/v2/sync-complete`
- **THEN** the other devices of the account get a `SyncComplete` message

### Requirement: Announced schema setting
Each account MUST have an announced schema: `3` or `4`. The default MUST
be `3`. `PATCH /api/v1/account` with `{"schemaVersion":4}` MUST change it
for the account of the owner token. `rmhub account set-schema <3|4>` MUST
call that endpoint. The server MUST accept root indexes of both schemas
for each value of the setting.

#### Scenario: Owner selects schema 4
- **WHEN** an owner runs `rmhub account set-schema 4`
- **THEN** `GET /sync/v4/root` answers with `"schemaVersion":4`

#### Scenario: Schema 3 root on a schema 4 account
- **GIVEN** the account announces schema 4
- **WHEN** a tablet writes a valid schema 3 root
- **THEN** the root write succeeds

#### Scenario: Invalid schema
- **WHEN** an owner client sends `PATCH /api/v1/account` with `{"schemaVersion":5}`
- **THEN** the server answers with status 400

### Requirement: Unknown tablet requests
The tablet listener MUST answer a request for an unknown path with status
404. The server MUST log the method and the path of the request at the
warning level. The log entry MUST NOT contain the request body, the query
or the `Authorization` header.

#### Scenario: Screenshare request
- **WHEN** a tablet sends `POST /screenshare/v1/rooms`
- **THEN** the server answers with status 404
- **AND** the log has a warning with `POST` and `/screenshare/v1/rooms`
