# Proposal

## Why

The tablet is the main client of rmhub. A paired tablet with reMarkable OS
3.27.1 must sync its library with rmhub as it does with the reMarkable
cloud. The tablet reaches rmhub through rmfakecloud-proxy, which the user
already runs. Without the sync protocol, rmhub has no use.

## What Changes

- Add the discovery endpoints. They name a host that the tablet sends to
  its proxy. The host is a setting, with the default `local.appspot.com`.
- Add the Sync 1.5 endpoints: root read (v3 and v4), root write, blob read
  and write, `check-files`, and the small endpoints that the tablet calls
  during a sync.
- Accept root indexes in schema 3 and schema 4. Announce schema 3 by
  default. That combination is known to work with OS 3.27.1. An owner can
  set schema 4 for the account.
- Check each uploaded blob against its key: a file by SHA-256 of its
  bytes, an index by the hash rule of its schema. Check the CRC32C in
  `x-goog-hash`. Refuse a root that names a missing blob. rmfakecloud
  does none of these checks.
- Answer `check-files` with the files that are really missing. A setting
  restores the rmfakecloud answer (nothing missing) in case the tablet
  does not accept the true answer.
- Add the notification websocket. After a root write with `broadcast`,
  the other devices of the account get a `SyncComplete` message.
- Answer the telemetry and report endpoints and discard their data.
- Add a CI check that syncs ddvk/rmapi 0.0.35 from nixpkgs against rmhub
  in schema 4 and in schema 3.

## Capabilities

### New Capabilities

- `hash-tree`: the index formats of schema 3 and schema 4, and the hash
  rules for files, document indexes and root indexes.
- `tablet-discovery`: the discovery and service endpoints and the
  announced host.
- `tablet-sync`: the Sync 1.5 endpoints, the blob checks and the schema
  setting.
- `tablet-notifications`: the notification websocket.
- `tablet-telemetry`: the telemetry, report and beta endpoints that
  discard data.

### Modified Capabilities

None.

## Impact

- New packages for the hash tree and the tablet endpoints.
- New dependency: `gorilla/websocket` (BSD-2-Clause).
- New configuration keys: `tablet.announced_host` and
  `tablet.check_files`.
- New flake check with rmapi from nixpkgs.
- Unknown: the behavior of the tablet in four cases that the rmfakecloud
  study cannot show. Design lists them. The user chose not to record the
  tablet traffic, so the first real sync with the tablet is the test.
- No machine or clan service is affected.
