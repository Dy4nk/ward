package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type ProcessEvent struct {
	Name         string
	Event        string // "started", "stopped", "crashed", "killed"
	PID          int
	ExitCode     int
	StartedAt    time.Time
	EndedAt      time.Time
	RestartCount int
}

type Store struct {
	db      *sql.DB
	eventCh chan ProcessEvent
	doneCh  chan struct{}
	wg      sync.WaitGroup
}

func NewStore(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	if err := Migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	s := &Store{
		db:      db,
		eventCh: make(chan ProcessEvent, 1000),
		doneCh:  make(chan struct{}),
	}

	s.wg.Add(1)
	go s.run()

	return s, nil
}

func (s *Store) Close() error {
	close(s.doneCh)
	s.wg.Wait()
	for {
		select {
		case e := <-s.eventCh:
			_ = s.writeEvent(e)
		default:
			return s.db.Close()
		}
	}
}

func (s *Store) RecordEvent(e ProcessEvent) {
	select {
	case s.eventCh <- e:
	default:
		slog.Warn("Store event queue full, event dropped", "process", e.Name, "event", e.Event)
	}
}

func (s *Store) run() {
	defer s.wg.Done()
	for {
		select {
		case e := <-s.eventCh:
			if err := s.writeEvent(e); err != nil {
				slog.Error("failed to write process event to sqlite", "error", err, "process", e.Name, "event", e.Event)
			}
		case <-s.doneCh:
			return
		}
	}
}

func (s *Store) writeEvent(e ProcessEvent) error {
	var startedAt, endedAt interface{}
	if !e.StartedAt.IsZero() {
		startedAt = e.StartedAt
	}
	if !e.EndedAt.IsZero() {
		endedAt = e.EndedAt
	}

	_, err := s.db.Exec(`
INSERT INTO process_events (name, event, pid, exit_code, started_at, ended_at, restart_count)
VALUES (?, ?, ?, ?, ?, ?, ?)
`, e.Name, e.Event, e.PID, e.ExitCode, startedAt, endedAt, e.RestartCount)
	return err
}

func (s *Store) QuerySummary() (map[string]ProcessEvent, error) {
	rows, err := s.db.Query(`
SELECT name, event, pid, exit_code, started_at, ended_at, restart_count
FROM process_events
WHERE id IN (SELECT MAX(id) FROM process_events GROUP BY name)
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summary := make(map[string]ProcessEvent)
	for rows.Next() {
		var e ProcessEvent
		var startedAtNull, endedAtNull sql.NullTime
		var pidNull, exitCodeNull sql.NullInt64

		err := rows.Scan(&e.Name, &e.Event, &pidNull, &exitCodeNull, &startedAtNull, &endedAtNull, &e.RestartCount)
		if err != nil {
			return nil, err
		}

		if pidNull.Valid {
			e.PID = int(pidNull.Int64)
		}
		if exitCodeNull.Valid {
			e.ExitCode = int(exitCodeNull.Int64)
		}
		if startedAtNull.Valid {
			e.StartedAt = startedAtNull.Time
		}
		if endedAtNull.Valid {
			e.EndedAt = endedAtNull.Time
		}

		summary[e.Name] = e
	}

	return summary, nil
}

func (s *Store) QueryRecent(limit int) ([]ProcessEvent, error) {
	rows, err := s.db.Query(`
SELECT name, event, pid, exit_code, started_at, ended_at, restart_count
FROM process_events
ORDER BY id DESC
LIMIT ?
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []ProcessEvent
	for rows.Next() {
		var e ProcessEvent
		var startedAtNull, endedAtNull sql.NullTime
		var pidNull, exitCodeNull sql.NullInt64

		err := rows.Scan(&e.Name, &e.Event, &pidNull, &exitCodeNull, &startedAtNull, &endedAtNull, &e.RestartCount)
		if err != nil {
			return nil, err
		}

		if pidNull.Valid {
			e.PID = int(pidNull.Int64)
		}
		if exitCodeNull.Valid {
			e.ExitCode = int(exitCodeNull.Int64)
		}
		if startedAtNull.Valid {
			e.StartedAt = startedAtNull.Time
		}
		if endedAtNull.Valid {
			e.EndedAt = endedAtNull.Time
		}

		events = append(events, e)
	}

	return events, nil
}
