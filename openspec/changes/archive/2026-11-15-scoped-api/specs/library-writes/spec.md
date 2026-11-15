# Spec Delta

## Purpose

API clients change the library with semantic operations on IDs. A tablet
can commit a root at the same time. So the server applies each operation
to the newest root and keeps the changes of the tablet.

## ADDED Requirements

### Requirement: Operations commit a new root
Each write operation MUST write the new blobs, then commit a new root
through the root log. The commit MUST record the token ID as the source
and MUST have the broadcast flag `true`. The new root MUST use the schema
of the current root index. Without a current root, it MUST use the
announced schema of the account.

#### Scenario: Notification after an API write
- **GIVEN** a tablet has an open notification websocket
- **WHEN** an API client renames a document
- **THEN** the tablet gets a `SyncComplete` message

### Requirement: Re-apply after a lost race
If the commit returns stale-generation, the server MUST read the new root,
check the operation again against the new root, and apply it again. After
5 lost races the server MUST answer with status 409. An operation that is
not valid on the new root MUST get the error of that check.

#### Scenario: Tablet commits first
- **GIVEN** an API client renames the document D
- **WHEN** a tablet commits a change to the document E before the API commit
- **THEN** the new root has the new name of D and the change to E

#### Scenario: Tablet trashed the target
- **GIVEN** an API client moves the document D into the folder F
- **WHEN** a tablet moves F into the trash before the API commit
- **THEN** the move succeeds, and D is in the trashed folder F

### Requirement: Keep unknown metadata
An operation that changes `.metadata` or `.content` MUST keep each field
that the operation does not change, with its value. The operation MUST
increment `version`, MUST set `lastModified` to the current time in
milliseconds as a string, and MUST set `metadatamodified` to `true`.

#### Scenario: Unknown field
- **GIVEN** the `.metadata` of the document D has the field `"pinned":true` and a field `"x":1`
- **WHEN** an API client renames D
- **THEN** the new `.metadata` has `"pinned":true` and `"x":1`

### Requirement: Create a folder
`POST /api/v1/library/folders` with `{"parent":"<id>","name":"<name>"}`
MUST create a folder with a new random UUID. The folder MUST have a
`.metadata` with the type `CollectionType` and a `.content` with `{}`. The
server MUST answer with status 201 and the entry.

#### Scenario: New folder
- **WHEN** an owner client creates the folder `Inbox` at the top level
- **THEN** the server answers with status 201 and an entry with `"type":"folder"`

### Requirement: Upload a document
`POST /api/v1/library/documents?parent=<id>&name=<name>` MUST be a content
endpoint. The body MUST be a PDF with the media type `application/pdf`,
or an EPUB with the media type `application/epub+zip`. The server MUST
create a document with a new random UUID, the source file, a `.metadata`
and a `.content` with the `fileType`. It MUST answer with status 201 and
the entry.

#### Scenario: Upload a PDF
- **WHEN** an owner client uploads a PDF with the name `Paper`
- **THEN** the server answers with status 201 and an entry with `"type":"pdf"`

### Requirement: Upload checks
The server MUST refuse with status 400 these bodies:

- A PDF body that does not start with `%PDF-`.
- An EPUB body that is not a ZIP file with the first entry `mimetype` and the value `application/epub+zip`.
- A body larger than `api.max_upload_bytes`.

The default of `api.max_upload_bytes` MUST be 268435456 (256 MiB).

#### Scenario: Wrong media type
- **WHEN** a client uploads a text file with the media type `application/pdf`
- **THEN** the server answers with status 400

### Requirement: Rename and move
`PATCH /api/v1/library/{id}` with `{"name":"<name>"}` MUST rename the
entry. With `{"parent":"<id>"}` it MUST move the entry into the folder.
The server MUST refuse these moves with status 400:

- A move into a parent that is not a folder.
- A move into the parent `trash`.
- A move of a folder into itself or into its descendant.

#### Scenario: Move into a descendant
- **GIVEN** the folder B is in the folder A
- **WHEN** a client moves A into B
- **THEN** the server answers with status 400

### Requirement: Trash
`POST /api/v1/library/{id}/trash` MUST set the parent of the entry to
`trash` and answer with status 200 and the entry. The API MUST NOT have an
operation that removes an entry from the tree.

#### Scenario: Trash a document
- **WHEN** an owner client trashes the document D
- **THEN** the entry D has `"trashed":true`

### Requirement: Conditional writes
A write request on an entry MUST accept the header `If-Match` with an
entity tag from the `ETag` of the entry. If the document index hash of
the entry is not the tag, the server MUST answer with status 412 and MUST
NOT commit. The server MUST check the header again after each lost race.

#### Scenario: Tablet changed the document
- **GIVEN** a client read the entry D with the `ETag` `"h1"`
- **WHEN** a tablet changes D, and then the client renames D with `If-Match: "h1"`
- **THEN** the server answers with status 412
