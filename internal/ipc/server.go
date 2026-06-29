package ipc

import (
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"time"

	"ward/internal/process"
)

type Server struct {
	listener net.Listener
	manager  *process.Manager
	socket   string
	done     chan struct{}
}

func NewServer(socketPath string, m *process.Manager) (*Server, error) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	l, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}

	if err := os.Chmod(socketPath, 0600); err != nil {
		l.Close()
		_ = os.Remove(socketPath)
		return nil, err
	}

	return &Server{
		listener: l,
		manager:  m,
		socket:   socketPath,
		done:     make(chan struct{}),
	}, nil
}

func (s *Server) Start() {
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.done:
					return
				default:
					slog.Error("Failed to accept connection on ipc socket", "error", err)
					continue
				}
			}
			go s.handleConnection(conn)
		}
	}()
}

func (s *Server) Close() {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}
	_ = os.Remove(s.socket)
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var cmd Command
	if err := json.NewDecoder(conn).Decode(&cmd); err != nil {
		s.writeResponse(conn, Response{OK: false, Error: "failed to decode command: " + err.Error()})
		return
	}

	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))

	var resp Response
	switch cmd.Type {
	case CmdList:
		list := s.manager.List()
		data, err := json.Marshal(list)
		if err != nil {
			resp = Response{OK: false, Error: "failed to marshal process list: " + err.Error()}
		} else {
			resp = Response{OK: true, Data: data}
		}

	case CmdStatus:
		if cmd.Target == "" {
			resp = Response{OK: false, Error: "target process name is required"}
		} else {
			status, err := s.manager.Status(cmd.Target)
			if err != nil {
				resp = Response{OK: false, Error: err.Error()}
			} else {
				data, err := json.Marshal(status)
				if err != nil {
					resp = Response{OK: false, Error: "failed to marshal status: " + err.Error()}
				} else {
					resp = Response{OK: true, Data: data}
				}
			}
		}

	case CmdStart:
		if cmd.Target == "" {
			resp = Response{OK: false, Error: "target process name is required"}
		} else {
			err := s.manager.Start(cmd.Target)
			if err != nil {
				resp = Response{OK: false, Error: err.Error()}
			} else {
				resp = Response{OK: true}
			}
		}

	case CmdStop:
		if cmd.Target == "" {
			resp = Response{OK: false, Error: "target process name is required"}
		} else {
			err := s.manager.Stop(cmd.Target)
			if err != nil {
				resp = Response{OK: false, Error: err.Error()}
			} else {
				resp = Response{OK: true}
			}
		}

	case CmdRestart:
		if cmd.Target == "" {
			resp = Response{OK: false, Error: "target process name is required"}
		} else {
			err := s.manager.Restart(cmd.Target)
			if err != nil {
				resp = Response{OK: false, Error: err.Error()}
			} else {
				resp = Response{OK: true}
			}
		}

	case CmdReload:
		err := triggerSelfSIGHUP()
		if err != nil {
			resp = Response{OK: false, Error: "failed to trigger reload: " + err.Error()}
		} else {
			resp = Response{OK: true}
		}

	case CmdLogs:
		if cmd.Target == "" {
			resp = Response{OK: false, Error: "target process name is required"}
		} else {
			logs, err := s.manager.GetLogs(cmd.Target, cmd.Lines)
			if err != nil {
				resp = Response{OK: false, Error: err.Error()}
			} else {
				data, err := json.Marshal(logs)
				if err != nil {
					resp = Response{OK: false, Error: "failed to marshal logs: " + err.Error()}
				} else {
					resp = Response{OK: true, Data: data}
				}
			}
		}

	case CmdTail:
		if cmd.Target == "" {
			resp = Response{OK: false, Error: "target process name is required"}
			s.writeResponse(conn, resp)
			return
		}
		broadcaster, err := s.manager.GetBroadcaster(cmd.Target)
		if err != nil {
			resp = Response{OK: false, Error: err.Error()}
			s.writeResponse(conn, resp)
			return
		}

		_ = conn.SetDeadline(time.Time{})
		s.writeResponse(conn, Response{OK: true})

		id, ch := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(id)

		for {
			select {
			case data, ok := <-ch:
				if !ok {
					return
				}
				_, err := conn.Write(data)
				if err != nil {
					return
				}
			case <-s.done:
				return
			}
		}

	default:
		resp = Response{OK: false, Error: "unknown command: " + string(cmd.Type)}
	}

	s.writeResponse(conn, resp)
}

func (s *Server) writeResponse(conn net.Conn, resp Response) {
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		slog.Error("Failed to write response to ipc connection", "error", err)
	}
}
