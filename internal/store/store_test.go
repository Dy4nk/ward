package store

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestStore_MigrationsAndEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ward_test.db")

	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Second)
	e1 := ProcessEvent{
		Name:         "api",
		Event:        "started",
		PID:          1234,
		StartedAt:    now,
		RestartCount: 0,
	}
	s.RecordEvent(e1)

	e2 := ProcessEvent{
		Name:         "worker",
		Event:        "crashed",
		PID:          5678,
		ExitCode:     1,
		StartedAt:    now.Add(-10 * time.Second),
		EndedAt:      now,
		RestartCount: 2,
	}
	s.RecordEvent(e2)

	time.Sleep(150 * time.Millisecond)

	summary, err := s.QuerySummary()
	if err != nil {
		t.Fatalf("QuerySummary failed: %v", err)
	}

	if len(summary) != 2 {
		t.Errorf("expected 2 summary entries, got %d", len(summary))
	}

	apiEvent, ok := summary["api"]
	if !ok {
		t.Errorf("api process event not found in summary")
	} else {
		if apiEvent.PID != 1234 {
			t.Errorf("expected api PID 1234, got %d", apiEvent.PID)
		}
		if apiEvent.Event != "started" {
			t.Errorf("expected api event 'started', got %q", apiEvent.Event)
		}
	}

	workerEvent, ok := summary["worker"]
	if !ok {
		t.Errorf("worker process event not found in summary")
	} else {
		if workerEvent.ExitCode != 1 {
			t.Errorf("expected worker exit code 1, got %d", workerEvent.ExitCode)
		}
		if workerEvent.RestartCount != 2 {
			t.Errorf("expected worker restart count 2, got %d", workerEvent.RestartCount)
		}
	}

	recent, err := s.QueryRecent(10)
	if err != nil {
		t.Fatalf("QueryRecent failed: %v", err)
	}

	if len(recent) != 2 {
		t.Errorf("expected 2 recent entries, got %d", len(recent))
	}
}

func TestStore_CloseDrainsAllEvents(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ward_test_close.db")

	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	const (
		numGoroutines    = 10
		eventsPerRoutine = 100
		totalEvents      = numGoroutines * eventsPerRoutine
	)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < eventsPerRoutine; j++ {
				s.RecordEvent(ProcessEvent{
					Name:         "routine-proc",
					Event:        "started",
					PID:          routineID*1000 + j,
					StartedAt:    time.Now(),
					RestartCount: j,
				})
			}
		}(i)
	}

	wg.Wait()

	// Immediately close the store. This should wait for run() to exit, then
	// drain all remaining events, then close s.db without SQL database closed errors.
	if err := s.Close(); err != nil {
		t.Fatalf("failed to close store: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite DB directly: %v", err)
	}
	defer db.Close()

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM process_events").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows: %v", err)
	}

	if count != totalEvents {
		t.Errorf("expected %d events written to database, but got %d", totalEvents, count)
	}
}
