# Tasks

## 1. Pairing ID

- [x] 1.1 Write a test in `internal/app`: remove a device, pair the same device ID again, and send the device token and the user token of the first pairing. Verify that the test fails with status 200 before the change.
- [x] 1.2 Add the pairing ID to the device record, to `Register`, `Exists` and `Check`, and to the tablet tokens. Verify that the test of 1.1 passes with status 401 for the two old tokens and status 200 for the new tokens.
- [x] 1.3 Verify with a test that a record and a device token without a pairing ID give a user token, and that a token with an ID that the record does not have gets status 401.

## 2. Render limits

- [x] 2.1 Write a test in `internal/render` with 400 segments that each cover the full page. Verify that it gives `ErrTooLarge` with the width 2048, with and without a PDF page, and an image with the width 100.
- [x] 2.2 Add the work count and `maxWork` to the renderer. Verify that the test of 2.1 passes and that the other render tests give the same images.
- [x] 2.3 Add the time limit and `Kill` to `PDF.Render` and `PDF.PageCount`. Verify with a test that a limit of 1 ms gives `ErrTimeout` and that the next render with the normal limit succeeds.
- [x] 2.4 Give `ErrTimeout` as status 409 in the render endpoint and in the page list. Verify that `go test ./internal/api ./internal/app` passes.
- [x] 2.5 Add a fuzz test of the `.rm` parser. Verify with `go test ./internal/rmfile -fuzz FuzzUnmarshalBinary -fuzztime 90s` that no input gives a panic.

## 3. Token limits

- [x] 3.1 Limit the label to 256 bytes and the grants to 64. Verify with a test that 257 bytes and 65 grants get status 400, for the admin endpoint and the account endpoint, and that 256 bytes get status 201.

## 4. Blob store

- [x] 4.1 Sync the namespace directory and the store directory after a blob write. Verify that the contract suite of `internal/blob/fs` passes. A test of a power failure is not practical.

## 5. Documents

- [x] 5.1 Add the render limits to the render section of `README.md` and of `docs/agents/SKILL.md`. Verify that the two files name status 400 for too much drawing work and status 409 for a PDF above the time limit.
- [x] 5.2 Verify that `go test ./...`, `golangci-lint run ./...` and `flake-fmt` pass.
