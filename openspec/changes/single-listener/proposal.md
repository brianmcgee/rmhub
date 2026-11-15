# Proposal

## Why

rmhub always opens two listeners: one for the tablet protocol and one
for the rmhub API. The split lets an operator put the tablet listener on
the internet and keep the API on a private network. An operator whose
tablet and clients are all on one private network, such as a VPN, does
not need the split. That operator must still configure two ports, two
firewall rules and two proxy hosts, and must know which URL is for which
client.

## What Changes

- **BREAKING**: Without `listen.api.address`, the server opens one
  listener. It serves the tablet protocol and the rmhub API on
  `listen.tablet.address`. The API has no default address of
  `127.0.0.1:8081`.
- With `listen.api.address`, the server opens two listeners, as before.
  The tablet listener then answers each path under `/api/` with status
  404.
- A key of `listen.api` without `listen.api.address` is a configuration
  error.
- The rules that named a listener now name the path. A request for a
  path under `/api/` gets the API error format. Each other request gets
  the answers of the tablet protocol.
- The NixOS module does not set `listen.api.address`. `openFirewall`
  opens the port of each listener that the settings have.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `server-configuration`: the requirement `Two listeners` goes away.
  The new requirement `Listeners` makes the API listener optional.
- `rmhub-api`: where the API is available.
- `rate-limiting`: the body of a refused request and the API request
  limit follow the path, not the listener.
- `nixos-module`: the firewall rule for an optional API listener.

## Impact

- Code: `internal/config` (defaults and checks), `internal/app` (the
  handlers), `cmd/rmhub/serve.go` (the listeners) and
  `modules/nixos/rmhub.nix`.
- A deployment that uses the default API address `127.0.0.1:8081` must
  set `listen.api.address`, or must change the URL of each client to the
  tablet address. A deployment that sets `listen.api.address` does not
  change.
- `examples/nixos.nix` sets `listen.api.address` and stays an example of
  the split. The checks `checks/nixos-module.nix` and
  `checks/rmapi-conformance.nix` also set or use the API address and
  must stay correct.
- With one listener on a public address, the admin endpoints and the
  library API are reachable from that address. The documentation must
  tell the operator to set `listen.api.address` in that case.
- Pages in `docs/` to update: `docs/explanation/architecture.md`,
  `docs/reference/configuration.md`, `docs/reference/http-api.md`,
  `docs/reference/nixos-module.md`, `docs/reference/cli.md`,
  `docs/tutorials/first-server.md`, `docs/how-to/deploy-on-nixos.md`,
  `docs/how-to/connect-a-tablet.md` and
  `docs/explanation/request-limits.md`.
- No machine or clan service is affected.
