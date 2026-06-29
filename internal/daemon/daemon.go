package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"ward/internal/config"
	"ward/internal/ipc"
	"ward/internal/process"
	"ward/internal/store"
)

type Daemon struct{}

func NewDaemon() *Daemon {
	return &Daemon{}
}

func (d *Daemon) Run(ctx context.Context, cancel context.CancelFunc, configPath string, socketOverride string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if socketOverride != "" {
		cfg.Settings.SocketPath = socketOverride
	}

	dbPath := cfg.Settings.DBPath
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to initialize store: %w", err)
	}
	defer s.Close()

	if err := process.Init(); err != nil {
		slog.Warn("Failed to initialize subreaper", "error", err)
	}

	m := process.NewManager(cfg, s, ctx)

	server, err := ipc.NewServer(cfg.Settings.SocketPath, m)
	if err != nil {
		return fmt.Errorf("failed to start ipc server: %w", err)
	}
	server.Start()
	defer server.Close()

	pidPath := cfg.Settings.PIDPath
	if err := writePIDFile(pidPath); err != nil {
		return fmt.Errorf("failed to write pid file: %w", err)
	}
	defer os.Remove(pidPath)

	slog.Info("Ward daemon is up and running", "pid", os.Getpid(), "socket", cfg.Settings.SocketPath, "db", dbPath)

	go handleSignals(ctx, cancel, m, configPath)

	go process.RunReaper(ctx)

	<-ctx.Done()

	slog.Info("Shutting down Ward daemon...")
	m.Wait()
	slog.Info("Ward daemon stopped cleanly")
	return nil
}

func writePIDFile(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	pid := os.Getpid()
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
}
