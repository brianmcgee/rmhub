# Tasks

## 1. Page count from the PDF file

- [ ] 1.1 Move the PDF count of `pdfPages` into a method `PDFPageCount` of `Renderer` that does not take a unit from the render limit, and call it from `pdfPages`. Verify that the existing tests of the page list pass, and add a test that a page list with an empty render bucket answers with status 200.
- [ ] 1.2 Add the field `PDFPages` to `library.Builder` and call it in `loadDoc` for a PDF document whose `.content` gives the count 0. Verify with tests in `internal/library` for a count from the function, for a count above 0 in `.content` that does not call the function, and for an error that gives the count 0 and a visible entry.
- [ ] 1.3 Set `Builder.PDFPages` in `cmd/rmhub/serve.go` to a function that reads the PDF blob and calls `PDFPageCount`. Verify with API tests for each scenario of the requirements `Page count` and `Page count cache`, also the entry of a change feed event.
- [ ] 1.4 Add an end-to-end test that `rmhub upload` of a PDF with 3 pages and then `rmhub stat` show 3 pages. Verify that the test fails without task 1.2.
- [ ] 1.5 Describe the source of `pageCount` in `docs/reference/http-api.md` and `docs/reference/cli.md`, and correct each text in `docs/` and `skills/rmhub/SKILL.md` that tells of the count 0 for an uploaded PDF. Verify each example with a local server.

## 2. Page count of an upload

- [ ] 2.1 Call `PDFPageCount` in the upload handler with a function that reads the temporary file, after the blob store call and before the commit. Verify with an API test whose blob store counts the reads that an upload of a PDF with 3 pages answers with `"pageCount":3` and does not read the PDF blob.
- [ ] 2.2 Log a failed count in the upload handler and continue with the commit. Verify with an API test that a body that starts with `%PDF-` and that PDFium cannot open gets status 201 and `"pageCount":0`.
- [ ] 2.3 Add to the upload section of `docs/reference/http-api.md` that the entry in the answer has the page count of the PDF. Verify the example with `curl` against a local server.

## 3. Integration

- [ ] 3.1 Run `nix fmt` and verify that no file changes.
- [ ] 3.2 Run `go test ./...` and verify that all tests pass.
- [ ] 3.3 Run `nix flake check` and verify that all checks pass.
