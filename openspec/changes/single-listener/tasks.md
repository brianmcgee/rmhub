# Tasks

## 1. Optional API listener in the server

- [ ] 1.1 Remove the default of `listen.api.address` in `internal/config`, and refuse a key of `listen.api` without the address. Change the message for two equal addresses. Verify with tests for each scenario of the requirement `Listener configuration errors` and for a configuration with no listener key.
- [ ] 1.2 Add a function to `internal/app` that returns one handler for the tablet protocol and the API. Verify with tests that `GET /sync/v4/root`, `GET /api/v1/account`, `GET /api/v1/nothing`, `GET /nothing` and `GET /healthz` on the handler give the answers of their protocol.
- [ ] 1.3 Open one listener or two in `cmd/rmhub/serve.go`, and add the field `serves` to the log entry `listening`. Verify with end-to-end tests for each scenario of the requirements `Listeners` and `Listener log`.
- [ ] 1.4 Add a test for the scenario `Both limits on one listener` of the capability `rate-limiting`. Verify that the API request gets the JSON body and the pairing request gets an empty body.
- [ ] 1.5 Change the test helpers and the tests that depend on the default API address `127.0.0.1:8081`. Verify with `go test ./...`.
- [ ] 1.6 Update `docs/reference/configuration.md`, `docs/reference/http-api.md`, `docs/explanation/architecture.md` and `docs/explanation/request-limits.md` for the optional API listener. State in `docs/explanation/architecture.md` when to use two listeners. Verify each configuration example with `rmhub serve`.
- [ ] 1.7 Change `docs/tutorials/first-server.md` and the example URL in `docs/reference/cli.md` to one listener on port 8080. Verify that each command of the tutorial runs against a local server.

## 2. NixOS module

- [ ] 2.1 Remove the default of `listen.api.address` from `modules/nixos/rmhub.nix`, and open the API port with `openFirewall` only when the settings have the key. Verify with `nix eval` of a host with one listener and of a host with two listeners that the firewall ports and the configuration file are correct.
- [ ] 2.2 Verify that `examples/nixos.nix`, `checks/nixos-module.nix` and `checks/rmapi-conformance.nix` set `listen.api.address` where they use the API port. Build the checks `nixos-module` and `rmapi-conformance`.
- [ ] 2.3 Update `docs/reference/nixos-module.md`, `docs/how-to/deploy-on-nixos.md` and `docs/how-to/connect-a-tablet.md`. State that a host with a public tablet address must set `listen.api.address`. Verify that each Nix example of the pages evaluates.

## 3. Integration

- [ ] 3.1 Run `nix fmt` and verify that no file changes.
- [ ] 3.2 Run `go test ./...` and verify that all tests pass.
- [ ] 3.3 Run `nix flake check` and verify that all checks pass.
