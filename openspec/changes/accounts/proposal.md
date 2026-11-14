# Proposal

## Why

A tablet must pair with an account before it can sync. rmhub has no web
interface, so an operator and the users need another way to create
accounts, get pairing codes and remove lost devices. The CLI is that
way. It is a client of the rmhub API, so the API needs authentication
from the start.

## What Changes

- Add the rmhub API on the API listener: JSON over HTTPS under `/api/v1`,
  with one error format and bearer tokens.
- Add an admin token. The operator puts it in a file that the
  configuration names. It can create users and issue owner tokens.
- Add users. Each user has an opaque ID, an email address and a name.
  rmhub keeps no password, because there is no login page.
- Add owner tokens. An owner token has full access to one account. It can
  create pairing codes, list and remove devices, and create and revoke
  other tokens. A later change adds tokens limited to a folder.
- Add tablet pairing with the endpoints that reMarkable OS 3.27.1 calls:
  one-time codes, device tokens and user tokens.
- Add a device registry. When a user removes a device, the device can no
  longer sync. rmfakecloud does not do this.
- Add the `rmhub` client commands for admin and account work. They follow
  rules that suit both people and AI agents: non-interactive, `--json`,
  fixed exit codes, and settings from the environment.

## Capabilities

### New Capabilities

- `rmhub-api`: the base rules of the rmhub API: path prefix, JSON bodies,
  error format, status codes and bearer authentication.
- `accounts`: users and the admin operations that manage them.
- `api-tokens`: the admin token and owner tokens: format, storage,
  check and revocation.
- `device-pairing`: pairing codes, the tablet token endpoints, tablet
  authentication and the device registry.
- `cli`: the client commands, their settings, output formats and exit
  codes.

### Modified Capabilities

None.

## Impact

- New packages for accounts, tokens, pairing, the API base and the CLI.
- New dependencies: `golang-jwt/jwt/v5` (MIT) and `spf13/cobra`
  (Apache-2.0).
- The server configuration gets the key `admin.token_file`.
- The tablet endpoints use the tablet listener from the change
  `foundation`. The rmhub API uses the API listener.
- No machine or clan service is affected.
