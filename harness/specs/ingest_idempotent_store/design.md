# Design — ingest_idempotent_store

## Layout (hexagonal: ports in the domain, adapters at the edges)

```
cmd/riu/main.go          # thin assembly: wire ports to adapters, run, drain
internal/wikievent/      # PURE domain: Event, Classification (F2), ports
                         #   EventSource (port) — implemented by source adapter
                         #   EventStore (port)  — implemented by store adapter
internal/source/         # SSE client adapter: net/http + bufio, reconnect,
                         # backoff+jitter, Last-Event-ID
internal/pipeline/       # bounded channel, store worker, counters, drain
internal/store/          # SQLite adapter: migrations, idempotent upsert
```

## Decisions

- **D1 — identity key `(meta.domain, meta.id)`**: `meta.id` is the event UUID
  assigned by the stream; combined with the wiki domain it is the natural
  dedup key. The Kafka offsets in the SSE `id` field are transport detail —
  they inform `Last-Event-ID`, not the domain identity.
- **D2 — `modernc.org/sqlite` (pure Go, no cgo)**: keeps CI and local builds
  cgo-free; performance is irrelevant at F1 volumes.
- **D3 — blocking backpressure (F1)**: the reader blocks when the bounded
  channel is full; SSE buffering absorbs it. A drop-with-counter policy is
  deferred to F2, where projections make latency matter. Recorded, not
  forgotten.
- **D4 — stdlib-first SSE parsing**: `net/http` + `bufio.Scanner` over the
  response body; no SSE library in F1 (three frame kinds: `event`, `id`,
  `data` — a page of code). Revisit if the dashboard (F3) needs a shared
  parser.
- **D5 — resume state is in-memory (F1)**: `Last-Event-ID` survives
  reconnects within a process; surviving restarts is an F2 candidate (persist
  last id alongside stats).
- **D6 — logging**: `log/slog` JSON handler, level via `RIU_LOG_LEVEL`; every
  log line carries `domain`/`event_id` fields where applicable.
- **D7 — single store worker (F1)**: one goroutine owns the SQLite writer
  (single-writer anyway); the worker *pool* arrives in F2 with classifiers.
  The concurrency skeleton (context, bounded channel, drain) is built now so
  F2 extends it instead of rewriting it.
- **D8 — errors**: adapters wrap with `%w` and return; `main` decides. The
  domain defines no error sentinels it cannot speak to (store/source failures
  stay in their adapters; the pipeline surfaces them via errgroup).
