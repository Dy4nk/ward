//go:build windows

package daemon

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"ward/internal/process"
)

func handleSignals(ctx context.Context, cancel context.CancelFunc, m *process.Manager, configPath string) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-ctx.Done():
			return
		case sig := <-sigCh:
			switch sig {
			case os.Interrupt, syscall.SIGTERM:
				slog.Info("Received termination signal, initiating shutdown", "signal", sig.String())
				cancel()
				return
			}
		}
	}
}
