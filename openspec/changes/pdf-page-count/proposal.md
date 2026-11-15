# Proposal

## Why

A PDF that the API uploaded has `"pageCount":0` in its entry until a
tablet opens the document. The page list of the same document shows the
correct pages, because it reads the PDF file. A program that reads
`pageCount` from `rmhub stat` or from a list thus sees an empty
document, and can skip it.

## What Changes

- The entry of a PDF document gets its page count from the PDF file
  when `.content` gives the count 0.
- The value is the same in each API answer and in the change feed.
- The server keeps the count in the render cache, so it opens each PDF
  file one time only.
- The upload endpoint counts the pages from the upload body. The server
  does not read the PDF file from the blob store for that count.
- A page count does not take a unit from the render limit of the
  account. Today the first page list of an uploaded PDF takes one unit.
- The `.content` file does not change, so the data that the tablet
  reads stays the same.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `library`: the source of the field `pageCount`, and the cache of the
  page count.

## Impact

- Code: `internal/library` (the snapshot builder), `internal/api` (the
  renderer, the page list and the upload handler) and
  `cmd/rmhub/serve.go` (the connection between them).
- The first snapshot of an account opens each PDF file that has no
  count in `.content` and no count in the cache. For a remote blob
  store, the server downloads these files. A PDF from the upload
  endpoint of this version has its count in the cache.
- A document that a different upload client created with the count 0,
  such as rmapi, gets the same correction.
- Pages in `docs/` to update: `docs/reference/http-api.md` and
  `docs/reference/cli.md`. The guide `skills/rmhub/SKILL.md` changes if
  it describes `pageCount`.
- No machine or clan service is affected.
