# Design

## Context

After the change `scoped-api`, the server has a library snapshot with
the document index of each document, and grants that limit access. This
change turns pages into images.

Sources:

- ddvk/rmapi 0.0.35 `encoding/rm` (AGPL-3.0): a parser for `.rm`
  versions 3, 5 and 6. It reads strokes (line items) and glyph
  highlights. It skips text, groups and layers. It imports only the Go
  standard library. Its v6 reader shifts X by 702 units, which is half
  the width of the reMarkable 2 screen (1404 × 1872).
- ddvk/rmapi `archive/file.go`: the `cPages` format with `idx`,
  `deleted` and `redir`.
- ricklupton/rmscene and ricklupton/rmc (MIT): references for the v6
  format and for the position of a PDF page under the strokes. rmhub
  does not copy their code.
- klippa-app/go-pdfium 1.21.4 (MIT): PDFium as a WebAssembly module on
  wazero. It needs no cgo. wazero compiles to native code on amd64 and
  arm64.

## Goals / Non-Goals

**Goals:**

- A legible PNG of each notebook page and each annotated PDF page.
- No cgo, so the build stays `CGO_ENABLED=0`.

**Non-Goals:**

- EPUB annotations. The tablet reflows EPUB pages, so a page of strokes
  has no fixed base page. A later change can solve this.
- Templates, typed text, layer visibility, and the exact look of each
  brush on the tablet.
- Export of a full annotated PDF. pdfcpu (Apache-2.0) is the candidate
  for a later change.

## Decisions

### Copy the parser, do not import rmapi

rmapi has the module path `github.com/juruen/rmapi`, but the code is in
`ddvk/rmapi`. An import needs a `replace` directive. The module graph of
rmapi also contains `unidoc/unipdf`, which the rules of this repository
forbid. The parser is about 1,000 lines with no dependencies. So the copy
goes into `internal/rmfile`, with an entry in `NOTICE` that names the
source, the commit and the licence. rmapi says AGPL-3.0 with no "or
later". The copy stays under AGPL-3.0, which is compatible with the
AGPL-3.0-or-later licence of rmhub. The test data of rmapi comes with
the copy.

Two changes to the copy:

- The v6 reader keeps the raw X value. The renderer adds the half page
  width. So a device with another screen width needs no parser change.
- Parse errors are returned. The original ignores a bad block, so a
  broken file gives an empty page with no error.

### Coordinates and page size

The renderer uses `.rm` units and a page width of 1404 and a height of
1872. These are the values of the reMarkable 2 and of rmc. A notebook
page grows to include each stroke. So a stroke outside the expected area
stays visible, even if the Paper Pro uses other values.

A PDF page is scaled to the page width and placed at the top left. This
is the rmc convention. [INFERENCE] The tablet does the same for a PDF
page at zoom 1.

The Paper Pro screen has 1620 × 2160 pixels. It is not known whether its
`.rm` files use the same units as the reMarkable 2. Task 6.1 gets a
fixture from the tablet of the user. The fixture has a notebook page with
marks at the four corners and a PDF page with a mark at a known PDF
position. The constants change if the fixture shows other values. The
renderer version then changes, so old cache entries stop to match.

### Drawing

`internal/render` builds a path for each stroke with tdewolff/canvas.
Each segment uses the mean width of its two points. The canvas
rasterizer writes PNG. The SVG writer of canvas writes SVG. Colors:

| Color | RGB |
|---|---|
| Black | `#000000` |
| Grey, GreyOverlap | `#7f7f7f` |
| White | `#ffffff` |
| Blue | `#1e4fd8` |
| Red | `#d8261e` |
| Green2 | `#2a9d3a` |
| Cyan | `#00a6c4` |
| Magenta | `#c4008c` |
| Yellow2 | `#e0c000` |
| HighlightYellow | `#fff176` |
| HighlightGreen | `#a5d6a7` |
| HighlightPink | `#f48fb1` |

An unknown color draws as black. Legibility matters more than a faithful
color.

### PDF pages

go-pdfium runs in a pool with one instance for each CPU thread. The
server opens the PDF from the blob store and renders the page at the
target width into an RGBA image. The canvas rasterizer then draws the
strokes onto that image. Each PDFium instance keeps no document open
between requests.

### Cache and entity tag

The cache key is the SHA-256 of these values: the account ID, the
renderer version, the format, the width, the `.rm` hash, the PDF hash and
the PDF page. The key
is also the `ETag`. The file name is `<key>.<format>` in
`render.cache_dir`. A write goes to a temporary file in the same
directory, and then a rename. A cache hit updates the modification time,
so the size limit deletes the least recently used files.

The account ID in the key keeps the cache in line with the blob
namespaces. The root check of the change `tablet-sync` already stops an
account from naming a blob that it does not have. The key does not
depend on that check.

The cache is on local disk, not in the blob store. The blob store takes
only content hashes. A cache file is disposable, so the loss of the
directory costs only render time.

`golang.org/x/sync/singleflight` joins parallel requests for one key. A
semaphore with one slot for each CPU thread limits the renders.

### Determinism

PNG output from the Go encoder is deterministic for one image. The canvas
rasterizer uses no random values. [INFERENCE] PDFium output is
deterministic for one PDF and one size. A test renders each fixture two
times and compares the bytes. If PDFium is not deterministic, the
requirement changes to apply to notebook pages only.

## Risks / Trade-offs

- [The Paper Pro uses other units] → The fixture task finds this before
  the change is complete. Notebook pages stay legible in either case,
  because the page grows.
- [Hidden layers are drawn] → The parser skips layers. A user who hides
  a layer sees it in the image. This is acceptable for a first version.
- [WebAssembly PDFium is slower than native PDFium] → wazero compiles
  the module, and the cache hides repeated renders. The first render of a
  large PDF page can take about one second. [INFERENCE] This number comes
  from no measurement.
- [The copied parser misses new format features] → The copy is small
  and has tests. A new feature of the format needs a change to the copy.
