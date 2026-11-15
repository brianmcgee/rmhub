# Spec Delta

## REMOVED Requirements

### Requirement: Two listeners
**Reason**: The API listener becomes optional. The requirement
`Listeners` replaces this requirement.

**Migration**: Set `listen.api.address = "127.0.0.1:8081"` to keep the
two listeners of the old default. Without the key, change the URL of
each API client to the address of the tablet listener.

## ADDED Requirements

### Requirement: Listeners
The server MUST open the tablet listener on `listen.tablet.address`. The
default MUST be `127.0.0.1:8080`. Without `listen.api.address`, the
tablet listener MUST serve the tablet protocol and the rmhub API. With
`listen.api.address`, the server MUST also open the API listener on that
address, and only the API listener MUST serve the rmhub API. Each
listener MUST accept a TLS certificate and key, or serve plain HTTP when
none is set.

#### Scenario: One listener
- **WHEN** the configuration does not set a listener address
- **THEN** the server listens only on `127.0.0.1:8080`
- **AND** `GET /sync/v4/root` and `GET /api/v1/account` on that address reach their endpoints

#### Scenario: Two listeners
- **WHEN** the configuration sets `listen.api.address = "127.0.0.1:8081"`
- **THEN** the server listens on `127.0.0.1:8080` and `127.0.0.1:8081`
- **AND** `GET /api/v1/account` on `127.0.0.1:8080` gets status 404

#### Scenario: Address in use
- **WHEN** the tablet listener address is in use by another process
- **THEN** the server stops with an error that names the address

### Requirement: Listener configuration errors
These MUST be configuration errors: a key of the table `listen.api`
without `listen.api.address`, and the same value for
`listen.tablet.address` and `listen.api.address`. The error message MUST
name the key. The message for the same address MUST tell the operator to
remove `listen.api.address` for one listener.

#### Scenario: API certificate without an address
- **WHEN** the configuration sets `listen.api.tls_cert_file` and `listen.api.tls_key_file` and not `listen.api.address`
- **THEN** the server stops with an error that names `listen.api.address`

#### Scenario: Same address
- **WHEN** the configuration sets both addresses to `127.0.0.1:8080`
- **THEN** the server stops with an error that names `listen.api.address`

### Requirement: Listener log
For each listener, the server MUST write one log entry at startup. The
entry MUST have the address and the protocols that the listener serves.

#### Scenario: Log of one listener
- **WHEN** the server starts without `listen.api.address`
- **THEN** the log has one entry for a listener, and the entry names the tablet protocol and the API
