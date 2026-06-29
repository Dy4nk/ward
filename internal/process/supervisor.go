package process

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"ward/internal/config"
	"ward/internal/store"
)

type ProcessState string

const (
	StateStopped  ProcessState = "stopped"
	StateStarting ProcessState = "starting"
	StateRunning  ProcessState = "running"
	StateStopping ProcessState = "stopping"
	StateCrashed  ProcessState = "crashed"
	StateBackoff  ProcessState = "backoff"
	StateFatal    ProcessState = "fatal"
)

type ControlAction string

const (
	ActionStart   ControlAction = "start"
	ActionStop    ControlAction = "stop"
	ActionRestart ControlAction = "restart"
	ActionUpdate  ControlAction = "update"
)

type ControlMessage struct {
	Action ControlAction
	Config config.ProcessConfig
	Resp   chan error
}

type Supervisor struct {
	config       config.ProcessConfig
	store        *store.Store
	state        atomic.Value // holds ProcessState
	pid          atomic.Int32
	pgid         atomic.Int32
	restartCount atomic.Int32
	lastExitCode atomic.Int32
	lastExitTime atomic.Value // holds time.Time
	uptimeStart  atomic.Value // holds time.Time

	controlCh chan ControlMessage
	exitCh    chan error

	healthTimer  *time.Timer
	backoffTimer *time.Timer
	graceTimer   *time.Timer

	stopResponseCh    chan error
	restartResponseCh chan error
	restartPending    bool

	drainWG sync.WaitGroup

	broadcaster *Broadcaster
	ringBuffer  *RingBuffer
	isDependencyRunning func(string) bool
}

func NewSupervisor(p config.ProcessConfig, s *store.Store, isDependencyRunning func(string) bool) *Supervisor {
	sup := &Supervisor{
		config:      p,
		store:       s,
		controlCh:   make(chan ControlMessage, 10),
		exitCh:      make(chan error, 1),
		broadcaster: NewBroadcaster(),
		ringBuffer:  NewRingBuffer(500),
		isDependencyRunning: isDependencyRunning,
	}
	sup.state.Store(StateStopped)
	sup.lastExitCode.Store(-1)
	sup.lastExitTime.Store(time.Time{})
	sup.uptimeStart.Store(time.Time{})
	return sup
}

func (s *Supervisor) Broadcaster() *Broadcaster {
	return s.broadcaster
}

func (s *Supervisor) RingBuffer() *RingBuffer {
	return s.ringBuffer
}

func (s *Supervisor) getState() ProcessState {
	val := s.state.Load()
	if val == nil {
		return StateStopped
	}
	return val.(ProcessState)
}

func (s *Supervisor) setState(state ProcessState) {
	s.state.Store(state)
}

func (s *Supervisor) Start() error {
	resp := make(chan error, 1)
	s.controlCh <- ControlMessage{Action: ActionStart, Resp: resp}
	return <-resp
}

func (s *Supervisor) Stop() error {
	resp := make(chan error, 1)
	s.controlCh <- ControlMessage{Action: ActionStop, Resp: resp}
	return <-resp
}

func (s *Supervisor) Restart() error {
	resp := make(chan error, 1)
	s.controlCh <- ControlMessage{Action: ActionRestart, Resp: resp}
	return <-resp
}

func (s *Supervisor) Run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Supervisor panic recovered", "process", s.config.Name, "panic", r)
			s.setState(StateFatal)
		}
		s.broadcaster.Close()
	}()

	if s.config.Autostart != nil && *s.config.Autostart {
		if err := s.startProcess(ctx); err != nil {
			slog.Error("Initial autostart failed", "process", s.config.Name, "error", err)
		}
	} else {
		s.setState(StateStopped)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("Supervisor received shutdown context signal", "process", s.config.Name)
			s.stopRunningProcess()
			return
		case msg := <-s.controlCh:
			s.handleControlMessage(ctx, msg)
		case exitErr := <-s.exitCh:
			s.handleProcessExit(ctx, exitErr)
		case <-s.healthTimerChan():
			s.transitionToRunningIfStarting()
		case <-s.backoffTimerChan():
			s.retryProcess(ctx)
		case <-s.graceTimerChan():
			s.forceKillProcess()
		}
	}
}

