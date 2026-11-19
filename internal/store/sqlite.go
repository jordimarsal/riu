// Package store is the SQLite adapter of the wikievent.EventStore port.
// Transport-adjacent concerns (SQL, pragmas, migrations) live here; the
// domain knows nothing about them.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver (pure Go, no cgo)

	"github.com/jordimarsal/riu/internal/wikievent"
)

// Store implements wikievent.EventStore on SQLite.
type Store struct {
	db *sql.DB
}

// migrations are applied in order on open; schema_version records the last
// applied one. Each entry runs in its own transaction with its stamp.
var migrations = []string{v1}

const v1 = `
CREATE TABLE events (
	domain  TEXT    NOT NULL,
	meta_id TEXT    NOT NULL,
	wiki_id INTEGER NOT NULL,
	type    TEXT    NOT NULL,
	title   TEXT    NOT NULL DEFAULT '',
	"user"  TEXT    NOT NULL DEFAULT '',
	bot     INTEGER NOT NULL DEFAULT 0,
	comment TEXT    NOT NULL DEFAULT '',
	old_len INTEGER NOT NULL DEFAULT 0,
	new_len INTEGER NOT NULL DEFAULT 0,
	at      TEXT    NOT NULL,
	PRIMARY KEY (domain, meta_id)
);
CREATE INDEX idx_events_domain_wiki ON events (domain, wiki_id);
`

// Open opens (creating it if needed) the database at path and applies any
// pending migration. The caller owns closing the returned Store.
func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	// modernc/sqlite: a single connection sidesteps SQLITE_BUSY contention —
	// F1 has exactly one writer anyway (design D7).
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// migrate applies pending migrations in order, stamping each inside its own
// transaction so a failed migration never leaves a half schema behind.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("store: schema_version: %w", err)
	}
	var applied int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&applied); err != nil {
		return fmt.Errorf("store: read schema_version: %w", err)
	}
	for i := applied; i < len(migrations); i++ {
		version := i + 1
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("store: begin v%d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx, migrations[i]); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: apply v%d: %w", version, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_version (version) VALUES (?)`, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: stamp v%d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("store: commit v%d: %w", version, err)
		}
	}
	return nil
}

// InsertIfNew implements wikievent.EventStore. An already-known identity
// (PRIMARY KEY (domain, meta_id)) is ignored by the database, not by us —
// the unique constraint is the idempotency mechanism, not application logic.
func (s *Store) InsertIfNew(ctx context.Context, e wikievent.Event) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO events
			(domain, meta_id, wiki_id, type, title, "user", bot, comment, old_len, new_len, at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Domain, e.MetaID, e.WikiID, e.Type, e.Title, e.User, e.Bot,
		e.Comment, e.OldLen, e.NewLen, e.At.Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("store: insert %s/%s: %w", e.Domain, e.MetaID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: rows affected %s/%s: %w", e.Domain, e.MetaID, err)
	}
	return n == 1, nil
}

// Count returns the number of persisted events (demo/stats support).
func (s *Store) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`).Scan(&n); err != nil {
		return 0, fmt.Errorf("store: count: %w", err)
	}
	return n, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("store: close: %w", err)
	}
	return nil
}
