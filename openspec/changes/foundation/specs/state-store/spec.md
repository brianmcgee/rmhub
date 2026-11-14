# Spec Delta

## Purpose

The state store keeps small mutable values such as account records and the
root of each library. Each write is conditional, so two writers cannot
silently overwrite each other.

## ADDED Requirements

### Requirement: State key format
A state key MUST be 1 to 512 bytes long. A state key MUST contain only the
characters `a-z`, `0-9`, `.`, `_`, `-` and `/`. A state key MUST NOT start
or end with `/`, contain `//`, or contain a segment that is `.` or `..`.
The state store MUST refuse each other key with an invalid-key error.

#### Scenario: Valid key
- **WHEN** a caller writes the key `accounts/u1/roots/00000000000000000001`
- **THEN** the state store accepts the key

#### Scenario: Key with a parent segment
- **WHEN** a caller writes the key `accounts/../server/keys`
- **THEN** the state store returns an invalid-key error
- **AND** the storage does not change

### Requirement: Versioned read
A read of a key that exists MUST return the value and its version. A
version MUST be an opaque string that identifies the value. A read of a key
that does not exist MUST return a not-found result.

#### Scenario: Read an existing key
- **GIVEN** the key K has the value V
- **WHEN** a caller reads the key K
- **THEN** the state store returns the value V and a version

#### Scenario: Read a missing key
- **WHEN** a caller reads a key that does not exist
- **THEN** the state store returns not-found

### Requirement: Put if absent
A put-if-absent write MUST store the value only if the key does not exist.
If the key exists, the write MUST return a conflict result and MUST NOT
change the value. A successful write MUST return the new version.

#### Scenario: Create a key
- **WHEN** a caller does put-if-absent on a key that does not exist
- **THEN** the write succeeds and returns a version
- **AND** a read returns the new value

#### Scenario: Key exists
- **GIVEN** the key K has the value V1
- **WHEN** a caller does put-if-absent on the key K with the value V2
- **THEN** the write returns conflict
- **AND** a read returns the value V1

#### Scenario: Parallel creates
- **WHEN** 32 callers do put-if-absent on the same new key at the same time
- **THEN** exactly one write succeeds
- **AND** each other write returns conflict

### Requirement: Put if match
A put-if-match write MUST store the value only if the current version of
the key is the given version. If the version is different, or the key
does not exist, the write MUST return a conflict result and MUST NOT
change the storage. A successful write MUST return the new version.

#### Scenario: Version matches
- **GIVEN** a read of the key K returned the version E1
- **WHEN** a caller does put-if-match on the key K with the version E1
- **THEN** the write succeeds and returns a version

#### Scenario: Stale version
- **GIVEN** the key K changed after a caller read the version E1
- **WHEN** the caller does put-if-match on the key K with the version E1
- **THEN** the write returns conflict
- **AND** the value does not change

#### Scenario: Missing key
- **WHEN** a caller does put-if-match on a key that does not exist
- **THEN** the write returns conflict
- **AND** the key does not exist after the write

#### Scenario: Parallel updates
- **GIVEN** 32 callers read the key K and got the same version
- **WHEN** the 32 callers do put-if-match on the key K at the same time
- **THEN** exactly one write succeeds
- **AND** each other write returns conflict

### Requirement: Identical values can share a version
Two writes of the same bytes to one key can give the same version. A
caller MUST NOT use a version to detect a write of identical bytes. The
contract test suite MUST NOT expect a new version for identical bytes.

#### Scenario: Rewrite of identical bytes
- **GIVEN** the key K has the value V and the version E1
- **WHEN** a caller does put-if-match on the key K with the version E1 and the value V
- **THEN** the write succeeds
- **AND** the returned version is E1 or a different version

### Requirement: Delete
After a delete, a read of the key MUST return not-found. A delete of a key
that does not exist MUST succeed.

#### Scenario: Delete an existing key
- **GIVEN** the key K exists
- **WHEN** a caller deletes the key K
- **THEN** a read of the key K returns not-found

#### Scenario: Delete a missing key
- **WHEN** a caller deletes a key that does not exist
- **THEN** the delete succeeds

### Requirement: Prefix listing
A list request MUST return the keys that start with a given prefix, in
ascending byte order. A list request MUST accept a start key and a limit.
The result MUST contain only keys after the start key, and no more keys
than the limit.

#### Scenario: List with a prefix
- **GIVEN** the keys `a/1`, `a/2`, `a/3` and `b/1`
- **WHEN** a caller lists the prefix `a/`
- **THEN** the state store returns `a/1`, `a/2` and `a/3` in that order

#### Scenario: List in pages
- **GIVEN** the keys `a/1`, `a/2` and `a/3`
- **WHEN** a caller lists the prefix `a/` after the key `a/1` with the limit 1
- **THEN** the state store returns only `a/2`

### Requirement: Unknown outcome
The state store MUST return one of these results for a write: success,
conflict, or failure. A failure MUST mean that the outcome is unknown.
The state store MUST NOT report a failure as a conflict or a conflict as a
failure.

#### Scenario: Connection lost during a write
- **WHEN** the connection to the backend stops before the answer of a write
- **THEN** the state store returns failure, not conflict

### Requirement: Same behavior on each backend
The SQLite backend and the S3 backend MUST pass the same contract test
suite. The suite MUST cover each requirement of this capability.

#### Scenario: Contract suite on the SQLite backend
- **WHEN** the contract suite runs against a new SQLite file
- **THEN** each test of the suite passes
