# Spec Delta

## Purpose

The NixOS module runs rmhub as a systemd service. It keeps secrets out of
the Nix store and limits what the service can do on the host.

## ADDED Requirements

### Requirement: Module output
The flake MUST export the module as `nixosModules.rmhub`. The module MUST
have the options `services.rmhub.enable`, `services.rmhub.package`,
`services.rmhub.settings`, `services.rmhub.credentials` and
`services.rmhub.openFirewall`. The default package MUST be the `rmhub`
package of the flake.

#### Scenario: Enable the module
- **WHEN** a host sets `services.rmhub.enable = true`
- **THEN** the host has the systemd service `rmhub.service`

### Requirement: Settings
`services.rmhub.settings` MUST use the type of `pkgs.formats.toml`. The
module MUST write the settings to a TOML file and start the server with
`rmhub serve --config <file>`. The module MUST set the defaults
`data_dir = "/var/lib/rmhub"` and `render.cache_dir = "/var/cache/rmhub/render"`.

#### Scenario: Custom listener
- **WHEN** a host sets `services.rmhub.settings.listen.tablet.address = "0.0.0.0:8443"`
- **THEN** the configuration file has that address in the table `listen.tablet`

### Requirement: Credentials
`services.rmhub.credentials` MUST be an attribute set from a name to a
file path. The module MUST pass each entry to systemd with
`LoadCredential`. The documentation of the option MUST tell the user to
refer to a credential as `/run/credentials/rmhub.service/<name>` in the
settings. The settings MUST NOT contain a secret value.

#### Scenario: Admin token
- **GIVEN** `credentials.admin-token = "/run/secrets/rmhub-admin"`
- **AND** `settings.admin.token_file = "/run/credentials/rmhub.service/admin-token"`
- **WHEN** the service starts
- **THEN** the server reads the admin token, and the token is not in the Nix store

### Requirement: Service user and directories
The service MUST run with `DynamicUser`. It MUST use `StateDirectory`
`rmhub` and `CacheDirectory` `rmhub`. The service MUST restart on
failure.

#### Scenario: Data survives a restart
- **GIVEN** a user and a document exist
- **WHEN** the service restarts
- **THEN** the user and the document still exist

### Requirement: Hardening
The service MUST set `ProtectSystem=strict`, `ProtectHome=true`,
`PrivateTmp=true`, `NoNewPrivileges=true`, `CapabilityBoundingSet=` and
`RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX`. The service MUST NOT
set `MemoryDenyWriteExecute`, because the WebAssembly compiler of the
PDF renderer needs executable memory.

#### Scenario: PDF render under hardening
- **WHEN** a client renders an annotated PDF page on a host with the module
- **THEN** the server answers with status 200

### Requirement: Firewall
When `openFirewall` is `true`, the module MUST open the TCP ports of the
tablet listener and of the API listener. The default MUST be `false`.

#### Scenario: Open the firewall
- **WHEN** a host sets `openFirewall = true` and the tablet address `0.0.0.0:8443`
- **THEN** the firewall allows TCP port 8443

### Requirement: VM test
The flake MUST have the check `nixos-module`. It MUST start a VM with the
module, a self-signed certificate on the tablet listener and an admin
token credential. It MUST pair rmapi through TLS, upload a PDF, restart
the service and list the PDF.

#### Scenario: Check passes
- **WHEN** a developer builds the check `nixos-module`
- **THEN** the build succeeds
