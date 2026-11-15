# Design

## Context

See `proposal.md` for the reason. These facts of the current code shape
the design:

- The tablet listener and the API listener are two `http.ServeMux`
  values. Pairing is `POST /token/json/2/device/new` in
  `internal/tablet`. It needs no token.
- `api.Auth` checks the token of each API request and puts the
  principal, with the token ID and the account, into the request
  context.
- `api.Renderer.image` is the one place that computes a render. It
  first reads the cache.
- A pairing code has 8 letters (26^8 values) and is valid for 5 minutes.
  An API token secret has 52 base32 characters (260 bits).
- The deployment guide puts nginx in front of the tablet listener. Then
  each connection comes from 127.0.0.1.
- The state store has `List` with a prefix, so expired pairing records
  can be found.

## Goals / Non-Goals

**Goals:**

- A search for a pairing code from one address is too slow to succeed.
- One token or one account cannot use so many requests or renders that
  the tablet sync of the same server becomes slow.
- A client above a limit gets a clear answer and the wait time.

**Non-Goals:**

- No limit on the requests of a paired tablet. The first sync of a
  large library sends many thousand requests in a short time. The
  behavior of the tablet on status 429 is not known, and a sync that
  stops is worse than a busy server. The blob size limit and the root
  size limit already apply.
- No limit on requests with a token that is not valid. A token secret
  has 260 bits, so a search cannot succeed. A flood of such requests is
  a network problem; the reverse proxy is the correct place to stop it.
- No limit on the number of open change streams or websockets.
- No counters that two server processes share.
- No automatic retry in the client.

## Decisions

### One token bucket implementation, no dependency

A new package `internal/ratelimit` has one type: a map from a key to a
bucket, with a clock that tests can set. `Allow(key)` returns `ok` and
the wait time. A bucket holds the number of units and the time of the
last change. The map removes a bucket that is full again, on a sweep at
most one time in each minute, and never holds more than 100 000 keys:
at that size, a new key gets the answer of an empty bucket. This bounds
the memory when an attacker uses many addresses.

Alternative: `golang.org/x/time/rate` (BSD-3-Clause, compatible). It
gives the bucket but not the keyed map, the sweep or the size bound, so
it saves about 20 lines and adds a dependency and a `vendorHash` change.

### Three limits, three keys

| Limit | Key | Where | Default for one minute |
|---|---|---|---|
| pairing | client address | `tablet.deviceNew`, before the body is read | 10 |
| API | token ID | `api.Auth`, after the token check | 600 |
| render | account | `Renderer.image`, after the cache miss, before the semaphore | 60 |

The API limit is after the token check because the key is the token ID.
The admin token has no ID; its key is the fixed text `admin`.

The render limit is for each account and not for each token, because
the cost of a render is memory and CPU of the server, and child tokens
are free to make. Parallel requests for one image share one render, and
take one unit.

The page count of a PDF also goes through `Renderer.image`. It takes a
render unit too. This is correct: it starts PDFium.

With the pairing limit of 10 for one minute, one address can try 50
codes while a code is valid. The chance to find that code is 50 in
26^8, about 1 in 4 000 000 000. An attacker with 10 000 addresses has a
chance of about 1 in 400 000 for each code.

### The value -1 turns a limit off

The configuration uses 0 for "not set" in each other key, so 0 cannot
mean "off". A value of 0 in the file is thus an error and not a silent
default, so that an operator who writes 0 to turn the limit off gets a
clear message.

### Client address from `X-Forwarded-For` only for trusted peers

Without this, a client could send a new header value for each attempt
and never reach the pairing limit. The server reads the header from the
right, because the proxy appends the address that it saw, and skips
addresses that are trusted proxies. An entry of `trusted_proxies` is an
IP address or a CIDR network; a host name is an error.

The key of an IPv6 address is its /64 network, because one client
usually controls a full /64.

Only the pairing limit uses the client address. The key exists for
each listener so that a later limit on the API listener needs no new
key.

### Status 429 with `Retry-After`, and exit code 6

`Retry-After` is the wait time in seconds, rounded up. The API adds the
code `rate_limited`. The client maps status 429 to the new exit code 6
and prints the seconds. An agent can then wait and decide, and does not
confuse the limit with a conflict (code 5).

`rmhub watch` reads one stream; the stream takes one unit at its start.

### Pairing records

The state store has no delete with a version. So `pairing.Use` keeps
the conditional write of `usedBy`, which lets exactly one of two
tablets succeed, and deletes the record after that write. A tablet that
comes later gets "not found", which is the same status 400. If the
delete fails, the record stays as a used record, as today.
`pairing.Create` lists the prefix `pairing/` and deletes each expired
record before it makes the new code. A list of some records for each
new code is cheap.

## Risks / Trade-offs

- [Many users behind one address, for example a company network, share
  one pairing bucket] → 10 attempts in one minute is much for people.
  The operator can set a higher value.
- [A wrong `trusted_proxies` entry, for example `0.0.0.0/0`, lets each
  client choose its address] → The guide gives the warning. The default
  is an empty list.
- [The default API limit stops a correct agent that lists a large
  library with many small requests] → 600 for one minute is 10 requests
  for each second with a burst of 600. The answer gives the wait time.
  The operator can set a higher value or -1.
- [The default render limit stops a client that reads a long notebook]
  → Each page counts one time only, because the cache keeps the result.
  A notebook of 200 pages needs about 3 minutes after the first 60
  pages.
- [Two server processes double each limit] → Documented. Counters in
  the state store would cost one write for each request.
- [The tablet shows a wrong message on status 429 for pairing] → Not
  known. A person does not reach 10 attempts in one minute.

## Migration Plan

No data migration. After the update, the limits are on with the default
values. To go back to the old behavior, set each key of `[limits]` to
-1. Operators behind a reverse proxy must add `trusted_proxies` and the
`X-Forwarded-For` line, or all tablets share one pairing bucket.

## Open Questions

- Does the tablet show a clear message when pairing gets status 429?
  Test with a real tablet after the change. The answer does not change
  the design.
