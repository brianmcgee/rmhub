# Spec Delta

## ADDED Requirements

### Requirement: Page count
The value of `pageCount` MUST be the page count that `.content` gives.
For a PDF document whose `.content` gives the count 0, the value MUST be
the number of pages of the PDF file. An entry MUST have the same value
in each API answer and in the change feed. If the server cannot read the
PDF file, the value MUST be 0, the entry MUST appear, and the server MUST
log the document ID.

#### Scenario: PDF from the API
- **WHEN** a client uploads a PDF with 3 pages
- **THEN** the entry in the upload answer has `"pageCount":3`
- **AND** the entry in the list of its folder has `"pageCount":3`

#### Scenario: Count in .content
- **GIVEN** a PDF document whose `.content` has `"pageCount":5` and whose PDF file has 3 pages
- **WHEN** a client reads the entry
- **THEN** the entry has `"pageCount":5`

#### Scenario: Change feed
- **GIVEN** a client reads the change feed
- **WHEN** a second client uploads a PDF with 3 pages
- **THEN** the entry of the `created` change has `"pageCount":3`

#### Scenario: PDF file that is not valid
- **GIVEN** the top level has the document `Good` and a PDF document `Bad` whose PDF file PDFium cannot open
- **WHEN** a client lists the top level
- **THEN** the list has both entries, and the entry `Bad` has `"pageCount":0`

#### Scenario: EPUB from the API
- **WHEN** a client uploads an EPUB
- **THEN** the entry has `"pageCount":0`

### Requirement: Page count cache
The server MUST keep the page count of each PDF file in the render
cache. It MUST NOT open a PDF file whose count is in the cache. It MUST
NOT open the PDF file of a document whose `.content` gives a count
above 0. A page count MUST NOT take a unit from the render limit of the
account.

#### Scenario: Count after a restart
- **GIVEN** a client uploaded a PDF and the server then started again
- **WHEN** a client lists the folder of the document
- **THEN** the entry has the page count, and the server does not start PDFium

#### Scenario: Empty render bucket
- **GIVEN** the render bucket of an account is empty
- **WHEN** a client of the account uploads a PDF with 3 pages
- **THEN** the entry in the upload answer has `"pageCount":3`

### Requirement: Page count of an upload
For a PDF upload, the server MUST count the pages from the request body
before the commit. It MUST NOT read the PDF file from the blob store for
that count. If the count fails, the upload MUST continue.

#### Scenario: No read from the blob store
- **WHEN** a client uploads a PDF with 3 pages
- **THEN** the entry in the upload answer has `"pageCount":3`
- **AND** the server did not read the PDF blob from the blob store

#### Scenario: PDF file that is not valid
- **WHEN** a client uploads a body that starts with `%PDF-` and that PDFium cannot open
- **THEN** the server answers with status 201 and the entry has `"pageCount":0`