func (s *Supervisor) startProcess(ctx context.Context) error {
	if len(s.config.DependsOn) > 0 && s.isDependencyRunning != nil {
		slog.Info("Process waiting for dependencies", "process", s.config.Name, "depends_on", s.config.DependsOn)
		for _, depName := range s.config.DependsOn {
			ticker := time.NewTicker(200 * time.Millisecond)
			defer ticker.Stop()
			for {
				if s.isDependencyRunning(depName) {
					break
				}
				select {
				case <-ctx.Done():
					return context.Canceled
				case <-ticker.C:
				}
			}
		}
	}

	s.setState(StateStarting)
	s.uptimeStart.Store(time.Now())

	cmd := exec.Command(s.config.Command, s.config.Args...)
	cmd.Dir = s.config.Dir
	if len(s.config.Env) > 0 {
		env := os.Environ()
		for k, v := range s.config.Env {
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
		cmd.Env = env
	}

	cmd.SysProcAttr = sysProcAttr()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		s.setState(StateFatal)
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		stdoutPipe.Close()
		s.setState(StateFatal)
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdoutPipe.Close()
		stderrPipe.Close()
		s.setState(StateFatal)
		slog.Error("failed to start process", "process", s.config.Name, "error", err)
		s.store.RecordEvent(store.ProcessEvent{
			Name:         s.config.Name,
			Event:        "crashed",
			StartedAt:    s.uptimeStart.Load().(time.Time),
			EndedAt:      time.Now(),
			RestartCount: int(s.restartCount.Load()),
		})
		return fmt.Errorf("failed to start: %w", err)
	}

	s.pid.Store(int32(cmd.Process.Pid))
	s.pgid.Store(int32(cmd.Process.Pid))

	slog.Info("Process started", "process", s.config.Name, "pid", cmd.Process.Pid)
	s.store.RecordEvent(store.ProcessEvent{
		Name:         s.config.Name,
		Event:        "started",
		PID:          cmd.Process.Pid,
		StartedAt:    s.uptimeStart.Load().(time.Time),
		RestartCount: int(s.restartCount.Load()),
	})

	s.broadcaster.Send([]byte("\n--- ward: process starting ---\n"))

	s.drainWG.Add(2)
	go func() {
		defer s.drainWG.Done()
		drainPipe(s, stdoutPipe, s.config.Stdout, false)
	}()
	go func() {
		defer s.drainWG.Done()
		drainPipe(s, stderrPipe, s.config.Stderr, true)
	}()

	s.exitCh = make(chan error, 1)
	go func() {
		err := cmd.Wait()
		s.drainWG.Wait()
		s.exitCh <- err
	}()

	s.healthTimer = time.NewTimer(2 * time.Second)

	return nil
}

func (s *Supervisor) handleControlMessage(ctx context.Context, msg ControlMessage) {
	switch msg.Action {
	case ActionUpdate:
		s.config = msg.Config
		msg.Resp <- nil

	case ActionStart:
		state := s.getState()
		if state == StateStarting || state == StateRunning || state == StateBackoff {
			msg.Resp <- nil
			return
		}
		s.restartCount.Store(0)
		err := s.startProcess(ctx)
		msg.Resp <- err

	case ActionStop:
		state := s.getState()
		if state == StateStopped || state == StateFatal || state == StateCrashed {
			msg.Resp <- nil
			return
		}

		if state == StateBackoff {
			if s.backoffTimer != nil {
				s.backoffTimer.Stop()
				s.backoffTimer = nil
			}
			s.setState(StateStopped)
			msg.Resp <- nil
			return
		}

		s.setState(StateStopping)
		s.stopResponseCh = msg.Resp

		err := killProcessGroup(int(s.pgid.Load()), sigTerm)
		if err != nil {
			slog.Warn("Failed to send SIGTERM to process group", "process", s.config.Name, "error", err)
		}

		grace, _ := time.ParseDuration(s.config.GracePeriod)
		s.graceTimer = time.NewTimer(grace)

	case ActionRestart:
		state := s.getState()
		if state == StateStopped || state == StateFatal || state == StateCrashed {
			s.restartCount.Store(0)
			err := s.startProcess(ctx)
			msg.Resp <- err
			return
		}

		if state == StateBackoff {
			if s.backoffTimer != nil {
				s.backoffTimer.Stop()
				s.backoffTimer = nil
			}
			s.restartCount.Store(0)
			err := s.startProcess(ctx)
			msg.Resp <- err
			return
		}

		s.setState(StateStopping)
		s.restartResponseCh = msg.Resp
		s.restartPending = true

		err := killProcessGroup(int(s.pgid.Load()), sigTerm)
		if err != nil {
			slog.Warn("Failed to send SIGTERM to process group", "process", s.config.Name, "error", err)
		}

		grace, _ := time.ParseDuration(s.config.GracePeriod)
		s.graceTimer = time.NewTimer(grace)
	}
}

