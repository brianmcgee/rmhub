# page-rendering Specification

## Purpose
A page image must let a person or a model read the handwriting and the
annotations of the page. The image does not need to look exactly like
the tablet screen.

## Requirements

### Requirement: Supported files
The renderer MUST read `.rm` files of the versions 3, 5 and 6. For a file
with an unknown header, the render MUST fail with an error that names the
page.

#### Scenario: Version 6 file
- **WHEN** the renderer reads a `.rm` file that reMarkable OS 3.27.1 wrote
- **THEN** the result has the strokes of the page

### Requirement: Strokes
The renderer MUST draw each stroke that is not deleted, in file order. The
line width of a segment MUST come from the width of its points. The
renderer MUST NOT draw strokes of the brush types `Eraser` and
`EraseArea`.

#### Scenario: Deleted stroke
- **GIVEN** a version 6 file with a stroke that has a deleted length above 0
- **WHEN** the renderer draws the page
- **THEN** the image does not show that stroke

### Requirement: Colors
The renderer MUST map each brush color to a fixed RGB value. Each stroke
of the type `Highlighter` and each glyph highlight MUST have an opacity
of 40 %. A highlight with `ColorRGBA` MUST use that color.

#### Scenario: Yellow highlight
- **WHEN** the renderer draws a glyph highlight with the color `HighlightYellow`
- **THEN** the image has a yellow rectangle with 40 % opacity at the highlight position

### Requirement: Notebook page size
A notebook page MUST have the size of the device screen in `.rm` units.
X = 0 of a version 6 file MUST be at the center of the top edge. X = 0 of
a version 3 or 5 file MUST be at the left end of the top edge. If a
stroke is outside the page, the renderer MUST extend the page to include
the stroke and a margin of 20 units.

#### Scenario: Long page
- **GIVEN** a notebook page with a stroke at the Y position 3000
- **WHEN** the renderer draws the page
- **THEN** the image shows the stroke

### Requirement: Annotated PDF page
For a page with a PDF page, the renderer MUST draw the PDF page with
PDFium at the top left of the page. The PDF page MUST have the width of
the device screen. The strokes and highlights MUST be drawn on top of the
PDF page.

#### Scenario: Annotated PDF
- **GIVEN** a PDF page 2 with a handwritten note in the `.rm` file of the page
- **WHEN** the renderer draws that page
- **THEN** the image shows the PDF text and the note

#### Scenario: PDF page without strokes
- **GIVEN** a PDF page with no `.rm` file
- **WHEN** the renderer draws that page
- **THEN** the image shows only the PDF page

### Requirement: Output formats
The renderer MUST write PNG with a white background for each page. For a
notebook page it MUST also write SVG. For a page with a PDF page, a
request for SVG MUST fail with an error of the kind bad request.

#### Scenario: SVG of a PDF page
- **WHEN** a client requests SVG for a page with a PDF page
- **THEN** the render fails with a bad-request error

### Requirement: Deterministic output
Two renders of the same page with the same renderer version, format and
width MUST give the same bytes.

#### Scenario: Render twice
- **WHEN** the renderer draws one page two times
- **THEN** the two results are equal byte for byte

### Requirement: Drawing work limit
The renderer MUST calculate the work of a PNG image before it draws
the strokes. The work MUST be the sum of the pixels in the bounds of
each stroke segment, of each highlighter stroke and of each glyph
highlight, plus 8 units for each pixel of the length of each segment.
If the work is more than 32 × 64 × 2^20 units, the render MUST fail
with the too-large error, and the renderer MUST NOT draw.

#### Scenario: Segments that each cover the full image
- **GIVEN** a `.rm` file of 6 KiB with 400 segments from the top left to the bottom right of a long page
- **WHEN** the renderer draws the page with the width 2048
- **THEN** the render fails with the too-large error

#### Scenario: Smaller width
- **GIVEN** the same `.rm` file
- **WHEN** the renderer draws the page with the width 100
- **THEN** the result is an image

### Requirement: PDF time limit
The PDFium work of one render or one page count MUST NOT take more than
60 seconds. After that time the renderer MUST stop the PDFium instance
and fail with a timeout error. The next render MUST get a new instance.

#### Scenario: PDF that keeps PDFium busy
- **WHEN** PDFium does not finish a page in the time limit
- **THEN** the render fails with the timeout error
- **AND** the next render of another PDF succeeds
