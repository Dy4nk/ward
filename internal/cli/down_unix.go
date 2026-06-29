//go:build !windows

package cli

import (
	"syscall"
)

func sendTermSignal(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

func sendHupSignal(pid int) error {
	return syscall.Kill(pid, syscall.SIGHUP)
}
