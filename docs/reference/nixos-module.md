# NixOS module

The flake exports the module as `nixosModules.rmhub`.
[Deploy on NixOS](../how-to/deploy-on-nixos.md) shows how to use it.

## Options

| Option | Default | Meaning |
| --- | --- | --- |
| `services.rmhub.enable` | `false` | Run `rmhub serve` as `rmhub.service`. |
| `services.rmhub.package` | the flake package | The rmhub package. |
| `services.rmhub.settings` | `{ }` | The TOML configuration (see [Server configuration](configuration.md#configuration-keys)). |
| `services.rmhub.credentials` | `{ }` | Secret files, from a name to a path on the host. Refer to a file in `settings` as `/run/credentials/rmhub.service/<name>`. |
| `services.rmhub.openFirewall` | `false` | Open the TCP ports of both listeners. |

## Defaults that the module sets

| Key in `settings` | Value |
|---|---|
| `data_dir` | `/var/lib/rmhub` |
| `render.cache_dir` | `/var/cache/rmhub/render` |
| `listen.tablet.address` | `127.0.0.1:8080` |
| `listen.api.address` | `127.0.0.1:8081` |

## Secrets

Do not put a secret in `settings`: the settings go into the Nix store.
The module gives each file in `credentials` to systemd with
`LoadCredential`. The secret files can be readable by root only, because
systemd reads them before the service user exists. systemd copies the
files at service start, so restart the service after a file changes.

## Service

The service runs as a dynamic user with systemd hardening. It does not
set `MemoryDenyWriteExecute`, because the PDF renderer compiles
WebAssembly to machine code. The module adds the package to
`environment.systemPackages`.
