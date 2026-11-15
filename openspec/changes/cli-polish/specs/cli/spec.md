# Spec Delta

## MODIFIED Requirements

### Requirement: Exit codes
The client MUST exit with these codes:

- 0: success.
- 1: an unexpected error.
- 2: a usage error, a setting error or status 400.
- 3: status 401 or 403.
- 4: status 404.
- 5: status 409 or 412, or an output file that exists.
- 6: status 429.

For status 429, the error message MUST give the number of seconds of
the `Retry-After` header. The client MUST NOT send the request again.

For status 404 without the error body of the rmhub API, the client MUST
exit with code 1. The error message MUST tell that the server at the URL
is not an rmhub API.

When the client cannot connect to the server, it MUST exit with code 1.
The error message MUST name the server URL.

#### Scenario: Unknown device
- **WHEN** a user removes a device ID that does not exist
- **THEN** the command exits with status 4

#### Scenario: Bad token
- **WHEN** a user runs a command with a revoked token
- **THEN** the command exits with status 3

#### Scenario: Request above a limit
- **WHEN** a user runs a command and the server answers with status 429 and `Retry-After: 7`
- **THEN** the command exits with status 6
- **AND** the error message contains `7 seconds`

#### Scenario: Request that is not valid
- **WHEN** a user runs `rmhub render /Paper --format svg` and page 1 of `Paper` is a PDF page
- **THEN** the server answers with status 400 and the command exits with status 2

#### Scenario: URL of the tablet listener
- **GIVEN** `RMHUB_URL` is the address of the tablet listener
- **WHEN** a user runs `rmhub ls`
- **THEN** the command exits with status 1 and the message tells that the URL is not an rmhub API

#### Scenario: No server at the URL
- **GIVEN** no program listens at the address of `RMHUB_URL`
- **WHEN** a user runs `rmhub ls`
- **THEN** the command exits with status 1 and the message contains the URL

### Requirement: Admin commands
The client MUST have these admin commands:

- `rmhub admin users add --email <email> [--name <name>]`
- `rmhub admin users list`
- `rmhub admin tokens issue <account> [--label <label>] [--expires <time>]`
- `rmhub admin tokens generate`

#### Scenario: Add a user and issue a token
- **WHEN** an operator runs `rmhub admin users add --email ada@example.org` and then `rmhub admin tokens issue <id>`
- **THEN** the second command prints an owner token for the new account

#### Scenario: Old command name
- **WHEN** an operator runs `rmhub admin user list`
- **THEN** the command exits with status 2

### Requirement: Help text
Each command MUST have help text that gives its purpose, its arguments,
its options, its JSON output form and an example. The help of `rmhub`
MUST show the commands in these groups: library, account, admin and
server.

#### Scenario: Help for a command
- **WHEN** a user runs `rmhub devices remove --help`
- **THEN** the output shows the purpose, the argument, the options and an example

#### Scenario: Command groups
- **WHEN** a user runs `rmhub --help`
- **THEN** the output shows `ls` in the library group and `serve` in the server group

## ADDED Requirements

### Requirement: Server URL check
Before a request, the client MUST check that the server URL has the
scheme `http` or `https` and a host. For a different value, the command
MUST exit with code 2. The error message MUST show the value and an
example of a correct URL.

#### Scenario: URL without a scheme
- **GIVEN** `RMHUB_URL` is `127.0.0.1:8080`
- **WHEN** a user runs `rmhub ls`
- **THEN** the command exits with status 2 and the message contains `http://`

### Requirement: Usage errors
For a wrong number of arguments, an unknown option or an unknown
command, the text of the error MUST contain the usage line of the
command and the text `--help`. With `--json`, the client MUST print the
error as JSON, as for each other client error.

#### Scenario: Too many arguments
- **WHEN** a user runs `rmhub find tax 2025`
- **THEN** the command exits with status 2
- **AND** standard error contains `rmhub find <text>` and `--help`

### Requirement: Expiry values
The option `--expires` MUST accept an RFC 3339 time, a duration with the
units `h`, `m` and `s`, and a number of days with the unit `d`. For a
different value, the command MUST exit with code 2.

#### Scenario: Days
- **WHEN** a user runs `rmhub tokens create --expires 30d`
- **THEN** the token expires 720 hours after the command

#### Scenario: Unknown unit
- **WHEN** a user runs `rmhub tokens create --expires 2y`
- **THEN** the command exits with status 2

### Requirement: Login command
`rmhub login` MUST read the server URL and the token from the same
sources as each other command. It MUST check them with one request to
the server. After a successful check, it MUST write both values to
`client.toml` with the file mode 0600 and MUST print the path of the
file. After a failed check, it MUST NOT change the file. The output MUST
NOT contain the token. With `--no-check`, the command MUST NOT send the
request.

#### Scenario: Save the settings
- **GIVEN** `RMHUB_TOKEN` is a valid owner token and no `client.toml` exists
- **WHEN** a user runs `rmhub login --url http://127.0.0.1:8080`
- **THEN** `client.toml` exists with the mode 0600 and contains the URL and the token
- **AND** `rmhub ls` then works without `RMHUB_URL` and `RMHUB_TOKEN`

#### Scenario: Token that is not valid
- **GIVEN** `client.toml` has a valid token
- **WHEN** a user runs `rmhub login --token rmh_bad`
- **THEN** the command exits with status 3 and `client.toml` does not change

#### Scenario: Admin token
- **GIVEN** `RMHUB_TOKEN` is the admin token of the server
- **WHEN** an operator runs `rmhub login --url http://127.0.0.1:8080`
- **THEN** the command exits with status 0 and `client.toml` contains the admin token

### Requirement: Admin token hint
When a command outside `rmhub admin` gets status 403 and the token is
the admin token, the error message MUST name the command
`rmhub admin tokens issue`.

#### Scenario: Library command with the admin token
- **GIVEN** `RMHUB_TOKEN` is the admin token
- **WHEN** an operator runs `rmhub ls`
- **THEN** the command exits with status 3 and the message contains `rmhub admin tokens issue`

### Requirement: Path completion
The shell completion of an entry argument MUST offer the paths of the
entries in the folder that the typed text names. It MUST get the paths
from the server with the client settings. When the server does not
answer in 2 seconds, or answers with an error, the completion MUST offer
nothing and MUST print nothing.

#### Scenario: Complete a document
- **GIVEN** the folder `Inbox` has the documents `paper` and `plan`
- **WHEN** a user types `rmhub stat /Inbox/p` and presses the tab key
- **THEN** the shell offers `/Inbox/paper` and `/Inbox/plan`

#### Scenario: No server
- **GIVEN** no program listens at the address of `RMHUB_URL`
- **WHEN** a user types `rmhub stat /In` and presses the tab key
- **THEN** the shell offers nothing and shows no error
