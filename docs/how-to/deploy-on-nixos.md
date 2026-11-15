# Deploy on NixOS

This guide runs rmhub as a systemd service on a NixOS host, with TLS
that the tablet accepts.

## Import the module

The flake exports the NixOS module `nixosModules.rmhub`. The file
[`examples/nixos.nix`](../../examples/nixos.nix) is a full host
configuration. It has TLS on the tablet listener with a certificate from
Let's Encrypt, and the admin token from a secret file. The VM test
`checks/nixos-module.nix` imports that file, so the example stays
correct.

Import the module and the example (or your copy of it) in a host:

```nix
{
  inputs.rmhub.url = "github:brianmcgee/rmhub";
  # in the host configuration:
  #   imports = [ inputs.rmhub.nixosModules.rmhub ./rmhub.nix ];
}
```

Do not put a secret in `services.rmhub.settings`: the settings go into
the Nix store. Give each secret file in `services.rmhub.credentials`.
[NixOS module](../reference/nixos-module.md) lists the options.

## TLS for the tablet

rmfakecloud-proxy on the tablet checks the certificate of its upstream
against the trusted roots of the tablet. A self-signed certificate
fails, unless you add its CA to the tablet. Use one of these setups.

### A public certificate on the tablet listener

`examples/nixos.nix` gets a certificate through `security.acme` and
gives it to rmhub as the credentials `tls-cert` and `tls-key`. systemd
copies credentials at service start, so rmhub must restart after each
renewal:

```nix
security.acme.certs."rmhub.example.org".reloadServices = [ "rmhub.service" ];
```

The tablet loses its notification connection at the restart and
connects again.

### A reverse proxy in front of a plain tablet listener

Keep the tablet listener on the loopback interface without TLS, and let
nginx terminate TLS. The proxy must pass websocket upgrades, because the
tablet gets change notifications through a websocket. The proxy must
also allow bodies of at least 256 MiB:

```nginx
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 443 ssl;
    server_name rmhub.example.org;
    ssl_certificate     /var/lib/acme/rmhub.example.org/fullchain.pem;
    ssl_certificate_key /var/lib/acme/rmhub.example.org/key.pem;

    client_max_body_size 256m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_read_timeout 1h;
        proxy_request_buffering off;
    }
}
```

With this setup, the `upstream` of the proxy on the tablet is
`https://rmhub.example.org` (port 443).

## Give the client address to rmhub

Behind a reverse proxy, rmhub sees only the address of the proxy. Then
all clients share one bucket of the pairing limit (see
[Request limits](../explanation/request-limits.md)). The
`X-Forwarded-For` line above gives the client address to rmhub. Tell
rmhub to trust that header from the proxy:

```toml
[listen.tablet]
address = "127.0.0.1:8080"
trusted_proxies = ["127.0.0.1"]
```

List only the addresses of your own proxies. rmhub believes the
`X-Forwarded-For` header of each address in the list. With a wrong
entry, for example `0.0.0.0/0`, each client can choose its own address,
and the pairing limit has no effect.

## Next steps

Create an account and an owner token as in
[Run your first server](../tutorials/first-server.md), steps 3 and 4.
Then [connect a tablet](connect-a-tablet.md).
