# Tokens and access

rmhub does not store a password for an account. A person or a program
reaches an account only with a token. This page explains the kinds of
token and what each kind can do.

## The kinds of token

| Token | Who has it | Access |
|---|---|---|
| Admin token | The operator of the server | Create and list accounts, and issue owner tokens. No access to a library. |
| Owner token | The owner of an account | Full access to one account: the library, the tokens, the pairing codes and the devices. |
| Scoped token | A program or an AI agent | Some folders of one account, to read or to write. |
| Device token and user token | A paired tablet or app | The sync protocol for one account, on the tablet listener. |

## The admin token

The admin token comes from a file that `admin.token_file` names. Without
that key, each admin request gets status 401. The admin token cannot
read a library. To use an account, the operator issues an owner token
for it.

## Owner tokens

The server stores only a hash of the secret of a token. It gives the
token text only in the answer that creates the token. Thus a token that
you lose cannot be read from the server. Revoke it and make a new one.

A token can have an expiry time.

## Scoped tokens

A scoped token has grants. A grant names a folder and gives `read` or
`write` access. The purpose is to give an agent a part of the library
and nothing else.

- The token sees only the grant folders and their descendants. Each
  other entry gets status 404, as if it did not exist.
- For the token, the top level is the list of its grant folders.
- The token cannot rename, move or trash a grant folder.
- The token cannot pair tablets, remove devices or change the account.
- While a grant folder is in the trash, the token does not see it.

A scoped token can create child tokens, with the same or fewer rights
and the same or an earlier expiry. Thus an agent can give a smaller part
of its access to a helper. The server checks the full chain of parent
tokens at each request. Revocation of a token also revokes each token
that it created, so the owner can stop a full chain with one command.

## Paired devices

A tablet or an app pairs with an account through a one-time code. The
owner makes the code, and it expires after 5 minutes. After pairing, the
device has a device token. With it, the device gets user tokens with a
short life, which the sync protocol needs.

The owner can remove a device. Then its device token is no longer valid,
and the server closes its notification websocket.

The sync endpoints of the tablet listener accept only a user token. The
tablet listener does not have the rmhub API.

## Related pages

- [Give an agent access to the library](../how-to/give-an-agent-access.md)
- [Connect a tablet](../how-to/connect-a-tablet.md)
- [Request limits](request-limits.md)
