# Proposal

## Why

rmhub has no limit on the number of requests of a client. A client on the
network can try pairing codes as fast as the server answers. An agent
with a scoped token that loops can send requests and start renders with
no limit, and so slow the server for the tablet. The security review of
the code found these points and left them open.

## What Changes

- Limit pairing attempts for each client address on the tablet listener.
- Limit the API requests for each token.
- Limit the page renders for each account. A render that the cache
  answers does not count.
- Answer a request above a limit with status 429 and a `Retry-After`
  header. Add the error code `rate_limited` to the API.
- Add the exit code 6 to the client for status 429. The client prints the
  wait time. It does not try again.
- Add the configuration table `[limits]` with one key for each limit, and
  the key `trusted_proxies` for each listener. The server reads the
  client address from `X-Forwarded-For` only if the connection comes from
  a trusted proxy.
- Delete the record of a pairing code after its use, and delete expired
  records when a new code is made. This does not change the behavior
  that a client sees.
- Add the proxy setting to the deployment guide.

The change does not limit the requests of a paired tablet. See the
Non-Goals in `design.md`.

## Capabilities

### New Capabilities

- `rate-limiting`: the limits, the keys of the limits, the answer above
  a limit, the client address behind a proxy, and the configuration.

### Modified Capabilities

- `rmhub-api`: the error format gets the code `rate_limited` with the
  status 429.
- `cli`: the exit codes get the code 6 for status 429.
- `deployment-guide`: the reverse proxy setup gets the forwarded client
  address and `trusted_proxies`.

## Impact

- New package for the limiter. No new dependency.
- `internal/config`: the table `[limits]` and `listen.<name>.trusted_proxies`.
- `internal/tablet`: the limit on `POST /token/json/2/device/new`.
- `internal/api`: the limit in the authentication step and in the
  renderer, and the new error code.
- `internal/pairing`: the deletion of code records.
- `cmd/rmhub`: the exit code 6.
- `README.md` and `docs/agents/SKILL.md`: the new keys, the nginx
  example and the exit code.
- The limits are in the memory of one server process. With two server
  processes on one S3 state store, each process has its own counters.
- Unknown: the behavior of the tablet when a pairing request gets status
  429. [INFERENCE] It shows the same failure as for a wrong code, because
  it shows that failure for each status that is not 200. The default
  limit permits 10 attempts in one minute, so a person does not reach it.
