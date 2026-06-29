package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var reloadCmd = &cobra.Command{
	Use:   "reload",
	Short: "Reload Ward configuration dynamically",
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

		err = sendHupSignal(pid)
		if err != nil {
			return fmt.Errorf("failed to reload ward daemon (PID %d): %w", pid, err)
		}

		fmt.Printf("Sent reload signal (SIGHUP) to Ward daemon (PID %d)\n", pid)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(reloadCmd)
}
