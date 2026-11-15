{ pkgs, ... }:
let
  inherit (pkgs) lib;
in
# Use the Go release that the `go` line of go.mod names. When that line
# changes, change the release here, in devshell.nix and in checks/.
pkgs.buildGo127Module {
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

  vendorHash = "sha256-Nt/A8wV+YDdjnbBZN8w6FALzgyDfPx+vGDkZHVncBdM=";

  subPackages = [ "cmd/rmhub" ];

  meta = {
    description = "Self-hosted alternative to the reMarkable cloud";
    license = lib.licenses.agpl3Plus;
    mainProgram = "rmhub";
  };
}
