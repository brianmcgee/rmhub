# Spec Delta

## MODIFIED Requirements

### Requirement: Exit codes
The client MUST exit with these codes:

- 0: success.
- 1: an unexpected error.
- 2: a usage error or a setting error.
- 3: status 401 or 403.
- 4: status 404.
- 5: status 409 or 412.
- 6: status 429.

For status 429, the error message MUST give the number of seconds of
the `Retry-After` header. The client MUST NOT send the request again.

#### Scenario: Unknown device
- **WHEN** a user removes a device ID that does not exist
- **THEN** the command exits with status 4

#### Scenario: Bad token
- **WHEN** a user runs a command with a revoked token
- **THEN** the command exits with status 3

#### Scenario: Request above a limit
- **WHEN** a user runs a command and the server answers with status 429 and `Retry-After: 7`
- **THEN** the command exits with status 6
- **AND** the error message contains `7 seconds`
