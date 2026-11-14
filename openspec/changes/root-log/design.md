# Design

## Context

The change `foundation` gives a state store with get, put-if-absent,
put-if-match, delete and prefix listing on single keys. A version can be
the same for identical bytes. The root log must give a compare-and-swap
on the generation with only these operations.

The reMarkable sync protocol puts the generation on the wire. A tablet
sends the generation it read. The server answers status 412 when that
generation is not current. The tablet then reads the root again and
merges.

## Goals / Non-Goals

**Goals:**

- A compare-and-swap on the generation that is correct on SQLite and on S3.
- A read of the current root in a small, fixed number of requests.
- A record of who made each generation.

**Non-Goals:**

- Retention, rollback and garbage collection. They use the history but
  come in a later change.
- Commit events across server processes. The events are local to one
  process. Correctness of the commit does not depend on them.

## Decisions

### Layout

```
accounts/<uid>/roots/00000000000000000042   immutable entry
accounts/<uid>/head                         {"generation":42}
```

The generation has 20 digits with leading zeros. 20 digits hold each
`uint64`, and the padding makes byte order equal to number order in a
listing. An entry is JSON:

```json
{"generation":42,"hash":"<64 hex>","time":"2026-11-14T22:00:00Z",
 "writeId":"<random 128-bit hex>","source":{"type":"device","id":"…"}}
```

### Commit

```
commit(uid, g, hash, source):
  if g > 0 and get(roots/<g>) is not-found: return stale
  w = new write ID
  put-if-absent(roots/<g+1>, entry)
    success  -> advance head, send event, return g+1
    conflict -> return stale
    failure  -> resolve(roots/<g+1>, w)
```

The test for `roots/<g>` finds a writer that is ahead of the server. That
test has no race: an entry never disappears and never changes. The
put-if-absent finds a writer that is behind, because `roots/<g+1>` then
exists.

The alternative was one mutable root object with put-if-match. It needs
fewer requests. It keeps no history, and its result after a lost answer
is harder to read, because the object can change again before the
reader reads it. The immutable entries give history, a clean way to
resolve a lost answer, and the input for a later garbage collector.

### Resolve an unknown outcome

The root log reads `roots/<g+1>`. A match of the write ID is success. A
different write ID is stale. Not-found means that the write did not
happen, so the root log retries the put-if-absent once with the same
entry. A second failure is returned to the caller.

### Head hint

After a successful commit the root log reads the head hint. If the hint
names a lower generation, the root log writes the new generation with
put-if-match. It writes with put-if-absent when there is no hint. On
conflict it reads again and repeats while the hint is lower. A failure of
the hint update is logged and ignored: readers walk forward.

### Read the current root

```
h = head.generation, or 0 if there is no hint
loop: get(roots/<h+1>); if found: h = h+1; else break
return roots/<h>, or generation 0 with no hash if h = 0
```

In normal operation the hint is current. A read then costs one get of the
hint, one get that returns not-found and one get of the entry. A cache
in memory of the last entry per account removes the last get.

### Commit events

An in-process broker keeps a list of listeners for each account. Each
listener has a buffered channel of 64 events. The broker never blocks. If
a buffer is full, the broker drops the event and sets a lost-events flag
on that listener. A listener that sees the flag reads the current root.
Tablets only need a signal that something changed. The change feed of
the rmhub API resumes by generation.

## Risks / Trade-offs

- [Two or three requests for each read] → This is acceptable at personal
  scale. The memory cache of the last entry reduces it.
- [Events are local to one process] → rmhub runs as one process for each
  state store. A later change can add a shared broker if rmhub ever runs
  as several processes.
- [The history grows without limit] → The entries are small. The later
  retention change removes old entries.
- [A writer that is ahead of the server] → The test for `roots/<g>`
  returns stale-generation. The tablet then reads the root again, which
  gives it the correct generation.
