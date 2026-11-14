# Design

## Context

The earlier changes give a server with a TOML configuration, two
listeners with optional TLS, secrets from files and a render cache. The
user runs NixOS hosts. The tablet runs ddvk/rmfakecloud-proxy, which
takes the configuration keys `cert`, `key`, `upstream`, `addr`,
`client_cert` and `client_key`.

Facts from the source of rmfakecloud-proxy (`main.go`):

- The proxy uses `httputil.ReverseProxy`. It sets the `Host` header to
  the upstream host. It passes websocket upgrades.
- It uses the default Go TLS client. So it checks the upstream
  certificate against the trusted roots of the tablet.
- With `client_cert` and `client_key`, it sends a client certificate.
  This makes mTLS possible in a later change with no proxy change.

## Goals / Non-Goals

**Goals:**

- One NixOS option set that runs rmhub with secrets from files and with
  systemd hardening.
- A switch from rmfakecloud that the user can undo by a change of one
  proxy setting.

**Non-Goals:**

- mTLS between the proxy and rmhub. The tablet listener with TLS is the
  first step.
- Container images and packages for other distributions.
- Changes on the tablet other than the proxy configuration.

## Decisions

### Module shape

`modules/nixos/rmhub.nix` follows the RFC 42 style: a free-form
`settings` option with the type of `pkgs.formats.toml`, and a few named
options. The module sets defaults with `lib.mkDefault`, so a host can
override each one. The TOML loader of rmhub refuses unknown keys. So a
typing error in `settings` stops the service with a clear error, and the
module needs no option for each key.

### Credentials

`credentials` maps a name to a path on the host. The module turns each
entry into `LoadCredential=<name>:<path>`. The settings refer to
`/run/credentials/rmhub.service/<name>`. This works with sops-nix,
agenix and clan vars, which all give a file path. The secret file can
be readable by root only, because systemd reads it before the dynamic
user exists.

systemd copies credentials at service start. A renewed certificate
needs a restart. `security.acme.certs.<name>.reloadServices` restarts
the service, because the unit has no `ExecReload`. The tablet loses its
websocket and connects again. [INFERENCE] This is no problem, because
the tablet also connects again after each network change.

### Hardening and WebAssembly

The service sets the common systemd hardening options. It does not set
`MemoryDenyWriteExecute`. wazero compiles the PDFium module to machine
code at runtime and maps that code as executable. With
`MemoryDenyWriteExecute`, this mapping fails. The VM test renders a PDF
page to prove that the setting is correct.

### VM test

`checks/nixos-module.nix` uses `pkgs.testers.runNixOSTest`. The node
imports `examples/nixos.nix`, which is also the example of the guide.
The test makes a self-signed CA and a certificate for `rmhub.test`, and
adds the CA to `security.pki.certificateFiles`. It writes an admin token
file. Then it does these steps:

1. Wait for `/healthz` on both listeners.
2. Create a user, an owner token and a pairing code with the CLI.
3. Pair rmapi with `RMAPI_HOST=https://rmhub.test:8443`.
4. Upload a PDF with rmapi and render page 1 through the API.
5. Restart `rmhub.service` and list the PDF with rmapi.

### Switch procedure

The guide describes this order:

```
backup (rmapi against rmfakecloud)
   │
   ▼
rmhub: user, pairing code
   │
   ▼
tablet: upstream → rmhub, restart proxy
   │
   ▼
tablet: pair with the code ──► first sync uploads the library
   │
   ▼
check: rmhub ls shows the library
```

The way back is the reverse of the proxy step. rmfakecloud keeps its
data, because rmhub does not import from it and the guide tells the user
not to delete it.

## Risks / Trade-offs

- [No pairing prompt after the switch] → The guide tells the user to
  disconnect the tablet from the cloud in its settings. Then the user
  pairs again. [INFERENCE] The tablet has such a
  setting in OS 3.27.1.
- [The first sync uploads the full library] → This takes time on a large
  library. rmhub checks each blob, so a stopped upload leaves no broken
  tree. The tablet continues the upload at the next sync.
- [Changes on the tablet during the rmhub period are not in rmfakecloud]
  → The guide warns about this. After a way back, the tablet syncs its
  local state to rmfakecloud. [INFERENCE] The tablet treats the old
  rmfakecloud root as a remote state and merges with it.
