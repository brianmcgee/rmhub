# Spec Delta

## Purpose

A tablet keeps a websocket open to learn that another device changed the
library. After a message it reads the root and syncs.

## ADDED Requirements

### Requirement: Notification websocket
`GET /notifications/ws/json/1` on the tablet listener MUST upgrade to a
websocket for a request with a valid user token. A request without a
valid user token MUST get status 401 and no upgrade.

#### Scenario: Tablet connects
- **WHEN** a tablet opens `/notifications/ws/json/1` with a valid user token
- **THEN** the server accepts the websocket

#### Scenario: No token
- **WHEN** a client opens `/notifications/ws/json/1` without a token
- **THEN** the server answers with status 401

### Requirement: SyncComplete message
For each notification, the server MUST send this text message to each
websocket of the account, except the websockets of the source device:
`{"message":{"attributes":{"auth0UserID":"<account ID>","event":"SyncComplete","sourceDeviceDesc":"<desc>","sourceDeviceID":"<device ID>"},"messageid":"<id>"}}`.
The message ID MUST be unique for each notification.

#### Scenario: Two tablets
- **GIVEN** the devices D1 and D2 of the account A have open websockets
- **WHEN** the device D1 writes a root with `broadcast` `true`
- **THEN** the device D2 gets one `SyncComplete` message with `sourceDeviceID` D1
- **AND** the device D1 gets no message

#### Scenario: Another account
- **GIVEN** a device of the account B has an open websocket
- **WHEN** a device of the account A writes a root with `broadcast` `true`
- **THEN** the device of the account B gets no message

### Requirement: Slow websocket
A websocket that does not read its messages MUST NOT delay a root write or
the messages to other websockets. The server MUST close a websocket that
has 64 unsent messages.

#### Scenario: Stalled tablet
- **GIVEN** a websocket that reads no data
- **WHEN** the account gets 100 notifications
- **THEN** each root write completes
- **AND** the server closes the stalled websocket

### Requirement: Keepalive
The server MUST send a websocket ping every 30 seconds. The server MUST
close a websocket that does not answer a ping within 60 seconds.

#### Scenario: Lost connection
- **WHEN** a tablet stops to answer pings
- **THEN** the server closes its websocket within 90 seconds

### Requirement: Websocket end at device removal
The server MUST close a websocket within 60 seconds after the device of
its token is removed.

#### Scenario: Device removed
- **GIVEN** the device D has an open websocket
- **WHEN** a user removes the device D
- **THEN** the server closes the websocket of the device D within 60 seconds
