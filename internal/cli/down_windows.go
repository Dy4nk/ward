//go:build windows

package cli

import (
	"errors"
	"os"
)

func sendTermSignal(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

func sendHupSignal(pid int) error {
	return errors.New("SIGHUP reload signal is not supported on Windows")
}
