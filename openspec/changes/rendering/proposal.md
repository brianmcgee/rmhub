# Proposal

## Why

The user writes by hand on the tablet and wants agents and scripts to
read those pages. The library API gives only the raw `.rm` files, which
no common tool can open. A legible image of each page lets an agent with
image input read handwriting and annotations. The user also annotates
PDFs, so a PDF page must show the PDF and the strokes together.

## What Changes

- Add a page model: the ordered list of pages of a document from its
  `.content`, with the PDF page and the `.rm` file of each page.
- Add a renderer for strokes and highlights from `.rm` files (versions
  3, 5 and 6). It writes PNG, and SVG for notebook pages. The parser is
  a copy of `encoding/rm` from ddvk/rmapi (AGPL-3.0), recorded in
  `NOTICE`.
- Draw annotated PDF pages: PDFium renders the PDF page, and the strokes
  go on top. PDFium runs as WebAssembly through go-pdfium, so the build
  stays free of cgo.
- Add API endpoints for the page list and for a page image, with a local
  disk cache and `ETag` support.
- Add the CLI commands `rmhub pages` and `rmhub render`.

## Capabilities

### New Capabilities

- `page-model`: the page list of a document and its API endpoint.
- `page-rendering`: the drawing rules for strokes, highlights and PDF
  pages, and the output formats.
- `render-api`: the render endpoint, the cache and the CLI commands.

### Modified Capabilities

None.

## Impact

- New dependencies: `tdewolff/canvas` (MIT), `klippa-app/go-pdfium`
  (MIT) with `tetratelabs/wazero` (Apache-2.0) and PDFium (Apache-2.0).
- New file `NOTICE` with the copied parser.
- New configuration keys: `render.cache_dir` and
  `render.cache_max_bytes`.
- The binary grows by the size of the PDFium WebAssembly module.
  [INFERENCE] This is about 5 MB.
- Not in this change: EPUB annotations, templates, typed text and an
  exact copy of the tablet look. An image of an EPUB page gets status
  404.
- Unknown: the page size and the coordinate origin on the reMarkable
  Paper Pro. The design gives the plan to find them.
- No machine or clan service is affected.
