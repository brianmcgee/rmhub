# Tasks

## 1. Limiter

- [x] 1.1 Write `internal/ratelimit` with the keyed token bucket and a clock that tests can set. Verify with a test that a limit of 10 accepts 10 requests, refuses request 11, and accepts one more after 6 seconds.
- [x] 1.2 Add the wait time to the answer of `Allow`. Verify with a test that the wait time of an empty bucket of 10 is 6 seconds.
- [x] 1.3 Add the sweep of full buckets and the bound of 100 000 keys. Verify with a test that the map has no key after a sweep of full buckets, and that key 100 001 is refused.
- [x] 1.4 Verify with `CGO_ENABLED=1 go test -race ./internal/ratelimit` that parallel calls of `Allow` have no data race.

## 2. Configuration

- [x] 2.1 Add the table `[limits]` with `pairing_per_minute`, `api_per_minute` and `render_per_minute` to `internal/config`. Verify with a test that the defaults are 10, 600 and 60, that -1 is accepted, and that 0 and -2 give an error that names the key.
- [x] 2.2 Add `trusted_proxies` to each listener. Verify with a test that an address and a CIDR network are accepted and that `proxy.local` gives an error that names the key.
- [x] 2.3 Add the new keys to the configuration table of `README.md`. Verify that each key of `internal/config` is in the table.

## 3. Client address

- [ ] 3.1 Write the function that returns the client address of a request from the peer address, `X-Forwarded-For` and the trusted proxies. Verify with a test for each scenario of the requirement `Client address`.
- [ ] 3.2 Make the key of an IPv6 address its /64 network. Verify with a test that two addresses of one /64 give one key.

## 4. Pairing limit

- [ ] 4.1 Write a failing test in `internal/app` that sends 11 pairing requests with wrong codes and expects status 429 with an empty body and a `Retry-After` header on request 11.
- [ ] 4.2 Add the limit to `POST /token/json/2/device/new`, before the body is read. Verify that the test of 4.1 passes.
- [ ] 4.3 Verify with a test that a pairing request through a trusted proxy with a different `X-Forwarded-For` address succeeds while the bucket of the first address is empty.
- [ ] 4.4 Delete the record of a pairing code after its use. Verify with a test that the state store has no key with the prefix `pairing/` after a pairing, and that the test of two tablets with one code still passes.
- [ ] 4.5 Delete each expired record when a new code is made. Verify with a test that an expired record is gone after the next code creation.

## 5. API and render limits

- [ ] 5.1 Add the code `rate_limited` with status 429 to `internal/api`, with the `Retry-After` header. Verify with a test that the error body and the header are correct.
- [ ] 5.2 Write a failing test in `internal/app` with `limits.api_per_minute = 5`: request 6 of a scoped token gets status 429, and a request of the owner token gets status 200.
- [ ] 5.3 Add the limit to `api.Auth`, after the token check. Verify that the test of 5.2 passes and that `GET /healthz` has no limit.
- [ ] 5.4 Write a failing test in `internal/app` with `limits.render_per_minute = 2`: the render of a third page gets status 429, and a second request for the first page gets status 200.
- [ ] 5.5 Add the limit to `Renderer.image`, after the cache miss. Verify that the test of 5.4 passes.
- [ ] 5.6 Verify with a test that `limits.api_per_minute = -1` accepts 1000 requests.

## 6. Client

- [ ] 6.1 Map status 429 to the exit code 6 in `cmd/rmhub`, with the seconds of `Retry-After` in the message. Verify with a test against a server with `limits.api_per_minute = 1` that the second command exits with code 6 and prints the seconds.
- [ ] 6.2 Add the exit code 6 to the help text of the root command and to `docs/agents/SKILL.md`, with the instruction to wait and try again. Verify that `TestEachCommandHasHelp` passes.

## 7. Deployment guide

- [ ] 7.1 Add the `X-Forwarded-For` line to the nginx example of `README.md`, and the `trusted_proxies` setting with the warning. Verify the nginx example with `nginx -t`.
- [ ] 7.2 Add a section on the limits to `README.md`: the three limits, the keys, the value -1 and the note about two server processes. Verify that the section names each key of `[limits]`.
- [ ] 7.3 Add `trusted_proxies` for the nginx setup to the NixOS VM test or to a new check, with one pairing through the proxy. Verify that the check passes.

## 8. Integration

- [ ] 8.1 Run `nix fmt` and verify that no file changes.
- [ ] 8.2 Run `golangci-lint run ./...` and verify that it reports 0 issues.
- [ ] 8.3 Run `go test ./...` and verify that all tests pass.
- [ ] 8.4 Run `nix flake check` and verify that all checks pass.
- [ ] 8.5 Start the built server, send 11 pairing requests with `curl`, and verify that request 11 gets status 429.

## Workflow follow-up

- Test a pairing above the limit with a real tablet and record the message that the tablet shows.
- Archive the change after the review.