func (s *Supervisor) handleProcessExit(ctx context.Context, exitErr error) {
	if s.healthTimer != nil {
		s.healthTimer.Stop()
		s.healthTimer = nil
	}
	if s.graceTimer != nil {
		s.graceTimer.Stop()
		s.graceTimer = nil
	}

	pid := int(s.pid.Load())
	s.pid.Store(0)
	s.pgid.Store(0)

	exitCode := 0
	if exitErr != nil {
		var exitExt *exec.ExitError
		if errors.As(exitErr, &exitExt) {
			exitCode = exitExt.ExitCode()
		} else {
			exitCode = -1
		}
	}
	s.lastExitCode.Store(int32(exitCode))
	s.lastExitTime.Store(time.Now())

	slog.Info("Process exited", "process", s.config.Name, "pid", pid, "exit_code", exitCode)
	s.broadcaster.Send([]byte(fmt.Sprintf("\n--- ward: process exited (exit code %d) ---\n", exitCode)))

	var eventType string
	if s.getState() == StateStopping {
		eventType = "stopped"
	} else if exitCode == 0 {
		eventType = "stopped"
	} else {
		eventType = "crashed"
	}

	s.store.RecordEvent(store.ProcessEvent{
		Name:         s.config.Name,
		Event:        eventType,
		PID:          pid,
		ExitCode:     exitCode,
		StartedAt:    s.uptimeStart.Load().(time.Time),
		EndedAt:      time.Now(),
		RestartCount: int(s.restartCount.Load()),
	})

	if s.getState() == StateStopping {
		if s.restartPending {
			s.restartPending = false
			s.restartCount.Store(0)
			err := s.startProcess(ctx)
			if s.restartResponseCh != nil {
				s.restartResponseCh <- err
				s.restartResponseCh = nil
			}
			return
		}

		s.setState(StateStopped)
		if s.stopResponseCh != nil {
			s.stopResponseCh <- nil
			s.stopResponseCh = nil
		}
		return
	}

	shouldRestart := false
	switch s.config.Restart {
	case "always":
		shouldRestart = true
	case "on-failure":
		shouldRestart = (exitCode != 0)
	case "never":
		shouldRestart = false
	}

	if shouldRestart {
		maxRestarts := s.config.MaxRestarts
		currentRestarts := int(s.restartCount.Load())
		if maxRestarts > 0 && currentRestarts >= maxRestarts {
			slog.Warn("Max restarts reached, stopping process supervision", "process", s.config.Name, "max_restarts", maxRestarts)
			s.setState(StateFatal)
			return
		}

		s.restartCount.Add(1)
		s.setState(StateBackoff)
		delay, _ := time.ParseDuration(s.config.RestartDelay)
		slog.Info("Process backing off before restart", "process", s.config.Name, "delay", delay)
		s.backoffTimer = time.NewTimer(delay)
	} else {
		if exitCode == 0 {
			s.setState(StateStopped)
		} else {
			s.setState(StateCrashed)
		}
	}
}

func (s *Supervisor) transitionToRunningIfStarting() {
	s.healthTimer = nil
	if s.getState() == StateStarting {
		s.setState(StateRunning)
		slog.Info("Process is now stable and healthy", "process", s.config.Name)
	}
}

func (s *Supervisor) retryProcess(ctx context.Context) {
	s.backoffTimer = nil
	if s.getState() == StateBackoff {
		slog.Info("Retrying process execution after backoff", "process", s.config.Name)
		if err := s.startProcess(ctx); err != nil {
			slog.Error("Retry failed", "process", s.config.Name, "error", err)
		}
	}
}

func (s *Supervisor) forceKillProcess() {
	s.graceTimer = nil
	if s.getState() == StateStopping {
		slog.Warn("Grace period exceeded, sending SIGKILL to process group", "process", s.config.Name)
		_ = killProcessGroup(int(s.pgid.Load()), sigKill)
	}
}

