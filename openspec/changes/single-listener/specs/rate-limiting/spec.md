# Spec Delta

## MODIFIED Requirements

### Requirement: Answer above a limit
A refused request MUST get status 429 and the header `Retry-After` with
the number of seconds until one unit is available, as an integer of 1 or
more. For a path under `/api/`, the body MUST be the API error format
with the code `rate_limited`. For each other path, the body MUST be
empty. A refused request MUST NOT change data.

#### Scenario: API request above the limit
- **WHEN** a token sends an API request and its bucket is empty
- **THEN** the server answers with status 429 and a `Retry-After` header
- **AND** the body is `{"error":{"code":"rate_limited","message":"…"}}`

#### Scenario: Both limits on one listener
- **GIVEN** the server has no API listener, the bucket of a token is empty and the pairing bucket of an address is empty
- **WHEN** the token sends an API request and the address sends a pairing request to the tablet listener
- **THEN** the API request gets status 429 with the API error format
- **AND** the pairing request gets status 429 with an empty body

### Requirement: API request limit
Each request of the rmhub API with a valid token MUST take one unit
from the bucket of that token. The key MUST be the token ID, so a child
token MUST NOT use the bucket of its parent. The default limit MUST be
600 for one minute. `GET /healthz` MUST NOT have a limit. A request of
the tablet protocol MUST NOT take a unit from the bucket of an API
token.

#### Scenario: Agent in a loop
- **GIVEN** an owner token and a scoped token of one account
- **WHEN** the scoped token sends 700 requests in one minute
- **THEN** the server refuses requests of the scoped token with status 429
- **AND** the server accepts a request of the owner token
