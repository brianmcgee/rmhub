# Design

## Context

After the change `tablet-sync`, the tablet syncs with rmhub, and
`internal/hashtree` can parse and write indexes. The root log records
each commit with its source. The change `accounts` gives owner tokens.
This change opens the library to programs, with access limited by
folder.

Facts from ddvk/rmapi 0.0.35, which writes trees that the reMarkable
cloud accepts:

- A folder has a `.metadata` with `"type":"CollectionType"` and a
  `.content`. A PDF upload writes `<id>.pdf`, `<id>.metadata` and
  `<id>.content`.
- A move or a rename changes `visibleName` and `parent`, increments
  `version` and sets `metadatamodified` to `true`.
- `rmapi rm` removes the entry from the root. rmhub does not offer this.
  A tablet removes an entry from the root when the user empties the
  trash.

## Goals / Non-Goals

**Goals:**

- An agent with a scoped token can read, upload and organize documents in
  its folders, and cannot see other folders.
- An API write never loses a tablet change.

**Non-Goals:**

- MCP. The CLI and the agent guide come first.
- Removal of an entry from the tree, restore from the trash with a scoped
  token, and changes to page contents.
- Rendering. The change `rendering` adds it.
- Macaroons or other offline token narrowing.

## Decisions

### Package layout

```
internal/library/   snapshot of one generation, entries, paths, search
internal/libops/    operations, re-apply loop, metadata edits
internal/grants/    token chain, visibility, write rules
internal/feed/      root log diff, SSE writer
```

### Library snapshot

A snapshot is the library of one root generation. To build it, the
server reads the root index, each document index and each `.metadata`
and `.content`. Blobs never change, so an LRU cache keyed by blob hash
holds the parsed metadata. The server keeps the snapshot of the current
generation for each account. The next snapshot reuses the entries whose
document index hash did not change. So a tablet commit that changes one
document costs about three blob reads.

A request reads the current snapshot once and uses it for the full
response. This gives the `RMHub-Generation` header and consistent paths.

### Operations

An operation is a value with a `Check(snapshot, grants)` method and an
`Apply(snapshot) → new blobs and new root index` method. The loop is:

```
for attempt in 1..5:
    s := current snapshot
    Check(s)            → 403, 404, 400 or 412 stops the loop
    blobs, root := Apply(s)
    put blobs (first write wins, so a retry costs nothing)
    commit(root, s.generation, source=token, broadcast=true)
        ok    → answer
        stale → next attempt
409 after 5 attempts
```

`Apply` edits `.metadata` and `.content` as `map[string]any` values. It
keeps unknown fields and sets `metadatamodified` to `true`, as rmapi does.
The root keeps the schema of the current root index. A schema 3 account
stays on schema 3 until the tablet writes schema 4. This matches the
behavior of the tablet, which is known to work.

The uploaded source file goes into the blob store before the loop. A
retry then writes only the small `.metadata`, `.content` and indexes
again.

### Token chain

The token record of the change `accounts` gets the fields `parent` (a
token ID, empty for an owner token) and `grants`. Revocation marks the
token and each descendant. The request check also walks the parent
chain. The walk protects against a crash between the two writes of a
revocation. The chain has at most 4 scoped tokens, so the walk reads at
most 5 records. The token cache of the change `accounts` (30 seconds)
also holds these records.

Visibility for one entry walks from the entry to the top level and stops
at the first grant folder. Each token of the chain does this walk. The
snapshot has a parent index, so the walk does no I/O.

### Change feed

The feed keeps the last sent generation `g`. It wakes on a commit event
or every 30 seconds. Then it does these steps for each generation after
`g`. It reads the root log entry and builds the snapshot of that
generation. It compares that snapshot with the snapshot of the previous
generation by document ID and index hash. Each change is classified as
follows:

- `created`: the ID is new.
- `removed`: the ID is gone.
- `trashed`: `trashed` became `true`.
- `moved`: the parent changed.
- `updated`: each other change.

For a scoped token, a change of visibility becomes `created` or
`removed`.

The root log keeps each generation, because the change `root-log` has no
retention. A retention change must define what the feed does with an
`after` value that is older than the oldest generation.

### Paths for scoped tokens

A scoped token sees each grant folder as a top-level entry. The path of
an entry starts at its grant folder. The grant folder of a token is not
writable for that token, so a scoped client cannot move its own root.

## Risks / Trade-offs

- [The tablet ignores or rejects an uploaded document] → The upload uses
  the file set of rmapi. The first test with the tablet checks this.
  [INFERENCE] The tablet adds the page list when it opens the document.
- [A large library makes the first snapshot slow] → The first snapshot
  reads each `.metadata` and `.content`. For 2,000 documents and the file
  system backend, this is about 4,000 small reads at startup. After
  that, each commit reads only the changed documents.
- [An agent token leaks] → Grants limit the damage to its folders. Trash
  is the only removal, so the owner can restore each document from the
  tablet.
- [Feed replay from an old generation is slow] → Each step reuses the
  entries of the previous snapshot. The cost grows with the number of
  changed documents, not the library size.
