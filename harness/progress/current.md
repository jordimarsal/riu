# Current session

**Date:** 2026-11-19
**Feature in play:** F1 `ingest_idempotent_store` — status `in_progress` (spec approved by human, 2026-11-19).

## What happened this session

- Spec gate passed → F1 `in_progress`.
- **Batch 1 done (T1+T2)**:
  - T1 `internal/wikievent`: `Event` (pure), ports `EventSource`/`EventStore`
    (channel-based source port), `ParseEvent` with `%w` sentinels
    (`ErrMalformed`, `ErrUnknownSchema`), unknown wire fields tolerated, `$schema`
    checked against the recentchange family. 11 table-driven cases + sentinel
    wrap test + identity test.
  - T2 `internal/store`: versioned migrations (schema_version stamp per tx),
    `InsertIfNew` via `INSERT OR IGNORE` on PK `(domain, meta_id)`, `Count`.
    Tests: idempotency (2 inserts → 1 row), domain-keyed distinctness, reopen
    survival, closed-store error context.
- Gate green (`go test ./... -race -count=1`), vet + gofmt clean.
- Learnt: `database/sql` closed-DB error is an unexported sentinel — the test
  asserts OUR wrap contract (store+identity context), not driver internals.

## Batch 2 (T3) — done

- `internal/source`: SSE client over net/http + bufio.Scanner (D4, no SSE
  library): `:comment` ignored, `data:` accumulates (multi-line joins with
  `\n`), `id:` → lastID (empty id resets, per SSE spec), blank line dispatches,
  trailing frame on clean EOF is delivered, on dirty EOF counted as invalid.
- Reconnect: bounded exponential backoff (500ms base → 30s cap, ±20% jitter),
  reset on healthy connect; resume via `Last-Event-ID` header (R2).
- Cancellation: every blocking send goes through `emit` (ctx-select); a
  watcher goroutine closes the response body so a blocked read unblocks.
- Malformed frames: counted (`InvalidFrames()`), never fatal (R10).
- Tests (httptest fake): happy path + invalid frame counting (R1/R10),
  drop → reconnect → resume header "A" (R2), 503 → retry → success,
  cancel → both channels close (assertCloses, 2s deadline).
- **Bug the tests caught**: lastID had two sources of truth (run-local var vs
  Client field) — reconnect sent empty Last-Event-ID. Single truth in the
  Client field now.

## Next

- Batch 3 (T4+T5): pipeline (bounded channel, worker, counters, drain) +
  `cmd/riu` assembly + `demo.sh` idempotency replay.

