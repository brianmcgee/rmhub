# Request limits

The request limits have two purposes. They keep one client from using
the server so much that other clients get slow answers. They also make a
search for a valid pairing code too slow to succeed.

[Server configuration](../reference/configuration.md#request-limits)
gives the keys and the default values.

## What each limit counts

- The **pairing limit** counts the attempts of one client address. All
  addresses of one IPv6 /64 network count as one address.
- The **API limit** counts the requests of one token. A scoped token and
  its parent token have separate limits.
- The **render limit** counts the page renders of one account, for all
  its tokens together. A page from the render cache does not count.

A client can use the full number of a limit at one time. After that, the
number fills again at a constant rate.

## What the limits do not cover

There is no limit on the sync requests of a paired tablet, and no limit
on requests with a token that is not valid. Use the reverse proxy to
stop a flood of such requests.

The server keeps the counts in memory. A restart sets them to zero. If
two server processes use one state store, each process has its own
counts, so a client can use each limit two times.

## Behind a reverse proxy

Behind a reverse proxy, the server sees only the address of the proxy.
Then all clients share one pairing limit. Set
`listen.tablet.trusted_proxies`, so that the server reads the client
address from the `X-Forwarded-For` header of the proxy. See
[Deploy on NixOS](../how-to/deploy-on-nixos.md#give-the-client-address-to-rmhub).
