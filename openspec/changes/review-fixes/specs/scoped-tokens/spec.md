# Spec Delta

## MODIFIED Requirements

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
