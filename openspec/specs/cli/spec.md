# cli Specification

## Purpose
The `rmhub` command is the client of the rmhub API for people, scripts
and AI agents. Its rules make it safe to use without a person at the
keyboard.

## Requirements

### Requirement: Client settings
The client MUST read the server URL from `RMHUB_URL` and the token from
`RMHUB_TOKEN`. When a variable is not set, the client MUST read the value
from `$XDG_CONFIG_HOME/rmhub/client.toml`, with `~/.config` when
`XDG_CONFIG_HOME` is not set. The options `--url` and `--token` MUST
override both sources.

#### Scenario: Token from the environment
- **GIVEN** `RMHUB_TOKEN` is set and `client.toml` has a different token
- **WHEN** a user runs `rmhub devices list`
- **THEN** the client sends the token from `RMHUB_TOKEN`

#### Scenario: No server URL
- **WHEN** a user runs `rmhub devices list` and no source gives a URL
- **THEN** the command exits with status 2 and names `RMHUB_URL`

### Requirement: Non-interactive commands
A client command MUST NOT wait for input from a terminal. A command that
needs a value MUST take it from an argument or an option.

#### Scenario: Command without a terminal
- **WHEN** a program runs `rmhub admin user add --email ada@example.org` with no terminal
- **THEN** the command completes without a prompt

### Requirement: Output formats
Each client command MUST print text for people by default. With `--json`,
each command MUST print one JSON value to standard output. Errors MUST go
to standard error. With `--json`, an error MUST also print the error body
of the API as JSON to standard error.

#### Scenario: JSON output
- **WHEN** a program runs `rmhub devices list --json`
- **THEN** standard output contains one JSON array

#### Scenario: JSON error
- **WHEN** a program runs `rmhub devices remove X --json` and the device X does not exist
- **THEN** standard error contains `{"error":{"code":"not_found",…}}`

### Requirement: Exit codes
The client MUST exit with these codes:

- 0: success.
- 1: an unexpected error.
- 2: a usage error or a setting error.
- 3: status 401 or 403.
- 4: status 404.
- 5: status 409 or 412.

#### Scenario: Unknown device
- **WHEN** a user removes a device ID that does not exist
- **THEN** the command exits with status 4

#### Scenario: Bad token
- **WHEN** a user runs a command with a revoked token
- **THEN** the command exits with status 3

### Requirement: Admin commands
The client MUST have these admin commands:

- `rmhub admin user add --email <email> [--name <name>]`
- `rmhub admin user list`
- `rmhub admin token issue <account> [--label <label>] [--expires <time>]`
- `rmhub admin token generate`

#### Scenario: Add a user and issue a token
- **WHEN** an operator runs `rmhub admin user add --email ada@example.org` and then `rmhub admin token issue <id>`
- **THEN** the second command prints an owner token for the new account

### Requirement: Account commands
The client MUST have these account commands:

- `rmhub account`
- `rmhub pair`
- `rmhub devices list` and `rmhub devices remove <id>`
- `rmhub tokens list`, `rmhub tokens create [--label <label>] [--expires <time>]` and `rmhub tokens revoke <id>`

#### Scenario: Pair a tablet
- **WHEN** a user runs `rmhub pair`
- **THEN** the command prints an 8-letter code and its expiry time

### Requirement: Help text
Each command MUST have help text that gives its purpose, its arguments,
its options, its JSON output form and an example.

#### Scenario: Help for a command
- **WHEN** a user runs `rmhub devices remove --help`
- **THEN** the output shows the purpose, the argument, the options and an example
