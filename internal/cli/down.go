package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop the Ward daemon and all supervised processes",
	RunE: func(cmd *cobra.Command, args []string) error {
		pidPath, err := resolvePIDPath()
		if err != nil {
			return err
		}

		data, err := os.ReadFile(pidPath)
		if err != nil {
			if os.IsNotExist(err) {
				return errors.New("ward daemon is not running (PID file not found)")
			}
			return fmt.Errorf("failed to read PID file: %w", err)
		}

		pidStr := strings.TrimSpace(string(data))
		if pidStr == "" {
			return errors.New("PID file is empty")
		}

		var pid int
		_, err = fmt.Sscanf(pidStr, "%d", &pid)
		if err != nil {
			return fmt.Errorf("invalid PID in PID file: %w", err)
		}

		err = sendTermSignal(pid)
		if err != nil {
			return fmt.Errorf("failed to stop ward daemon (PID %d): %w", pid, err)
		}

		fmt.Printf("Sent termination signal to Ward daemon (PID %d)\n", pid)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(downCmd)
}

func resolvePIDPath() (string, error) {
	cPath := expandPath(cfgPath)

	type miniSettings struct {
		PIDPath string `toml:"pid_path"`
	}
	type miniConfig struct {
		Settings miniSettings `toml:"settings"`
	}

	fallbackPID := "/tmp/ward.pid"

	data, err := os.ReadFile(cPath)
	if err != nil {
		return fallbackPID, nil
	}

	var mCfg miniConfig
	if _, err := toml.Decode(string(data), &mCfg); err == nil && mCfg.Settings.PIDPath != "" {
		return expandPath(mCfg.Settings.PIDPath), nil
	}

	return fallbackPID, nil
}
