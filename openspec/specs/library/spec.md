# library Specification

## Purpose
The library is the tree of folders and documents in the current root of
an account. API clients read it by ID. Paths are only a way to find an ID,
because two entries in one folder can have the same name.

## Requirements

### Requirement: Entry model
Each document of the current root MUST appear as an entry with these
fields: `id`, `name`, `type`, `parent`, `trashed`, `modified`, `hash` and
`pageCount`. The value of `type` MUST be `folder`, `pdf`, `epub` or
`notebook`. The value of `hash` MUST be the document index hash.

#### Scenario: PDF document
- **GIVEN** the root has a document with `"visibleName":"Paper"`, `"type":"DocumentType"` and `"fileType":"pdf"`
- **WHEN** a client reads the entry
- **THEN** the entry has `"name":"Paper"` and `"type":"pdf"`

### Requirement: Parent and trash
The value of `parent` MUST be the `parent` field of `.metadata`, with the
value `""` for the top level. An entry whose parent is `trash`, or whose
ancestor has the parent `trash`, MUST have `trashed` `true`. A document
with `"deleted":true` in `.metadata` MUST NOT appear.

#### Scenario: Folder in the trash
- **GIVEN** the folder F has the parent `trash` and contains the document D
- **WHEN** an owner client reads the entry D
- **THEN** the entry has `"trashed":true`

### Requirement: List a folder
`GET /api/v1/library?parent=<id>` MUST return the entries whose parent is
the ID, sorted by name and then by ID. Without `parent`, an owner token
MUST get the top level. The value `trash` MUST list the trash.

#### Scenario: Top level
- **WHEN** an owner client requests `GET /api/v1/library`
- **THEN** the server returns the entries with the parent `""`

#### Scenario: Unknown folder
- **WHEN** a client lists a folder ID that does not exist
- **THEN** the server answers with status 404

### Requirement: Read an entry
`GET /api/v1/library/{id}` MUST return the entry and its `path`. The path
MUST be the names from the top level to the entry, each after a `/`. The
response MUST have the header `ETag` with the value `"<hash>"`.

#### Scenario: Nested document
- **GIVEN** the document D with the name `Paper` is in the folder `Work`
- **WHEN** an owner client reads the entry D
- **THEN** the entry has `"path":"/Work/Paper"` and an `ETag` header

### Requirement: Resolve a path
`GET /api/v1/library/resolve?path=<path>` MUST return the entry at the
path. If no entry has the path, the server MUST answer with status 404. If
more than one entry has the path, the server MUST answer with status 409.
The error message MUST name the IDs of the matching entries.

#### Scenario: Two entries with one name
- **GIVEN** the top level has two documents with the name `Notes`
- **WHEN** a client resolves `/Notes`
- **THEN** the server answers with status 409 and a message with both IDs

### Requirement: Search by name
`GET /api/v1/library/search?q=<text>` MUST return each entry whose name
contains the text, without regard to case. The result MUST NOT contain
trashed entries. The result MUST have at most 100 entries.

#### Scenario: Search
- **GIVEN** the documents `Tax 2025` and `taxonomy`
- **WHEN** a client searches for `TAX`
- **THEN** the result contains both documents

### Requirement: Download the source file
`GET /api/v1/library/{id}/content` MUST be a content endpoint. For a PDF
it MUST return the PDF with the media type `application/pdf`. For an EPUB
it MUST return the EPUB with the media type `application/epub+zip`. For a
folder or a notebook it MUST answer with status 404.

#### Scenario: Download a PDF
- **WHEN** a client downloads the content of a PDF document
- **THEN** the bytes equal the uploaded PDF

### Requirement: Download an archive
`GET /api/v1/library/{id}/archive` MUST be a content endpoint. It MUST
return a ZIP file with the media type `application/zip`. The ZIP file MUST
contain each file of the document index, with the file name of the index
entry. For a folder it MUST answer with status 404.

#### Scenario: Notebook archive
- **WHEN** a client downloads the archive of a notebook with two pages
- **THEN** the ZIP file contains `.metadata`, `.content` and two `.rm` files of the document

### Requirement: Consistent reads
Each read request MUST use one root generation for its full response. The
response MUST have the header `RMHub-Generation` with that generation.

#### Scenario: Tablet commits during a list
- **WHEN** a tablet commits a root during a list request
- **THEN** the response shows the library of one generation only
