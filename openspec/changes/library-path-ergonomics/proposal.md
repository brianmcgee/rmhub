# Proposal

## Why

A test of each library command against a local server showed that a
person cannot address an entry easily. The client reads `Inbox` as an
ID, so `rmhub ls Inbox` fails. The commands `mkdir` and `upload` make a
second entry with the same name without a warning. After that, each
command with the path of these entries stops with exit code 5. A path
argument works only in some places: `mkdir /Inbox/Sub` and
`mv /Inbox/paper /Work/new` fail.

## What Changes

- **BREAKING**: An entry argument is an ID only if it has the UUID form
  or is `trash`. Each other argument is a path from the top level, with
  or without a `/` at its start. Before this change, an argument without
  a `/` at its start was always an ID.
- **BREAKING**: `mkdir`, `upload`, `mv` and `rename` refuse a name that
  a different entry in the same folder has. They exit with code 5. The
  option `--allow-duplicate` gives the old behavior. The API gets the
  field `unique` for this check. The default of the API does not change.
- **BREAKING**: `rmhub mkdir` takes a path, not a name. The option `-p`
  creates each missing parent folder and accepts a folder that exists.
- **BREAKING**: The option `--to` of `upload` becomes `--in`. The
  destination folder then has one name in `upload`, `mkdir` and
  `restore`.
- `rmhub mv <entry> <destination>` also accepts a destination path that
  does not exist. The command then moves the entry into the parent
  folder of the path and gives it the last name of the path.
- The path `/trash` is the trash. The new command `rmhub restore` moves
  an entry out of the trash.
- `rmhub find` shows the path of each entry. The search endpoint of the
  API returns the path. `rmhub ls` shows a `/` after the name of each
  folder.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library-cli`: the rule for entry arguments, the list of commands, the
  duplicate check, folder paths, the move destination, the restore
  command and the text output of `ls` and `find`.
- `library-writes`: the field `unique` of the create, upload and patch
  requests, and the move of an entry out of the trash.
- `library`: the search result contains the path of each entry.

## Impact

- Code: `cmd/rmhub/library.go`, `cmd/rmhub/account.go` (the option
  `--grant` uses the entry rule), `internal/api/library.go`,
  `internal/libops` and `internal/library`.
- API: the new optional field `unique` and the new field `path` in the
  search result. An API client that does not send `unique` sees no
  change in the write operations.
- Scripts that use `--to`, `mkdir <name> --in`, or an ID that does not
  have the UUID form must change. The tablet and rmhub create only IDs
  in the UUID form. It is not verified that each other sync client does
  the same. An entry with a different ID form stays reachable by its
  path.
- Pages in `docs/` to update: `docs/reference/cli.md`,
  `docs/reference/http-api.md`, `docs/tutorials/first-server.md` and
  `docs/how-to/give-an-agent-access.md`. The guide
  `skills/rmhub/SKILL.md` also changes.
- The change `cli-polish` changes the same commands. Apply this change
  first.
- No machine or clan service is affected.
