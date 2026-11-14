# Proposal

## Why

The user runs services on NixOS. rmhub needs a NixOS module that runs it
safely with secrets from files. The user also needs a guide to point the
rmfakecloud-proxy on the tablet at rmhub, and to go back to rmfakecloud
if a problem occurs.

## What Changes

- Add the NixOS module `nixosModules.rmhub`. It writes the configuration
  with `pkgs.formats.toml`, passes secrets as systemd credentials, and
  runs the server as a dynamic user with systemd hardening.
- Add a NixOS VM test that starts the module, pairs rmapi through TLS,
  and checks that the data stays after a restart.
- Add a deployment guide to `README.md`. It covers TLS for the tablet
  listener and the proxy configuration on the tablet. It also covers the
  backup, the switch from rmfakecloud and the way back.

## Capabilities

### New Capabilities

- `nixos-module`: the options and the systemd service of the module.
- `deployment-guide`: the content of the deployment guide.

### Modified Capabilities

None.

## Impact

- New files `modules/nixos/rmhub.nix` and `checks/nixos-module.nix`.
- The guide tells the user to change the proxy configuration on the
  tablet. This step is manual and is outside rmhub.
- Unknown: whether the tablet asks for a new pairing after the proxy
  upstream changes. [INFERENCE] The old device token has a signature
  that rmhub does not accept, so the tablet gets status 401 and asks for
  a new pairing. The guide describes this and the fallback.
- No machine or clan service is affected in this repository. A later
  change in the fleet repository can enable the module.
