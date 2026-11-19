// Package wikievent holds the pure domain of riu: events flowing through the
// pipeline and the ports that transport and storage adapters implement.
//
// It imports nothing from adapters: source dependencies point inward only.
package wikievent

import (
	"context"
	"errors"
	"time"
)

// Errors returned when normalizing raw wire payloads. Callers classify with
// errors.Is — never by string matching.
var (
	// ErrMalformed wraps any payload that cannot become an Event (bad JSON,
	// missing required fields, wrong types, unparsable timestamps).
	ErrMalformed = errors.New("wikievent: malformed event")
	// ErrUnknownSchema wraps payloads whose $schema is not a
	// mediawiki/recentchange schema (schema drift must be visible, not silent).
	ErrUnknownSchema = errors.New("wikievent: unknown schema")
)

// Event is one normalized change on a Wikimedia wiki. Pure value type: no
// transport, storage or framework concerns.
type Event struct {
	Domain  string    // wiki domain, e.g. "ca.wikipedia.org"
	MetaID  string    // stream event UUID (meta.id) — half of the identity
	WikiID  int64     // recentchange numeric id (revision id for edits)
	Type    string    // edit | new | log | categorize | ...
	Title   string    // page title
	User    string    // editor name
	Bot     bool      // flagged bot edit
	Comment string    // edit summary, may be empty
	OldLen  int64     // page length before the change (0 when unknown)
	NewLen  int64     // page length after the change (0 when unknown)
	At      time.Time // meta.dt — when the change happened
}

// Identity returns the dedup key: wiki domain plus stream event UUID.
func (e Event) Identity() (domain, metaID string) { return e.Domain, e.MetaID }

// EventSource produces events until the context is done. Errors surface on
// the error channel; both channels close when the stream ends. Implemented by
// transport adapters (the SSE client today).
type EventSource interface {
	Stream(ctx context.Context) (<-chan Event, <-chan error)
}

// EventStore persists events idempotently. Implemented by storage adapters.
type EventStore interface {
	// InsertIfNew stores e and reports whether it was new. Persisting an
	// already-known identity is not an error: it returns inserted=false.
	InsertIfNew(ctx context.Context, e Event) (inserted bool, err error)
}
