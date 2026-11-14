# Spec Delta

## Purpose

The tablet sends telemetry, crash reports and beta queries. rmhub answers
these requests so that the tablet does not retry them, and it does not
keep their data.

## ADDED Requirements

### Requirement: Analytics events
`POST /analytics/v2/events` on the tablet listener MUST answer with status
201 and `{"message":"Success"}`. The server MUST discard the request body.

#### Scenario: Analytics upload
- **WHEN** a tablet sends `POST /analytics/v2/events` with a body
- **THEN** the server answers with status 201 and `{"message":"Success"}`
- **AND** the server stores nothing

### Requirement: Reports
`POST /v1/reports`, `POST /v2/reports`, `POST /report/v1` and `POST
/v2/events` on the tablet listener MUST answer with status 200 and an
empty body. These endpoints MUST NOT require a token. The server MUST
discard the request body.

#### Scenario: Crash report
- **WHEN** a tablet sends `POST /v2/reports` without a token
- **THEN** the server answers with status 200 and an empty body

### Requirement: Sync reports
`POST /sync/reports/v1` on the tablet listener MUST require a user token.
It MUST answer with status 200 and an empty body, and discard the body.

#### Scenario: Sync report without a token
- **WHEN** a client sends `POST /sync/reports/v1` without a token
- **THEN** the server answers with status 401

### Requirement: Beta settings
`GET /settings/v1/beta` on the tablet listener MUST answer with status 200
and `{"enrolled":false,"available":true}`. `POST /settings/v1/beta` MUST
answer with status 200 and discard the body.

#### Scenario: Beta query
- **WHEN** a tablet requests `GET /settings/v1/beta`
- **THEN** the server answers `{"enrolled":false,"available":true}`

### Requirement: Size limit for discarded bodies
The server MUST read at most 1 MiB of a discarded body. For a larger body
the server MUST close the connection after it answers.

#### Scenario: Large report
- **WHEN** a client sends a report body of 10 MiB
- **THEN** the server reads at most 1 MiB of the body
