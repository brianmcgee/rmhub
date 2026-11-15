# Proposal

## Why

A second review of the code for correctness and security found five
defects. An account holder or a scoped token can use three of them to
stop the page renders of the server or to fill the state store. One
defect makes the tokens of a removed device valid again. One defect can
lose a blob after a power failure.

## What Changes

- Give each pairing of a device a random pairing ID. The device record
  and the tablet tokens hold it. A token of an earlier pairing of the
  same device ID stays invalid after the device pairs again.
- Limit the drawing work of one page image. A small `.rm` file whose
  segments each cover the full image gets status 400, not a render of
  many minutes.
- Limit the PDFium work of one render to 60 seconds. The server stops a
  PDFium instance that is busy for longer, and answers with status 409.
- Limit a token label to 256 bytes and a token to 64 grants.
- Sync the parent directories of a new blob in the file system backend.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `device-pairing`: the pairing ID in the device token, in the user
  token and in the token checks.
- `page-rendering`: the limit of the drawing work.
- `render-api`: the status for a page that needs too much work, and the
  time limit of a PDF.
- `api-tokens`: the limit of the label.
- `scoped-tokens`: the limit of the number of grants.
- `blob-store`: a successful write stays after a power failure.

## Impact

- `internal/devices`, `internal/tablet`, `internal/tablet/notify`: the
  pairing ID. The signatures of `Register`, `Exists`, `Check`,
  `DeviceToken` and `UserToken` change.
- `internal/render`: the work limit and the time limit. PDFium runs with
  `WithCloseOnContextDone`, which adds a small cost to each render.
- `internal/api`: the two new error answers and the token limits.
- `internal/blob/fs`: two more directory syncs for each blob write.
- `internal/rmfile`: a fuzz test of the parser. The review ran it for
  11 million inputs and found no panic.
- No new dependency. No change of the configuration.
- A device that paired with an earlier version continues to sync: its
  record and its tokens have no pairing ID, and the two match.
- Unknown: the tablet gets one more claim, `rmhub-pairing`, in its
  tokens. [INFERENCE] The tablet ignores a claim that it does not know.
  No tablet was available for a test.
