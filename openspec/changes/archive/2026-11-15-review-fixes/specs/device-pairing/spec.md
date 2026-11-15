# Spec Delta

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Device token
A device token MUST be a JWT signed with HS256 and the header `kid` `1`.
Its claims MUST be `auth0-userid` with the value `auth0|<account ID>`,
`device-desc`, `device-id`, `rmhub-pairing` and `aud` with the value
`api`. A device token MUST NOT have an expiry time.

#### Scenario: Claims of a device token
- **WHEN** a tablet with the description `remarkable` and the ID D pairs with the account A
- **THEN** the token has `auth0-userid` `auth0|A`, `device-desc` `remarkable` and `device-id` D

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
