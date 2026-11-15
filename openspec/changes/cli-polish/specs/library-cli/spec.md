# Spec Delta

## MODIFIED Requirements

### Requirement: Library commands
The client MUST have these commands:

- `rmhub ls [entry]`
- `rmhub stat <entry>...`
- `rmhub find <text>`
- `rmhub get <entry>... [-o <file>] [--archive] [--force]`
- `rmhub upload <file>... [--in <entry>] [--name <name>] [--type <pdf|epub>] [--allow-duplicate]`
- `rmhub mkdir <path> [--in <entry>] [-p] [--allow-duplicate]`
- `rmhub mv <entry> <destination> [--allow-duplicate]`
- `rmhub rename <entry> <name> [--allow-duplicate]`
- `rmhub trash <entry>...`
- `rmhub restore <entry>... [--in <entry>] [--allow-duplicate]`

The option `--in` MUST name the destination folder. Without `--in`, the
destination folder MUST be the top level.

#### Scenario: Upload into a folder
- **WHEN** a user runs `rmhub upload paper.pdf --in /Inbox`
- **THEN** the document `paper` is in the folder `Inbox`

#### Scenario: Old option
- **WHEN** a user runs `rmhub upload paper.pdf --to /Inbox`
- **THEN** the command exits with code 2

#### Scenario: Upload two files
- **WHEN** a user runs `rmhub upload a.pdf b.epub --in /Inbox`
- **THEN** the folder `Inbox` has the documents `a` and `b`

### Requirement: Upload media type
`rmhub upload` MUST send a file with the extension `.pdf` as a PDF and a
file with the extension `.epub` as an EPUB. For other extensions it MUST
exit with code 2. The default name MUST be the file name without the
extension. The option `--type` MUST replace the type from the extension.
For the file argument `-`, the command MUST read the document from
standard input. It MUST then exit with code 2 unless `--name` and
`--type` are set.

#### Scenario: Unknown extension
- **WHEN** a user runs `rmhub upload notes.txt`
- **THEN** the command exits with code 2

#### Scenario: Standard input
- **WHEN** a program runs `rmhub upload - --name Report --type pdf` with a PDF on standard input
- **THEN** the top level has the PDF document `Report`

#### Scenario: Standard input without a name
- **WHEN** a program runs `rmhub upload - --type pdf`
- **THEN** the command exits with code 2 and does not read standard input

### Requirement: Download output
`rmhub get` MUST write the source file to `-o`, or to the entry name with
the extension of the type. With `--archive` it MUST write the archive of
the document with the extension `.zip`. With `-o -` it MUST write the
bytes to standard output. When `-o` is a directory, the command MUST
write the file with its default name into the directory. The command
MUST NOT replace an existing file unless `--force` is set. For an
existing file without `--force`, it MUST exit with code 5. When the
directory of the output file does not exist, it MUST exit with code 2
and MUST name the directory.

#### Scenario: Existing file
- **GIVEN** the file `Paper.pdf` exists
- **WHEN** a user runs `rmhub get /Paper`
- **THEN** the command exits with code 5 and does not change `Paper.pdf`

#### Scenario: Replace a file
- **GIVEN** the file `Paper.pdf` exists
- **WHEN** a user runs `rmhub get /Paper --force`
- **THEN** the file `Paper.pdf` holds the source file of the document

#### Scenario: Output directory
- **GIVEN** the directory `out` exists
- **WHEN** a user runs `rmhub get /Paper -o out/`
- **THEN** the file `out/Paper.pdf` holds the source file of the document

#### Scenario: Missing directory
- **GIVEN** the directory `sub` does not exist
- **WHEN** a user runs `rmhub get /Paper -o sub/x.pdf`
- **THEN** the command exits with code 2 and the message names the directory `sub`

## ADDED Requirements

### Requirement: Several arguments
A command with `<entry>...` or `<file>...` MUST do the operation for each
argument in the sequence of the arguments. At the first error, it MUST
stop and exit with the code of that error. With more than one argument
and `--json`, the command MUST print one JSON array with the result of
each completed argument. With one argument, the JSON output MUST be the
object of that argument.

#### Scenario: Trash two entries
- **WHEN** a user runs `rmhub trash /Inbox/a /Inbox/b --json`
- **THEN** both entries are in the trash
- **AND** standard output contains one JSON array with two entries

#### Scenario: Error at the second argument
- **GIVEN** the folder `Inbox` has the document `a` and no entry `x`
- **WHEN** a user runs `rmhub trash /Inbox/a /Inbox/x /Inbox/b --json`
- **THEN** the command exits with code 4, and standard output contains one JSON array with the entry `a`
- **AND** the entry `b` is not in the trash

#### Scenario: One argument
- **WHEN** a user runs `rmhub stat /Inbox/a --json`
- **THEN** standard output contains one JSON object

### Requirement: Options for one argument
With more than one argument, a command MUST refuse these options with
exit code 2, before the first request: `--name`, `--if-match`, `-o -`
and `-o` with a path that is not an existing directory. A command MUST
also refuse the file argument `-` together with a second file argument.

#### Scenario: Name for two files
- **WHEN** a user runs `rmhub upload a.pdf b.pdf --name Report`
- **THEN** the command exits with code 2 and uploads no file

#### Scenario: Two documents into a directory
- **GIVEN** the directory `out` exists
- **WHEN** a user runs `rmhub get /Inbox/a /Inbox/b -o out`
- **THEN** the directory `out` has the files `a.pdf` and `b.pdf`

### Requirement: Local file paths
In a local file argument and in the value of `-o`, the client MUST
replace a `~` that is the full first name of the path with the home
directory of the user. The client MUST NOT change a `~` in a different
position.

#### Scenario: Path in quotation marks
- **GIVEN** the file `paper.pdf` is in the home directory of the user
- **WHEN** a user runs `rmhub upload "~/paper.pdf"`
- **THEN** the command uploads the file

#### Scenario: File with the name ~
- **GIVEN** the current directory has the file `~.pdf`
- **WHEN** a user runs `rmhub upload "~.pdf"`
- **THEN** the command uploads the file `~.pdf` of the current directory

### Requirement: Write command text
Without `--json`, the commands `upload`, `mkdir`, `mv`, `rename`, `trash`
and `restore` MUST print one line for each changed entry. The line MUST
contain the ID and the path of the entry. With `--json`, the output MUST
be the full entry.

#### Scenario: Text of an upload
- **WHEN** a user runs `rmhub upload paper.pdf --in /Inbox`
- **THEN** standard output has one line with the ID of the document and `/Inbox/paper`

#### Scenario: JSON of an upload
- **WHEN** a program runs `rmhub upload paper.pdf --in /Inbox --json`
- **THEN** standard output contains one JSON object with the field `hash`
