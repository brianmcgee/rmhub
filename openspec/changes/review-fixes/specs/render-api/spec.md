# Spec Delta

## MODIFIED Requirements

### Requirement: Render endpoint
`GET /api/v1/library/{id}/pages/{number}/render?format=<png|svg>&width=<px>`
MUST be a content endpoint. It MUST return the page image with the media
type `image/png` or `image/svg+xml`. The default format MUST be `png`. The
default width MUST be 1404 pixels. A width below 100 or above 4096 MUST
get status 400. A PNG image MUST NOT have more than 64 × 2^20 pixels: a
page that is too long for the requested width MUST get status 400. A
page above the drawing work limit of the capability `page-rendering`
MUST get status 400. A PDF above the PDF time limit of that capability
MUST get status 409.

#### Scenario: Long page at the largest width
- **WHEN** a client requests a page that is 16 pages long with the width 4096
- **THEN** the server answers with status 400

#### Scenario: Render page 3
- **WHEN** a client requests page 3 of a notebook as PNG
- **THEN** the server answers with status 200 and a PNG image 1404 pixels wide

#### Scenario: Page number out of range
- **WHEN** a client requests page 9 of a document with 3 pages
- **THEN** the server answers with status 404

#### Scenario: Too much drawing work
- **WHEN** a client requests a page whose strokes are above the drawing work limit
- **THEN** the server answers with status 400 and a message that asks for a smaller width

#### Scenario: PDF above the time limit
- **WHEN** a client requests a page of a PDF that PDFium does not draw in 60 seconds
- **THEN** the server answers with status 409
