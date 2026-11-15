# The NixOS module of rmhub. It writes the TOML configuration, gives the
# secrets to the service as systemd credentials, and runs the server as a
# dynamic user with systemd hardening.
{ flake, ... }:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.rmhub;
  toml = pkgs.formats.toml { };
  configFile = toml.generate "rmhub.toml" cfg.settings;

  # port returns the port of a listener address such as "0.0.0.0:8443"
  # or "[::]:8443".
  port =
    address:
    let
      parts = lib.splitString ":" address;
    in
    lib.toInt (lib.last parts);
in
{
  options.services.rmhub = {
    enable = lib.mkEnableOption "rmhub, a self-hosted alternative to the reMarkable cloud";

    package = lib.mkOption {
      type = lib.types.package;
      default = flake.packages.${pkgs.stdenv.hostPlatform.system}.rmhub;
      defaultText = lib.literalExpression "rmhub.packages.\${system}.rmhub";
      description = "The rmhub package.";
    };

    settings = lib.mkOption {
      inherit (toml) type;
      default = { };
      example = lib.literalExpression ''
        {
          listen.tablet.address = "0.0.0.0:8443";
          admin.token_file = "/run/credentials/rmhub.service/admin-token";
        }
      '';
      description = ''
        The configuration of rmhub serve. The file
        `docs/reference/configuration.md` lists the keys. rmhub
        refuses unknown keys, so a typing error stops the service with an
        error. Do not put a secret value here, because the value goes into
        the Nix store. Give the secret in `credentials`, and refer to it as
        `/run/credentials/rmhub.service/<name>`.
      '';
    };

    credentials = lib.mkOption {
      type = lib.types.attrsOf lib.types.path;
      default = { };
      example = {
        admin-token = "/run/secrets/rmhub-admin";
        tls-cert = "/var/lib/acme/rmhub.example.org/fullchain.pem";
        tls-key = "/var/lib/acme/rmhub.example.org/key.pem";
      };
      description = ''
        Secret files for the service, from a name to a path on the host.
        The module gives each file to systemd with `LoadCredential`. Refer
        to a file in `settings` as `/run/credentials/rmhub.service/<name>`.
        The file can be readable by root only. systemd copies the files at
        service start, so restart the service after a file changes.
      '';
    };

    openFirewall = lib.mkOption {
      type = lib.types.bool;
      default = false;
      description = "Open the TCP ports of the tablet listener and of the API listener.";
    };
  };

  config = lib.mkIf cfg.enable {
    services.rmhub.settings = {
      data_dir = lib.mkDefault "/var/lib/rmhub";
      render.cache_dir = lib.mkDefault "/var/cache/rmhub/render";
      listen.tablet.address = lib.mkDefault "127.0.0.1:8080";
      listen.api.address = lib.mkDefault "127.0.0.1:8081";
    };

    networking.firewall.allowedTCPPorts = lib.mkIf cfg.openFirewall [
      (port cfg.settings.listen.tablet.address)
      (port cfg.settings.listen.api.address)
    ];

    environment.systemPackages = [ cfg.package ];

    systemd.services.rmhub = {
      description = "rmhub, a self-hosted alternative to the reMarkable cloud";
      wantedBy = [ "multi-user.target" ];
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      restartTriggers = [ configFile ];
      serviceConfig = {
        ExecStart = "${lib.getExe' cfg.package "rmhub"} serve --config ${configFile}";
        Restart = "on-failure";
        RestartSec = "5s";

        DynamicUser = true;
        StateDirectory = "rmhub";
        StateDirectoryMode = "0700";
        CacheDirectory = "rmhub";
        CacheDirectoryMode = "0700";
        LoadCredential = lib.mapAttrsToList (name: path: "${name}:${path}") cfg.credentials;

        # Hardening. MemoryDenyWriteExecute stays off: wazero compiles the
        # PDFium module to machine code and maps that code as executable.
        ProtectSystem = "strict";
        ProtectHome = true;
        PrivateTmp = true;
        PrivateDevices = true;
        NoNewPrivileges = true;
        CapabilityBoundingSet = "";
        AmbientCapabilities = "";
        RestrictAddressFamilies = [
          "AF_INET"
          "AF_INET6"
          "AF_UNIX"
        ];
        RestrictNamespaces = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        LockPersonality = true;
        ProtectClock = true;
        ProtectControlGroups = true;
        ProtectHostname = true;
        ProtectKernelLogs = true;
        ProtectKernelModules = true;
        ProtectKernelTunables = true;
        ProtectProc = "invisible";
        ProcSubset = "pid";
        SystemCallArchitectures = "native";
        SystemCallFilter = [
          "@system-service"
          "~@privileged"
          "~@resources"
        ];
        UMask = "0077";
      };
    };
  };
}
