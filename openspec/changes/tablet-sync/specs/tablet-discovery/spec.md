# Spec Delta

## Purpose

The tablet asks discovery endpoints which hosts to use. rmhub must name a
host that the tablet sends to its rmfakecloud-proxy, so that each request
reaches rmhub through the proxy.

## ADDED Requirements

### Requirement: Announced host
The configuration key `tablet.announced_host` MUST set the host that the
discovery endpoints return. The default MUST be `local.appspot.com`. The
value MUST be a host name with an optional port, and no scheme or path.

#### Scenario: Default host
- **WHEN** the configuration does not set `tablet.announced_host`
- **THEN** each discovery response names `local.appspot.com`

#### Scenario: Value with a scheme
- **WHEN** the configuration sets `tablet.announced_host = "https://rmhub.example.org"`
- **THEN** the server stops at startup with an error that names `tablet.announced_host`

### Requirement: Endpoint discovery
`GET /discovery/v1/endpoints` on the tablet listener MUST answer with
status 200 and `{"notifications":"<host>","webapp":"<host>"}`. The
response MUST NOT contain `mqttbroker`.

#### Scenario: Endpoints
- **WHEN** a tablet requests `GET /discovery/v1/endpoints`
- **THEN** the server answers `{"notifications":"local.appspot.com","webapp":"local.appspot.com"}`

### Requirement: Web app discovery
`GET /discovery/v1/webapp` on the tablet listener MUST answer with status
200 and `{"Host":"<host>","Status":"OK"}`.

#### Scenario: Web app host
- **WHEN** a tablet requests `GET /discovery/v1/webapp`
- **THEN** the server answers `{"Host":"local.appspot.com","Status":"OK"}`

### Requirement: Service locator
`GET /service/json/1/{service}` on the tablet listener MUST answer with
status 200 and `{"Host":"<host>","Status":"OK"}`. For the service
`blob-storage`, the value of `Host` MUST be `https://<host>`.

#### Scenario: Blob storage service
- **WHEN** a tablet requests `GET /service/json/1/blob-storage`
- **THEN** the server answers `{"Host":"https://local.appspot.com","Status":"OK"}`

#### Scenario: Other service
- **WHEN** a tablet requests `GET /service/json/1/document-storage`
- **THEN** the server answers `{"Host":"local.appspot.com","Status":"OK"}`

### Requirement: No authentication for discovery
The discovery endpoints and the service locator MUST answer without a
token.

#### Scenario: Request without a token
- **WHEN** a tablet requests `GET /discovery/v1/endpoints` without an `Authorization` header
- **THEN** the server answers with status 200
