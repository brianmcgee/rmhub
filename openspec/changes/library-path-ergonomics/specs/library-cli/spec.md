# Spec Delta

## MODIFIED Requirements

### Requirement: Entry arguments
Each command that takes an entry MUST accept an ID or a path. An argument
in the UUID form MUST be an ID. The argument `trash` MUST be the ID of
the trash. Each other argument MUST be a path, and the client MUST
resolve it through the API. A path MUST start at the top level, with or
without a `/` at its start. The path `/` MUST be the top level. The path
`/trash` MUST be the trash.

#### Scenario: Ambiguous path
- **GIVEN** the top level has two documents with the name `Notes`
- **WHEN** a user runs `rmhub stat /Notes`
- **THEN** the command exits with code 5 and prints both IDs

#### Scenario: Path without a slash at its start
- **GIVEN** the top level has the folder `Inbox`
- **WHEN** a user runs `rmhub ls Inbox`
- **THEN** the command lists the folder `Inbox`

#### Scenario: Argument in the UUID form
- **WHEN** a user runs `rmhub stat 0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3`
- **THEN** the client reads the entry with that ID and does not resolve a path

#### Scenario: Name in the UUID form
- **GIVEN** the top level has a folder with the name `0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3`
- **WHEN** a user runs `rmhub stat /0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3`
- **THEN** the client resolves the argument as a path

#### Scenario: Trash path
- **WHEN** an owner runs `rmhub ls /trash`
- **THEN** the command lists the entries in the trash

### Requirement: Library commands
The client MUST have these commands:

- `rmhub ls [entry]`
- `rmhub stat <entry>`
- `rmhub find <text>`
- `rmhub get <entry> [-o <file>] [--archive]`
- `rmhub upload <file> [--in <entry>] [--name <name>] [--allow-duplicate]`
- `rmhub mkdir <path> [--in <entry>] [-p] [--allow-duplicate]`
- `rmhub mv <entry> <destination> [--allow-duplicate]`
- `rmhub rename <entry> <name> [--allow-duplicate]`
- `rmhub trash <entry>`
- `rmhub restore <entry> [--in <entry>] [--allow-duplicate]`

The option `--in` MUST name the destination folder. Without `--in`, the
destination folder MUST be the top level.

#### Scenario: Upload into a folder
- **WHEN** a user runs `rmhub upload paper.pdf --in /Inbox`
- **THEN** the document `paper` is in the folder `Inbox`

#### Scenario: Old option
- **WHEN** a user runs `rmhub upload paper.pdf --to /Inbox`
- **THEN** the command exits with code 2

### Requirement: Conditional option
The commands `mv`, `rename`, `trash` and `restore` MUST accept
`--if-match <hash>` and send it as the `If-Match` header.

#### Scenario: Stale hash
- **WHEN** a user runs `rmhub rename D New --if-match h1` and the hash of D is not `h1`
- **THEN** the command exits with code 5

## ADDED Requirements

### Requirement: Unique names
The commands `mkdir`, `upload`, `mv`, `rename` and `restore` MUST send the
field `unique` with the value `true`. With `--allow-duplicate`, they MUST
NOT send the field. When the server refuses the name, the command MUST
exit with code 5 and MUST print the ID of the entry that has the name.

#### Scenario: Second folder with one name
- **GIVEN** the top level has the folder `Inbox`
- **WHEN** a user runs `rmhub mkdir Inbox`
- **THEN** the command exits with code 5 and prints the ID of the folder `Inbox`
- **AND** the top level has one folder with the name `Inbox`

#### Scenario: Duplicate on request
- **GIVEN** the folder `Inbox` has the document `paper`
- **WHEN** a user runs `rmhub upload paper.pdf --in /Inbox --allow-duplicate`
- **THEN** the folder `Inbox` has two documents with the name `paper`

### Requirement: Folder path
`rmhub mkdir <path>` MUST create a folder with the last name of the path.
The parent of the new folder MUST be the folder at the other names of the
path. With `--in`, the path MUST start at the folder of `--in`. If a
parent folder of the path does not exist, the command MUST exit with code
4 and MUST NOT create a folder.

#### Scenario: Folder in a folder
- **GIVEN** the top level has the folder `Inbox`
- **WHEN** a user runs `rmhub mkdir /Inbox/Sub`
- **THEN** the folder `Inbox` has the folder `Sub`

#### Scenario: Path from a folder ID
- **GIVEN** the folder with the ID F
- **WHEN** a user runs `rmhub mkdir 2026 --in F`
- **THEN** the folder F has the folder `2026`

