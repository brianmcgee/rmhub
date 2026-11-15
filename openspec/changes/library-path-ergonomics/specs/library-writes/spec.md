# Spec Delta

## ADDED Requirements

### Requirement: Unique name option
The create request, the upload request and the patch request MUST accept
the option `unique`: the JSON field `"unique":true`, or the query
`unique=true` for an upload. With the option, the server MUST answer with
status 409 when a different entry in the destination folder has the same
name. The message MUST name the ID of that entry. The server MUST do the
check again after each lost race. Without the option, the server MUST
accept the name.

#### Scenario: Folder name in use
- **GIVEN** the top level has the folder `Inbox` with the ID F
- **WHEN** a client creates the folder `Inbox` at the top level with `"unique":true`
- **THEN** the server answers with status 409 and a message with the ID F

#### Scenario: No option
- **GIVEN** the top level has the folder `Inbox`
- **WHEN** a client creates the folder `Inbox` at the top level without `unique`
- **THEN** the server answers with status 201

#### Scenario: Upload refused before storage
- **GIVEN** the folder F has the document `Paper`
- **WHEN** a client uploads a PDF with `parent=F&name=Paper&unique=true`
- **THEN** the server answers with status 409 and does not store the body

#### Scenario: Move into a folder with the name
- **GIVEN** the folders A and B each have a document with the name `Paper`
- **WHEN** a client moves the document of A into B with `"unique":true`
- **THEN** the server answers with status 409, and the document keeps its parent

#### Scenario: Same name for the same entry
- **GIVEN** the document D has the name `Paper`
- **WHEN** a client renames D to `Paper` with `"unique":true`
- **THEN** the server answers with status 200

#### Scenario: Tablet creates the name first
- **GIVEN** an API client creates the folder `Inbox` with `"unique":true`
- **WHEN** a tablet commits a folder `Inbox` in the same parent before the API commit
- **THEN** the server answers with status 409

### Requirement: Move out of the trash
`PATCH /api/v1/library/{id}` with `{"parent":"<id>"}` on a trashed entry
MUST move the entry out of the trash into the folder. The value `""` MUST
move the entry to the top level. The entry and its descendants MUST then
have `trashed` `false`.

#### Scenario: Restore a folder
- **GIVEN** the folder F is in the trash and contains the document D
- **WHEN** an owner client sends `{"parent":""}` for F
- **THEN** the server answers with status 200, and the entries F and D have `"trashed":false`
