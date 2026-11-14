{ pkgs, ... }:
let
  inherit (pkgs) lib;
in
pkgs.buildGoModule {
  pname = "rmhub";
  version = "0.0.0-dev";

  src = lib.fileset.toSource {
    root = ../..;
    fileset = lib.fileset.unions [
      ../../go.mod
      (lib.fileset.maybeMissing ../../go.sum)
      ../../cmd
      (lib.fileset.maybeMissing ../../internal)
    ];
  };

  env.CGO_ENABLED = 0;

  vendorHash = "sha256-02nrm461EWGH0xO0Xp23nLMCuOG9d1NhoJL9Enj9jH0=";

  subPackages = [ "cmd/rmhub" ];

  meta = {
    description = "Self-hosted alternative to the reMarkable cloud";
    license = lib.licenses.agpl3Plus;
    mainProgram = "rmhub";
  };
}
