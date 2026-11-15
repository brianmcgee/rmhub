# rate-limiting Specification

## Purpose
Rate limiting keeps one client from using the server so much that other
clients get slow answers, and makes a search for a valid pairing code too
slow to succeed.

## Requirements

### Requirement: Limit model
Each limit MUST be a bucket of N units for one key. A request MUST take
one unit. The bucket MUST fill at the rate of N units in one minute, up
to N units. A request for a key with an empty bucket MUST be refused.
The server MUST keep the buckets in memory. A restart MUST fill each
bucket.

#### Scenario: Burst and refill
- **GIVEN** a limit of 10 for one minute
- **WHEN** a client sends 11 requests in one second
- **THEN** the server accepts 10 requests and refuses request 11
- **AND** the server accepts one more request 6 seconds later

### Requirement: Answer above a limit
A refused request MUST get status 429 and the header `Retry-After` with
the number of seconds until one unit is available, as an integer of 1 or
more. On the API listener, the body MUST be the API error format with
the code `rate_limited`. On the tablet listener, the body MUST be empty.
A refused request MUST NOT change data.

#### Scenario: API request above the limit
- **WHEN** a token sends an API request and its bucket is empty
- **THEN** the server answers with status 429 and a `Retry-After` header
- **AND** the body is `{"error":{"code":"rate_limited","message":"…"}}`

### Requirement: Pairing limit
`POST /token/json/2/device/new` MUST have a limit for each client
address. Each request MUST take one unit, with a valid code or not. The
key of an IPv6 address MUST be its /64 network. The default limit MUST
be 10 for one minute.

#### Scenario: Search for a code
- **WHEN** one address sends 11 pairing requests with wrong codes in one minute
- **THEN** request 11 gets status 429 with an empty body

#### Scenario: Second address
- **GIVEN** the bucket of the address A is empty
- **WHEN** the address B sends a pairing request with a valid code
- **THEN** the server pairs the device

### Requirement: API request limit
Each request on the API listener with a valid token MUST take one unit
from the bucket of that token. The key MUST be the token ID, so a child
token MUST NOT use the bucket of its parent. The default limit MUST be
600 for one minute. `GET /healthz` MUST NOT have a limit.

#### Scenario: Agent in a loop
- **GIVEN** an owner token and a scoped token of one account
- **WHEN** the scoped token sends 700 requests in one minute
- **THEN** the server refuses requests of the scoped token with status 429
- **AND** the server accepts a request of the owner token

### Requirement: Render limit
Each page render that the server computes MUST take one unit from the
bucket of the account. A render request that the cache answers, or that
gets status 304, MUST NOT take a unit. The default limit MUST be 60 for
one minute.

#### Scenario: Cached page
- **GIVEN** the render bucket of an account is empty
- **WHEN** a client requests a page that is in the render cache
- **THEN** the server answers with status 200 and the image

#### Scenario: New pages above the limit
- **WHEN** a client requests 61 different pages in one minute
- **THEN** request 61 gets status 429

### Requirement: Limit configuration
The configuration table `[limits]` MUST have the keys
`pairing_per_minute`, `api_per_minute` and `render_per_minute`. A key
that is not set MUST use its default. The value -1 MUST turn the limit
off. A value of 0 or a value below -1 MUST be a configuration error
that names the key.

#### Scenario: Limit turned off
- **GIVEN** `limits.api_per_minute = -1`
- **WHEN** a token sends 1000 requests in one minute
- **THEN** the server accepts each request

#### Scenario: Bad value
- **WHEN** an operator sets `limits.render_per_minute = 0`
- **THEN** the server stops with an error that names `limits.render_per_minute`

### Requirement: Client address
The client address MUST be the peer address of the connection. Each
listener MUST have the key `trusted_proxies`, a list of addresses and
CIDR networks, empty by default. If the peer address is in the list,
the client address MUST be the rightmost address in `X-Forwarded-For`
that is not in the list. If there is no such address, the client
address MUST be the peer address.

#### Scenario: Header from an untrusted peer
- **GIVEN** `trusted_proxies` is empty
- **WHEN** a client sends a pairing request with `X-Forwarded-For: 203.0.113.9`
- **THEN** the limit uses the peer address of the client

#### Scenario: Header from a trusted proxy
- **GIVEN** `listen.tablet.trusted_proxies = ["127.0.0.1"]`
- **WHEN** the proxy on 127.0.0.1 sends a request with `X-Forwarded-For: 198.51.100.7, 203.0.113.9`
- **THEN** the limit uses the address 203.0.113.9

#### Scenario: Bad entry
- **WHEN** an operator sets `listen.api.trusted_proxies = ["proxy.local"]`
- **THEN** the server stops with an error that names `listen.api.trusted_proxies`
