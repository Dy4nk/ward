//go:build !windows

package ipc

import (
	"os"
	"syscall"
)

func triggerSelfSIGHUP() error {
	return syscall.Kill(os.Getpid(), syscall.SIGHUP)
}
