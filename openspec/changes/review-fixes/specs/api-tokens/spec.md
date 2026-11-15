# Spec Delta

## MODIFIED Requirements

### Requirement: Issue an owner token
`POST /api/v1/admin/users/{id}/tokens` with the body `{"label":"…"}` MUST
create an owner token and return status 201 with
`{"id":"…","token":"…","label":"…","expiresAt":…}`. The body can contain
`expiresAt` as an RFC 3339 time. Without `expiresAt`, the token MUST NOT
expire. An owner client MUST be able to create an owner token for its own
account with `POST /api/v1/account/tokens`. A label of more than 256
bytes MUST get status 400.

#### Scenario: Admin issues a token
- **WHEN** an admin client issues a token with the label `laptop` for the account A
- **THEN** the server returns status 201 and the token text

#### Scenario: Unknown account
- **WHEN** an admin client issues a token for an account that does not exist
- **THEN** the server returns status 404

#### Scenario: Long label
- **WHEN** a client creates a token with a label of 257 bytes
- **THEN** the server returns status 400
