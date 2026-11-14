# Spec Delta

## Purpose

An agent or a program reacts to changes in the library, for example a new
page in a notebook. The change feed tells it which entries changed, in
the order of the root log, with no gaps after a reconnect.

## ADDED Requirements

### Requirement: Feed endpoint
`GET /api/v1/changes` MUST be a stream endpoint with the media type
`text/event-stream`. Each root generation with visible changes MUST give
one event with the name `change`, the SSE `id` set to the generation and
a JSON `data` line.

#### Scenario: Tablet adds a page
- **GIVEN** a client has the feed open
- **WHEN** a tablet commits a change to the notebook N
- **THEN** the client gets one `change` event with the new generation

### Requirement: Event data
The data MUST be
`{"generation":<g>,"source":{"type":"…","id":"…"},"changes":[…]}`. Each
change MUST have `id`, `op` and the entry as in the capability `library`,
or `null` for the entry when `op` is `removed`. The value of `op` MUST be
`created`, `updated`, `moved`, `trashed` or `removed`.

#### Scenario: Document trashed
- **WHEN** a tablet moves the document D into the trash
- **THEN** the event has a change with the ID D and `"op":"trashed"`

### Requirement: Start position
With the query `after=<g>` or the header `Last-Event-ID: <g>`, the feed
MUST start with the generation after `g`. Without both, the feed MUST
start after the current generation. A value larger than the current
generation MUST get status 400.

#### Scenario: Reconnect
- **GIVEN** a client got the event with the id 40, and then lost the connection
- **WHEN** the client connects again with `Last-Event-ID: 40` after the generations 41 and 42
- **THEN** the client gets the events 41 and 42 in that order

### Requirement: No gaps
The feed MUST compute the events from the entries of the root log. The
commit events of the root log MUST only wake the feed. A lost commit event
MUST NOT cause a missing feed event.

#### Scenario: Lost commit events
- **GIVEN** the feed listener lost events in the broker
- **WHEN** the feed wakes up
- **THEN** the feed sends an event for each generation that it did not send

### Requirement: Scoped feed
For a scoped token, the feed MUST contain only the changes of visible
entries. An entry that becomes visible MUST appear as `created`. An entry
that stops to be visible MUST appear as `removed`. A generation with no
visible change MUST NOT give an event.

#### Scenario: Document moved out of the grant
- **GIVEN** a token has a grant on the folder F that contains D
- **WHEN** a tablet moves D out of F
- **THEN** the feed of the token has a change with the ID D and `"op":"removed"`

### Requirement: Keepalive and token end
The server MUST send an SSE comment line every 30 seconds. The server MUST
end the stream within 60 seconds after the token of the stream is revoked
or expires.

#### Scenario: Revoked token
- **GIVEN** a client has the feed open with the token T
- **WHEN** the owner revokes T
- **THEN** the server ends the stream within 60 seconds
