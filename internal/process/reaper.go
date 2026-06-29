//go:build linux

package process

import (
	"context"
	"log/slog"
	"syscall"
	"time"
)

func Init() error {
	// 36 is PR_SET_CHILD_SUBREAPER on Linux.
	_, _, errno := syscall.Syscall(syscall.SYS_PRCTL, 36, 1, 0)
	if errno != 0 {
		return errno
	}
	slog.Info("Registered as child subreaper")
	return nil
}

// Run reaps zombie processes periodically.
func RunReaper(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			reapZombies()
		}
	}
}

func reapZombies() {
	for {
		var wstatus syscall.WaitStatus
		wpid, err := syscall.Wait4(-1, &wstatus, syscall.WNOHANG, nil)
		if err != nil {
			if err == syscall.ECHILD {
				break
			}
			slog.Debug("Error in subreaper wait4", "error", err)
			break
		}
		if wpid <= 0 {
			break
		}
		slog.Debug("Subreaper reaped zombie process", "pid", wpid)
	}
}
