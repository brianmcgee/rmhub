# Spec Delta

## MODIFIED Requirements

### Requirement: API listener and path prefix
Each endpoint path of the rmhub API MUST start with `/api/v1/`. When the
server has an API listener, the rmhub API MUST be available only on the
API listener, and the tablet listener MUST answer a request for a path
under `/api/` with status 404. When the server has no API listener, the
rmhub API MUST be available on the tablet listener. A request for an
unknown path under `/api/` MUST get status 404 in the API error format
on each listener that serves the API.

#### Scenario: API path on the tablet listener
- **GIVEN** the server has an API listener
- **WHEN** a client sends `GET /api/v1/account/devices` to the tablet listener
- **THEN** the server answers with status 404

#### Scenario: API path with one listener
- **GIVEN** the server has no API listener
- **WHEN** a client sends `GET /api/v1/account/devices` with an owner token to the tablet listener
- **THEN** the server answers with status 200

#### Scenario: Unknown API path with one listener
- **GIVEN** the server has no API listener
- **WHEN** a client sends `GET /api/v1/nothing` to the tablet listener
- **THEN** the server answers with status 404 and the body `{"error":{"code":"not_found","message":"…"}}`
