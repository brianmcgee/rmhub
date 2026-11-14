# Tasks

## 1. Commit and read

- [x] 1.1 Write the entry type and the key layout with 20-digit generations in `internal/rootlog`. Verify with a unit test that byte order of the keys equals number order for the generations 9, 10 and 100.
- [x] 1.2 Write the commit with the test for `roots/<g>` and put-if-absent of `roots/<g+1>`. Verify with tests on the SQLite backend for the scenarios new account, first commit, old generation and future generation.
- [x] 1.3 Write a test with 32 parallel commits of one expected generation. Verify that exactly one commit succeeds and that 31 commits return stale-generation.
- [x] 1.4 Write the read of the current root and the read of one generation. Verify with tests for a missing head hint, a stale head hint and a generation that does not exist.
- [x] 1.5 Make the `internal/rootlog` tests also run on the S3 state backend when `RMHUB_TEST_S3_ENDPOINT` is set. Add them to the `s3-contract` check. Verify that `nix build .#checks.x86_64-linux.s3-contract` runs the parallel commit test.

## 2. Head hint and unknown outcome

- [x] 2.1 Write the head hint update that moves only forward. Verify with a test where the update for the generation 19 runs after the update for the generation 20.
- [x] 2.2 Write a test store that stores a write and then returns a failure. Write the resolve logic. Verify with tests for a lost answer, a write by another writer and a second failure.
- [x] 2.3 Add a cache in memory of the last entry for each account. Verify with a test that a commit by a second root log instance on the same store is visible to the first instance.

## 3. Commit events

- [x] 3.1 Write the broker with one buffer of 64 events for each listener and a lost-events flag. Verify with a test that two listeners each receive one event for each commit.
- [x] 3.2 Write a test with one blocked listener and 100 commits. Verify that the commits are not delayed and that the other listeners receive all 100 events. Verify that the blocked listener sees the lost-events flag.
- [x] 3.3 Verify with a test that a stale-generation commit sends no event.

## 4. Integration

- [x] 4.1 Run `nix fmt` and verify that no file changes.
- [x] 4.2 Run `go test ./...` and verify that all tests pass.
- [x] 4.3 Run `nix flake check` and verify that all checks pass.
