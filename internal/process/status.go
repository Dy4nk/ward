package process

import "time"

type ProcessStatus struct {
	Name         string     `json:"name"`
	State        string     `json:"state"`
	PID          int        `json:"pid"`
	UptimeMs     int64      `json:"uptime_ms"`
	RestartCount int        `json:"restart_count"`
	LastExitCode *int       `json:"last_exit_code"`
	LastExitTime *time.Time `json:"last_exit_time"`
}
