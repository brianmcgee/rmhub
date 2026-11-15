# Rules for agents

## Subagents

Do not use subagents in this repository. Do all research, planning and
implementation in the main session. The main session keeps the design
context that a subagent does not have.

## Licence

rmhub is licensed under AGPL-3.0-or-later.

- Do not add a dependency that is not compatible with AGPL-3.0-or-later.
- Do not use `unidoc/unipdf`. It has a commercial licence.
- rmfakecloud is a reference for the protocol. Do not copy its code.
  Reuse a small, self-contained piece only for a strong reason. Record
  each reused piece in `NOTICE`.

## Documentation

The user documentation is in `docs/`. It follows the Diátaxis system:
`tutorials/`, `how-to/`, `reference/` and `explanation/`.

- Keep the documentation in sync with the code. A change to
  the behavior that a user sees must update `docs/` in the same commit
  or the same branch. Examples are a configuration key, a command, an
  exit code, an endpoint, a NixOS option and a procedure.
- `docs/README.md` has a table from each type of change to the pages to
  update. Read it before you change the documentation.
- Put each page in one part only. Add each new page to `docs/README.md`.
- Run each command that a page shows, and copy the real output.
- Do not add user documentation to `README.md`. Link to `docs/`.
