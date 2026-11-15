# rmhub documentation

The documentation follows the [Diátaxis](https://diataxis.fr) system.
Each page has one of four purposes. Select the part that agrees with
your need.

| Part | Purpose | Use it when |
|---|---|---|
| [Tutorials](#tutorials) | A lesson with a known result. | You are new to rmhub. |
| [How-to guides](#how-to-guides) | The steps to one goal. | You know rmhub and have a task. |
| [Reference](#reference) | The facts: keys, commands, endpoints. | You must find an exact name or value. |
| [Explanation](#explanation) | The design and its reasons. | You want to understand rmhub. |

## Tutorials

- [Run your first server](tutorials/first-server.md): start a server on
  your computer, create an account, and upload and render a document.

## How-to guides

- [Connect a tablet](how-to/connect-a-tablet.md)
- [Deploy on NixOS](how-to/deploy-on-nixos.md), with TLS for the tablet
- [Switch from rmfakecloud](how-to/switch-from-rmfakecloud.md)
- [Store data in S3](how-to/store-data-in-s3.md)
- [Give an agent access to the library](how-to/give-an-agent-access.md)

## Reference

- [Server configuration](reference/configuration.md)
- [Command line](reference/cli.md)
- [HTTP API](reference/http-api.md)
- [NixOS module](reference/nixos-module.md)
- [Guide for AI agents](https://github.com/brianmcgee/rmhub/blob/main/skills/rmhub/SKILL.md):
  a skill file that you give to an agent together with a scoped token.

## Explanation

- [Architecture](explanation/architecture.md)
- [Tokens and access](explanation/access.md)
- [Request limits](explanation/request-limits.md)

## Keep the documentation correct

The documentation must agree with the code. A change to the
behavior that a user sees must update the documentation in the same
branch. Use this table to find the pages.

| Change | Pages to update |
|---|---|
| A configuration key, a default value or the behavior of `rmhub serve` | [reference/configuration.md](reference/configuration.md) |
| A command, an option, the output or an exit code | [reference/cli.md](reference/cli.md), `skills/rmhub/SKILL.md` |
| An endpoint of the rmhub API or the error format | [reference/http-api.md](reference/http-api.md) |
| An option of the NixOS module | [reference/nixos-module.md](reference/nixos-module.md) |
| A setup procedure or a deployment procedure | The tutorial and the how-to guides that contain the procedure |
| The design: storage, sync, tokens, limits | The pages in [explanation/](explanation/) |

Rules for each page:

- Put a page in one part only. Do not mix a procedure into a reference
  page, and do not mix design reasons into a how-to guide. Link to the
  other part.
- Add each new page to this index and to `SUMMARY.md`. `SUMMARY.md` is
  the table of contents of the documentation site.
- Run each command that a page shows, and copy the real output.
- State each fact one time. Other pages link to it.
- Write in ASD-STE100 Simplified Technical English.
