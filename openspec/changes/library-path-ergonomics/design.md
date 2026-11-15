# Design

## Context

See `proposal.md` for the reasons. These facts come from the code and
from a run of each command against a local server:

- `cmd/rmhub/library.go` has one function, `entryID`, that reads an entry
  argument. It calls `GET /api/v1/library/resolve` only for an argument
  that starts with `/`.
- The resolve endpoint accepts `/trash/<name>` for an owner token. It
  answers with status 404 for `/trash`, because the trash is not an
  entry.
- `PATCH /api/v1/library/{id}` accepts `name` and `parent` in one
  request and applies them in one commit. The same request with a parent
  moves a trashed entry out of the trash. No spec states this behavior.
- `internal/libops` runs each operation in a loop that applies it again
  after a lost race. A check in the operation is thus correct when a
  tablet commits at the same time.
- rmhub creates IDs with `ids.UUID()`. The tablet uses the same form.
  The server does not check the form of an ID that a sync client sends.
- The tablet permits two entries with one name in one folder. So the
  library can always contain duplicate names, and a path can always be
  ambiguous.

## Goals / Non-Goals

**Goals:**

- A person can type a name or a path in each place that takes an entry.
- The CLI does not make a path ambiguous unless the user asks for it.
- The rules stay deterministic for scripts and agents. The client does
  not guess between an ID and a path with a request.

**Non-Goals:**

- A server rule that forbids duplicate names. The tablet creates them,
  and the server must accept each tree that the tablet commits.
- A restore for a scoped token. A scoped token cannot see the trash.
- A record of the folder that an entry had before the trash. The tablet
  does not store it.
- More than one entry argument for one command. The change `cli-polish`
  adds that.

## Decisions

### An argument is an ID only in the UUID form

The client compares the argument with the UUID pattern (8-4-4-4-12
hexadecimal digits, without regard to case) and with `trash`. A match is
an ID. Each other argument is a path. The client adds a `/` at the start
of a path that has none.

Alternative: send the argument as an ID first, and resolve it as a path
after status 404. This costs a second request for each path, and one
argument can then mean two entries. Alternative: keep the rule and add a
hint to the error message. The user must then type the command again.

A folder whose name has the UUID form is reachable with a `/` at the
start. An entry whose ID does not have the UUID form is reachable by its
path only.

### The client maps `/trash` to the ID `trash`

`entryID` returns `trash` for the path `/trash` before it calls the
resolve endpoint. The server does not change. For a scoped token, the
list request with the parent `trash` already answers with status 404.

### The duplicate check is a server option

The option `unique` goes into `libops.CreateFolder`, `libops.Upload` and
`libops.Patch`. Each operation looks at the children of the destination
folder in the snapshot of the attempt. `CheckNew` does the same check
before an upload stores the body. The comparison is exact, as in the
resolve endpoint. The check ignores the entry of the operation itself,
so a rename to the current name is not an error.

For a scoped token, the check sees only the entries that the token can
see. A grant covers a folder with all its descendants, so the children
of a destination folder that the token can write are all visible.

Alternative: the client lists the folder before the write. A tablet or a
second client can create the name between the two requests.
Alternative: make `unique` the default of the API. That breaks API
clients that create duplicates on purpose, and the tablet can create
duplicates in each case.

### `mkdir -p` is a loop in the client

For each name of the path, the client resolves the path to that name.
After status 404, it creates the folder with `unique`. After status 409
from the create request, it resolves the path again, because a different
client created the folder. An ambiguous path stops the command with
exit code 5.

Alternative: a server option that creates the parents in one commit. The
loop needs no new endpoint, and a path has few names.

### `mv` resolves the destination, then its parent

1. The client resolves the destination. If the result is a folder, the
   client sends `{"parent":<id>}`.
2. If the result is a different type, the command stops with exit code 5.
3. After status 404, and when the destination is a path without a `/`
   at its end, the client resolves the parent of the path. It then sends
   `{"parent":<id>,"name":<last name>}` in one PATCH request.

The command `rename` stays. It changes a name without a path, which is
the safe form for a script that has an ID.

### `restore` is a PATCH request

The client reads the entry, checks that `trashed` is `true`, and sends
`{"parent":<id>}` with `unique`. The API gets no new endpoint. A delta
spec states the existing behavior of the PATCH request, so a test
protects it.

### `--in` is the one name of the destination folder

`mkdir` had `--in` and `upload` had `--to`. `--in` reads correctly for
`upload`, `mkdir` and `restore`. `mv` keeps a positional destination, as
in the shell. The option `--to` goes away without an alias.

### The search result contains the path

`Snapshot.Path` gives the path from the memory of the snapshot. The
result has at most 100 entries, so the cost is small. The list endpoint
does not get the path: each entry of a list has the path of the folder
and its name.

## Risks / Trade-offs

- [A sync client that is not the tablet writes an ID that does not have
  the UUID form] → The entry is reachable by its path. The reference
  page and the agent guide state the rule.
- [A script uses `--to` or `mkdir <name> --in`] → The command exits with
  code 2 and names the option. The reference page lists the change.
- [`mv` with a wrong destination path renames the entry] → The move
  needs an existing parent folder, and the output shows the new path. A
  `/` at the end of the destination forces a folder.
- [`mkdir -p` sends one or two requests for each name] → Acceptable for
  a path with few names. The request limits of the server apply.
- [The check `unique` reads the snapshot of the attempt] → A tablet
  commit after the API commit can still create a duplicate. That is the
  behavior of the tablet, and the resolve endpoint reports it.

## Migration Plan

The CLI and the server are one program, so they change together. An old
client with a new server sees no change, because it does not send
`unique`. A new client with an old server fails: the old server refuses
an unknown JSON field with status 400. So `mkdir`, `mv`, `rename` and
`restore` of a new client need a new server, or the option
`--allow-duplicate`. The reference page of the CLI must state this.
