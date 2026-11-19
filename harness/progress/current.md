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

## Next

- Batch 2 (T3): SSE client adapter `internal/source` against an httptest fake.

