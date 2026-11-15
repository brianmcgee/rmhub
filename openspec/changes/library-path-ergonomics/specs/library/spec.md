# Spec Delta

## MODIFIED Requirements

### Requirement: Search by name
`GET /api/v1/library/search?q=<text>` MUST return each entry whose name
contains the text, without regard to case. The result MUST NOT contain
trashed entries. The result MUST have at most 100 entries. Each entry of
the result MUST have its `path`.

#### Scenario: Search
- **GIVEN** the documents `Tax 2025` and `taxonomy`
- **WHEN** a client searches for `TAX`
- **THEN** the result contains both documents

#### Scenario: Path in the result
- **GIVEN** the document `Paper` is in the folder `Work`
- **WHEN** an owner client searches for `paper`
- **THEN** the entry of the result has `"path":"/Work/Paper"`

#### Scenario: Path for a scoped token
- **GIVEN** a scoped token with a grant on the folder `Work`, the folder `Work` is in the folder `Archive`, and the document `Paper` is in `Work`
- **WHEN** a client with the token searches for `paper`
- **THEN** the entry of the result has `"path":"/Work/Paper"`
