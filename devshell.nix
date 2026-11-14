{ pkgs, perSystem }:
pkgs.mkShell {
  packages = [
    pkgs.go
    pkgs.gopls
    pkgs.gotools
    pkgs.golangci-lint
    pkgs.delve
    perSystem.self.formatter
  ];

  env.CGO_ENABLED = "0";
}
