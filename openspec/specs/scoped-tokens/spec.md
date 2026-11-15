# scoped-tokens Specification

## Purpose
A scoped token gives a program or an agent access to some folders of one
account and to nothing else. A token can pass a smaller part of its
access to a child token. The owner can stop a full chain of tokens
with one revocation.

## Requirements

### Requirement: Grants
A scoped token MUST have one or more grants. A grant MUST have a folder ID
and an access level: `read` or `write`. A grant MUST apply to each
descendant of its folder. Grants MUST follow the folder ID, so a move of
the folder keeps its grants.

#### Scenario: Folder moved
- **GIVEN** a token has a read grant on the folder F
- **WHEN** a tablet moves F into another folder
- **THEN** the token can still read the documents in F

### Requirement: Create a scoped token
`POST /api/v1/account/tokens` with `{"label":"…","grants":[{"folder":"<id>","access":"read"}]}`
MUST create a scoped token with the token format of the capability
`api-tokens`. The token record MUST store the grants and the ID of the
token that created it. A grant folder MUST be an existing folder that is
not trashed. A request with more than 64 grants MUST get status 400.

#### Scenario: Owner creates an agent token
- **WHEN** an owner client creates a token with a write grant on the folder `Agent`
- **THEN** the server answers with status 201 and the token text

#### Scenario: Grant on a document
- **WHEN** a client creates a token with a grant on a document ID
- **THEN** the server answers with status 400

#### Scenario: Too many grants
- **WHEN** a client creates a token with 65 grants
- **THEN** the server answers with status 400

### Requirement: Child tokens have fewer rights
A scoped token MUST be able to create a child token. Each grant of the
child MUST be on the folder of a parent grant or on its descendant. Its
access MUST NOT be higher than the access of that parent grant. The
expiry of the child MUST NOT be later than the expiry of the parent. The
chain of parents MUST have at most 4 scoped tokens.

#### Scenario: Wider grant
- **GIVEN** a token has a read grant on the folder F
- **WHEN** it creates a child token with a write grant on F
- **THEN** the server answers with status 403

#### Scenario: Narrower grant
- **GIVEN** a token has a write grant on the folder F with the subfolder G
- **WHEN** it creates a child token with a read grant on G
- **THEN** the server answers with status 201

### Requirement: Chain check at each request
For each request with a scoped token, the server MUST check each token of
the parent chain. If one of them is revoked or expired, the request MUST
get status 401. An entry MUST be visible only if each token of the chain
can see it. The same rule MUST apply to write access.

#### Scenario: Parent revoked
- **GIVEN** the token T created the child token C
- **WHEN** the owner revokes T
- **THEN** the next request with C gets status 401

#### Scenario: Child folder moved out of the parent grant
- **GIVEN** T has a grant on F, and its child C has a grant on G in F
- **WHEN** a tablet moves G out of F
- **THEN** C can no longer read G

### Requirement: Visibility
A scoped token MUST see only the folders of its grants and their
descendants. A request for any other entry MUST get status 404. Without
`parent`, `GET /api/v1/library` MUST return the grant folders. The path of
an entry MUST start at its grant folder, as `/<grant folder name>/…`. A
path resolve MUST use these paths. While a grant folder is in the trash,
the token MUST NOT see the folder or its descendants. The access MUST
come back when the folder leaves the trash.

#### Scenario: Grant folder in the trash
- **GIVEN** a token has a read grant on the folder F
- **WHEN** a tablet moves F into the trash
- **THEN** a request of the token for a document in F gets status 404

#### Scenario: Entry outside the grants
- **WHEN** a scoped token reads the ID of a document outside its grants
- **THEN** the server answers with status 404

### Requirement: Write rules
A write on an entry MUST need write access to the entry. A create MUST
need write access to the parent. A move MUST need write access to the
entry and to the new parent. A trash MUST need write access to the entry.
A grant folder MUST NOT be renamed, moved or trashed with a scoped token.
A read-only entry MUST get status 403 for a write.

#### Scenario: Rename the grant folder
- **GIVEN** a token has a write grant on the folder F
- **WHEN** it renames F
- **THEN** the server answers with status 403

#### Scenario: Move into a read-only folder
- **GIVEN** a token has a write grant on F and a read grant on R
- **WHEN** it moves a document from F into R
- **THEN** the server answers with status 403

### Requirement: Token management with a scoped token
A scoped token MUST be able to list and revoke only its descendants. A
scoped token MUST get status 403 on the endpoints for devices, pairing
codes, the account settings and the admin endpoints.

#### Scenario: Pairing code with a scoped token
- **WHEN** a scoped token requests `POST /api/v1/account/pairing-codes`
- **THEN** the server answers with status 403

### Requirement: Revocation of a chain
When a token is revoked, the server MUST also mark each descendant of the
token as revoked.

#### Scenario: Token list after revocation
- **GIVEN** the token T has the child C
- **WHEN** the owner revokes T and lists the tokens
- **THEN** the list does not contain T or C
