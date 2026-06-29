package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"ward/internal/daemon"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Start the Ward daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		cPath := expandPath(cfgPath)
		if _, err := os.Stat(cPath); err != nil {
			return fmt.Errorf("config file %q does not exist. Please create it first", cPath)
		}

		sOverride := ""
		if cmd.Flags().Changed("socket") {
			sOverride = expandPath(socketPath)
		}

		ctx, cancel := context.WithCancel(context.Background())
		d := daemon.NewDaemon()
		return d.Run(ctx, cancel, cPath, sOverride)
	},
}

func init() {
	RootCmd.AddCommand(upCmd)
}
