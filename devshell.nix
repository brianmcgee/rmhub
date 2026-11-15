{ pkgs, perSystem }:
pkgs.mkShell {
  packages = [
    pkgs.go_1_27
    pkgs.gopls
    pkgs.gotools
    pkgs.golangci-lint
    pkgs.delve
    pkgs.mdbook
    perSystem.self.formatter
  ];

  env.CGO_ENABLED = "0";
}
