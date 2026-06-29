//go:build !windows

package daemon

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ward/internal/config"
	"ward/internal/process"
)

func handleSignals(ctx context.Context, cancel context.CancelFunc, m *process.Manager, configPath string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				slog.Info("Received termination signal, initiating shutdown", "signal", sig.String())
				cancel()
				return
			case syscall.SIGHUP:
				slog.Info("Received SIGHUP signal, reloading configuration", "path", configPath)
				newCfg, err := config.LoadConfig(configPath)
				if err != nil {
					slog.Error("Configuration reload failed (keeping old config)", "error", err)
					continue
				}
				if err := m.Reload(newCfg, ctx); err != nil {
					slog.Error("Failed to reload process manager", "error", err)
				} else {
					slog.Info("Configuration reload completed successfully")
				}
			}
		}
	}
}
