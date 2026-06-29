//go:build windows

package ipc

import (
	"errors"
)

func triggerSelfSIGHUP() error {
	return errors.New("reload socket command is not supported on Windows; send SIGHUP via signal helper instead")
}
