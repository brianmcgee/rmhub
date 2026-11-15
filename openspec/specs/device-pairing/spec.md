# device-pairing Specification

## Purpose
A reMarkable tablet or app pairs with an account through a one-time code.
After pairing it gets the tokens that the sync protocol needs. A user can
remove a device to stop its access.

## Requirements

### Requirement: Create a pairing code
`POST /api/v1/account/pairing-codes` MUST return status 201 with
`{"code":"…","expiresAt":"…"}`. The code MUST be 8 random lowercase letters.
The code MUST expire 5 minutes after its creation.

#### Scenario: New code
- **WHEN** an owner client creates a pairing code
- **THEN** the server returns 8 lowercase letters and a time 5 minutes later

### Requirement: Pair a device
`POST /token/json/2/device/new` on the tablet listener MUST accept the body
`{"code":"…","deviceDesc":"…","deviceID":"…"}`. The code comparison MUST
ignore case. For a valid code, the server MUST register the device in the
account. The server MUST then answer with status 200, the media type
`text/plain` and a device token as the body.

#### Scenario: Valid code
- **GIVEN** an unused pairing code `abcdefgh` that has not expired
- **WHEN** a tablet sends the code `ABCDEFGH` with the device ID D
- **THEN** the server answers with status 200 and a device token
- **AND** the account lists the device D

### Requirement: One-time codes
A pairing code MUST pair at most one device. An unknown code, an expired
code or a used code MUST get status 400 with an empty body.

#### Scenario: Used code
- **GIVEN** a pairing code that paired a device
- **WHEN** a second tablet sends the same code
- **THEN** the server answers with status 400 and an empty body

#### Scenario: Two tablets race
- **WHEN** two tablets send the same valid code at the same time
- **THEN** exactly one tablet gets a device token
- **AND** the other tablet gets status 400

#### Scenario: Expired code
- **WHEN** a tablet sends a code 6 minutes after its creation
- **THEN** the server answers with status 400

### Requirement: Device token
A device token MUST be a JWT signed with HS256 and the header `kid` `1`.
Its claims MUST be `auth0-userid` with the value `auth0|<account ID>`,
`device-desc`, `device-id`, `rmhub-pairing` and `aud` with the value
`api`. A device token MUST NOT have an expiry time.

#### Scenario: Claims of a device token
- **WHEN** a tablet with the description `remarkable` and the ID D pairs with the account A
- **THEN** the token has `auth0-userid` `auth0|A`, `device-desc` `remarkable` and `device-id` D

### Requirement: User token
`POST /token/json/2/user/new` with a device token as the bearer token MUST
answer with status 200, the media type `text/plain` and a user token. The
user token MUST be a JWT signed with HS256 that expires 3 hours after its
creation. A device token for a removed device MUST get status 401.

#### Scenario: Renew a user token
- **WHEN** a paired tablet sends its device token to `/token/json/2/user/new`
- **THEN** the server answers with a user token that expires in 3 hours

#### Scenario: Removed device
- **GIVEN** a user removed the device D
- **WHEN** the device D sends its device token to `/token/json/2/user/new`
- **THEN** the server answers with status 401

### Requirement: User token claims
A user token MUST have these claims: `auth0-profile` with `UserID`
`auth0|<account ID>`, `Email`, `Name`, `EmailVerified` `true`, and
`CreatedAt` and `UpdatedAt` as RFC 3339 times. It MUST also have
`device-desc`, `device-id`, `rmhub-pairing`, `scopes`, `level`
`connect`, `tectonic` `eu`, `version` `10`, `iss` `rM WebApp`, `sub`,
`jti`, `iat`, `nbf` and `exp`.

#### Scenario: Scopes select sync 1.5
- **WHEN** the server issues a user token
- **THEN** the claim `scopes` is `intgr screenshare docedit sync:tortoise`

### Requirement: Tablet authentication
Each sync endpoint on the tablet listener MUST require a user token as the
bearer token. The token MUST have a valid signature, MUST NOT be expired,
MUST have `version` `10` and MUST have a `UserID` in `auth0-profile`. A
request that fails a condition MUST get status 401 with the body
`Unauthorized`.

#### Scenario: Device token on a sync endpoint
- **WHEN** a tablet sends a device token to a sync endpoint
- **THEN** the server answers with status 401

### Requirement: Device registry
`GET /api/v1/account/devices` MUST list the devices of the account with
device ID, description and pairing time. `DELETE
/api/v1/account/devices/{id}` MUST remove the device and return status
204. Within 60 seconds after the removal, each request with a user token
of the device MUST get status 401.

#### Scenario: Remove a lost tablet
- **GIVEN** the device D has a user token that expires in 2 hours
- **WHEN** a user removes the device D
- **THEN** a sync request with that user token gets status 401 within 60 seconds

### Requirement: Device delete endpoint
`POST /token/json/2/device/delete` and `POST /token/json/3/device/delete`
with a device token MUST remove the device from the registry and answer
with status 204. An invalid device token MUST get status 401.

#### Scenario: Tablet unpairs
- **WHEN** a tablet sends its device token to `/token/json/3/device/delete`
- **THEN** the server answers with status 204
- **AND** the account no longer lists the device

### Requirement: Device ID format
The server MUST refuse a pairing request whose `deviceID` is empty, is
longer than 128 bytes, or contains a character outside `A-Z`, `a-z`,
`0-9` and `-`, with status 400.

#### Scenario: Device ID with a slash
- **WHEN** a tablet sends the device ID `../x` with a valid code
- **THEN** the server answers with status 400
- **AND** the code stays unused

### Requirement: Pairing ID
Each pairing of a device MUST get a random pairing ID. The device token
and each user token of that pairing MUST hold the ID in the claim
`rmhub-pairing`. The server MUST refuse a device token or a user token
whose pairing ID is not the ID of the current pairing of the device,
with status 401. A device record without a pairing ID MUST accept only
a token without a pairing ID.

#### Scenario: Device pairs again after its removal
- **GIVEN** a user removed the device D, and the device D paired again with a new code
- **WHEN** a client sends the device token of the first pairing to `/token/json/2/user/new`
- **THEN** the server answers with status 401

#### Scenario: User token of an earlier pairing
- **GIVEN** the device D has a user token, and the device D paired again
- **WHEN** a client sends that user token to a sync endpoint
- **THEN** the server answers with status 401

#### Scenario: Device of an earlier version
- **GIVEN** a device record and a device token that have no pairing ID
- **WHEN** the device sends the token to `/token/json/2/user/new`
- **THEN** the server answers with status 200 and a user token
