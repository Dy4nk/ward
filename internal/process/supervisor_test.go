package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"ward/internal/config"
	"ward/internal/store"
)

var helperPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "ward-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	helperPath = filepath.Join(tmpDir, "helper")
	if runtime.GOOS == "windows" {
		helperPath += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", helperPath, "./testdata/helper/main.go")
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

func TestSupervisor_StartStop(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ward_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	autostart := false
	pConf := config.ProcessConfig{
		Name:         "sleep-proc",
		Command:      helperPath,
		Args:         []string{"sleep"},
		Restart:      "never",
		RestartDelay: "1s",
		GracePeriod:  "1s",
		Stdout:       "discard",
		Stderr:       "discard",
		Autostart:    &autostart,
	}

	sup := NewSupervisor(pConf, s, func(string) bool { return true })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sup.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	if sup.getState() != StateStopped {
		t.Errorf("expected initially stopped, got %v", sup.getState())
	}

	if err := sup.Start(); err != nil {
		t.Fatalf("failed to start supervisor: %v", err)
	}

	if sup.getState() != StateStarting {
		t.Errorf("expected starting, got %v", sup.getState())
	}

	time.Sleep(2200 * time.Millisecond)
	if sup.getState() != StateRunning {
		t.Errorf("expected running, got %v", sup.getState())
	}

	if err := sup.Stop(); err != nil {
		t.Fatalf("failed to stop supervisor: %v", err)
	}

	if sup.getState() != StateStopped {
		t.Errorf("expected stopped after Stop(), got %v", sup.getState())
	}
}

func TestSupervisor_RestartAlways(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ward_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	autostart := true
	pConf := config.ProcessConfig{
		Name:         "exit1-proc",
		Command:      helperPath,
		Args:         []string{"exit1"},
		Restart:      "always",
		RestartDelay: "100ms",
		GracePeriod:  "1s",
		Stdout:       "discard",
		Stderr:       "discard",
		Autostart:    &autostart,
	}

	sup := NewSupervisor(pConf, s, func(string) bool { return true })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sup.Run(ctx)

	// Sleep to let it crash and restart multiple times
	time.Sleep(800 * time.Millisecond)

	restarts := sup.restartCount.Load()
	if restarts < 2 {
		t.Errorf("expected at least 2 restarts, got %d", restarts)
	}

	if err := sup.Stop(); err != nil {
		t.Errorf("failed to stop supervisor: %v", err)
	}
}

func TestSupervisor_NoRestartOnSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ward_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	autostart := true
	pConf := config.ProcessConfig{
		Name:         "exit0-proc",
		Command:      helperPath,
		Args:         []string{"exit0"},
		Restart:      "on-failure",
		RestartDelay: "100ms",
		GracePeriod:  "1s",
		Stdout:       "discard",
		Stderr:       "discard",
		Autostart:    &autostart,
	}

	sup := NewSupervisor(pConf, s, func(string) bool { return true })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sup.Run(ctx)

	time.Sleep(300 * time.Millisecond)

	if sup.getState() != StateStopped {
		t.Errorf("expected stopped after exit 0, got %v", sup.getState())
	}
	if sup.restartCount.Load() != 0 {
		t.Errorf("expected 0 restarts, got %d", sup.restartCount.Load())
	}
}

func TestSupervisor_ForceKill(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ward_test.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}
	defer s.Close()

	autostart := true
	pConf := config.ProcessConfig{
		Name:         "ignoreterm-proc",
		Command:      helperPath,
		Args:         []string{"ignoreterm"},
		Restart:      "never",
		RestartDelay: "1s",
		GracePeriod:  "100ms",
		Stdout:       "discard",
		Stderr:       "discard",
		Autostart:    &autostart,
	}

	sup := NewSupervisor(pConf, s, func(string) bool { return true })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sup.Run(ctx)

	time.Sleep(200 * time.Millisecond)

	// Now stop it, it will ignore SIGTERM, forcing SIGKILL after 100ms
	startStop := time.Now()
	if err := sup.Stop(); err != nil {
		t.Fatalf("failed to stop supervisor: %v", err)
	}
	duration := time.Since(startStop)

	if sup.getState() != StateStopped {
		t.Errorf("expected stopped after Stop(), got %v", sup.getState())
	}

	if duration > 2*time.Second {
		t.Errorf("expected force kill to happen quickly, but took %v", duration)
	}
}
