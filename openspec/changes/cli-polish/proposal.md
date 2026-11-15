# Proposal

## Why

A test of each command against a local server showed many small
problems for a person at the keyboard. Error messages show internal
text, such as the name of a temporary file or a Go URL error. Some exit
codes are wrong: status 400 gives code 1, and a wrong server address
gives code 4. A user must write `client.toml` by hand. Most commands
take one argument only, and the help is one flat list of 21 commands.

## What Changes

Local files:

- The client expands `~` at the start of a local file argument.
- `-o` accepts a directory. A directory that does not exist gives a
  clear error.
- **BREAKING**: `get` and `render` exit with code 5, not 1, when the
  output file exists. The new option `--force` replaces the file.
- `render` uses page 1 without `--page`, and `-p` is the short form.
- `upload -` reads the document from standard input.
- `stat`, `get`, `upload`, `trash` and `restore` accept more than one
  argument.

Errors and exit codes:

- **BREAKING**: Status 400 gives exit code 2, not 1.
- **BREAKING**: Status 404 without the error body of the rmhub API
  gives exit code 1, not 4. The message tells that the URL is not an
  rmhub API.
- The client checks the form of the server URL before the request.
- A usage error shows the usage line of the command.
- `--expires` accepts days, such as `30d`.
- A command that uses the admin token for an account endpoint tells how
  to get an account token.

Structure:

- The new command `rmhub login` checks the URL and the token and writes
  `client.toml` with mode 0600.
- **BREAKING**: `admin user` becomes `admin users` and `admin token`
  becomes `admin tokens`. All command groups then use the plural form.
- The help shows the commands in groups.
- Shell completion completes an entry argument with paths from the
  server.
- **BREAKING**: The write commands print one line in text mode, not the
  full entry. The JSON output does not change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cli`: exit codes, admin command names, help text, the server URL
  check, usage errors, expiry values, the login command, the admin
  token hint and shell completion.
- `library-cli`: more than one argument, upload from standard input,
  the output file rules, local file paths and the text of write
  commands.
- `render-api`: the default page, the output file rules and the order
  of the checks of `rmhub render`.

## Impact

- Code: all files in `cmd/rmhub`. The server does not change.
- Scripts that read exit code 1 for an existing file or for status 400
  must change. Scripts that use `admin user` or `admin token` must
  change. Scripts that read the text output of a write command must
  use `--json`.
- The NixOS module and the examples: it is not verified that they call
  `admin user` or `admin token`. The implementation must search
  `modules/`, `examples/` and `nix/` for these commands.
- Pages in `docs/` to update: `docs/reference/cli.md`,
  `docs/tutorials/first-server.md`, `docs/how-to/deploy-on-nixos.md`,
  `docs/how-to/give-an-agent-access.md`, `docs/explanation/access.md`
  and `docs/reference/configuration.md`. The guide
  `skills/rmhub/SKILL.md` also changes.
- This change builds on the change `library-path-ergonomics`. Its
  delta of `library-cli` contains the command list of that change.
  Apply and archive `library-path-ergonomics` first.
- No machine or clan service is affected.
