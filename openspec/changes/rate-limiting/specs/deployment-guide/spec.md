# Spec Delta

## ADDED Requirements

### Requirement: Client address behind a reverse proxy
The guide MUST tell the user that, behind a reverse proxy, the server
sees only the address of the proxy, so each pairing attempt uses one
bucket. The reverse proxy example MUST set the `X-Forwarded-For` header.
The guide MUST give the `trusted_proxies` setting for that example, and
MUST warn that a wrong entry lets a client choose its own address.

#### Scenario: Proxy setup
- **WHEN** a user reads the reverse proxy setup
- **THEN** the user finds the `X-Forwarded-For` line of the proxy and the `trusted_proxies` key of rmhub
