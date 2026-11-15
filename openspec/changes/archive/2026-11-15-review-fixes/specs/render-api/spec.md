# Spec Delta

## ADDED Requirements

### Requirement: Render limits
A page above the drawing work limit of the capability `page-rendering`
MUST get status 400. A PDF above the PDF time limit of that capability
MUST get status 409, from the render endpoint and from the page list.

#### Scenario: Too much drawing work
- **WHEN** a client requests a page whose strokes are above the drawing work limit
- **THEN** the server answers with status 400 and a message that asks for a smaller width

#### Scenario: PDF above the time limit
- **WHEN** a client requests a page of a PDF that PDFium does not draw in 60 seconds
- **THEN** the server answers with status 409
