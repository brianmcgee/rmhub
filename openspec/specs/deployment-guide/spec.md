# deployment-guide Specification

## Purpose
The user switches a tablet from rmfakecloud to rmhub by a change of the
proxy configuration on the tablet. The guide must make this switch safe
and easy to undo.

## Requirements

### Requirement: TLS section
The guide MUST explain that rmfakecloud-proxy checks the certificate of
its upstream against the trusted roots of the tablet. It MUST describe
these two setups:

- A public certificate on the tablet listener through `security.acme`.
- A reverse proxy with a public certificate in front of a plain tablet listener.

#### Scenario: Reverse proxy
- **WHEN** a user follows the reverse proxy setup
- **THEN** the guide gives an nginx example that passes websocket upgrades and allows bodies of at least 256 MiB

### Requirement: Certificate renewal
The guide MUST tell the user to restart `rmhub.service` after each
certificate renewal, because systemd copies credentials at service
start. It MUST give the `security.acme` option that does this.

#### Scenario: Renewal setting
- **WHEN** a user reads the TLS section
- **THEN** the guide shows `security.acme.certs.<name>.reloadServices = [ "rmhub.service" ]`

### Requirement: Backup before the switch
The guide MUST tell the user to keep the rmfakecloud data unchanged and to
download a backup of the library with rmapi before the switch. It MUST
give the rmapi commands.

#### Scenario: Backup step
- **WHEN** a user reads the switch section
- **THEN** the first step is the backup with rmapi against rmfakecloud

### Requirement: Switch steps
The guide MUST give these steps in this order:

1. Create the rmhub user.
2. Create a pairing code.
3. Change `upstream` in the proxy configuration on the tablet.
4. Restart the proxy.
5. Pair the tablet with the code.
6. Check the sync.

The guide MUST tell the user that the first sync uploads the full library
from the tablet.

#### Scenario: Proxy configuration
- **WHEN** a user reads the switch section
- **THEN** the guide shows a proxy configuration with `upstream: https://<rmhub host>`

### Requirement: Way back
The guide MUST tell the user how to go back: restore the old `upstream`
and restart the proxy. It MUST warn that changes made while the tablet
used rmhub are not in rmfakecloud until the tablet syncs with it again.

#### Scenario: Return to rmfakecloud
- **WHEN** a user reads the way-back section
- **THEN** the guide names the proxy setting to restore and the warning about changes

### Requirement: Guide matches the code
The NixOS example of the guide MUST be the file `examples/nixos.nix`. The
guide MUST refer to that file and MUST NOT hold a second copy of it. The
VM test MUST import that file.

#### Scenario: Example drifts
- **WHEN** a developer renames a configuration key and does not change `examples/nixos.nix`
- **THEN** the VM test fails

### Requirement: Client address behind a reverse proxy
The guide MUST tell the user that, behind a reverse proxy, the server
sees only the address of the proxy, so each pairing attempt uses one
bucket. The reverse proxy example MUST set the `X-Forwarded-For` header.
The guide MUST give the `trusted_proxies` setting for that example, and
MUST warn that a wrong entry lets a client choose its own address.

#### Scenario: Proxy setup
- **WHEN** a user reads the reverse proxy setup
- **THEN** the user finds the `X-Forwarded-For` line of the proxy and the `trusted_proxies` key of rmhub
