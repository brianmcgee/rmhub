# Spec Delta

## Purpose

The rmhub API is the interface for the CLI, scripts and AI agents. These
base rules apply to each endpoint of the API.

## ADDED Requirements

### Requirement: API listener and path prefix
The rmhub API MUST be available only on the API listener. Each endpoint
path MUST start with `/api/v1/`. The tablet listener MUST answer a request
for a path under `/api/` with status 404.

#### Scenario: API path on the tablet listener
- **WHEN** a client sends `GET /api/v1/account/devices` to the tablet listener
- **THEN** the server answers with status 404

### Requirement: JSON bodies
Each request body and each response body of the rmhub API MUST be JSON with
the media type `application/json`. A request with a body that is not valid
JSON MUST get status 400. An endpoint that a spec names as a content
endpoint or a stream endpoint MUST follow the media types of that spec.
Its error responses MUST be JSON.

#### Scenario: Body is not JSON
- **WHEN** a client sends `POST /api/v1/admin/users` with the body `name=x`
- **THEN** the server answers with status 400 and an error body

### Requirement: Error format
Each error response MUST have this body:
`{"error":{"code":"<code>","message":"<text>"}}`. The code MUST be one of
`bad_request`, `unauthorized`, `forbidden`, `not_found`, `conflict`,
`precondition_failed` and `internal`. Each code MUST use the status 400,
401, 403, 404, 409, 412 and 500, in that order.

#### Scenario: Unknown user
- **WHEN** an admin client requests a user ID that does not exist
- **THEN** the server answers with status 404
- **AND** the body is `{"error":{"code":"not_found","message":"…"}}`

### Requirement: Bearer authentication
Each endpoint MUST require the header `Authorization: Bearer <token>`. A
request with no token, a malformed token, an unknown token, a revoked
token or an expired token MUST get status 401. A valid token without the
permission for the endpoint MUST get status 403.

#### Scenario: No token
- **WHEN** a client sends a request without an `Authorization` header
- **THEN** the server answers with status 401

#### Scenario: Owner token on an admin endpoint
- **WHEN** a client sends `GET /api/v1/admin/users` with an owner token
- **THEN** the server answers with status 403
