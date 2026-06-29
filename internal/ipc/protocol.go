package ipc

import (
	"encoding/json"
)

type CommandType string

const (
	CmdList    CommandType = "list"
	CmdStatus  CommandType = "status"
	CmdStart   CommandType = "start"
	CmdStop    CommandType = "stop"
	CmdRestart CommandType = "restart"
	CmdReload  CommandType = "reload"
	CmdLogs    CommandType = "logs"
	CmdTail    CommandType = "tail"
)

type Command struct {
	Type   CommandType `json:"command"`
	Target string      `json:"target,omitempty"`
	Lines  int         `json:"lines,omitempty"`
}

type Response struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}
