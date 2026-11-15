# Spec Delta

## MODIFIED Requirements

### Requirement: Firewall
When `openFirewall` is `true`, the module MUST open the TCP port of the
tablet listener. It MUST also open the TCP port of the API listener when
the settings have `listen.api.address`. The default MUST be `false`. The
module MUST NOT set a default for `listen.api.address`.

#### Scenario: Open the firewall
- **WHEN** a host sets `openFirewall = true` and the tablet address `0.0.0.0:8443`
- **THEN** the firewall allows TCP port 8443

#### Scenario: Open the firewall for two listeners
- **WHEN** a host sets `openFirewall = true`, the tablet address `0.0.0.0:8443` and the API address `0.0.0.0:8444`
- **THEN** the firewall allows the TCP ports 8443 and 8444

#### Scenario: No API listener by default
- **WHEN** a host sets only `services.rmhub.enable = true`
- **THEN** the configuration file has no table `listen.api`
