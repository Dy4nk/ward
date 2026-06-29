package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"ward/internal/ipc"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorDim    = "\033[2m"
)

var statusCmd = &cobra.Command{
	Use:     "status [name]",
	Aliases: []string{"ps"},
	Short:   "Show status of all or a specific supervised process",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sAddr, err := resolveSocketPath()
		if err != nil {
			return err
		}

		client := ipc.NewClient(sAddr)
		isTTY := term.IsTerminal(int(os.Stdout.Fd()))

		if len(args) == 1 {
			// Detail view of single process
			name := args[0]
			status, err := client.Status(name)
			if err != nil {
				return err
			}

			pidStr := dash(isTTY)
			if status.PID > 0 {
				pidStr = strconv.Itoa(status.PID)
			}

			exitStr := dash(isTTY)
			if status.LastExitCode != nil {
				exitStr = strconv.Itoa(*status.LastExitCode)
			}

			exitTimeStr := dash(isTTY)
			if status.LastExitTime != nil && !status.LastExitTime.IsZero() {
				exitTimeStr = status.LastExitTime.Format(time.RFC3339)
			}

			fmt.Printf("Name:           %s\n", status.Name)
			fmt.Printf("State:          %s\n", formatState(status.State, isTTY))
			fmt.Printf("PID:            %s\n", pidStr)
			fmt.Printf("Uptime:         %s\n", formatUptime(status.UptimeMs, isTTY))
			fmt.Printf("Restarts:       %d\n", status.RestartCount)
			fmt.Printf("Last Exit Code: %s\n", exitStr)
			fmt.Printf("Last Exit Time: %s\n", exitTimeStr)
			return nil
		}

		// Tabular list view of all processes
		list, err := client.List()
		if err != nil {
			return err
		}

		headers := []string{"NAME", "STATE", "PID", "UPTIME", "RESTARTS", "LAST EXIT"}
		widths := make([]int, len(headers))
		for i, h := range headers {
			widths[i] = len(h)
		}

		type row struct {
			cols []string
		}
		rows := make([]row, 0, len(list))
		for _, p := range list {
			pidStr := dash(isTTY)
			if p.PID > 0 {
				pidStr = strconv.Itoa(p.PID)
			}
			cols := []string{
				p.Name,
				formatState(p.State, isTTY),
				pidStr,
				formatUptime(p.UptimeMs, isTTY),
				strconv.Itoa(p.RestartCount),
				formatLastExit(p.LastExitCode, p.LastExitTime, isTTY),
			}
			rows = append(rows, row{cols: cols})

			for i, c := range cols {
				l := displayLength(c)
				if l > widths[i] {
					widths[i] = l
				}
			}
		}

		for i, h := range headers {
			fmt.Print(padRight(h, widths[i]+4))
		}
		fmt.Println()

		for _, r := range rows {
			for i, c := range r.cols {
				fmt.Print(padRight(c, widths[i]+4))
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(statusCmd)
}

func dash(isTTY bool) string {
	if isTTY {
		return "—"
	}
	return "-"
}

func formatState(state string, isTTY bool) string {
	if !isTTY {
		return state
	}
	switch state {
	case "running":
		return colorGreen + state + colorReset
	case "crashed", "fatal":
		return colorRed + state + colorReset
	case "starting", "stopping", "backoff":
		return colorYellow + state + colorReset
	case "stopped":
		return colorDim + state + colorReset
	default:
		return state
	}
}

func formatUptime(ms int64, isTTY bool) string {
	if ms <= 0 {
		return dash(isTTY)
	}
	dur := time.Duration(ms) * time.Millisecond
	dur = dur.Round(time.Second)

	if !isTTY {
		return dur.String()
	}

	if dur < time.Minute {
		return dur.String()
	}

	h := dur / time.Hour
	dur -= h * time.Hour
	m := dur / time.Minute
	dur -= m * time.Minute
	s := dur / time.Second

	if h > 24 {
		d := h / 24
		h -= d * 24
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}

	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}

	return fmt.Sprintf("%dm %ds", m, s)
}

func formatLastExit(code *int, t *time.Time, isTTY bool) string {
	if code == nil {
		return dash(isTTY)
	}
	if !isTTY || t == nil || t.IsZero() {
		return fmt.Sprintf("%d", *code)
	}
	ago := time.Since(*t).Round(time.Second)
	if ago < 0 {
		ago = 0
	}

	var agoStr string
	if ago < time.Minute {
		agoStr = fmt.Sprintf("%s ago", ago)
	} else if ago < time.Hour {
		agoStr = fmt.Sprintf("%dm ago", int(ago.Minutes()))
	} else if ago < 24*time.Hour {
		agoStr = fmt.Sprintf("%dh ago", int(ago.Hours()))
	} else {
		agoStr = fmt.Sprintf("%dd ago", int(ago.Hours()/24))
	}

	return fmt.Sprintf("%d (%s)", *code, agoStr)
}

func displayLength(str string) int {
	return len([]rune(stripANSI(str)))
}

func stripANSI(str string) string {
	var sb strings.Builder
	inESC := false
	for i := 0; i < len(str); i++ {
		if str[i] == '\033' {
			inESC = true
			continue
		}
		if inESC {
			if str[i] == 'm' {
				inESC = false
			}
			continue
		}
		sb.WriteByte(str[i])
	}
	return sb.String()
}

func padRight(str string, length int) string {
	l := displayLength(str)
	if l >= length {
		return str
	}
	return str + strings.Repeat(" ", length-l)
}
