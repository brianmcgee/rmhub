# Spec Delta

## ADDED Requirements

### Requirement: Durable blob write
A blob write that returned success MUST stay after a power failure of
the host. The file system backend MUST sync the blob file, the
directory of the blob and each parent directory inside the store
before it returns success.

#### Scenario: First blob of a new account
- **GIVEN** the store has no directory for the namespace A
- **WHEN** a caller writes a blob in the namespace A and the host loses power after the write returned
- **THEN** a read of the key after the restart returns the bytes of the blob
