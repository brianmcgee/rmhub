# rmhub

> [!WARNING]
> This is LLM-ware. I'm playing around with some ideas. Use at your own risk.

rmhub is a self-hosted alternative to the reMarkable cloud service.
It syncs documents between reMarkable tablets and the desktop and mobile apps.
You keep your notes on a server that you control.

## Status

Early development. The server has accounts, API tokens, tablet pairing
and Sync 1.5 for tablets, with change notifications. It has a library
API with scoped tokens and a change feed for scripts and agents. It
renders notebook pages and annotated PDF pages as images. It does not
render EPUB pages.

## Documentation

Read the documentation at <https://brianmcgee.github.io/rmhub/>. The
source is in [`docs/`](docs/README.md). It follows the
[Diátaxis](https://diataxis.fr) system:

| Part | Start here |
|---|---|
| Tutorial | [Run your first server](docs/tutorials/first-server.md) |
| How-to guides | [Connect a tablet](docs/how-to/connect-a-tablet.md), [Deploy on NixOS](docs/how-to/deploy-on-nixos.md), [Switch from rmfakecloud](docs/how-to/switch-from-rmfakecloud.md), [Store data in S3](docs/how-to/store-data-in-s3.md), [Give an agent access to the library](docs/how-to/give-an-agent-access.md) |
| Reference | [Server configuration](docs/reference/configuration.md), [Command line](docs/reference/cli.md), [HTTP API](docs/reference/http-api.md), [NixOS module](docs/reference/nixos-module.md) |
| Explanation | [Architecture](docs/explanation/architecture.md), [Tokens and access](docs/explanation/access.md), [Request limits](docs/explanation/request-limits.md) |

[`skills/rmhub/SKILL.md`](skills/rmhub/SKILL.md) is a guide for AI
agents. Give it to an agent together with a scoped token.

## Development

The repository is a Nix flake that uses
[blueprint](https://github.com/numtide/blueprint) and
[treefmt-nix](https://github.com/numtide/treefmt-nix).

```console
nix develop          # Go toolchain, gopls, golangci-lint, mdbook
nix build .#rmhub    # build the server
nix run .#docs       # serve the documentation site on port 8000
nix fmt              # format Nix and Go code
nix flake check      # run the formatter, lint and S3 contract checks
```

The S3 contract tests run when `RMHUB_TEST_S3_ENDPOINT` is set. The
check `s3-contract` runs them against versitygw in the build sandbox.
Other variables: `RMHUB_TEST_S3_BUCKET`, `RMHUB_TEST_S3_ACCESS_KEY_ID`,
`RMHUB_TEST_S3_SECRET_ACCESS_KEY` and `RMHUB_TEST_S3_REGION`.

```console
go test ./...
```

With direnv, run `direnv allow` once to load the devshell.

[mdBook](https://rust-lang.github.io/mdBook/) builds the documentation
site from `docs/`. `nix run .#docs` builds the site and serves it on
<http://127.0.0.1:8000>. Give a different port as an argument. In the
devshell, `mdbook serve` shows the site on <http://localhost:3000> and
builds it again after each change. A push to `main` publishes the site
to GitHub Pages.

## Specifications

Changes go through [OpenSpec](https://github.com/Fission-AI/OpenSpec).
The specs are in `openspec/specs/`. The proposed changes are in
`openspec/changes/`.

A change to the behavior that a user sees must update the
documentation in the same branch. See
[Keep the documentation correct](docs/README.md#keep-the-documentation-correct).

## Licence

rmhub is free software under the GNU Affero General Public License,
version 3 or later. See `LICENSE`.
