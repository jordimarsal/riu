# Tasks — ingest_idempotent_store

## Batch 1 — domain + store

- [ ] T1 Create `internal/wikievent`: `Event` struct (pure — no net/http), the
      `EventSource`/`EventStore` ports, and `ParseEvent([]byte) (Event, error)`
      with `%w`-wrapped errors. Table-driven tests including malformed JSON
      fixtures (truncated payload, wrong types, unknown `$schema`).
- [ ] T2 Create `internal/store`: migrations (`events` table, unique index on
      `(domain, meta_id)`), `InsertIfNew` idempotent upsert returning
      inserted/duplicate. Test: same event twice → one row, second call
      reports duplicate.

## Batch 2 — source adapter

- [ ] T3 Create `internal/source`: SSE client satisfying `EventSource` —
      connect, parse frames, `Last-Event-ID` resume, reconnect with bounded
      exponential backoff + jitter, context cancellation. Tests against an
      `httptest` fake server: happy path, malformed frame skipped (counted,
      not fatal), server drop → reconnect → resume sends Last-Event-ID.

## Batch 3 — pipeline + assembly

- [ ] T4 Create `internal/pipeline`: bounded channel, store worker, counters
      (received/persisted/duplicate), graceful drain on context cancellation.
      Tests: backpressure (slow store blocks reader, no loss), drain finishes
      in-flight events, counters consistent.
- [ ] T5 Create `cmd/riu/main.go`: wire adapters to ports, signal handling
      (SIGINT → cancel), stats output on shutdown and on ticker. End-to-end
      test against a fake source; `demo.sh` replaying a recorded fixture twice
      → row counts stable (the idempotency demo).
