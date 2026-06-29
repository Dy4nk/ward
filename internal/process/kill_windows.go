//go:build windows

package process

import (
	"os"
	"syscall"
)

func killProcessGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

const (
	sigTerm = syscall.Signal(15)
	sigKill = syscall.Signal(9)
)
