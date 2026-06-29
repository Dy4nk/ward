package process

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"ward/internal/config"
	"ward/internal/store"
)

type Manager struct {
	store       *store.Store
	supervisors map[string]*Supervisor
	mu          sync.RWMutex
	wg          sync.WaitGroup
}

func NewManager(cfg *config.Config, s *store.Store, ctx context.Context) *Manager {
	m := &Manager{
		store:       s,
		supervisors: make(map[string]*Supervisor),
	}

	depCheck := func(name string) bool {
		m.mu.RLock()
		depSup, ok := m.supervisors[name]
		m.mu.RUnlock()
		if !ok {
			return false
		}
		return depSup.getState() == StateRunning
	}

	m.mu.Lock()
	for _, p := range cfg.Process {
		sup := NewSupervisor(p, s, depCheck)
		m.supervisors[p.Name] = sup
	}
	m.mu.Unlock()

	m.mu.RLock()
	for _, sup := range m.supervisors {
		m.wg.Add(1)
		go func(s *Supervisor) {
			defer m.wg.Done()
			s.Run(ctx)
		}(sup)
	}
	m.mu.RUnlock()

	return m
}

func (m *Manager) Start(name string) error {
	m.mu.RLock()
	sup, ok := m.supervisors[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process %q not found", name)
	}
	return sup.Start()
}

func (m *Manager) Stop(name string) error {
	m.mu.RLock()
	sup, ok := m.supervisors[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process %q not found", name)
	}
	return sup.Stop()
}

func (m *Manager) Restart(name string) error {
	m.mu.RLock()
	sup, ok := m.supervisors[name]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("process %q not found", name)
	}
	return sup.Restart()
}

func (m *Manager) StopAll(timeout time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var wg sync.WaitGroup
	for _, sup := range m.supervisors {
		wg.Add(1)
		go func(s *Supervisor) {
			defer wg.Done()
			_ = s.Stop()
		}(sup)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("StopAll timed out after %v", timeout)
	}
}

func (m *Manager) Status(name string) (ProcessStatus, error) {
	m.mu.RLock()
	sup, ok := m.supervisors[name]
	m.mu.RUnlock()

	if !ok {
		return ProcessStatus{}, fmt.Errorf("process %q not found", name)
	}

	return sup.Status(), nil
}

func (m *Manager) List() []ProcessStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	list := make([]ProcessStatus, 0, len(m.supervisors))
	for _, sup := range m.supervisors {
		list = append(list, sup.Status())
	}
	return list
}

func (m *Manager) Reload(newCfg *config.Config, ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newProcesses := make(map[string]config.ProcessConfig)
	for _, p := range newCfg.Process {
		newProcesses[p.Name] = p
	}

	for name, sup := range m.supervisors {
		newP, ok := newProcesses[name]
		if !ok {
			slog.Info("Removing process from supervisor list", "process", name)
			go func(s *Supervisor) {
				_ = s.Stop()
			}(sup)
			delete(m.supervisors, name)
		} else {
			if executionContextChanged(sup.config, newP) {
				slog.Info("Execution context changed, restarting process", "process", name)
				go func(s *Supervisor, cfg config.ProcessConfig) {
					resp := make(chan error, 1)
					s.controlCh <- ControlMessage{Action: "update", Config: cfg, Resp: resp}
					_ = <-resp
					_ = s.Restart()
				}(sup, newP)
			} else {
				slog.Info("Updating parameters for process", "process", name)
				go func(s *Supervisor, cfg config.ProcessConfig) {
					resp := make(chan error, 1)
					s.controlCh <- ControlMessage{Action: "update", Config: cfg, Resp: resp}
					_ = <-resp
				}(sup, newP)
			}
		}
	}

	depCheck := func(name string) bool {
		m.mu.RLock()
		depSup, ok := m.supervisors[name]
		m.mu.RUnlock()
		if !ok {
			return false
		}
		return depSup.getState() == StateRunning
	}

	for name, newP := range newProcesses {
		if _, ok := m.supervisors[name]; !ok {
			slog.Info("Adding new process to supervisor list", "process", name)
			sup := NewSupervisor(newP, m.store, depCheck)
			m.supervisors[name] = sup
			m.wg.Add(1)
			go func(s *Supervisor) {
				defer m.wg.Done()
				s.Run(ctx)
			}(sup)
		}
	}

	return nil
}

func executionContextChanged(old, new config.ProcessConfig) bool {
	if old.Command != new.Command {
		return true
	}
	if len(old.Args) != len(new.Args) {
		return true
	}
	for i := range old.Args {
		if old.Args[i] != new.Args[i] {
			return true
		}
	}
	if old.Dir != new.Dir {
		return true
	}
	if len(old.Env) != len(new.Env) {
		return true
	}
	for k, v := range old.Env {
		if new.Env[k] != v {
			return true
		}
	}
	if old.Stdout != new.Stdout {
		return true
	}
	if old.Stderr != new.Stderr {
		return true
	}
	return false
}

func (m *Manager) GetLogs(name string, lines int) ([]byte, error) {
	m.mu.RLock()
	sup, ok := m.supervisors[name]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("process %q not found", name)
	}

	return sup.RingBuffer().ReadJoined(lines), nil
}

func (m *Manager) GetBroadcaster(name string) (*Broadcaster, error) {
	m.mu.RLock()
	sup, ok := m.supervisors[name]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("process %q not found", name)
	}

	return sup.Broadcaster(), nil
}

func (m *Manager) Wait() {
	m.wg.Wait()
}
