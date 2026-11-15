# Architecture

This page explains how rmhub keeps a reMarkable library, and why tablets
and API clients can change it at the same time.

## Two listeners

rmhub has two different groups of clients, and one listener for each.

- The **tablet listener** speaks the protocol of the reMarkable cloud:
  discovery, pairing, Sync 1.5 and the notification websocket. Tablets
  and the reMarkable apps use it.
- The **API listener** has the rmhub API under `/api/v1/`. The `rmhub`
  command, scripts and AI agents use it.

The tablet listener does not answer API paths. Thus you can make the
tablet listener available on the internet and keep the API listener on a
private network.

## How a tablet finds rmhub

A tablet talks to fixed cloud host names over HTTPS. rmfakecloud-proxy
on the tablet receives those requests and sends them to the tablet
listener.

The tablet also asks discovery endpoints which hosts to use for sync and
notifications. rmhub must name a host that the tablet sends to its
proxy. This is the purpose of `tablet.announced_host`.

The tablet sends telemetry and crash reports too. rmhub answers these
requests, so that the tablet does not send them again, and does not keep
their data.

## Two stores

rmhub keeps data in two stores with different rules.

- The **blob store** keeps the files of all documents. A blob has a
  SHA-256 key and never changes after it is written. Each account has
  its own namespace, so a key gives no access to the blobs of another
  account.
- The **state store** keeps small values that change, such as the
  account records and the root of each library. Each write is
  conditional, so two writers cannot silently overwrite each other.

Each store has a local backend and an S3 backend. The rules are the same
on each backend. The root log uses the conditional write to select one
winner when writers race. For this reason the server checks at startup
that the state backend refuses a stale conditional write.

## The library is a hash tree

In the sync protocol, a library is a tree of hashes. A document is a
list of its files with their hashes. The root index is a list of the
documents and folders with their hashes. Thus one root hash identifies a
full state of the library.

rmhub parses this tree to check uploads and to read the library. It
writes the tree when an API client changes the library.

## The root log

The root log keeps every generation of the library root of an account. A
writer commits a new root with a compare-and-swap on the generation
number. If two writers race, one writer wins and the other writer gets
an error.

An entry of the log does not change after its commit. Thus each old root
is a snapshot of the library.

## How a tablet syncs

A tablet uploads the new blobs first. Then it replaces the root with a
compare-and-swap on the generation. rmhub checks each upload and each
new root, so that no client can store a broken or false tree.

After a commit, the server sends a message to the other devices of the
account through their websocket. Each of them then reads the root and
syncs.

## How an API client changes the library

An API client does not write a root. It sends an operation on an ID, for
example "rename this entry" or "move this entry into that folder". The
server applies the operation to the newest root and commits the result.

A tablet can commit a root at the same time. If the commit of the server
loses the race, the server reads the new root, checks the operation
again and applies it again. Thus the change of the tablet and the change
of the API client are both in the library.

Names are not unique in a reMarkable library: two entries in one folder
can have the same name. For this reason the API uses IDs. A path is only
a way to find an ID.

## The change feed

The change feed tells a program which entries changed. It follows the
order of the root log. A client that connects again gives the last
generation that it got, so it loses no change.

## Page images

rmhub renders a notebook page or an annotated PDF page as an image. The
purpose is that a person or a model can read the handwriting. The image
does not need to look exactly like the tablet screen.

A render costs CPU time and memory. Thus the server keeps each result in
a disk cache. It also limits the number of renders at the same time
(`render.max_workers`) and the renders of one account in one minute.

## More detail

The specifications in [`openspec/specs/`](../../openspec/specs/) give
the exact rules of each part.
