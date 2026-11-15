# Spec Delta

## ADDED Requirements

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
