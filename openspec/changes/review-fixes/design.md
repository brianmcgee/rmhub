# Design

## Context

See `proposal.md` for the reason. These facts of the code shape the
design:

- A device token is a JWT with no expiry time. Before this change, the
  server accepted it if the registry had a record for the account and
  the device ID. `Register` replaces the record of a device ID that
  pairs again.
- The renderer draws each segment of a stroke with its own mask. The
  mask has the size of the bounds of the segment. So the cost of one
  segment is the area of its bounds, up to the full image of
  64 × 2^20 pixels. A file of 3 KiB with 200 such segments needed 80
  seconds.
- A render does not stop when the client goes away, because other
  clients can wait for the same result. A render with no end keeps one
  of the `render.max_workers` workers.
- go-pdfium has `Kill` for an instance. With the wazero option
  `WithCloseOnContextDone`, `Kill` stops WebAssembly code that runs.
- The file system blob store makes `<namespace>/<2 characters>/` for
  the first blob with that prefix. It synced only that last directory.

## Goals / Non-Goals

**Goals:**

- A token of an earlier pairing cannot sync.
- One file cannot keep a render worker for more than about one minute.
- A token record has a bounded size.
- A blob write that succeeded stays after a power failure.

**Non-Goals:**

- No storage quota for an account.
- No limit on the number of tokens of an account.
- No time limit on the drawing of strokes. The work limit is a fixed
  bound, and it gives the same answer on each host.

## Decisions

### Pairing ID in the record and in the tokens

`Register` writes a random ID of 16 base32 characters into the device
record and returns it. The device token and the user token hold it in
the claim `rmhub-pairing`. `Exists` and `Check` compare the ID of the
token with the ID of the record. A record and a token without an ID
match, so the devices of an earlier version continue to work.

Alternative: a list of revoked tokens. It grows without limit, because
a device token does not expire.

Alternative: refuse a second pairing of a device ID. A tablet after a
factory reset must be able to pair again.

### Work limit, not a time limit, for strokes

The renderer adds up the work before it draws: the area of the bounds
of each mask, and 8 units for each pixel of the length of each segment.
Above 32 × 64 × 2^20 units it returns the too-large error, which the
API gives as status 400. A smaller width needs less work, so the
message of that error stays correct.

A page of handwriting has short segments. 100 000 segments of 10 pixels
need about 20 million units, which is 1 % of the limit.

Alternative: a deadline in the drawing loop. The result then depends on
the load of the host, and a cached error is not possible.

### Kill the PDFium instance after 60 seconds

`PDF.use` gets an instance, starts a timer and calls `Kill` when the
timer ends or the context ends. The pool makes a new instance for the
next call. The timer starts after the pool gave the instance, so the
compile time of PDFium at the first render does not count. The API
gives the error as status 409, as for a `.rm` file that is too large to
read: the document is the cause, and the same request fails again.

### Sync three directories for each blob

`link` syncs the directory of the blob, the namespace directory and the
store directory, also when the blob exists. A sync of a directory that
did not change is cheap. The alternative, to sync a parent only after
the creation of a directory, has a race between two writers of the same
new directory.

## Risks / Trade-offs

- [Risk] The tablet refuses a token with the claim `rmhub-pairing`.
  → Mitigation: none in the code. The claim name has a prefix that the
  vendor does not use. A test with a tablet is necessary.
- [Risk] A page with many long highlighter strokes at the width 4096
  reaches the work limit. → Mitigation: the client requests a smaller
  width, as the error message says.
- [Trade-off] `WithCloseOnContextDone` makes wazero check for the end
  of the context in loops. The render tests became about 10 % slower.
