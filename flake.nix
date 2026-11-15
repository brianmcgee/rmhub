{
  description = "rmhub: a self-hosted alternative to the reMarkable cloud";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs?ref=nixos-unstable";
    blueprint.url = "github:numtide/blueprint";
    blueprint.inputs.nixpkgs.follows = "nixpkgs";
    treefmt-nix.url = "github:numtide/treefmt-nix";
    treefmt-nix.inputs.nixpkgs.follows = "nixpkgs";
  };

  # blueprint adds a top-level __functor that calls self.lib.__functor.
  # rmhub has no lib, so the call fails. nix-eval-jobs calls the functor
  # when it evaluates the flake root, so remove it. Upstream issue:
  # https://github.com/numtide/blueprint/issues/110
  #
  # rmhub is a Linux server, so build only for Linux.
  outputs =
    inputs:
    builtins.removeAttrs (inputs.blueprint {
      inherit inputs;
      systems = [
        "aarch64-linux"
        "x86_64-linux"
      ];
    }) [ "__functor" ];
}
