---
name: rmhub
description: Read and change the documents of a reMarkable library on an rmhub server with the rmhub command. Use it to list, find, download, upload, move, rename and trash documents and folders, and to follow changes.
---

# rmhub library for agents

rmhub keeps the documents of reMarkable tablets. The `rmhub` command
reads and changes the library through the rmhub API. Each change that
you make gets to the tablets at their next sync.

## Setup

Set two environment variables before you run a command:

```sh
export RMHUB_URL=https://rmhub.example.org   # URL of the rmhub API
export RMHUB_TOKEN=rmh_…                      # the token that the owner gave you
```

The options `--url` and `--token` override the variables. Do not write
the token into a file or into the output.

Usually the token is a scoped token. It sees only its grant folders and
their descendants. For a scoped token, the top level is the list of the
grant folders, and each path starts at a grant folder. A `read` grant
lets you read. A `write` grant also lets you change entries in the
folder. You cannot rename, move or trash a grant folder, and you cannot
write at the top level.

## Rules

- Always add `--json`. Each command then prints one JSON value on
  standard output. Errors go to standard error, as
  `{"error":{"code":"…","message":"…"}}`.
- Check the exit code after each command (see "Exit codes").
- Prefer IDs to paths when you have them.
- Use `--if-match <hash>` when you change an entry that you read before.
  Then the command does not overwrite a change of the owner.
- The API cannot delete an entry. `trash` moves it into the trash. The
  owner can restore it.
- `get` never replaces a file. Choose a new file name, or use `-o -`.

## IDs and paths

An entry is a folder or a document. Each entry has an ID, such as
`0b7c1d2e-3f40-4a5b-8c6d-7e8f90a1b2c3`. The ID does not change when the
entry moves or gets a new name.

An `<entry>` argument is an ID or a path:

- An argument that starts with `/` is a path, such as `/Agent/Inbox/Paper`.
  The command resolves it to an ID through the server.
- Each other argument is an ID.
- `/` is the top level. It is valid only as the target of `mv` and in
  `ls /`.

Names are not unique. Two entries in one folder can have the same name.
Then a path gives exit code 5, and standard error lists the IDs. Use one
of the IDs.

## The entry object

`ls`, `stat`, `find`, `upload`, `mkdir`, `mv`, `rename` and `trash` print
entries in this form:

```json
{"id":"0b7c…","name":"Paper","type":"pdf","parent":"9a1f…","trashed":false,
 "modified":"2026-01-02T10:00:00Z","hash":"5f2a…","pageCount":12,"path":"/Agent/Paper"}
```

- `type` is `folder`, `pdf`, `epub` or `notebook`.
- `parent` is the ID of the folder. It is `""` at the top level and
  `"trash"` in the trash.
- `hash` changes with each change of the entry. Give it to `--if-match`.
- `modified` can be `null`.
- `path` is only in the output of `stat` and of the write commands.

## Commands

### List a folder

```sh
rmhub ls --json                   # top level (grant folders for a scoped token)
rmhub ls /Agent/Inbox --json      # a folder by path
rmhub ls 9a1f… --json             # a folder by ID
```

Output: a JSON array of entries, sorted by name.

### Show an entry

```sh
rmhub stat /Agent/Inbox/Paper --json
```

Output: one entry with `path`.

### Find entries by name

```sh
rmhub find invoice --json
```

Output: a JSON array of at most 100 entries whose name contains the
text, without regard to case. Trashed entries are not in the result.

### Download a document

```sh
rmhub get /Agent/Paper --json              # writes Paper.pdf
rmhub get 0b7c… -o /tmp/paper.pdf --json   # writes /tmp/paper.pdf
rmhub get /Agent/Paper -o -                # writes the bytes to standard output
rmhub get /Agent/Sketch --archive --json   # writes Sketch.zip
```

Without `--archive`, the command downloads the source file of a PDF or
an EPUB. A notebook has no source file; it gives exit code 4. With
`--archive`, the command downloads a ZIP file with each file of the
document, also for a notebook.

Output: `{"id":"…","file":"Paper.pdf","size":12345}`.

### Upload a document

```sh
rmhub upload report.pdf --to /Agent/Outbox --json
rmhub upload book.epub --to /Agent --name "A Book" --json
```

The file must have the extension `.pdf` or `.epub`. Other extensions give
exit code 2. The default name is the file name without the extension.

Output: the new entry.

### Create a folder

```sh
rmhub mkdir Outbox --in /Agent --json
```

Output: the new folder.

### Move an entry

```sh
rmhub mv /Agent/Inbox/Paper /Agent/Done --if-match 5f2a… --json
```

Output: the moved entry. A folder cannot go into itself or into its
descendant.

### Rename an entry

```sh
rmhub rename /Agent/Inbox/scan "Invoice 2026-01" --if-match 5f2a… --json
```

Output: the renamed entry. A name must not be empty or contain `/`.

### Move an entry into the trash

```sh
rmhub trash /Agent/Inbox/old --if-match 5f2a… --json
```

Output: the entry with `"trashed":true`.

### Follow changes

```sh
rmhub watch                # changes after now
rmhub watch --after 41     # changes after generation 41
```

The command runs until you stop it. It prints one JSON line for each
generation of the library that has a change that you can see:

```json
{"generation":42,"source":{"type":"device","id":"RM110-…"},
 "changes":[{"id":"0b7c…","op":"created","entry":{…}}]}
```

- `op` is `created`, `updated`, `moved`, `trashed` or `removed`.
- `entry` is `null` for `removed`. `removed` also means that the entry
  moved out of your grants. A scoped token cannot see the trash, so for
  a scoped token an entry that goes into the trash gives `removed`.
- `source.type` is `device` for a tablet or an app, and `token` for an
  API client.

After a lost connection, the command connects again and continues after
the last printed generation, so no generation is lost. Store the last
generation if you stop and start `watch`, and give it to `--after`.

### Create a token for a helper

```sh
rmhub tokens create --label helper --grant /Agent/Inbox:read --expires 24h --json
rmhub tokens list --json
rmhub tokens revoke <id> --json
```

A scoped token can create only a token with the same or fewer rights,
and with the same or an earlier expiry. It can list and revoke only the
tokens that it created and their descendants.

## Exit codes

| Code | Meaning | What to do |
|---|---|---|
| 0 | Success. | Read standard output. |
| 1 | Other error: status 400 (for example a file that is not a valid PDF, or a move into a document), status 500, a network error or an existing output file. | Read standard error. Correct the request, try again, or choose a new file name. |
| 2 | Usage or setting error, for example an unknown extension or no `RMHUB_URL`. | Correct the command. |
| 3 | Status 401 or 403: the token is not valid, or it has no right for this operation. | Do not try again. Ask the owner. |
| 4 | Status 404: no entry with this ID or path, or it is outside your grants. | Use `ls` or `find` to get the entry. |
| 5 | Status 409 or 412: a path with more than one entry, or a stale `--if-match` hash. | Use an ID, or read the entry again with `stat` and decide again. |
