package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"ward/internal/ipc"
)

var startCmd = &cobra.Command{
	Use:   "start <name>",
	Short: "Start a supervised process",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sAddr, err := resolveSocketPath()
		if err != nil {
			return err
		}
		name := args[0]
		client := ipc.NewClient(sAddr)
		if err := client.Start(name); err != nil {
			return err
		}
		fmt.Printf("Started process %q\n", name)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(startCmd)
}
