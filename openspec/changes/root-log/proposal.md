# Proposal

## Why

The root of a library names the hash of its whole document tree. A tablet,
an app or an rmhub API client changes the library only when it replaces
the root. Two writers can try to replace the same root at the same time.
Exactly one must win, and the loser must see a stale-generation result. The
state store gives only single-key conditional writes, so the root must fit
that model.

## What Changes

- Keep each root of an account as an immutable entry
  `accounts/<uid>/roots/<generation>`. A put-if-absent write of generation
  `g+1` is the compare-and-swap: only one writer can create it.
- Keep a head hint `accounts/<uid>/head`. It makes a read fast. A reader
  walks forward from the hint to the newest entry, so a stale hint gives
  no wrong result.
- Record in each entry the write ID, the time and the source of the
  change: a device ID or an rmhub token ID.
- Resolve an unknown write outcome: read the new entry and compare the
  write ID.
- Tell local listeners after each successful commit. The tablet
  notifications and the rmhub change feed use this.
- Keep all old entries. Every generation is a snapshot of the whole
  library. A later change adds retention, rollback and garbage collection.

## Capabilities

### New Capabilities

- `root-log`: the generations of the library root of each account, with
  commit, read, history and commit events.

### Modified Capabilities

None.

## Impact

- A new package on top of the state store from the change `foundation`.
- No new dependency.
- Storage grows by one small entry for each sync. At personal scale that
  is a few thousand entries each year.
- No machine or clan service is affected.