func (s *Supervisor) stopRunningProcess() {
	state := s.getState()
	if state == StateStopped || state == StateFatal || state == StateCrashed {
		return
	}
	if state == StateBackoff {
		if s.backoffTimer != nil {
			s.backoffTimer.Stop()
			s.backoffTimer = nil
		}
		s.setState(StateStopped)
		return
	}

	s.setState(StateStopping)
	_ = killProcessGroup(int(s.pgid.Load()), sigTerm)

	select {
	case <-s.exitCh:
	case <-time.After(3 * time.Second):
		slog.Warn("Shutdown timeout: force killing", "process", s.config.Name)
		_ = killProcessGroup(int(s.pgid.Load()), sigKill)
		select {
		case <-s.exitCh:
		case <-time.After(1 * time.Second):
			slog.Error("Failed to exit command group even after SIGKILL", "process", s.config.Name)
		}
	}
}

func (s *Supervisor) Status() ProcessStatus {
	state := s.getState()
	pid := int(s.pid.Load())
	restartCount := int(s.restartCount.Load())

	var uptimeMs int64
	if state == StateRunning || state == StateStarting || state == StateStopping {
		startVal := s.uptimeStart.Load()
		if startVal != nil {
			start := startVal.(time.Time)
			if !start.IsZero() {
				uptimeMs = time.Since(start).Milliseconds()
			}
		}
	}

	var lastExitCode *int
	if codeVal := s.lastExitCode.Load(); codeVal != -1 {
		val := int(codeVal)
		lastExitCode = &val
	}

	var lastExitTime *time.Time
	if timeVal := s.lastExitTime.Load(); timeVal != nil {
		val := timeVal.(time.Time)
		if !val.IsZero() {
			lastExitTime = &val
		}
	}

	return ProcessStatus{
		Name:         s.config.Name,
		State:        string(state),
		PID:          pid,
		UptimeMs:     uptimeMs,
		RestartCount: restartCount,
		LastExitCode: lastExitCode,
		LastExitTime: lastExitTime,
	}
}

func (s *Supervisor) healthTimerChan() <-chan time.Time {
	if s.healthTimer == nil {
		return nil
	}
	return s.healthTimer.C
}

func (s *Supervisor) backoffTimerChan() <-chan time.Time {
	if s.backoffTimer == nil {
		return nil
	}
	return s.backoffTimer.C
}

func (s *Supervisor) graceTimerChan() <-chan time.Time {
	if s.graceTimer == nil {
		return nil
	}
	return s.graceTimer.C
}

type SafeWriter struct {
	w    io.Writer
	ch   chan []byte
	done chan struct{}
}

func NewSafeWriter(w io.Writer) *SafeWriter {
	sw := &SafeWriter{
		w:    w,
		ch:   make(chan []byte, 100),
		done: make(chan struct{}),
	}
	go sw.run()
	return sw
}

func (sw *SafeWriter) Write(p []byte) (int, error) {
	cpy := make([]byte, len(p))
	copy(cpy, p)
	select {
	case sw.ch <- cpy:
		return len(p), nil
	case <-time.After(2 * time.Second):
		return 0, errors.New("write timeout (queue full)")
	}
}

func (sw *SafeWriter) Close() {
	close(sw.done)
}

func (sw *SafeWriter) run() {
	for {
		select {
		case data := <-sw.ch:
			_, _ = sw.w.Write(data)
		case <-sw.done:
			for {
				select {
				case data := <-sw.ch:
					_, _ = sw.w.Write(data)
				default:
					return
				}
			}
		}
	}
}

func drainPipe(s *Supervisor, pipe io.ReadCloser, dest string, isStderr bool) {
	defer pipe.Close()

	var baseWriter io.Writer
	var fileToClose *os.File

	switch dest {
	case "inherit":
		if isStderr {
			baseWriter = os.Stderr
		} else {
			baseWriter = os.Stdout
		}
	case "discard":
		baseWriter = io.Discard
	default:
		f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			slog.Error("Failed to open output log file", "process", s.config.Name, "path", dest, "error", err)
			baseWriter = io.Discard
		} else {
			fileToClose = f
			baseWriter = f
		}
	}

	sw := NewSafeWriter(baseWriter)
	defer sw.Close()
	if fileToClose != nil {
		defer fileToClose.Close()
	}

	splitter := newLineSplitter(func(line []byte) {
		s.ringBuffer.WriteLine(line)
	})

	buf := make([]byte, 32*1024)
	for {
		n, err := pipe.Read(buf)
		if n > 0 {
			_, writeErr := sw.Write(buf[:n])
			if writeErr != nil {
				slog.Warn("Slow consumer warning: dropping output log bytes", "process", s.config.Name, "error", writeErr)
			}
			s.broadcaster.Send(buf[:n])
			splitter.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
