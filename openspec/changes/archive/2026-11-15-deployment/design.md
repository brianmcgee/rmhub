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

## Implementation notes

### Decisions

- **The example has no `tablet.announced_host`.** The discovery host must
  resolve to the proxy on the tablet, so the default `local.appspot.com`
  is correct for rmfakecloud-proxy. The proxy `upstream` points at
  rmhub, not the announced host.
- **The example uses ACME with the HTTP-01 challenge** (`listenHTTP =
  ":80"`) and opens port 80. The VM has no internet, so the test replaces
  `security.acme.certs` with an empty set and gives the test certificate
  through `credentials`.
- **Extra hardening.** The module sets more options than the spec names:
  `PrivateDevices`, `ProtectKernel*`, `ProtectProc=invisible`,
  `SystemCallFilter=@system-service ~@privileged ~@resources` and
  `UMask=0077`, among others. `systemd-analyze security` gives the
  exposure level 1.3 (OK) in the VM test log.
- **`openFirewall` opens both listener ports**, as the spec says, also
  when the API listener is on the loopback interface. The port comes from
  the text after the last `:` of the address.
- **The VM test renders a PDF page without strokes.** rmapi cannot add a
  `.rm` file. The render goes through PDFium in wazero, which is the part
  that the hardening can break.
- **The test PDF** is written by a small Python script with correct
  cross-reference offsets. The test CA and the server key are in the Nix
  store, which is acceptable for a test only.

### Finding for review: MemoryDenyWriteExecute

The design says that `MemoryDenyWriteExecute` breaks wazero. A throwaway
run of the VM test with `MemoryDenyWriteExecute = true` rendered the PDF
page with no error (2.4 s). The cause is unknown. [INFERENCE] wazero
can map its code in a way that the seccomp filter of systemd does not
block, or it falls back to its interpreter. The module follows the spec
and does not set the option. Enabling it needs a check of the mappings
of the running process first.

### Verification

- `nix eval .#nixosModules` lists `rmhub`.
- An evaluation of the VM test node shows `DynamicUser`,
  `StateDirectory`, `CacheDirectory`, `LoadCredential`,
  `Restart=on-failure`, each named hardening option, no
  `MemoryDenyWriteExecute`, and the firewall ports `[ 80 8081 8443 ]`
  for the tablet address `0.0.0.0:8443`.
- `checks.x86_64-linux.nixos-module` passes: health on both listeners,
  user, token and pairing code with the CLI, rmapi pairing through TLS
  with the test CA, rmapi upload, `rmhub render` of page 1 (a 1404-pixel
  PNG), restart, rmapi listing, and no admin token in the generated TOML.
- The nginx example of the guide passes `nginx -t` (port changed to
  18443 for an unprivileged run).
- The rmhub commands of the switch section ran against a local server.