#### Scenario: Missing parent folder
- **GIVEN** the top level has no entry with the name `Work`
- **WHEN** a user runs `rmhub mkdir Work/Deep`
- **THEN** the command exits with code 4 and the top level has no folder `Work`

### Requirement: Parent folders
With `-p`, `rmhub mkdir` MUST create each folder of the path that does
not exist. If the folder at the path exists, the command MUST print that
folder and exit with code 0. The command MUST refuse `-p` together with
`--allow-duplicate` with exit code 2.

#### Scenario: Two new folders
- **GIVEN** the top level has no entry with the name `Work`
- **WHEN** a user runs `rmhub mkdir -p Work/Deep`
- **THEN** the top level has the folder `Work`, and `Work` has the folder `Deep`

#### Scenario: Folder exists
- **GIVEN** the top level has the folder `Inbox` with the ID F
- **WHEN** a user runs `rmhub mkdir -p Inbox --json`
- **THEN** the command exits with code 0 and prints the entry with the ID F

### Requirement: Move destination
`rmhub mv <entry> <destination>` MUST move the entry into the destination
when the destination is a folder. When the destination is a path that no
entry has, and the parent of the path is a folder, the command MUST move
the entry into that parent and give the entry the last name of the path,
in one request. A destination path that ends with `/` MUST be a folder.
When the destination is an entry that is not a folder, the command MUST
exit with code 5.

#### Scenario: Move into a folder
- **GIVEN** the folders `Inbox` and `Work`, and the document `paper` in `Inbox`
- **WHEN** a user runs `rmhub mv Inbox/paper Work`
- **THEN** the entry has the path `/Work/paper`

#### Scenario: Move with a new name
- **GIVEN** the folder `Work` has no entry with the name `final`
- **WHEN** a user runs `rmhub mv /Inbox/paper /Work/final`
- **THEN** the entry has the path `/Work/final`

#### Scenario: New name in the same folder
- **WHEN** a user runs `rmhub mv /Inbox/paper /Inbox/draft`
- **THEN** the entry has the path `/Inbox/draft`

#### Scenario: Destination is a document
- **GIVEN** the folder `Work` has the document `final`
- **WHEN** a user runs `rmhub mv /Inbox/paper /Work/final`
- **THEN** the command exits with code 5, and the entry keeps the path `/Inbox/paper`

#### Scenario: Destination with a slash at its end
- **GIVEN** the top level has no entry with the name `Work`
- **WHEN** a user runs `rmhub mv /Inbox/paper /Work/`
- **THEN** the command exits with code 4

### Requirement: Name arguments
The argument `<name>` of `rename` and the option `--name` of `upload` MUST
NOT contain a `/`. For such a value, the command MUST exit with code 2
and MUST NOT send a request. The error message of `rename` MUST name the
command `mv`.

#### Scenario: Path as a name
- **WHEN** a user runs `rmhub rename /Inbox/paper /Inbox/new`
- **THEN** the command exits with code 2 and the message names `rmhub mv`

### Requirement: Restore command
`rmhub restore <entry>` MUST move an entry out of the trash into the
folder of `--in`, or to the top level. If the entry is not in the trash,
the command MUST exit with code 5 and MUST NOT change the entry.

#### Scenario: Restore to the top level
- **GIVEN** the document `paper` is in the trash
- **WHEN** an owner runs `rmhub restore /trash/paper`
- **THEN** the entry has the path `/paper` and `"trashed":false`

#### Scenario: Restore into a folder
- **GIVEN** the document `paper` is in the trash
- **WHEN** an owner runs `rmhub restore /trash/paper --in /Work`
- **THEN** the entry has the path `/Work/paper`

#### Scenario: Entry is not in the trash
- **WHEN** a user runs `rmhub restore /Work/paper`
- **THEN** the command exits with code 5

### Requirement: List text
The text output of `rmhub ls` MUST show a `/` after the name of each
folder. The text output of `rmhub find` MUST show the path of each entry
in place of the name. The JSON output of `rmhub find` MUST have the field
`path` in each entry. The JSON output of `rmhub ls` MUST NOT change.

#### Scenario: Folder mark
- **GIVEN** the top level has the folder `Inbox` and the document `paper`
- **WHEN** a user runs `rmhub ls`
- **THEN** the output has the names `Inbox/` and `paper`

#### Scenario: Two results with one name
- **GIVEN** the folders `Inbox` and `Work` each have a document `paper`
- **WHEN** a user runs `rmhub find paper`
- **THEN** the output has the paths `/Inbox/paper` and `/Work/paper`
