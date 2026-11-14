# hash-tree Specification

## Purpose
The hash tree is the format of a reMarkable library in the sync protocol.
rmhub parses it to check uploads and to read the library. rmhub writes it
when it changes a library for an rmhub API client.

## Requirements

### Requirement: Index grammar
An index MUST be UTF-8 text with lines that end in `\n`. The first line
MUST be the schema: `3` or `4`. In schema 4 the second line MUST be a
summary `0:.:<count>:<size>`. Each other line MUST be an entry
`<hash>:<type>:<name>:<subfiles>:<size>`. The hash MUST be 64 lowercase
hexadecimal characters. Subfiles and size MUST be decimal integers.

#### Scenario: Schema 3 document index
- **WHEN** rmhub parses `3\n<h1>:0:d1.metadata:0:120\n<h2>:0:d1.content:0:800\n`
- **THEN** the result has schema 3 and two entries

#### Scenario: Schema 4 root index
- **WHEN** rmhub parses `4\n0:.:1:920\n<h3>:0:d1:2:920\n`
- **THEN** the result has schema 4 and one entry for the document `d1`

#### Scenario: Entry with four fields
- **WHEN** rmhub parses an index with the line `<h1>:0:d1.metadata:0`
- **THEN** the parse fails with an error that names the line

#### Scenario: Unknown schema
- **WHEN** rmhub parses an index with the first line `5`
- **THEN** the parse fails

### Requirement: Lenient fields
The parser MUST ignore empty lines. The parser MUST accept the type
`0` and the type `80000000` in each schema. The parser MUST accept a
schema 4 summary whose count or size does not match the entries.

#### Scenario: Root entry type in schema 3
- **WHEN** rmhub parses a schema 3 root index with the entry type `80000000`
- **THEN** the parse succeeds

#### Scenario: Wrong count in the summary
- **WHEN** rmhub parses `4\n0:.:5:920\n<h3>:0:d1:2:920\n`
- **THEN** the parse succeeds with one entry

### Requirement: File hash
The hash of a file MUST be the lowercase hex SHA-256 of its bytes.

#### Scenario: Hash of a file
- **WHEN** rmhub computes the hash of the bytes `test`
- **THEN** the hash is `9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08`

### Requirement: Entry list hash
The entry list hash MUST be the SHA-256 of the binary values of the entry
hashes, joined in ascending byte order of the entry names. The hash of a
document index MUST be its entry list hash in schema 3 and in schema 4.

#### Scenario: Order of entries does not change the hash
- **GIVEN** a document index with the entries `d1.metadata` and `d1.content` in that order
- **WHEN** rmhub computes its hash
- **THEN** the hash equals the hash of the same entries in the other order

### Requirement: Root index hash
The hash of a schema 3 root index MUST be its entry list hash. The hash of
a schema 4 root index MUST be the SHA-256 of its bytes.

#### Scenario: Schema 4 root
- **GIVEN** the bytes B of a schema 4 root index
- **WHEN** rmhub computes its hash
- **THEN** the hash is the SHA-256 of B

### Requirement: Index serialization
When rmhub writes an index, it MUST sort the entries in ascending byte
order of the names. A root entry MUST have the type `80000000` in schema 3
and `0` in schema 4. A file entry MUST have the type `0` and the subfiles
`0`. A schema 4 root index MUST have a summary with the entry count and the
sum of the entry sizes. A document index MUST have the schema `3`.

#### Scenario: Write a schema 4 root
- **WHEN** rmhub writes a schema 4 root index for the documents `b` (size 10) and `a` (size 5)
- **THEN** the index is `4\n0:.:2:15\n<ha>:0:a:<n>:5\n<hb>:0:b:<n>:10\n`

#### Scenario: Round trip
- **WHEN** rmhub parses an index that it wrote
- **THEN** the result has the same schema and entries, and the same hash
