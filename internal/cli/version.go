package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Ward",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ward version %s\n", Version)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
