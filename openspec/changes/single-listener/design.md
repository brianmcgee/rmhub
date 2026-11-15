# Design

## Context

See `proposal.md` for the reason. These facts come from the code:

- `App.TabletHandler` and `App.APIHandler` in `internal/app/app.go` each
  make one mux. `cmd/rmhub/serve.go` gives each mux to one listener.
- Each path of the rmhub API starts with `/api/`. The tablet protocol
  uses `/discovery/`, `/service/`, `/token/`, `/sync/`, `/notifications/`,
  `/analytics/`, `/settings/` and some report paths. No tablet path
  starts with `/api/`. The two handlers have only `GET /healthz` in
  common, and it is the same function.
- The tablet mux answers each unknown path with status 404 and a log
  entry at the warning level. The API mux answers each unknown path
  under `/api/` with status 404 in the API error format.
- The limits run in the handlers, not in the listeners. The pairing
  handler writes the empty 429 answer. The API authentication writes the
  JSON 429 answer, after it accepted the token. A request with a token
  that is not valid takes no unit.
- Only `listen.tablet.trusted_proxies` has an effect: the pairing limit
  reads the client address. `listen.api.trusted_proxies` is checked and
  not used.
- `config.Load` sets the default `127.0.0.1:8081` for
  `listen.api.address` and refuses two equal addresses.
- The NixOS module sets both addresses with `mkDefault` and opens both
  ports with `openFirewall`.

## Goals / Non-Goals

**Goals:**

- A server on one private network needs one address.
- An operator can keep the API away from a public tablet address, as
  today.
- A configuration that sets `listen.api.address` behaves as before.

**Non-Goals:**

- A new name for the table `listen.tablet`. The name stays, so each
  existing configuration of the tablet listener stays valid.
- A separate listener for the admin endpoints only.
- A limit for API requests with a token that is not valid. The risk
  below names it.
- More than one certificate on one listener.

## Decisions

### The key `listen.api.address` selects the mode

Without the key, the server has one listener. With the key, it has two.
No second key such as `listen.mode` exists, so the configuration cannot
contradict itself.

The default is one listener. A first server then needs no decision, and
the tutorial has one URL. The operator of a public tablet address must
add one key. The documentation of TLS for the tablet tells that.

Alternative: keep the default of two listeners and accept the same
address in both keys as "one listener". Each table has its own TLS files
and proxy list, and two tables for one socket can disagree.
Alternative: a list of listeners, each with a list of protocols. It
permits more forms than an operator needs.

### One mux for one listener

`App` gets a function that returns the handlers for the configuration:

- One listener: one mux with the tablet endpoints, the pattern `/api/`
  for the API mux, `GET /healthz`, and the tablet answer for each other
  path.
- Two listeners: the two handlers of today.

The patterns of `http.ServeMux` do not conflict: `/api/` is more
specific than `/`. The answers thus follow the path: the API error
format under `/api/`, the tablet answers for each other path. No handler
changes.

### Keys of `listen.api` need the address

`listen.api.tls_cert_file`, `listen.api.tls_key_file` or
`listen.api.trusted_proxies` without `listen.api.address` is a
configuration error. Without the check, the server ignores the keys and
the operator does not see it. The error for two equal addresses stays.
Its message tells the operator to remove `listen.api.address`.

### The log names the protocols

The entry `listening` gets the field `serves` with the value
`tablet,api`, `tablet` or `api`. The field `listener` stays. An operator
then sees in the log where the API is.

### The NixOS module follows the server default

The module does not set `listen.api.address`. `openFirewall` opens the
tablet port, and the API port only when the settings have the key.
`examples/nixos.nix` keeps its explicit API address on the loopback
interface, because its tablet listener is public. The VM test thus
continues to test two listeners. A Go test covers one listener.

## Risks / Trade-offs

- [An operator changes a public tablet address to the new version and
  does not read the notes. The admin endpoints are then public] → Only
  a configuration without `listen.api.address` changes, and that
  configuration had the API on the loopback interface. Each client of
  that API fails with a connection error, which shows the change. The
  log entry names the protocols of each listener. The deployment guide
  tells the operator to set the key for a public address.
- [With one public listener, a search for an API token has no limit,
  because only a valid token takes a unit] → Tokens have high entropy.
  The documentation recommends two listeners for a public address. A
  limit for each client address can be a later change.
- [reMarkable adds a tablet endpoint under `/api/`] → No such endpoint
  is in the code of the tablet protocol today. It is not verified with
  a capture of a current tablet. With two listeners, the tablet listener
  can serve such a path.
- [The name "tablet listener" is not exact for a listener that serves
  the API] → The documentation tells that the tablet listener serves the
  API when no API listener exists. A new name breaks each configuration.
- [The change of the CLI message for a wrong URL in the change
  `cli-polish` has less use] → It stays correct for two listeners.

## Migration Plan

1. A deployment with `listen.api.address` in its configuration needs no
   step.
2. A deployment that used the default API address has two choices: add
   `listen.api.address = "127.0.0.1:8081"`, or change `RMHUB_URL` and
   `client.toml` of each client to the tablet address.
3. A NixOS host that used the module default has the same two choices
   in `services.rmhub.settings`.

A return to the old version needs no data change.
