package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"ward/internal/ipc"
)

var (
	followLogs bool
	numLines   int
)

var logsCmd = &cobra.Command{
	Use:   "logs <name>",
	Short: "Show logs of a supervised process",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		sAddr, err := resolveSocketPath()
		if err != nil {
			return err
		}

		client := ipc.NewClient(sAddr)

		if !followLogs {
			data, err := client.Logs(name, numLines)
			if err != nil {
				return err
			}
			fmt.Print(string(data))
			return nil
		}

		// Dump last 50 lines first
		data, err := client.Logs(name, 50)
		if err == nil && len(data) > 0 {
			fmt.Print(string(data))
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		err = client.Tail(ctx, name, os.Stdout)
		if err != nil {
			return err
		}

		fmt.Println()
		return nil
	},
}

func init() {
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "follow log output")
	logsCmd.Flags().IntVarP(&numLines, "lines", "n", 50, "number of recent log lines to show")
	RootCmd.AddCommand(logsCmd)
}
