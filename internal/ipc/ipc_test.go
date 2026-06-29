package ipc

import (
	"context"
	"path/filepath"
	"testing"

	"ward/internal/config"
	"ward/internal/process"
	"ward/internal/store"
)

func TestIPC_ServerClient(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "ward_test.sock")
	dbPath := filepath.Join(tmpDir, "ward_test.db")

	s, err := store.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer s.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{}
	m := process.NewManager(cfg, s, ctx)

	server, err := NewServer(socketPath, m)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	server.Start()
	defer server.Close()

	client := NewClient(socketPath)

	list, err := client.List()
	if err != nil {
		t.Fatalf("client.List failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}

	_, err = client.Status("non-existent")
	if err == nil {
		t.Errorf("expected error for non-existent process status, got nil")
	}

	err = client.Start("non-existent")
	if err == nil {
		t.Errorf("expected error for non-existent process start, got nil")
	}
}

func TestIPC_ClientDisconnectedError(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "non_existent.sock")
	client := NewClient(socketPath)

	_, err := client.List()
	if err == nil {
		t.Fatalf("expected error from list with disconnected server, got nil")
	}

	expectedMsg := "ward is not running. start it with: ward up"
	if err.Error() != expectedMsg {
		t.Errorf("expected error msg %q, got %q", expectedMsg, err.Error())
	}
}
