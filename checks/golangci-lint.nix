{ pkgs, perSystem, ... }:
let
  inherit (pkgs) lib;
  inherit (perSystem.self) rmhub;
  hasVendor = rmhub.vendorHash != null;
in
pkgs.runCommand "golangci-lint-check"
  {
    nativeBuildInputs = [
      pkgs.go_1_27
      pkgs.golangci-lint
    ];
  }
  ''
    export HOME=$(mktemp -d)
    export GOCACHE=$TMPDIR/gocache
    export GOLANGCI_LINT_CACHE=$TMPDIR/golangci-lint
    export GOPROXY=off
    export CGO_ENABLED=0

    # Copy the source to a writable location.
    workdir=$(mktemp -d)
    cp -r ${rmhub.src}/* "$workdir"/
    chmod -R u+w "$workdir"
    cd "$workdir"

    ${lib.optionalString hasVendor ''
      # Use the vendored modules when vendorHash is set.
      mkdir -p vendor
      cp -r ${rmhub.goModules}/* vendor/
      export GOFLAGS=-mod=vendor
    ''}

    golangci-lint run ./...

    touch $out
  ''
