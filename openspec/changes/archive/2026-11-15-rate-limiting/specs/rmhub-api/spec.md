# Spec Delta

## MODIFIED Requirements

### Requirement: Error format
Each error response MUST have this body:
`{"error":{"code":"<code>","message":"<text>"}}`. The code MUST be one of
`bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`,
`precondition_failed`, `rate_limited` and `internal`. Each code MUST use
the status 400, 401, 403, 404, 409, 412, 429 and 500, in that order.

#### Scenario: Unknown user
- **WHEN** an admin client requests a user ID that does not exist
- **THEN** the server answers with status 404
- **AND** the body is `{"error":{"code":"not_found","message":"…"}}`

#### Scenario: Request above a limit
- **WHEN** a client sends a request and the bucket of its token is empty
- **THEN** the server answers with status 429
- **AND** the body is `{"error":{"code":"rate_limited","message":"…"}}`
