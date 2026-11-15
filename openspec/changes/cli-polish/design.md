# Design

## Context

See `proposal.md` for the reasons. This change builds on the change
`library-path-ergonomics`. These facts come from the code in `cmd/rmhub`
and from a run of each command against a local server:

- `exitCode` in `client.go` maps only the statuses 401, 403, 404, 409,
  412 and 429. Each other status gives code 1. `parseAPIError` keeps the
  error body only when it has the form of the rmhub API.
- The tablet listener answers an API path with status 404 and no rmhub
  error body.
- `writeNew` writes a temporary file and links it to the output path.
  The link fails when the path exists. An error from `os.CreateTemp`
  shows the name of the temporary file.
- The admin token starts with `rmh_admin_`. An account token starts with
  `rmh_` and the account ID.
- `resolveSettings` reads `client.toml` only when the options and the
  environment do not give both values. It refuses an unknown key.
- The root command uses cobra. cobra has command groups and a completion
  function for arguments. `rmhub completion` exists already.
- The spec `cli` forbids a command that waits for input from a terminal.

## Goals / Non-Goals

**Goals:**

- Each error message tells the user what to change.
- Each exit code agrees with its class in the spec `cli`.
- A new user can go from a token to a working client with one command.

**Non-Goals:**

- A command that asks for a value on the terminal. `rmhub login` reads
  the same sources as each other command.
- Automatic retry of a request.
- `mv` with more than one source entry. The destination of `mv` can be a
  new name, so a list of sources is ambiguous.
- A fix for the page count of an uploaded PDF. `stat` shows
  `pageCount` 0 until a tablet opens the document, while `rmhub pages`
  shows the pages. That is a server topic.
- Completion of IDs, tokens or device IDs.

## Decisions

### Exit codes follow the error body

`exitCode` gets two changes. Status 400 gives code 2. Status 404 gives
code 4 only when `apiError.code` is set, which means that the answer has
the rmhub error body. In the other case the code is 1, and the message
names the URL and tells the user to check that it is the API listener.

Alternative: apply the body rule to all statuses. A reverse proxy can
answer with 401 or 429 without the body, and code 3 or 6 is then still
the correct class. The wrong-address problem shows as 404 only, so the
rule stays narrow.

A connection error gets the text `cannot connect to <url>: <reason>`.
The client checks the URL one time in `resolveSettings`: the scheme must
be `http` or `https`, and the host must not be empty.

### An existing output file is a conflict

`get` and `render` exit with code 5 for an existing file. Code 1 means
"unexpected", and a script cannot tell an existing file from a network
error. `--force` writes the temporary file and then renames it to the
output path, so a failed download does not damage the old file.

`-o` with an existing directory, or with a `/` at its end, writes the
default file name into that directory. Before the download, the client
checks that the directory of the output file exists.

### `~` expands in the client

One function expands the path. It changes `~` and `~/…` to the home
directory from `os.UserHomeDir`. It does not expand `~user`. The
function runs for the file arguments of `upload`, for `-o` and for
`serve --config`. A shell does this for an argument without quotation
marks. A program that is not a shell does not, and agents call rmhub
that way.

A file with the name `~` is reachable as `./~`.

### Several arguments run in sequence and stop at the first error

The client does one request for each argument, in the given sequence. It
does not run them in parallel: the request limits of the server apply to
each token, and the output must have a stable sequence.

With `--json`, a command with one argument prints the object, as before.
A command with more than one argument prints one array. On an error, the
array contains the completed results, and the error goes to standard
error.

Alternative: always print an array. That breaks each script that reads
the object. Alternative: print one object for each line. The spec `cli`
demands one JSON value. The cost of the decision is that the form
depends on the number of arguments. The reference page must state this.

### `rmhub login` saves the resolved settings

`login` calls `resolveSettings`, so `--url`, `--token`, the environment
and the old file all work. The recommended form is
`RMHUB_TOKEN=… rmhub login --url …`, because an option value is visible
in the process list.

The check is one GET request. For an admin token, the request is
`/api/v1/admin/users`. For each other token, it is `/api/v1/library`,
which owner tokens and scoped tokens can read. The client writes the
file to a temporary file with mode 0600 in the same directory and
renames it. It creates the directory with mode 0700.

Alternative: `rmhub config set <key> <value>`. It needs two commands and
does not check the values.

### The admin token hint uses the token prefix

The client knows from the prefix `rmh_admin_` that the token is the
admin token. When a command outside `rmhub admin` gets status 403 with
such a token, the client adds the hint to the message. The server does
not change.

### Command groups and plural names

cobra groups give the help four parts: library, account, admin and
server. `version`, `completion` and `help` go into the server group.
`admin user` and `admin token` get the plural form, as `tokens` and
`devices` have. The old names go away without an alias.

### Usage errors show the usage line

`wrapArgs` and the flag error function add `usage: <use line>` and
`Run rmhub <command> --help` to the error. The messages of cobra for
the number of arguments stay, with the correct plural.

### Completion reads one folder

A completion function for entry arguments splits the typed text at the
last `/`. It resolves the folder part and lists that folder with a
timeout of 2 seconds. It offers `<folder>/<name>`, with a `/` after a
folder name. Each error gives no completion and no output, because
output from a completion function damages the command line.

### One line for a write

The text of a write command is `<id>  <path>`. The full entry stays in
`--json` and in `rmhub stat`. A trashed entry of a scoped token has no
path, so the line then has the ID only.

### Days in `--expires`

`parseExpires` accepts `<n>d` as n times 24 hours before it calls
`time.ParseDuration`. Weeks, months and years stay out: a month has no
fixed length.

## Risks / Trade-offs

- [Scripts depend on exit code 1 for status 400 or an existing file] →
  The reference page lists each changed code. The project is early, and
  the agent guide changes in the same commit.
- [The JSON form depends on the number of arguments] → A script that
  needs one form passes one argument for each call.
- [A token in `--token` of `login` is visible in the process list] →
  The help text and the reference page recommend `RMHUB_TOKEN`.
- [`--force` replaces a file that the user wants to keep] → The option
  has no short form and no default.
- [Completion sends requests while the user types] → One list request
  for each tab key, with a timeout of 2 seconds. The request limits of
  the server apply.
- [The deployment files call `admin user` or `admin token`] → Task 6.2
  searches `modules/`, `examples/` and `nix/`.

## Migration Plan

Apply this change after `library-path-ergonomics` is archived, because
both changes modify the requirement `Library commands`. The server does
not change, so a new client works with an old server.
