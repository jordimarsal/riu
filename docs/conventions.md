# Conventions

This document defines the coding and project conventions for the project. These rules
exist to enforce extreme homogeneity across the codebase. The goal is that any developer
can open any file and immediately understand its structure, naming, and intent without
having to learn a new style.

**Default policy: no comments.** Code must be self-documenting through clear naming and
structure. Comments are permitted only when they explain *why* a non-obvious decision was
made. Comments that describe *what* the code does are prohibited; if the code cannot be
understood without comments, rewrite the code.

---

## Style Rules

- `gofmt`/`goimports` non-negotiable; `golangci-lint` (errcheck, staticcheck,
  revive, gosec) at zero warnings — `//nolint` needs a why-comment.
- Model the domain in types: small structs; closed sets as typed constants
  (`type Status int` + `iota`); no boolean-trap fields.
- Interfaces at the consumer, 1–3 methods, named after the behavior
  (`Notifier`, `PolicyStore`); accept interfaces, return concrete structs.
  Hexagonal exception: ports live in the domain package, named by role —
  adapters satisfy them implicitly (no `implements`).
- Composition over inheritance: embedding is implementation reuse, never
  is-a modeling.

---

## Naming Rules

- Visibility by case: `CamelCase` exported, `camelCase` internal — there is
  no `public`/`private`.
- Getters bare (`p.Holder()`), never `GetHolder`; `SetX` allowed. Initialisms
  consistently capitalized: `ServeHTTP`, `apiKey`.
- Package names short, lowercase, singular, content-addressed (`telemetry`,
  never `utils`/`helpers`/`common`); the package name is half the API:
  `telemetry.Sink`, not `TelemetrySink`.

---

## File Structure

- One `go.mod` at root; binaries in `cmd/<name>/main.go` — thin assembly
  only, zero business logic.
- `internal/` for everything not meant to be imported; `pkg/` only with a
  real external consumer (default: no).
- Domain packages import no framework, ORM or transport: I/O at the edges,
  behind small interfaces.

---

## Test Rules

- Tests next to the code (`xxx_test.go`); table-driven is the default shape;
  stdlib assertions, no testify unless a spec says so.
- `go test ./... -race -count=1` green is part of done.
- Every goroutine has an owner: lifecycle via `errgroup`, done channel or
  `context` — leaks are review-blocking. Fuzz (`go test -fuzz`) for parsers.

---

## Error Handling

- Errors are values: returned or explicitly handled — `_ = err` banned. No
  `panic` outside `main` and startup failures.
- Wrap with context (`fmt.Errorf("loading policy %s: %w", id, err)`); match
  with `errors.Is/As`, never string matching. The caller decides what an
  error means — no log-and-return.
- `context.Context` is the first parameter of anything doing I/O;
  cancellation always propagates.

---

## Design Principles

Minimum bar for every stack and language — the language sections above may add
rules, never subtract these:

- **Semantic types over loose dictionaries.** Model domain concepts as enums,
  DTOs, value objects, records or dataclasses instead of passing raw
  dictionaries/maps around — the type name carries the meaning.
- **One concept per class, one responsibility per method** (Single
  Responsibility Principle).
- **No nested try/catch.** Extract a named function that handles one failure
  mode instead.
- **Prefer composition over inheritance.**
- **Prefer immutability:** final fields, immutable objects, unmodifiable
  collections — initialize once, never mutate shared state in place.
- **Tell, don't ask.** Behavior lives on the object that owns the data; value
  objects are rich (they validate and act), not anemic data bags.

---

## Commit Rules

Every commit in this repository follows
[Conventional Commits](https://www.conventionalcommits.org/):

- Format: `<type>(<optional scope>)!: <summary>` — e.g. `feat(auth): add refresh-token rotation`.
- Types: `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `release`, `revert`.
- `!` (or a `BREAKING CHANGE:` footer) marks a breaking change.
- `.git/hooks/commit-msg` (source: `harness/tools/commit-msg`) enforces the
  format on every commit; `--no-verify` is reserved for exceptional cases,
  never a workaround.
- Use `harness/tools/commit.sh -m "<subject>" <path>…` to commit: it stages
  exactly the paths you name and never `-a`, so sessions sharing this tree
  cannot pick up each other's work.

---

## Code quality

- `go vet`, `gofmt -l .` empty, `govulncheck ./...` in CI.
- Structured logging via `log/slog` with fields; no `fmt.Println` outside
  `cmd/`.
<!-- harness:module:architecture:start -->
<!-- harness:architecture:start -->
## Architecture: Hexagonal (Ports & Adapters)

- Source dependencies point **inward only** (adapter → port → domain); the domain
  imports no framework, ORM or transport type.
- Ports live in the domain and are named by role (`PolicyStore`), never by technology;
  adapters implement them and own the mapping.
- Every use case is driven through its inbound port — adapters never call each other
  and never reach into the domain except through use cases.
<!-- harness:architecture:end -->

<!-- harness:module:architecture:end -->
