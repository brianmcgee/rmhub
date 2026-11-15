# Tasks

## 1. Exit codes and error messages

- [ ] 1.1 Map status 400 to exit code 2 in `exitCode`. Verify with an end-to-end test that `rmhub mkdir " "` exits with code 2.
- [ ] 1.2 Map status 404 without the rmhub error body to exit code 1, with a message that names the URL. Verify with a test against a server that answers 404 with plain text.
- [ ] 1.3 Check the scheme and the host of the server URL in `resolveSettings`. Verify with tests for `127.0.0.1:8080`, `ftp://x` and `http://`, each with exit code 2.
- [ ] 1.4 Change the message of a connection error to `cannot connect to <url>: <reason>`. Verify with a test against a closed port.
- [ ] 1.5 Add the usage line and the text `--help` to each usage error. Verify with tests for too many arguments, an unknown option and an unknown command, in text and with `--json`.
- [ ] 1.6 Add the hint for the admin token to a status 403 outside `rmhub admin`. Verify with an end-to-end test of `rmhub ls` with the admin token.
- [ ] 1.7 Update the exit code table and the error examples in `docs/reference/cli.md` and `skills/rmhub/SKILL.md`. Verify that each example is the real output of a local server.

## 2. Output files and local paths

- [ ] 2.1 Write one function that expands `~` in a local path, and call it for the file arguments of `upload`, for `-o` and for `serve --config`. Verify with unit tests for `~`, `~/a`, `~a`, `a/~` and `./~`.
- [ ] 2.2 Accept a directory as the value of `-o` in `get` and `render`. Verify with end-to-end tests for an existing directory and for a path with a `/` at its end.
- [ ] 2.3 Check the directory of the output file before the download. Verify with a test that the exit code is 2 and that the message has no temporary file name.
- [ ] 2.4 Exit with code 5 for an existing output file, and add the option `--force` to `get` and `render`. Verify with tests that `--force` replaces the file and that a failed download keeps the old file.
- [ ] 2.5 Use page 1 as the default of `rmhub render`, add the short option `-p`, and check the page, the format and the width before the output file. Verify with tests for each scenario of the requirement `Page commands`.
- [ ] 2.6 Update the text of `get` and `render` in `docs/reference/cli.md`, `skills/rmhub/SKILL.md` and `docs/tutorials/first-server.md`. Verify that each command of the changed text runs against a local server.

## 3. Several arguments and standard input

- [ ] 3.1 Accept more than one argument in `stat`, `get`, `upload`, `trash` and `restore`, with the JSON rule of the requirement `Several arguments`. Verify with end-to-end tests for two arguments, for an error at the second of three arguments, and for one argument.
- [ ] 3.2 Refuse the options of the requirement `Options for one argument` before the first request. Verify with a test for each option that the server gets no request.
- [ ] 3.3 Add the option `--type` and the file argument `-` to `rmhub upload`. Verify with end-to-end tests for a PDF on standard input, for a missing `--name` and for `-` with a second file.
- [ ] 3.4 Update the text of the five commands in `docs/reference/cli.md` and `skills/rmhub/SKILL.md`. State that the JSON form depends on the number of arguments. Verify that each example runs against a local server.

## 4. Text of write commands

- [ ] 4.1 Print one line with the ID and the path for `upload`, `mkdir`, `mv`, `rename`, `trash` and `restore` in text mode. Verify with tests that the text has one line for each entry and that the JSON output does not change.
- [ ] 4.2 Replace the output examples of the write commands in `docs/reference/cli.md`, `docs/tutorials/first-server.md` and `docs/how-to/give-an-agent-access.md`. Verify that each example is the real output of a local server.

## 5. Login command and expiry values

- [ ] 5.1 Add the command `rmhub login [--no-check]`. Verify with end-to-end tests for each scenario of the requirement `Login command`, also the file mode 0600 and the directory mode 0700.
- [ ] 5.2 Accept a number of days in `parseExpires`. Verify with unit tests for `30d`, `0d`, `2y` and `1d12h`.
- [ ] 5.3 Replace the manual steps for `client.toml` with `rmhub login` in `docs/tutorials/first-server.md` and `docs/how-to/give-an-agent-access.md`. Add `login` and the days unit to `docs/reference/cli.md`, `docs/explanation/access.md` and `skills/rmhub/SKILL.md`. Verify that each command of the changed text runs against a local server.

## 6. Command names and help groups

- [ ] 6.1 Rename `admin user` to `admin users` and `admin token` to `admin tokens`. Verify with a test that the old names exit with code 2.
- [ ] 6.2 Search `modules/`, `examples/`, `nix/` and `docs/` for `admin user` and `admin token`, and replace each use. Verify with `grep` that no use stays, and build the NixOS test of the flake.
- [ ] 6.3 Put the commands into the groups library, account, admin and server in the help of `rmhub`. Verify with a test that reads `rmhub --help`.
- [ ] 6.4 Update the command names and the help example in `docs/reference/cli.md`, `docs/tutorials/first-server.md`, `docs/how-to/deploy-on-nixos.md`, `docs/explanation/access.md` and `docs/reference/configuration.md`. Verify that each command of the changed text runs against a local server.

## 7. Path completion

- [ ] 7.1 Add a completion function for each entry argument and for the option `--in`. Verify with tests that call `rmhub __complete stat /Inbox/p` against a test server and against a closed port.
- [ ] 7.2 Add a section about shell completion to `docs/reference/cli.md`. Verify the procedure in bash with a local server.

## 8. Integration

- [ ] 8.1 Run `nix fmt` and verify that no file changes.
- [ ] 8.2 Run `go test ./...` and verify that all tests pass.
- [ ] 8.3 Run `nix flake check` and verify that all checks pass.
