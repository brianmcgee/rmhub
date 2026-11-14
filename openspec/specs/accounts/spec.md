# accounts Specification

## Purpose
An account holds one library and the devices and tokens that can use it.
The operator manages accounts through the admin endpoints of the rmhub API.

## Requirements

### Requirement: Create a user
`POST /api/v1/admin/users` with the body `{"email":"…","name":"…"}` MUST
create an account and return status 201 with
`{"id":"…","email":"…","name":"…"}`. The ID MUST be 26 random characters
from `a-z` and `2-7`. The email address MUST be unique without regard to
case. A second account with the same email address MUST get status 409.

#### Scenario: New user
- **WHEN** an admin client creates a user with the email `Ada@Example.org`
- **THEN** the server returns status 201 and an ID of 26 characters

#### Scenario: Email in use
- **GIVEN** an account with the email `ada@example.org`
- **WHEN** an admin client creates a user with the email `ADA@example.org`
- **THEN** the server returns status 409 with the code `conflict`

#### Scenario: Email missing
- **WHEN** an admin client creates a user without an email
- **THEN** the server returns status 400 with the code `bad_request`

### Requirement: Parallel creates with one email
Two parallel creates with the same email address MUST give exactly one
account.

#### Scenario: Two operators race
- **WHEN** two admin clients create a user with the email `ada@example.org` at the same time
- **THEN** one request gets status 201
- **AND** the other request gets status 409

### Requirement: List and read users
`GET /api/v1/admin/users` MUST return all accounts, ordered by ID.
`GET /api/v1/admin/users/{id}` MUST return one account or status 404.

#### Scenario: List users
- **GIVEN** two accounts
- **WHEN** an admin client lists the users
- **THEN** the server returns both accounts ordered by ID

### Requirement: No passwords
rmhub MUST NOT store a password for an account. A person or a program MUST
reach an account only with a token.

#### Scenario: Account record
- **WHEN** an admin client creates a user
- **THEN** the stored account has no password field
