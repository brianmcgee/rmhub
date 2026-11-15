# render-api Specification

## Purpose
Clients get a page image through the rmhub API and the CLI. Renders cost
CPU time. So the server keeps each result in a disk cache, and a client
can skip the download of an image that it already has.

## Requirements

### Requirement: Render endpoint
`GET /api/v1/library/{id}/pages/{number}/render?format=<png|svg>&width=<px>`
MUST be a content endpoint. It MUST return the page image with the media
type `image/png` or `image/svg+xml`. The default format MUST be `png`. The
default width MUST be 1404 pixels. A width below 100 or above 4096 MUST
get status 400. A PNG image MUST NOT have more than 64 × 2^20 pixels: a
page that is too long for the requested width MUST get status 400.

#### Scenario: Long page at the largest width
- **WHEN** a client requests a page that is 16 pages long with the width 4096
- **THEN** the server answers with status 400

#### Scenario: Render page 3
- **WHEN** a client requests page 3 of a notebook as PNG
- **THEN** the server answers with status 200 and a PNG image 1404 pixels wide

#### Scenario: Page number out of range
- **WHEN** a client requests page 9 of a document with 3 pages
- **THEN** the server answers with status 404

### Requirement: Render access
The render endpoint MUST follow the visibility rules of the capability
`scoped-tokens`. For a folder or an EPUB document, the endpoint MUST
answer with status 404.

#### Scenario: EPUB page
- **WHEN** a client requests a render of an EPUB page
- **THEN** the server answers with status 404

### Requirement: Entity tags
The response MUST have an `ETag` header. The tag MUST change when the
`.rm` file, the PDF, the format, the width or the renderer version
changes. A request with `If-None-Match` and the current tag MUST get
status 304 with no body.

#### Scenario: Unchanged page
- **GIVEN** a client has the image of page 2 with its tag
- **WHEN** the client requests page 2 again with `If-None-Match` and the tag
- **THEN** the server answers with status 304

### Requirement: Render cache
The server MUST keep each render result in the directory
`render.cache_dir`. The default MUST be `$XDG_CACHE_HOME/rmhub/render`,
with `~/.cache` when `XDG_CACHE_HOME` is not set. A request for a cached
result MUST NOT start a render.

#### Scenario: Second request
- **GIVEN** a client requested page 1 of the document D as PNG
- **WHEN** another client requests the same page, format and width
- **THEN** the server returns the cached file

### Requirement: Cache size limit
When the cache holds more than `render.cache_max_bytes` bytes, the server
MUST delete the files with the oldest modification time until the cache
is below the limit. The default MUST be 1073741824 (1 GiB). A cache write
MUST be atomic.

#### Scenario: Full cache
- **GIVEN** the cache is at its limit
- **WHEN** the server adds a new render result
- **THEN** the server deletes the oldest files, and the cache stays at or below the limit

### Requirement: Parallel renders
The server MUST run at most `render.max_workers` renders at the same
time. The default MUST be the number of CPU threads, but not more than
4. Other render requests MUST wait. Two requests for the same uncached
image MUST start only one render. A value below 0 MUST be a
configuration error that names the key.

#### Scenario: Host with many CPU threads
- **GIVEN** a host with 32 CPU threads and no `render.max_workers`
- **WHEN** 32 clients request different uncached PDF pages at the same time
- **THEN** the server runs at most 4 renders at the same time

#### Scenario: Same page twice
- **WHEN** two clients request the same uncached image at the same time
- **THEN** the server renders the image once and returns it to both clients

### Requirement: Page commands
The client MUST have the commands `rmhub pages <entry>` and
`rmhub render <entry> --page <number> [--format png|svg] [--width <px>] [-o <file>]`.
Without `-o`, `render` MUST write to `<name>-p<number>.<format>`. With
`-o -` it MUST write to standard output. It MUST NOT replace an existing
file.

#### Scenario: Agent renders a page
- **WHEN** an agent runs `rmhub render /Notes --page 3 -o page.png`
- **THEN** the file `page.png` holds the PNG image of page 3
