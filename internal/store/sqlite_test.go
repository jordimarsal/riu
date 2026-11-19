package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jordimarsal/riu/internal/wikievent"
)

func testEvent(n int) wikievent.Event {
	return wikievent.Event{
		Domain:  "ca.wikipedia.org",
		MetaID:  "evt-000042",
		WikiID:  int64(100 + n),
		Type:    "edit",
		Title:   "Riu",
		User:    "jordi",
		Bot:     false,
		Comment: "test",
		OldLen:  10,
		NewLen:  12,
		At:      time.Date(2026, 11, 19, 12, 25, 50, 0, time.UTC),
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "riu.db"))
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestInsertIfNewIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	e := testEvent(0)

	inserted, err := s.InsertIfNew(ctx, e)
	if err != nil {
		t.Fatalf("first InsertIfNew() error: %v", err)
	}
	if !inserted {
		t.Fatal("first InsertIfNew() = false, want true (new identity)")
	}

	inserted, err = s.InsertIfNew(ctx, e)
	if err != nil {
		t.Fatalf("replay InsertIfNew() error: %v", err)
	}
	if inserted {
		t.Fatal("replay InsertIfNew() = true, want false (known identity)")
	}

	n, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows = %d, want 1 after one insert and one replay", n)
	}
}

func TestSameMetaIDDifferentDomainsAreDistinct(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	a, b := testEvent(0), testEvent(0)
	b.Domain = "fr.wikipedia.org"

	if inserted, err := s.InsertIfNew(ctx, a); err != nil || !inserted {
		t.Fatalf("first domain insert = (%v, %v), want (true, nil)", inserted, err)
	}
	if inserted, err := s.InsertIfNew(ctx, b); err != nil || !inserted {
		t.Fatalf("second domain insert = (%v, %v), want (true, nil) — identity is (domain, meta_id)", inserted, err)
	}
}

func TestReopenSurvivesAndMigrationsAreStable(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "riu.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("first Open() error: %v", err)
	}
	if _, err := s.InsertIfNew(ctx, testEvent(0)); err != nil {
		t.Fatalf("InsertIfNew() error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	s2, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("reopen Open() error: %v", err)
	}
	defer func() { _ = s2.Close() }()

	// Re-applying migrations on an already-current schema is a no-op: the
	// v1 CREATE TABLE uses IF NOT EXISTS semantics via the version stamp —
	// a second open must not fail nor lose data.
	n, err := s2.Count(ctx)
	if err != nil {
		t.Fatalf("Count() after reopen: %v", err)
	}
	if n != 1 {
		t.Fatalf("rows after reopen = %d, want 1 (data survives)", n)
	}
}

func TestClosedStoreReturnsError(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "riu.db")
	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	// After Close the driver refuses operations; our contract is that the
	// error is wrapped with store context and the event identity — callers
	// must be able to tell what failed without string-matching driver text.
	_, err = s.InsertIfNew(ctx, testEvent(0))
	if err == nil {
		t.Fatal("InsertIfNew() on closed store = nil, want error")
	}
	if !strings.Contains(err.Error(), "store: insert ca.wikipedia.org/evt-000042") {
		t.Fatalf("InsertIfNew() error lost store/identity context: %v", err)
	}
}
