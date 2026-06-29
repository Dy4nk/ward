package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"ward/internal/process"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

func isConnectionRefused(err error) bool {
	return errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.Errno(10061))
}

func (c *Client) send(cmd Command) (*Response, error) {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || isConnectionRefused(err) {
			return nil, errors.New("ward is not running. start it with: ward up")
		}
		return nil, fmt.Errorf("failed to connect to ward daemon socket: %w", err)
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return nil, fmt.Errorf("failed to write command to socket: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response from socket: %w", err)
	}

	if !resp.OK {
		return nil, errors.New(resp.Error)
	}

	return &resp, nil
}

func (c *Client) List() ([]process.ProcessStatus, error) {
	resp, err := c.send(Command{Type: CmdList})
	if err != nil {
		return nil, err
	}

	var list []process.ProcessStatus
	if err := json.Unmarshal(resp.Data, &list); err != nil {
		return nil, fmt.Errorf("failed to unmarshal process list response: %w", err)
	}

	return list, nil
}

func (c *Client) Status(name string) (*process.ProcessStatus, error) {
	resp, err := c.send(Command{Type: CmdStatus, Target: name})
	if err != nil {
		return nil, err
	}

	var status process.ProcessStatus
	if err := json.Unmarshal(resp.Data, &status); err != nil {
		return nil, fmt.Errorf("failed to unmarshal process status response: %w", err)
	}

	return &status, nil
}

func (c *Client) Start(name string) error {
	_, err := c.send(Command{Type: CmdStart, Target: name})
	return err
}

func (c *Client) Stop(name string) error {
	_, err := c.send(Command{Type: CmdStop, Target: name})
	return err
}

func (c *Client) Restart(name string) error {
	_, err := c.send(Command{Type: CmdRestart, Target: name})
	return err
}

func (c *Client) Reload() error {
	_, err := c.send(Command{Type: CmdReload})
	return err
}

func (c *Client) Logs(name string, lines int) ([]byte, error) {
	resp, err := c.send(Command{Type: CmdLogs, Target: name, Lines: lines})
	if err != nil {
		return nil, err
	}

	var data []byte
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal logs data: %w", err)
	}
	return data, nil
}

func (c *Client) Tail(ctx context.Context, name string, w io.Writer) error {
	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || isConnectionRefused(err) {
			return errors.New("ward is not running. start it with: ward up")
		}
		return fmt.Errorf("failed to connect to ward daemon socket: %w", err)
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	cmd := Command{Type: CmdTail, Target: name}
	if err := json.NewEncoder(conn).Encode(cmd); err != nil {
		return fmt.Errorf("failed to write tail command to socket: %w", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return fmt.Errorf("failed to read tail response from socket: %w", err)
	}

	if !resp.OK {
		return errors.New(resp.Error)
	}

	_ = conn.SetDeadline(time.Time{})

	copyErrCh := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(w, conn)
		copyErrCh <- copyErr
	}()

	select {
	case err := <-copyErrCh:
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, syscall.EPIPE) && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("error during streaming logs: %w", err)
		}
		return nil
	case <-ctx.Done():
		_ = conn.Close()
		<-copyErrCh
		return nil
	}
}
