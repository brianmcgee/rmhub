# Design

## Context

See `proposal.md` for the reason. These facts come from the code:

- `libops.Upload` writes `.content` with `"pageCount":0` and no page
  list. rmapi writes the same form. It is not known if the tablet
  accepts a `.content` with a count and no page list.
- `fill` in `internal/library/library.go` sets `Entry.PageCount` from
  `.content` only. The package does not know the renderer.
- `pdfPages` in `internal/api/library.go` counts the pages of a PDF for
  the page list. It reads the full PDF into memory, calls PDFium through
  `Renderer.image`, and keeps the number in the render cache with a key
  from the hash of the PDF file. `Renderer.image` takes one unit from
  the render limit for each result that is not in the cache.
- The snapshot builder loads only the documents whose index hash
  changed. It copies each other document from the previous snapshot.
- The change feed sends `library.Entry` values from snapshots. The
  list, read, search and write endpoints do the same.
- The tablet sync endpoints read the root log. They do not build a
  snapshot.

## Goals / Non-Goals

**Goals:**

- `pageCount` of an uploaded PDF is correct in each place that shows an
  entry.
- The data that the tablet reads does not change.
- A PDF file that is not valid does not hide the library.

**Non-Goals:**

- A page count for an EPUB. rmhub does not read EPUB pages, and the
  tablet makes the pages of an EPUB from its text settings.
- A change of the page list endpoint. It keeps its status 409 for a PDF
  above the time limit.
- A count in `.content` at upload time. That is the first approach,
  which this change does not use.
- A refusal of an upload whose PDF file PDFium cannot open. The upload
  check stays the same.

## Decisions

### The snapshot builder sets the count

`library.Builder` gets an optional function field:

```go
PDFPages func(ctx context.Context, account string, pdf hashtree.Entry, docID string) (int, error)
```

`loadDoc` calls it after `fill`, when the type is `pdf`, the count from
`.content` is 0, and the document index has a `.pdf` file. The count is
then part of the `Doc`, so each reader of the snapshot and the change
feed see the same value. The count is a function of the document index,
so the rule "the hash changes with each change of the entry" stays true.

Alternative: set the count in `view.entry` of the API. The change feed
does not use that function, so a feed event and a list answer can then
show different values.

### The renderer owns the count

`Renderer` gets a method `PDFPageCount`. Its parameters are the account,
the hash of the PDF file and a function that returns the bytes of the
file. It contains the cache key, the single run for parallel calls, the
worker limit and the PDFium call that `pdfPages` has today. The method
calls the function only when the cache does not have the count.

The method has three users, and they have one cache entry for each PDF
file: `pdfPages`, the function in `Builder.PDFPages` that
`cmd/rmhub/serve.go` sets, and the upload handler. The first two give a
function that reads the blob from the blob store.

The method does not take a unit from the render limit. The spec
`rate-limiting` counts page renders only, and a snapshot build must not
fail because an account rendered many pages.

### An error gives the count 0

When `PDFPages` returns an error, `loadDoc` logs the document ID and the
error at the level `warn` and keeps the count 0. The existing rule of
the builder is that one broken document does not hide the library.

The builder does not keep the error. A document from the previous
snapshot is not loaded again, so the server tries each PDF file one time
for each start and for each change of the document.

Alternative: write the count 0 to the cache after an error. The page
list then answers with an empty list and not with status 409, which the
spec `render-api` forbids.

### The upload handler counts from its temporary file

The upload handler has the body in a temporary file on the local disk.
For a PDF, it calls `PDFPageCount` with a function that reads that file.
It does this after it stored the blob and before the commit. The
snapshot build after the commit then finds the count in the cache and
does not read the blob. For a remote blob store, this saves one download
of each uploaded PDF.

When the count fails, the handler logs the error at the level `warn` and
continues with the commit. The upload does not fail because of the
count. The builder then tries the count again and sets 0 after a second
failure.

The fallback in the builder stays necessary: for documents from rmapi,
for a cache that lost the count, and for an upload whose cache write
failed.

Alternative: count before the blob store gets the file. A failed store
call then wastes the PDFium work.

## Risks / Trade-offs

- [The first snapshot of an account with many uploaded PDFs is slow,
  for example after a migration from rmfakecloud with documents from
  rmapi] → The builder loads documents in parallel, the renderer limits
  the PDFium work to `render.max_workers`, and the cache keeps each
  count on disk. The cost comes one time for each PDF file.
- [The count reads the full PDF file into memory, up to the upload
  limit] → This is the behavior of the page list today. The worker
  limit bounds the number of files in memory.
- [A PDF above the PDF time limit of 60 seconds blocks the snapshot
  build for that time, at each server start. Its upload takes two times
  the limit, because the handler and the builder each try the count] →
  Such a file is rare. The log names the document. If it becomes a
  problem, a later change can keep the failure.
- [An upload of a PDF takes more time, because the count runs before
  the answer] → The count ran in the same request before this decision,
  in the snapshot build. Only its position changes.
- [The render cache deletes old files when it is full, and a count can
  be one of them] → The server counts the PDF again at the next load of
  the document. Documents of the current process keep their count in
  the snapshot.
- [A tablet or a client reads `pageCount` 0 as a sign for "not opened
  by a tablet"] → No such use is known. The page list still shows an
  empty `id` for such pages.

## Migration Plan

No data changes. After the update, the first request of each account
counts the PDF files that need it. A return to the old version needs no
step: the cache entries have the key that the page list already uses.
