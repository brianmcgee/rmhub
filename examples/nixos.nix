# An example host configuration for rmhub. Import it in a NixOS host
# together with the module `rmhub.nixosModules.rmhub`.
#
# The tablet listener has TLS, because rmfakecloud-proxy on the tablet
# checks the certificate of its upstream. The API listener stays on the
# loopback interface; put a reverse proxy in front of it or use an SSH
# tunnel. The secrets come from files outside the Nix store, for example
# from sops-nix, agenix or clan vars.
_: {
  services.rmhub = {
    enable = true;
    openFirewall = true;

    credentials = {
      admin-token = "/run/secrets/rmhub-admin-token";
      tls-cert = "/var/lib/acme/rmhub.example.org/fullchain.pem";
      tls-key = "/var/lib/acme/rmhub.example.org/key.pem";
    };

    settings = {
      admin.token_file = "/run/credentials/rmhub.service/admin-token";
      listen.tablet = {
        address = "0.0.0.0:8443";
        tls_cert_file = "/run/credentials/rmhub.service/tls-cert";
        tls_key_file = "/run/credentials/rmhub.service/tls-key";
      };
      listen.api.address = "127.0.0.1:8081";
    };
  };

  # A public certificate from Let's Encrypt with the HTTP-01 challenge.
  # The challenge needs TCP port 80 from the internet.
  security.acme = {
    acceptTerms = true;
    defaults.email = "admin@example.org";
    certs."rmhub.example.org" = {
      listenHTTP = ":80";
      # Restart rmhub after each renewal: systemd copies the credentials
      # at service start.
      reloadServices = [ "rmhub.service" ];
    };
  };
  networking.firewall.allowedTCPPorts = [ 80 ];
}
