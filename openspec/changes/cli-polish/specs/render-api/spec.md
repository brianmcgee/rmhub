# Spec Delta

## MODIFIED Requirements

### Requirement: Page commands
The client MUST have the commands `rmhub pages <entry>` and
`rmhub render <entry> [--page <number>] [--format png|svg] [--width <px>] [-o <file>] [--force]`.
Without `--page`, `render` MUST render page 1. The option `-p` MUST be
the short form of `--page`. Without `-o`, `render` MUST write to
`<name>-p<number>.<format>`. With `-o -` it MUST write to standard
output. When `-o` is a directory, it MUST write the file with its
default name into the directory. It MUST NOT replace an existing file
unless `--force` is set. For an existing file without `--force`, it MUST
exit with code 5. The command MUST check the page number, the format and
the width before it looks for the output file.

#### Scenario: Agent renders a page
- **WHEN** an agent runs `rmhub render /Notes --page 3 -o page.png`
- **THEN** the file `page.png` holds the PNG image of page 3

#### Scenario: Default page
- **WHEN** a user runs `rmhub render /Notes`
- **THEN** the file `Notes-p1.png` holds the PNG image of page 1

#### Scenario: Short option
- **WHEN** a user runs `rmhub render /Notes -p 2`
- **THEN** the file `Notes-p2.png` holds the PNG image of page 2

#### Scenario: Existing image
- **GIVEN** the file `Notes-p1.png` exists
- **WHEN** a user runs `rmhub render /Notes`
- **THEN** the command exits with code 5 and does not change the file

#### Scenario: Width out of range and existing image
- **GIVEN** the file `Notes-p1.png` exists
- **WHEN** a user runs `rmhub render /Notes --width 50`
- **THEN** the command exits with code 2 and the message names the width
