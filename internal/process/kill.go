//go:build !windows

package process

import (
	"syscall"
)

func killProcessGroup(pgid int, sig syscall.Signal) error {
	if pgid <= 0 {
		return nil
	}
	return syscall.Kill(-pgid, sig)
}

const (
	sigTerm = syscall.SIGTERM
	sigKill = syscall.SIGKILL
)
