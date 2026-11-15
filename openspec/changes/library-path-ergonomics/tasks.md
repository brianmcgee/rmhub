# Tasks

## 1. Unique name option of the API

- [ ] 1.1 Add the option `unique` to `libops.CreateFolder`, `libops.Upload`, `libops.Patch` and `CheckNew`. Verify with tests in `internal/libops` for a name in use, a rename to the current name, and a tablet commit that creates the name before the API commit.
- [ ] 1.2 Read the field `unique` in the create and patch requests and the query `unique` in the upload request in `internal/api/library.go`. Verify with API tests for each scenario of the requirement `Unique name option`, also that a refused upload stores no blob.
- [ ] 1.3 Add an API test for the requirement `Move out of the trash`. Verify that a trashed folder and its document have `"trashed":false` after the PATCH request.
- [ ] 1.4 Describe the option `unique` and the move out of the trash in `docs/reference/http-api.md`. Verify each example with `curl` against a local server.

## 2. Path in the search result

- [ ] 2.1 Return the `path` of each entry from `GET /api/v1/library/search`. Verify with API tests for an owner token and for a scoped token whose grant folder is in a different folder.
- [ ] 2.2 Add the field `path` of the search result to `docs/reference/http-api.md`. Verify the example with `curl` against a local server.

## 3. Entry argument rule

- [ ] 3.1 Change `entryID` and `stat` in `cmd/rmhub/library.go` to the rule of the requirement `Entry arguments`. Verify with end-to-end tests for a path without `/`, an argument in the UUID form, a name in the UUID form with `/`, and `/trash`.
- [ ] 3.2 Replace the help text `entryLong` with the new rule. Verify that `rmhub ls --help` shows the rule and an example without `/`.
- [ ] 3.3 Verify with an end-to-end test that `rmhub tokens create --grant Inbox:read` creates a grant on the folder `Inbox`.
- [ ] 3.4 Update the section about IDs and paths in `docs/reference/cli.md` and `skills/rmhub/SKILL.md`. Verify that each command of the two sections runs against a local server.

## 4. Unique names, folder paths and the option `--in`

- [ ] 4.1 Send `unique` from `mkdir`, `upload`, `mv` and `rename`, and add the option `--allow-duplicate`. Verify with end-to-end tests that a second `mkdir Inbox` exits with code 5 and that `--allow-duplicate` creates the duplicate.
- [ ] 4.2 Change `rmhub mkdir` to take a path, with `--in` as the start folder. Verify with end-to-end tests for each scenario of the requirement `Folder path`.
- [ ] 4.3 Add the option `-p` to `rmhub mkdir`. Verify with end-to-end tests for two new folders, a folder that exists, an ambiguous parent and `-p` with `--allow-duplicate`.
- [ ] 4.4 Replace the option `--to` of `rmhub upload` with `--in`. Verify with an end-to-end test that `--in` works and that `--to` exits with code 2.
- [ ] 4.5 Refuse a `/` in the name of `rename` and in `--name` of `upload` before the request. Verify with a test that the message of `rename` names `rmhub mv`.
- [ ] 4.6 Update `docs/reference/cli.md`, `skills/rmhub/SKILL.md`, `docs/tutorials/first-server.md` and `docs/how-to/give-an-agent-access.md` for `--in`, `mkdir <path>`, `-p` and `--allow-duplicate`. State in `docs/reference/cli.md` that the duplicate check needs a server of the same version. Verify that each command of the changed text runs against a local server.

## 5. Move destination

- [ ] 5.1 Change `rmhub mv` to the rule of the requirement `Move destination`. Verify with end-to-end tests for each scenario of the requirement, and that a move with a new name is one commit in the root log.
- [ ] 5.2 Update the text of `mv` in `docs/reference/cli.md` and `skills/rmhub/SKILL.md`. Verify that each example runs against a local server.

## 6. Restore command

- [ ] 6.1 Add the command `rmhub restore <entry> [--in <entry>] [--allow-duplicate] [--if-match <hash>]`. Verify with end-to-end tests for each scenario of the requirement `Restore command` and for a stale hash.
- [ ] 6.2 Change the help text of `rmhub trash` to name `rmhub restore`. Verify that `rmhub trash --help` shows the command.
- [ ] 6.3 Add `restore` and the path `/trash` to `docs/reference/cli.md` and `skills/rmhub/SKILL.md`. Verify that each example runs against a local server.

## 7. List text

- [ ] 7.1 Show a `/` after each folder name in the text output of `rmhub ls`. Verify with a test that the JSON output does not change.
- [ ] 7.2 Show the path column in the text output of `rmhub find`, and keep the field `path` in its JSON output. Verify with an end-to-end test for two documents with one name in two folders.
- [ ] 7.3 Update the output examples of `ls` and `find` in `docs/reference/cli.md` and `skills/rmhub/SKILL.md`. Verify that each example is the real output of a local server.

## 8. Integration

- [ ] 8.1 Run `nix fmt` and verify that no file changes.
- [ ] 8.2 Run `go test ./...` and verify that all tests pass.
- [ ] 8.3 Run `nix flake check` and verify that all checks pass.
