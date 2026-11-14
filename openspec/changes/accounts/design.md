# Design

## Context

The change `foundation` gives the state store, the two listeners and the
configuration. The state store has only single-key conditional writes.
Each record of this change must fit that model, and each operation that
touches two keys must stay correct when it stops halfway.

The tablet side copies the wire behavior that rmfakecloud shows to work
with reMarkable OS 3.27.1. The source is the rmfakecloud study (commit
`962a73b`): `internal/app/handlers.go`, `codeconnector.go`, `claims.go`
and `middleware.go`. rmhub does not copy its code.

## Goals / Non-Goals

**Goals:**

- Accounts, owner tokens, pairing and devices with no multi-key
  transaction.
- Tablet tokens that OS 3.27.1 accepts.
- Device removal that takes effect without a token expiry.

**Non-Goals:**

- Delete an account and its library. This needs garbage collection.
- Tokens limited to a folder. The change `scoped-api` adds them.
- Rotation of the JWT signing key.
- The `Rm-Token-Ttl-Hint` header. rmfakecloud defines it, but the study
  did not show that a tablet needs it.

## Decisions

### Layout in the state store

```
server/jwt-key                         32 random bytes, base64
emails/<sha256(lowercase email)>       {"account":"<id>"}
accounts/<id>/account                  {"id","email","name","createdAt"}
accounts/<id>/tokens/<token id>        {"secretHash","label","createdAt","expiresAt"}
accounts/<id>/devices/<sha256(device ID)>  {"deviceId","desc","pairedAt"}
pairing/<code>                         {"account","expiresAt","usedBy"}
```

The device key is a hash because a device ID can contain uppercase
letters, and the state key format does not allow them. The record keeps
the real ID.

### Create a user with two keys

The server writes the email claim first with put-if-absent, and then the
account record. If the claim exists, the server reads the account that
the claim names. If that account exists, the result is status 409. If it
does not exist, an earlier create stopped halfway. The server then takes
over the claim with put-if-match and continues. So an orphan claim never
blocks an email address for ever, and a listing never shows an account
without its claim.

### Admin token

The operator generates the token with `rmhub admin token generate` and
puts it in a file, for example with sops-nix or agenix. The server reads
the file at startup and keeps the SHA-256 hash. A configuration value is
a better fit for NixOS than a token that the server generates on its
first start, because the operator already has a secret store.

### Owner tokens

The format `rmh_<account>_<id>_<secret>` lets the server find the record
with one get: `accounts/<account>/tokens/<id>`. The secret has 260 bits
of entropy, so a fast hash (SHA-256) is enough. A slow password hash is
not necessary. The server reads the record on each request, so a
revocation takes effect at once.

The alternative was macaroons, which allow a client to narrow a token
offline. The change `scoped-api` gets the same result with child tokens
that the server stores. That is simpler and makes revocation easy.

### JWT signing key

At its first start the server writes `server/jwt-key` with put-if-absent.
If the key exists, the server reads it. So all processes that share a
state store use one key. The library is `golang-jwt/jwt/v5`.

### Pairing codes

A code is 8 letters from `crypto/rand`, which gives about 37 bits. It
lives 5 minutes. To use a code, the server reads the record and writes
`usedBy` with put-if-match. Only one writer can win, so a code pairs one
device. A request with an expired or used record gets status 400. The
server deletes the record of a code when it finds that the code expired.
If the device ID is not valid, the server answers with status 400 before
it writes the record, so the code stays unused.

When a device pairs again with the same device ID, the server replaces
the device record.

### Device removal within 60 seconds

User tokens live 3 hours and carry no revocation data. Each request on
the tablet listener checks that the device record exists. A cache in
memory keeps the result for 30 seconds for each account and device. The
DELETE endpoint removes the cache entry in its own process. Another
process sees the removal within 30 seconds. The spec allows 60 seconds.

### Tablet token endpoints

The endpoints answer with the same media types, status codes and claims
as rmfakecloud. The user token omits the scopes `hwcmail:-1`, `hwc` and
`mail:-1`, because rmhub has no handwriting recognition and no email.
The `Email` claim holds only the email address. rmfakecloud adds the
text `(via <host>)`.

### CLI

The library is `spf13/cobra`. It gives nested commands and help text.
The admin command `rmhub admin token generate` and the server command
`rmhub serve` are in the same program as the client commands. The client
maps the API error code to an exit code. A connection error gives exit
code 1.

`client.toml` holds `url` and `token`. The client warns on standard
error when the file can be read by other users.

## Risks / Trade-offs

- [A tablet needs a claim or header that rmfakecloud sets and rmhub
  omits] → The omitted scopes are optional features. The Email claim is
  display text. The ddvk/rmapi check of the change `tablet-sync` pairs a
  client with rmhub in CI. That catches a broken token form for rmapi,
  but it does not prove the tablet.
- [37 bits in a pairing code] → A code lives 5 minutes and works once.
  The risk is small at personal scale. A later change can add a rate
  limit on `device/new`.
- [One state read for each tablet request for the device check] → The
  30-second cache removes most reads.
