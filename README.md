# rmhub

rmhub is a self-hosted alternative to the reMarkable cloud service.
It syncs documents between reMarkable tablets and the desktop and mobile apps.
You keep your notes on a server that you control.

## Status

Early development. Nothing works yet.

## Development

The repository is a Nix flake that uses
[blueprint](https://github.com/numtide/blueprint) and
[treefmt-nix](https://github.com/numtide/treefmt-nix).

```console
nix develop          # Go toolchain, gopls, golangci-lint
nix build .#rmhub    # build the server
nix fmt              # format Nix and Go code
nix flake check      # run the formatter and lint checks
```

With direnv, run `direnv allow` once to load the devshell.

## Specifications

Changes go through [OpenSpec](https://github.com/Fission-AI/OpenSpec).
The specs are in `openspec/specs/`. The proposed changes are in
`openspec/changes/`.
