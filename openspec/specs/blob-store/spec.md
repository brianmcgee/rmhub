# blob-store Specification

## Purpose
The blob store keeps the immutable files of all documents. A blob has a
SHA-256 key and never changes after it is written. Each account has its
own namespace, so a key gives no access to the blobs of another account.

## Requirements

### Requirement: Blob key format
The blob store MUST accept only a key of 64 lowercase hexadecimal
characters. The blob store MUST refuse each other key with an invalid-key
error. The blob store MUST NOT read or write storage for an invalid key.

#### Scenario: Valid key
- **WHEN** a caller writes a blob with the key `9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08`
- **THEN** the blob store accepts the key

#### Scenario: Key with path characters
- **WHEN** a caller writes or reads a blob with the key `../accounts/x`
- **THEN** the blob store returns an invalid-key error
- **AND** the storage does not change

#### Scenario: Uppercase key
- **WHEN** a caller reads a blob with a key that has uppercase hexadecimal characters
- **THEN** the blob store returns an invalid-key error

### Requirement: Account namespaces
Each blob operation MUST name a namespace. A namespace MUST be an account
ID of 26 characters from `a-z` and `2-7`. The blob store MUST refuse each
other namespace with an invalid-key error, and MUST NOT read or write
storage for it. A blob in one namespace MUST NOT be visible in another
namespace.

#### Scenario: Same key in two accounts
- **GIVEN** the namespace A has a blob with the key K
- **WHEN** a caller reads or checks the key K in the namespace B
- **THEN** the blob store returns not-found

#### Scenario: Same key written in two accounts
- **GIVEN** the namespace A has a blob with the key K and the bytes X
- **WHEN** a caller writes the key K with the bytes X in the namespace B
- **THEN** the namespace B has its own blob with the key K

#### Scenario: Namespace with path characters
- **WHEN** a caller writes a blob in the namespace `../tmp`
- **THEN** the blob store returns an invalid-key error
- **AND** the storage does not change

### Requirement: Atomic blob write
A blob write MUST be atomic. A reader MUST get all the bytes of a blob or a
not-found result. A reader MUST NOT get part of a blob. If a write fails,
the blob store MUST NOT keep a partial blob under the key.

#### Scenario: Write stops before the end
- **WHEN** the source of a blob write fails after part of the bytes
- **THEN** the write returns an error
- **AND** a read of the key returns not-found

#### Scenario: Read during a write
- **WHEN** a caller reads a key while a write of that key is in progress
- **THEN** the read returns not-found or all bytes of the blob

### Requirement: First write wins
A blob MUST NOT change after the first successful write of its key. A write
to a key that exists MUST succeed and MUST keep the bytes of the first
write.

#### Scenario: Second write of the same key
- **GIVEN** a blob with the key K and the bytes A
- **WHEN** a caller writes the key K with the bytes B
- **THEN** the write succeeds
- **AND** a read of the key K returns the bytes A

#### Scenario: Parallel writes of one key
- **WHEN** 32 callers write the key K at the same time
- **THEN** all writes succeed
- **AND** a read of the key K returns the bytes of exactly one writer

### Requirement: Blob read
A read of a key that exists MUST return the bytes of the blob and its size
in bytes. A read of a key that does not exist MUST return a not-found
result. A not-found result MUST be different from a storage failure.

#### Scenario: Read an existing blob
- **GIVEN** a blob with the key K and 1048576 bytes
- **WHEN** a caller reads the key K
- **THEN** the blob store returns the size 1048576 and the same bytes

#### Scenario: Read a missing blob
- **WHEN** a caller reads a key that the blob store does not have
- **THEN** the blob store returns not-found

#### Scenario: Storage is not available
- **WHEN** a caller reads a key and the backend does not answer
- **THEN** the blob store returns a storage failure, not not-found

### Requirement: Blob existence check
The blob store MUST tell if a key exists without a read of the blob bytes.

#### Scenario: Check keys
- **GIVEN** a blob with the key K1 and no blob with the key K2
- **WHEN** a caller checks the keys K1 and K2
- **THEN** the blob store reports that K1 exists and that K2 does not exist

### Requirement: Same behavior on each backend
The file system backend and the S3 backend MUST pass the same contract
test suite. The suite MUST cover each requirement of this capability.

#### Scenario: Contract suite on the S3 backend
- **WHEN** the contract suite runs against a local S3 server
- **THEN** each test of the suite passes
