# Tasks

## 1. Parser

- [x] 1.1 Copy `encoding/rm` from ddvk/rmapi commit `74a8e2e` into `internal/rmfile` with its test data, without `marshal.go`. Add `NOTICE` with the source, the commit and the licence AGPL-3.0. Verify that the copied tests pass.
- [x] 1.2 Change the v6 reader to keep the raw X value and to return parse errors. Verify with a test that a truncated v6 file gives an error and that a v6 stroke keeps its raw X value.
- [x] 1.3 Verify with a test that a stroke with a deleted length above 0 is not in the result.

## 2. Page model

- [x] 2.1 Write the page list from `cPages` and from the old format in `internal/pages`. Verify with tests for a deleted page, the `idx` order, an old content file and an inserted blank page in a PDF.
- [x] 2.2 Add `GET /api/v1/library/{id}/pages` with the scoped visibility rules. Verify with tests for a notebook, a folder, a document outside the grants and a broken `.content`.

## 3. Notebook rendering

- [x] 3.1 Add `tdewolff/canvas`. Write the stroke renderer in `internal/render` with the color table, the highlighter opacity and the skipped eraser types. Verify with tests against small fixtures that check the pixel color at known stroke positions.
- [x] 3.2 Write the page size rule with the X origin for each file version and the growth for strokes outside the page. Verify with a test that a stroke at the Y position 3000 is in the image.
- [x] 3.3 Write PNG and SVG output. Verify with a test that two renders of one page give the same bytes in each format.

## 4. PDF rendering

- [x] 4.1 Add `klippa-app/go-pdfium` in WebAssembly mode with a pool of one instance for each CPU thread. Verify that `nix build .#rmhub` still builds with `CGO_ENABLED=0`.
- [x] 4.2 Draw the PDF page scaled to the page width, with the strokes on top. Verify with a test that a pixel of the PDF text and a pixel of a stroke both have the expected color.
- [x] 4.3 Verify with a test that a PDF page without a `.rm` file renders the PDF only. Verify that SVG for a PDF page gives a bad-request error.

## 5. Render endpoint and CLI

- [x] 5.1 Add `render.cache_dir` and `render.cache_max_bytes` to the configuration. Verify with configuration tests for the XDG default.
- [x] 5.2 Write the cache with atomic writes, the key and the size limit. Verify with a test that a full cache deletes the oldest files and stays at or below the limit.
- [x] 5.3 Write the render endpoint with the width limits, the `ETag`, status 304, singleflight and the semaphore. Verify with tests for page 3, page 9 of 3, an EPUB page, status 304 and two parallel requests that cause one render.
- [x] 5.4 Add `rmhub pages` and `rmhub render`. Verify with an end-to-end test that renders page 1 of a notebook to a file and to standard output. Verify that the command refuses to replace a file.
- [x] 5.5 Add `rmhub render` to `docs/agents/SKILL.md`. Verify that the example in the guide runs against a local server.

## 6. Paper Pro calibration

- [ ] 6.1 Ask the user for a fixture from the tablet. It must have a notebook page with a mark in each corner, and a PDF page with a mark on a known PDF position. Add the fixture to the test data.
- [ ] 6.2 Measure the positions of the marks in the fixture. Correct the page size constants if they differ, and increment the renderer version. Verify with a test that each corner mark is inside the image and that the PDF mark is within 5 pixels of its expected position.

## 7. Integration

- [x] 7.1 Run `nix fmt` and verify that no file changes.
- [x] 7.2 Run `go test ./...` and verify that all tests pass.
- [x] 7.3 Run `nix flake check` and verify that all checks pass.
