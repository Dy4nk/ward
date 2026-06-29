package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"ward/internal/ipc"
)

var restartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart a supervised process",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sAddr, err := resolveSocketPath()
		if err != nil {
			return err
		}
		name := args[0]
		client := ipc.NewClient(sAddr)
		if err := client.Restart(name); err != nil {
			return err
		}
		fmt.Printf("Restarted process %q\n", name)
		return nil
	},
}

func init() {
	RootCmd.AddCommand(restartCmd)
}
