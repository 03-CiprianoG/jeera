// Package store is Jeera's local-first system of record. It persists the core
// domain model in a single SQLite database (via the pure-Go modernc driver, so
// the binary stays static and CGO-free) and publishes a change event after every
// committed mutation, so the TUI can refresh the instant an agent writes through
// the MCP server.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/03-CiprianoG/jeera/internal/core"
	"github.com/pressly/goose/v3"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the SQLite database and a small in-process change-event bus.
// Mutating methods serialize through writeMu to avoid SQLITE_BUSY churn while
// still allowing concurrent reads; the event bus lets the TUI subscribe to
// changes made by either front-end.
type Store struct {
	db      *sql.DB
	writeMu sync.Mutex

	subMu   sync.Mutex
	subs    map[int]chan core.Event
	nextSub int
}

// Open opens (creating if necessary) the SQLite database at path, applies all
// embedded migrations, and returns a ready Store. Pass ":memory:" for an
// ephemeral database (tests). The parent directory is created as needed.
func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if dir := filepath.Dir(path); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, fmt.Errorf("store: create data dir: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	// A single writer connection keeps WAL writes serialized and sidesteps
	// busy errors; readers reuse it fine for Jeera's scale.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db, subs: make(map[int]chan core.Event)}, nil
}

// dsn builds a modernc DSN with the pragmas Jeera relies on: a busy timeout so
// concurrent access waits rather than failing, WAL for reader/writer
// concurrency, and enforced foreign keys (off by default in SQLite).
func dsn(path string) string {
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	return "file:" + path + "?" + q.Encode()
}

// migrate applies the embedded goose migrations. goose's logger is redirected to
// io.Discard because Jeera shares stdout with the full-screen TUI.
func migrate(db *sql.DB) error {
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(log.New(io.Discard, "", 0))
	goose.SetVerbose(false)
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store: set dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Close releases the database and closes every subscriber channel.
func (s *Store) Close() error {
	s.subMu.Lock()
	for id, ch := range s.subs {
		close(ch)
		delete(s.subs, id)
	}
	s.subMu.Unlock()
	return s.db.Close()
}

// DB exposes the underlying handle for advanced callers (e.g. integration
// tests). Most code should use the repository methods.
func (s *Store) DB() *sql.DB { return s.db }

// Subscribe registers a listener for change events. It returns the receive
// channel and a cancel func that unregisters and closes it. The channel is
// buffered and lossy: if a slow subscriber falls behind, events are dropped
// rather than blocking a writer, since each event is only a hint to re-read.
func (s *Store) Subscribe() (<-chan core.Event, func()) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	id := s.nextSub
	s.nextSub++
	ch := make(chan core.Event, 64)
	s.subs[id] = ch
	return ch, func() {
		s.subMu.Lock()
		defer s.subMu.Unlock()
		if existing, ok := s.subs[id]; ok {
			close(existing)
			delete(s.subs, id)
		}
	}
}

// publish fans an event out to all current subscribers without blocking.
func (s *Store) publish(ev core.Event) {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- ev:
		default: // subscriber is behind; drop — the next read picks up the truth
		}
	}
}

// now returns the current time in the canonical UTC form used for timestamps.
func (s *Store) now() time.Time { return time.Now().UTC() }
