# riu

**A river of live events, processed in Go.**

`riu` drinks from the [Wikimedia EventStreams](https://stream.wikimedia.org/v2/stream/recentchange)
SSE feed — every edit on every Wikipedia, pushed in real time (a Kafka topic
under the hood: the SSE `id` field carries the topic offsets) — and flows it
through a small, deliberately idiomatic Go pipeline into SQLite, with
idempotent persistence, backpressure, and a live dashboard.

```
Wikipedia EventStreams ──SSE──▶ source ──▶ bounded chan ──▶ workers ──▶ SQLite (idempotent)
   (mediawiki.recentchange)     reconnect+    backpressure    pool         unique (domain, event)
                                backoff                        │
                                                               ▼
                                              classifiers ──▶ projections ──▶ SSE fan-out ──▶ dashboard
```

## Why this repo exists (learning in the open)

I have spent nine years building backends in Java and Python. This repo is my
deliberate, public practice of Go: the strongly-typed judgment the pipeline
needs — concurrency, error handling, data modeling — is what I have been doing
all those years; the language is what I am learning here. The commit history
shows the progression honestly.

Everything is built with [harness](https://github.com/jordimarsal/harness)
(Spec Driven Development: human gates between spec and code), following the
conventions in `docs/conventions.md` — including the Go stack that was added
to harness for exactly this purpose (v0.12.0).

## Phases

| Phase | Feature | What it proves | Status |
|---|---|---|---|
| F1 | ingest → idempotent store | SSE client (reconnect/backoff/Last-Event-ID), bounded channels, graceful drain, idempotent upserts | spec_ready |
| F2 | classification + projections | pure rule engine (bot/anomaly/heat), rolling windows — table-driven | pending |
| F3 | dashboard | SSE fan-out + charts | pending |
| F4 | AI scoring | embeddings batch worker (rate limit + backoff), anomaly scoring, mini-RAG CLI | pending |

## Stack

Go 1.27 · SQLite (pure Go driver, no cgo) · stdlib-first · `log/slog` ·
`go test ./... -race -count=1` as part of done · GitHub Actions.

## Development

```bash
go run ./cmd/riu          # run the pipeline (once F1 lands)
go test ./... -race -count=1
./harness/init.sh         # project gate (tests + structure)
```
