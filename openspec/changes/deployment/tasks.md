# Tasks

## 1. NixOS module

- [ ] 1.1 Write `modules/nixos/rmhub.nix` with the options `enable`, `package`, `settings`, `credentials` and `openFirewall`. Verify with `iets eval` that `nixosModules.rmhub` exists.
- [ ] 1.2 Write the systemd service with `rmhub serve --config`, `DynamicUser`, `StateDirectory`, `CacheDirectory`, `LoadCredential` and restart on failure. Verify with an evaluation of a test host that the unit has each setting.
- [ ] 1.3 Add the hardening options without `MemoryDenyWriteExecute`. Verify with `systemd-analyze security` in the VM test that the service starts and that its exposure score is printed in the test log.
- [ ] 1.4 Write the firewall rule that reads the ports from the listener addresses. Verify with an evaluation that `0.0.0.0:8443` opens TCP port 8443.

## 2. VM test

- [ ] 2.1 Write `examples/nixos.nix` with TLS on the tablet listener and an admin token credential. Verify that it evaluates as a NixOS module.
- [ ] 2.2 Write `checks/nixos-module.nix` with `pkgs.testers.runNixOSTest`, the self-signed CA and the steps of the design. Verify that the check passes.
- [ ] 2.3 Verify with the VM test that a PDF page renders under the hardening options and that the PDF is still listed after a restart.

## 3. Deployment guide

- [ ] 3.1 Write the TLS section of `README.md` with the ACME setup, the nginx setup and the renewal setting. Verify that the nginx example passes `nginx -t` in a throwaway VM or container.
- [ ] 3.2 Write the switch section with the backup, the steps in order, the proxy configuration and the note about the first sync. Verify that each rmhub command in the section runs against a local server.
- [ ] 3.3 Write the way-back section with the warning about changes. Verify that the guide refers to `examples/nixos.nix` and holds no copy of it.

## 4. Integration

- [ ] 4.1 Run `nix fmt` and verify that no file changes.
- [ ] 4.2 Run `go test ./...` and verify that all tests pass.
- [ ] 4.3 Run `nix flake check` and verify that all checks pass, including `nixos-module`.
