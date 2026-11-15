# page-model Specification

## Purpose
A document has an ordered list of pages in its `.content`. A page can
have a `.rm` file with strokes and, in a PDF, a page of the PDF. Clients
need this list to select a page to render and to see which page changed.

## Requirements

### Requirement: Page list from cPages
When `.content` has `cPages.pages`, the page list MUST contain each entry
whose `deleted` value is absent or 0. The order MUST be the ascending byte
order of `idx.value`, and then of `id`. The page number MUST start at 1.

#### Scenario: Deleted page
- **GIVEN** `cPages.pages` has the pages A, B and C, and B has `"deleted":{"value":1}`
- **WHEN** rmhub builds the page list
- **THEN** the list has A as page 1 and C as page 2

### Requirement: Page list from the old format
When `.content` has no `cPages`, the page list MUST contain the IDs of
`pages` in their order. For a PDF, the PDF page of each page MUST come
from `redirectionPageMap` at the same position, if the value is 0 or more.

#### Scenario: Old content file
- **GIVEN** `.content` has `"pages":["a","b"]` and no `cPages`
- **WHEN** rmhub builds the page list
- **THEN** the list has `a` as page 1 and `b` as page 2

### Requirement: PDF without page list
When the page list of a PDF document is empty, the page list MUST have
one page for each page of the PDF. The page number and `pdfPage` MUST be
the PDF page. `id` MUST be empty and `rmHash` MUST be `null`. A PDF that
the API uploaded has no page list until a tablet opens it.

#### Scenario: Uploaded PDF
- **GIVEN** the API uploaded a PDF with 3 pages, and no tablet opened it
- **WHEN** rmhub builds the page list
- **THEN** the list has 3 pages, and page 2 has `"pdfPage":2`

### Requirement: Page fields
Each page MUST have `number`, `id`, `pdfPage` and `rmHash`. `pdfPage` MUST
be the 1-based PDF page from `redir.value` + 1, or `null`. `rmHash` MUST be
the hash of the file `<document ID>/<page ID>.rm` in the document index,
or `null` when the index has no such file.

#### Scenario: Inserted blank page in a PDF
- **GIVEN** a PDF document has a page with no `redir` and a `.rm` file
- **WHEN** rmhub builds the page list
- **THEN** that page has `"pdfPage":null` and a non-null `rmHash`

### Requirement: Page list endpoint
`GET /api/v1/library/{id}/pages` MUST return `{"pages":[…]}` with the
pages of the document. For a folder it MUST answer with status 404. The
endpoint MUST follow the visibility rules of the capability
`scoped-tokens`.

#### Scenario: Notebook with three pages
- **WHEN** a client requests the pages of a notebook with three pages
- **THEN** the response has three pages with the numbers 1, 2 and 3

#### Scenario: Document outside the grants
- **WHEN** a scoped token requests the pages of a document outside its grants
- **THEN** the server answers with status 404

### Requirement: Malformed content file
If `.content` is not valid JSON, the page list endpoint MUST answer with
status 500 and the code `internal`. The server MUST log the document ID.

#### Scenario: Broken content file
- **GIVEN** the `.content` of the document D is not JSON
- **WHEN** a client requests the pages of D
- **THEN** the server answers with status 500
