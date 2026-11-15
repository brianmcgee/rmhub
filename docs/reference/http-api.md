# HTTP API

The rmhub API is available only on the API listener
(`listen.api.address`). Each endpoint path starts with `/api/v1/`. The
tablet listener answers a request for a path under `/api/` with status
404.

This page lists the endpoints and the base rules. The specifications in
[`openspec/specs/`](../../openspec/specs/) give the parameters and the
bodies of each endpoint. The `rmhub` command has one command for each
operation (see [Command line](cli.md)).

## Base rules

- Each request needs the header `Authorization: Bearer <token>`.
- A request with no token or with a token that is not valid gets status
  401. A valid token without the permission for the endpoint gets status
  403.
- Each request body and each response body is JSON with the media type
  `application/json`. The content endpoints and the stream endpoint
  below are the exceptions. Their error responses are JSON too.

## Errors

Each error response has this body:

```json
{"error":{"code":"<code>","message":"<text>"}}
```

| Code | Status |
|---|---|
| `bad_request` | 400 |
| `unauthorized` | 401 |
| `forbidden` | 403 |
| `not_found` | 404 |
| `conflict` | 409 |
| `precondition_failed` | 412 |
| `rate_limited` | 429 |
| `internal` | 500 |

A response with status 429 has a `Retry-After` header with the wait time
in seconds.

## Endpoints

The column "Token" gives the token that the endpoint accepts. "Account"
means an owner token or a scoped token. A scoped token sees only the
entries of its grants.

### Accounts (admin)

| Endpoint | Token | Purpose |
|---|---|---|
| `POST /api/v1/admin/users` | Admin | Create an account. |
| `GET /api/v1/admin/users` | Admin | List the accounts. |
| `GET /api/v1/admin/users/{id}` | Admin | Read an account. |
| `POST /api/v1/admin/users/{id}/tokens` | Admin | Issue an owner token. |

### Account

| Endpoint | Token | Purpose |
|---|---|---|
| `GET /api/v1/account` | Owner | Read the account of the token. |
| `PATCH /api/v1/account` | Owner | Change the settings of the account. |
| `GET /api/v1/account/tokens` | Account | List the tokens. |
| `POST /api/v1/account/tokens` | Account | Create a token. |
| `DELETE /api/v1/account/tokens/{id}` | Account | Revoke a token. |
| `POST /api/v1/account/pairing-codes` | Owner | Create a pairing code. |
| `GET /api/v1/account/devices` | Owner | List the paired devices. |
| `DELETE /api/v1/account/devices/{id}` | Owner | Remove a paired device. |

### Library

| Endpoint | Token | Purpose |
|---|---|---|
| `GET /api/v1/library?parent=<id>` | Account | List a folder. Without `parent`, list the top level. |
| `GET /api/v1/library/resolve?path=<path>` | Account | Get the entry at a path. |
| `GET /api/v1/library/search?q=<text>` | Account | Find entries by name. At most 100 entries. |
| `GET /api/v1/library/{id}` | Account | Read an entry. |
| `GET /api/v1/library/{id}/content` | Account | Download the source file of a PDF or an EPUB. Content endpoint. |
| `GET /api/v1/library/{id}/archive` | Account | Download a ZIP file with each file of the document. Content endpoint. |
| `GET /api/v1/library/{id}/pages` | Account | List the pages of a document. |
| `GET /api/v1/library/{id}/pages/{number}/render?format=<png\|svg>&width=<px>` | Account | Render a page as an image. Content endpoint. |
| `POST /api/v1/library/folders` | Account | Create a folder. |
| `POST /api/v1/library/documents` | Account | Upload a PDF or an EPUB. |
| `PATCH /api/v1/library/{id}` | Account | Rename or move an entry. |
| `POST /api/v1/library/{id}/trash` | Account | Move an entry into the trash. |

### Changes

| Endpoint | Token | Purpose |
|---|---|---|
| `GET /api/v1/changes` | Account | Follow the changes of the library. Stream endpoint with the media type `text/event-stream`. |

### Health

Each listener answers `GET /healthz` with status 200. This endpoint
needs no token.
