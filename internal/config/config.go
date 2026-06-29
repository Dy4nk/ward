package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var nameRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)

type Settings struct {
	SocketPath string `toml:"socket_path"`
	DBPath     string `toml:"db_path"`
	PIDPath    string `toml:"pid_path"`
}

type ProcessConfig struct {
	Name         string            `toml:"name"`
	Command      string            `toml:"command"`
	Args         []string          `toml:"args"`
	Dir          string            `toml:"dir"`
	Env          map[string]string `toml:"env"`
	Restart      string            `toml:"restart"`
	RestartDelay string            `toml:"restart_delay"`
	MaxRestarts  int               `toml:"max_restarts"`
	GracePeriod  string            `toml:"grace_period"`
	Stdout       string            `toml:"stdout"`
	Stderr       string            `toml:"stderr"`
	Autostart    *bool             `toml:"autostart"`
	DependsOn    []string          `toml:"depends_on"`
}

type Config struct {
	Settings Settings        `toml:"settings"`
	Process  []ProcessConfig `toml:"process"`
}

func LoadConfig(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	cfg.applyDefaults()
	cfg.expandPaths()

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Settings.SocketPath == "" {
		c.Settings.SocketPath = "/tmp/ward.sock"
	}
	if c.Settings.DBPath == "" {
		c.Settings.DBPath = "~/.local/share/ward/ward.db"
	}
	if c.Settings.PIDPath == "" {
		c.Settings.PIDPath = "/tmp/ward.pid"
	}

	for i := range c.Process {
		p := &c.Process[i]
		if p.Restart == "" {
			p.Restart = "always"
		}
		if p.RestartDelay == "" {
			p.RestartDelay = "5s"
		}
		if p.GracePeriod == "" {
			p.GracePeriod = "10s"
		}
		if p.Stdout == "" {
			p.Stdout = "inherit"
		}
		if p.Stderr == "" {
			p.Stderr = "inherit"
		}
		if p.Autostart == nil {
			autostartVal := true
			p.Autostart = &autostartVal
		}
	}
}

func (c *Config) expandPaths() {
	c.Settings.SocketPath = os.ExpandEnv(expandPath(c.Settings.SocketPath))
	c.Settings.DBPath = os.ExpandEnv(expandPath(c.Settings.DBPath))
	c.Settings.PIDPath = os.ExpandEnv(expandPath(c.Settings.PIDPath))

	for i := range c.Process {
		p := &c.Process[i]
		p.Dir = os.ExpandEnv(expandPath(p.Dir))
		if p.Stdout != "inherit" && p.Stdout != "discard" {
			p.Stdout = os.ExpandEnv(expandPath(p.Stdout))
		}
		if p.Stderr != "inherit" && p.Stderr != "discard" {
			p.Stderr = os.ExpandEnv(expandPath(p.Stderr))
		}
	}
}

func expandPath(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
	} else if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func (c *Config) Validate() error {
	names := make(map[string]bool)

	for i, p := range c.Process {
		if p.Name == "" {
			return fmt.Errorf("process[%d]: name cannot be empty", i)
		}
		if !nameRegex.MatchString(p.Name) {
			return fmt.Errorf("process %q: name must match [a-z0-9_-]+", p.Name)
		}
		if names[p.Name] {
			return fmt.Errorf("process %q: name must be unique", p.Name)
		}
		names[p.Name] = true

		if err := validateCommand(p.Command, p.Dir); err != nil {
			return fmt.Errorf("process %q: %w", p.Name, err)
		}

		switch p.Restart {
		case "always", "on-failure", "never":
			// valid
		default:
			return fmt.Errorf("process %q: invalid restart policy %q", p.Name, p.Restart)
		}

		if _, err := time.ParseDuration(p.RestartDelay); err != nil {
			return fmt.Errorf("process %q: invalid restart_delay %q: %w", p.Name, p.RestartDelay, err)
		}

		if _, err := time.ParseDuration(p.GracePeriod); err != nil {
			return fmt.Errorf("process %q: invalid grace_period %q: %w", p.Name, p.GracePeriod, err)
		}

		if p.MaxRestarts < 0 {
			return fmt.Errorf("process %q: max_restarts must be >= 0", p.Name)
		}
	}

	adj := make(map[string][]string)
	for _, p := range c.Process {
		adj[p.Name] = p.DependsOn
	}

	for _, p := range c.Process {
		for _, dep := range p.DependsOn {
			if !names[dep] {
				return fmt.Errorf("process %q: depends_on process %q, but %q does not exist", p.Name, dep, dep)
			}
			if dep == p.Name {
				return fmt.Errorf("process %q: cannot depend on itself", p.Name)
			}
		}

		visited := make(map[string]int)
		var dfs func(node string) error
		dfs = func(node string) error {
			visited[node] = 1
			for _, neighbor := range adj[node] {
				if visited[neighbor] == 1 {
					return fmt.Errorf("cyclic dependency detected involving process %q", neighbor)
				}
				if visited[neighbor] == 0 {
					if err := dfs(neighbor); err != nil {
						return err
					}
				}
			}
			visited[node] = 2
			return nil
		}

		if err := dfs(p.Name); err != nil {
			return fmt.Errorf("process %q: %w", p.Name, err)
		}
	}

	return nil
}

func validateCommand(cmd string, dir string) error {
	if cmd == "" {
		return errors.New("command cannot be empty")
	}

	if strings.Contains(cmd, string(filepath.Separator)) {
		path := cmd
		if !filepath.IsAbs(path) && dir != "" {
			path = filepath.Join(dir, path)
		}
		_, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("command path %q not found: %w", path, err)
		}
		return nil
	}

	_, err := exec.LookPath(cmd)
	if err != nil {
		return fmt.Errorf("command %q not found in PATH: %w", cmd, err)
	}
	return nil
}
