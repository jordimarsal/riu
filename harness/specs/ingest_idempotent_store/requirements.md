# Requirements — ingest_idempotent_store

- R1: WHEN the Wikimedia EventStreams endpoint is reachable, the system SHALL
  establish an SSE connection to
  `https://stream.wikimedia.org/v2/stream/recentchange` and receive
  `recentchange` events.
- R2: WHEN the connection drops, the source SHALL reconnect with bounded
  exponential backoff and jitter, and SHALL send the last received event id as
  `Last-Event-ID` so the feed resumes where it left off.
- R3: WHEN a raw event arrives, the pipeline SHALL normalize it into the pure
  domain type `wikievent.Event`; the domain package SHALL NOT import transport
  types (`net/http`, SSE framing) — I/O stays at the edges.
- R4: WHEN an event whose `(domain, meta.id)` is already persisted arrives,
  the store SHALL ignore it; replaying the same event stream SHALL leave row
  counts unchanged (idempotent upsert).
- R5: The pipeline SHALL carry events over a bounded channel; WHEN the channel
  is full, the source reader SHALL block (F1 policy — drop policy is a
  recorded decision deferred to F2).
- R6: WHEN SIGINT arrives or the context is cancelled, the pipeline SHALL stop
  the source, finish in-flight work, close the store and exit 0.
- R7: The store SHALL apply schema changes through versioned migrations
  (single `events` table with a unique index on the identity key).
- R8: A stats output SHALL report `received`, `persisted` and `duplicate`
  counters — the demo evidence that idempotency works.
- R9: Errors SHALL be wrapped with `%w` and logged via `log/slog` with
  structured fields; no log-and-return; no `panic` outside `main` and startup
  failures.
- R10: `go test ./... -race -count=1` SHALL be green, covering: event
  normalization (table-driven, including malformed JSON fixtures), store
  idempotency (insert twice → one row), and the SSE client against an
  `httptest` fake (happy path, malformed frame, drop → reconnect → resume).

## Traceability

| Requirement | Test(s) | Implementation file(s) |
|---|---|---|
| R1 | TestSourceReceivesEvents (fake) | internal/source/client.go |
| R2 | TestSourceReconnectResume (fake) | internal/source/client.go |
| R3 | TestParseEvent table-driven | internal/wikievent/event.go |
| R4 | TestStoreIdempotent | internal/store/sqlite.go |
| R5 | TestPipelineBackpressure | internal/pipeline/pipeline.go |
| R6 | TestGracefulDrain | internal/pipeline/pipeline.go |
| R7 | TestMigrations | internal/store/migrations.go |
| R8 | TestStatsCounters | internal/pipeline/stats.go |
| R9 | (convention — enforced by review) | — |
| R10 | suite-wide | — |
