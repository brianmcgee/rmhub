# library-cli Specification

## Purpose
The CLI is the interface for people, scripts and AI agents. Each library
operation of the rmhub API has a CLI command with stable JSON output. So
an agent can use rmhub with a shell only.

## Requirements

### Requirement: Entry arguments
Each command that takes an entry MUST accept an ID or a path. An argument
that starts with `/` MUST be a path, and the client MUST resolve it
through the API. Each other argument MUST be an ID.

#### Scenario: Ambiguous path
- **GIVEN** the top level has two documents with the name `Notes`
- **WHEN** a user runs `rmhub stat /Notes`
- **THEN** the command exits with code 5 and prints both IDs

### Requirement: Library commands
The client MUST have these commands:

- `rmhub ls [entry]`
- `rmhub stat <entry>`
- `rmhub find <text>`
- `rmhub get <entry> [-o <file>] [--archive]`
- `rmhub upload <file> [--to <entry>] [--name <name>]`
- `rmhub mkdir <name> [--in <entry>]`
- `rmhub mv <entry> <folder>`
- `rmhub rename <entry> <name>`
- `rmhub trash <entry>`

#### Scenario: Upload into a folder
- **WHEN** a user runs `rmhub upload paper.pdf --to /Inbox`
- **THEN** the document `paper` is in the folder `Inbox`

### Requirement: Upload media type
`rmhub upload` MUST send a file with the extension `.pdf` as a PDF and a
file with the extension `.epub` as an EPUB. For other extensions it MUST
exit with code 2. The default name MUST be the file name without the
extension.

#### Scenario: Unknown extension
- **WHEN** a user runs `rmhub upload notes.txt`
- **THEN** the command exits with code 2

### Requirement: Download output
`rmhub get` MUST write the source file to `-o`, or to the entry name with
the extension of the type. With `--archive` it MUST write the archive of
the document with the extension `.zip`. With `-o -` it MUST write the
bytes to standard output. The command MUST NOT replace an existing file.

#### Scenario: Existing file
- **GIVEN** the file `Paper.pdf` exists
- **WHEN** a user runs `rmhub get /Paper`
- **THEN** the command exits with code 1 and does not change `Paper.pdf`

### Requirement: Conditional option
The commands `mv`, `rename` and `trash` MUST accept `--if-match <hash>`
and send it as the `If-Match` header.

#### Scenario: Stale hash
- **WHEN** a user runs `rmhub rename D New --if-match h1` and the hash of D is not `h1`
- **THEN** the command exits with code 5

### Requirement: Watch command
`rmhub watch [--after <g>]` MUST read the change feed and print each event
as one line of JSON on standard output. After a lost connection it MUST
connect again with the last generation, and wait at most 30 seconds
between attempts.

#### Scenario: Lines for a script
- **WHEN** a tablet commits two changes while `rmhub watch` runs
- **THEN** the command prints two lines, and each line is a JSON object with a `generation` field

### Requirement: Scoped token commands
`rmhub tokens create` MUST accept `--grant <entry>:<read|write>` one or
more times, and `--expires <duration>`. With a grant it MUST create a
scoped token. Without a grant it MUST create an owner token.

#### Scenario: Agent token
- **WHEN** an owner runs `rmhub tokens create --label agent --grant /Agent:write`
- **THEN** the command prints a token with a write grant on the folder `Agent`

### Requirement: Agent guide
The repository MUST have the file `docs/agents/SKILL.md`. The guide MUST
contain these topics:

- How to set `RMHUB_URL` and `RMHUB_TOKEN`.
- How to use each library command with `--json`.
- How IDs and paths differ.
- The meaning of each exit code.

#### Scenario: Guide covers the commands
- **WHEN** a reviewer compares the guide with `rmhub --help`
- **THEN** the guide has an example for each library command
