# Spec Delta

## Purpose

The root log keeps every generation of the library root of an account. It
gives one winner when writers race, and it keeps each old root as a
snapshot of the library.

## ADDED Requirements

### Requirement: Generation numbers
Each account MUST have a generation number. An account with no commit MUST
have the generation 0 and no root hash. Each successful commit MUST add
exactly 1 to the generation.

#### Scenario: New account
- **WHEN** a caller reads the root of an account with no commit
- **THEN** the root log returns the generation 0 and no root hash

#### Scenario: First commit
- **GIVEN** an account with the generation 0
- **WHEN** a caller commits the hash H with the expected generation 0
- **THEN** the commit succeeds with the generation 1
- **AND** a read returns the generation 1 and the hash H

### Requirement: Compare and swap
A commit MUST give an expected generation and a new root hash. The commit
MUST succeed only if the expected generation is the current generation.
Otherwise the commit MUST return a stale-generation result and MUST NOT
change the root.

#### Scenario: Expected generation is current
- **GIVEN** an account with the generation 41
- **WHEN** a caller commits with the expected generation 41
- **THEN** the commit succeeds with the generation 42

#### Scenario: Expected generation is old
- **GIVEN** an account with the generation 45
- **WHEN** a caller commits with the expected generation 41
- **THEN** the commit returns stale-generation
- **AND** the current generation stays 45

#### Scenario: Expected generation is in the future
- **GIVEN** an account with the generation 45
- **WHEN** a caller commits with the expected generation 50
- **THEN** the commit returns stale-generation
- **AND** the current generation stays 45

#### Scenario: Parallel commits
- **GIVEN** an account with the generation 7
- **WHEN** 32 callers commit different hashes with the expected generation 7 at the same time
- **THEN** exactly one commit succeeds with the generation 8
- **AND** each other commit returns stale-generation

### Requirement: Immutable history
The root log MUST keep the entry of each generation after later commits.
An entry MUST NOT change after its commit. A caller MUST be able to read
the entry of any kept generation.

#### Scenario: Read an old generation
- **GIVEN** an account with the generation 10
- **WHEN** a caller reads the generation 4
- **THEN** the root log returns the root hash of the commit that made the generation 4

#### Scenario: Read a generation that does not exist
- **GIVEN** an account with the generation 10
- **WHEN** a caller reads the generation 11
- **THEN** the root log returns not-found

### Requirement: Commit record
Each entry MUST contain the root hash, the generation, the commit time in
UTC, a write ID and the source of the commit. The source MUST be a device
ID or an rmhub token ID. The write ID MUST be unique for each commit
attempt.

#### Scenario: Commit from a tablet
- **WHEN** a tablet with the device ID D commits a root
- **THEN** the new entry records the source type device and the ID D

#### Scenario: Commit from an API token
- **WHEN** an rmhub API client with the token ID T commits a root
- **THEN** the new entry records the source type token and the ID T

### Requirement: Correct read with a stale head hint
A read of the current root MUST return the newest committed generation.
This MUST be true when the head hint is older than the newest entry, and
when the head hint does not exist.

#### Scenario: Head hint update failed
- **GIVEN** a commit created the generation 12 and the head hint still names the generation 11
- **WHEN** a caller reads the current root
- **THEN** the root log returns the generation 12

#### Scenario: Head hint is missing
- **GIVEN** an account with the generation 3 and no head hint
- **WHEN** a caller reads the current root
- **THEN** the root log returns the generation 3

### Requirement: Head hint moves only forward
The root log MUST NOT set the head hint to a generation that is lower than
the generation it names.

#### Scenario: Late hint update
- **GIVEN** the head hint names the generation 20
- **WHEN** the hint update of the commit for the generation 19 runs late
- **THEN** the head hint still names the generation 20

### Requirement: Unknown commit outcome
If the storage reports a failure for the write of a new entry, the root log
MUST read that entry. If the entry has the write ID of the commit, the
commit MUST succeed. If the entry has a different write ID, the commit
MUST return stale-generation. If the read also fails, the commit MUST
return a failure.

#### Scenario: Write succeeded but the answer was lost
- **GIVEN** the storage stored the entry for the generation 9 and then reported a failure
- **WHEN** the root log resolves the outcome
- **THEN** the commit succeeds with the generation 9

#### Scenario: Another writer won
- **GIVEN** the storage reported a failure and the entry for the generation 9 has a different write ID
- **WHEN** the root log resolves the outcome
- **THEN** the commit returns stale-generation

### Requirement: Commit events
After each successful commit, the root log MUST send one event to each
local listener of the account. The event MUST contain the account, the new
generation, the root hash and the source. A slow listener MUST NOT delay
the commit or other listeners.

#### Scenario: Two listeners
- **GIVEN** two listeners for the account A
- **WHEN** a commit for the account A succeeds
- **THEN** each listener receives one event with the new generation

#### Scenario: Failed commit
- **WHEN** a commit returns stale-generation
- **THEN** no listener receives an event

#### Scenario: Blocked listener
- **GIVEN** a listener that does not read its events
- **WHEN** 100 commits succeed
- **THEN** each commit completes
- **AND** the other listeners receive all 100 events
