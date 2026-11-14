# Spec Delta

## Purpose

API tokens let people and programs use the rmhub API. The admin token
manages accounts. An owner token has full access to one account.

## ADDED Requirements

### Requirement: Admin token from a file
The configuration key `admin.token_file` MUST name a file that holds the
admin token. The admin token MUST start with `rmh_admin_` and MUST have at
least 32 characters after that prefix. If the key is not set, each admin
endpoint MUST answer with status 401. If the file is missing or the token
has the wrong form, the server MUST stop at startup.

#### Scenario: Admin token is valid
- **GIVEN** `admin.token_file` names a file that holds a valid admin token T
- **WHEN** a client sends `GET /api/v1/admin/users` with the token T
- **THEN** the server answers with status 200

#### Scenario: No admin token is configured
- **WHEN** a client sends `GET /api/v1/admin/users` and `admin.token_file` is not set
- **THEN** the server answers with status 401

#### Scenario: Short admin token
- **WHEN** the admin token file holds `rmh_admin_abc`
- **THEN** the server stops at startup with an error that names `admin.token_file`

### Requirement: Generate an admin token
`rmhub admin token generate` MUST print a new random admin token. The
command MUST NOT need a server.

#### Scenario: Generate
- **WHEN** an operator runs `rmhub admin token generate`
- **THEN** the command prints a token that starts with `rmh_admin_`

### Requirement: Owner token format
An owner token MUST have the form `rmh_<account>_<id>_<secret>`. The
account part MUST be the account ID. The ID MUST be 16 random characters
from `a-z` and `2-7`. The secret MUST be 52 random characters from `a-z`
and `2-7`.

#### Scenario: Token parts
- **WHEN** an admin client issues an owner token for the account A
- **THEN** the token starts with `rmh_A_`

### Requirement: Token secret storage
rmhub MUST store only the SHA-256 hash of a token secret. rmhub MUST
return the token text only in the response that creates the token. A
token check MUST compare the hashes in constant time.

#### Scenario: List tokens
- **WHEN** an owner client lists the tokens of its account
- **THEN** no item in the response contains a token secret

### Requirement: Issue an owner token
`POST /api/v1/admin/users/{id}/tokens` with the body `{"label":"…"}` MUST
create an owner token and return status 201 with
`{"id":"…","token":"…","label":"…","expiresAt":…}`. The body can contain
`expiresAt` as an RFC 3339 time. Without `expiresAt`, the token MUST NOT
expire. An owner client MUST be able to create an owner token for its own
account with `POST /api/v1/account/tokens`.

#### Scenario: Admin issues a token
- **WHEN** an admin client issues a token with the label `laptop` for the account A
- **THEN** the server returns status 201 and the token text

#### Scenario: Unknown account
- **WHEN** an admin client issues a token for an account that does not exist
- **THEN** the server returns status 404

### Requirement: Token expiry
A token with an `expiresAt` time in the past MUST get status 401.

#### Scenario: Expired token
- **GIVEN** an owner token with an `expiresAt` time one minute in the past
- **WHEN** a client uses the token
- **THEN** the server answers with status 401

### Requirement: Token list and revocation
`GET /api/v1/account/tokens` MUST list the tokens of the account with ID,
label, creation time and `expiresAt`. `DELETE /api/v1/account/tokens/{id}`
MUST revoke the token and return status 204. The next request with a
revoked token MUST get status 401.

#### Scenario: Revoke a token
- **GIVEN** an owner client revokes the token T
- **WHEN** a client uses the token T
- **THEN** the server answers with status 401

#### Scenario: Token of another account
- **WHEN** an owner client of the account A revokes a token ID of the account B
- **THEN** the server answers with status 404
- **AND** the token stays valid

### Requirement: Current account
`GET /api/v1/account` MUST return the account of the owner token with
`{"id":"…","email":"…","name":"…"}`.

#### Scenario: Who am I
- **WHEN** an owner client of the account A requests `GET /api/v1/account`
- **THEN** the server returns the ID A
