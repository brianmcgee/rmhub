# Proposal

## Why

The user wants programs and AI agents to read and change the library
without the tablet. An agent must get access to one folder only, not to
the full library. rmfakecloud has a web UI with full access and no API
for scoped clients. rmhub has no web UI, so this API is the only way into
the library apart from the tablet.

## What Changes

- Add a read model of the library. A client can list a folder, read one
  entry, resolve a path and search by name. A client can download the
  source file or an archive of a document.
- Add write operations: create a folder, upload a PDF or an EPUB, rename,
  move and trash. Each operation is a semantic operation on IDs. When a
  tablet commits first, the server reads the new root and applies the
  operation again. An optional `If-Match` header refuses a write when
  the document changed.
- Add scoped tokens. A scoped token has grants on folders, each with
  read or write access. A token can create a child token with fewer
  rights. Revocation of a token stops all its child tokens.
- Add a change feed over server-sent events (SSE). The feed computes the
  changes from the root log, so it has no gaps.
- Add CLI commands for each operation, a `watch` command with JSON lines
  and a guide for agents. The CLI is the agent interface. MCP is not in
  this change.

## Capabilities

### New Capabilities

- `library`: the read model of the library and the read endpoints.
- `library-writes`: the write operations, the retry after a lost root
  race and `If-Match`.
- `scoped-tokens`: grants, child tokens and the visibility rules.
- `change-feed`: the SSE feed of library changes.
- `library-cli`: the CLI commands for the library and the agent guide.

### Modified Capabilities

None.

## Impact

- New packages for the library model, the operations and the grants.
- New configuration key: `api.max_upload_bytes`.
- The token record of the change `accounts` gets a parent and grants.
- Tablets see each API write after their next sync. A write sends a
  notification, so a connected tablet syncs at once.
- Unknown: whether the tablet accepts an uploaded document that has no
  page list in `.content`. rmapi uploads documents this way, and the
  reMarkable cloud accepts them.
- No machine or clan service is affected.
