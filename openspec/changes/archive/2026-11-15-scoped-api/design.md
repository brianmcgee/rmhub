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

## Implementation notes

These are the decisions that the implementation made without a question
to the owner. Review them.

### Tokens

- Revocation deletes the records of the token and of its descendants. It
  does not mark them. The chain check fails when a parent is missing, so
  a crash between two deletes does not leave a valid child.
- The field `parent` is the token ID of the creator. An owner token has
  no parent. The admin endpoint issues only owner tokens. It does not
  accept grants.
- A child token without `expiresAt` gets the expiry of its parent. A
  later expiry gives status 403. A scoped token that creates a token
  without grants gets status 403.
- For a scoped creator, the narrowing check (403) comes before the
  folder check (400). Thus an unknown folder gives 403 to a scoped
  creator and 400 to an owner. An access value other than `read` or
  `write` gives 400.
- A scoped token gets 403 on `GET` and `PATCH /api/v1/account`, on the
  device endpoints, on pairing codes and on the admin endpoints.
- The server reads the token records on each request. The 30-second
  token cache of the design is not implemented, because the reads are
  fast and the cache delays revocation.

### Library

- For a scoped token, a grant folder has `parent: ""`, and paths start
  at the grant folder. A scoped token cannot list the trash. A write
  response after `trash` has no path for a scoped token.
- Resolve: for an owner, the first segment `trash` is the trash. The path
  `/` or an empty path gives 400. Two entries with the path give 409 with
  their IDs in the message.
- `modified` comes from `lastModified` (milliseconds). Without it,
  `modified` is `null`.
- `pageCount` comes from `pageCount` in `.content`. Without it, it is the
  length of `pages`, else the number of `cPages.pages` that are not
  deleted.
- Type: `CollectionType` is `folder`. The `fileType` `pdf` and `epub` give
  `pdf` and `epub`. Each other document is `notebook`.
- A document with a missing or bad index, `.metadata` or `.content` is
  not visible. It stays in the root index, so a write does not remove it.
- The new package `internal/lru` replaces the root cache of sync15. The
  snapshot builder uses it for parsed metadata.
- The archive endpoint checks that each file exists before it sends the
  first byte, so a missing blob gives 404 and not a cut ZIP file.

### Writes

- Upload: the media type must be `application/pdf` or
  `application/epub+zip`, else 400. The query `name` is necessary. The
  server writes the body to a temporary file while it calculates the
  hash. A body above `api.max_upload_bytes` gives 400. The server stores
  the source blob before the operation loop, so a retry does not upload
  it again.
- The new `.content` uses the defaults of rmapi `createZipContent`
  (margins 180, lineHeight -1, textScale 1, orientation portrait,
  pageCount 0). New `.metadata` has version 0 and `synced: true`.
- A scoped token cannot create an entry at the top level or move an
  entry to the top level (403). A move into `trash` gives 400; use the
  trash endpoint. A name must not be empty or contain `/`. A `PATCH`
  without `name` and `parent` gives 400.

### Change feed

- The feed stops on server shutdown through the server context, so a
  shutdown does not wait for open streams. It checks the token every 30
  seconds; the tests use 50 milliseconds.
- Operations: `created` when an entry becomes visible, `trashed` when
  `trashed` changes from false to true, `moved` when the parent changes,
  `updated` when the hash or `trashed` changes otherwise, `removed` when
  the entry is no longer visible. Because a scoped token cannot see the
  trash, a trashed entry gives `removed` to a scoped token.

### CLI

- `/` as an entry argument is the top level. It is valid for `ls` and as
  the target of `mv`.
- `get` writes to a temporary file and then makes a hard link to the
  target name. The link fails when the file exists, so `get` never
  replaces a file, also when the file appears during the download.
- A server answer with status 400 gives exit code 1, as in the
  foundation exit code contract. The usage code 2 is for errors that the
  client finds, such as an unknown upload extension.
- `watch` waits 1 second after the first lost connection, and doubles the
  wait up to 30 seconds. A status below 500 (for example 401 after a
  revocation) ends the command. `watch` prints the same JSON lines with
  and without `--json`.
- `tokens create --grant` resolves a path to an ID before it sends the
  request.

### Verification

- The rmapi conformance check uploads a PDF through the CLI and checks
  that rmapi lists, downloads and moves it (task 2.6).
- Task 5.5: each command of `docs/agents/SKILL.md` ran against a local
  server with a scoped token with a write grant.
