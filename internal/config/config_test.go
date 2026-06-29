package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_DefaultsAndExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "ward.toml")

	tomlContent := `
[settings]
socket_path = "/tmp/ward_test.sock"

[[process]]
name = "test-process"
command = "go"
args = ["version"]
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Settings.SocketPath != "/tmp/ward_test.sock" {
		t.Errorf("expected SocketPath /tmp/ward_test.sock, got %q", cfg.Settings.SocketPath)
	}
	home, _ := os.UserHomeDir()
	expectedDBPath := filepath.Join(home, ".local/share/ward/ward.db")
	if cfg.Settings.DBPath != expectedDBPath {
		t.Errorf("expected DBPath %q, got %q", expectedDBPath, cfg.Settings.DBPath)
	}
	if cfg.Settings.PIDPath != "/tmp/ward.pid" {
		t.Errorf("expected PIDPath /tmp/ward.pid, got %q", cfg.Settings.PIDPath)
	}

	if len(cfg.Process) != 1 {
		t.Fatalf("expected 1 process config, got %d", len(cfg.Process))
	}
	p := cfg.Process[0]
	if p.Name != "test-process" {
		t.Errorf("expected process name test-process, got %q", p.Name)
	}
	if p.Restart != "always" {
		t.Errorf("expected default restart policy 'always', got %q", p.Restart)
	}
	if p.RestartDelay != "5s" {
		t.Errorf("expected default restart_delay '5s', got %q", p.RestartDelay)
	}
	if p.GracePeriod != "10s" {
		t.Errorf("expected default grace_period '10s', got %q", p.GracePeriod)
	}
	if p.Stdout != "inherit" {
		t.Errorf("expected default stdout 'inherit', got %q", p.Stdout)
	}
	if p.Stderr != "inherit" {
		t.Errorf("expected default stderr 'inherit', got %q", p.Stderr)
	}
	if p.Autostart == nil || !*p.Autostart {
		t.Errorf("expected default autostart to be true")
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		wantErr     bool
	}{
		{
			name: "Valid simple config",
			tomlContent: `
[[process]]
name = "api"
command = "go"
`,
			wantErr: false,
		},
		{
			name: "Invalid process name characters",
			tomlContent: `
[[process]]
name = "api process"
command = "go"
`,
			wantErr: true,
		},
		{
			name: "Duplicate process names",
			tomlContent: `
[[process]]
name = "api"
command = "go"

[[process]]
name = "api"
command = "go"
`,
			wantErr: true,
		},
		{
			name: "Missing command",
			tomlContent: `
[[process]]
name = "api"
`,
			wantErr: true,
		},
		{
			name: "Non-existent command in path",
			tomlContent: `
[[process]]
name = "api"
command = "non_existent_binary_name_abc_123"
`,
			wantErr: true,
		},
		{
			name: "Invalid restart policy",
			tomlContent: `
[[process]]
name = "api"
command = "go"
restart = "sometimes"
`,
			wantErr: true,
		},
		{
			name: "Invalid restart delay duration",
			tomlContent: `
[[process]]
name = "api"
command = "go"
restart_delay = "invalid-delay"
`,
			wantErr: true,
		},
		{
			name: "Invalid grace period duration",
			tomlContent: `
[[process]]
name = "api"
command = "go"
grace_period = "10years"
`,
			wantErr: true,
		},
		{
			name: "Negative max restarts",
			tomlContent: `
[[process]]
name = "api"
command = "go"
max_restarts = -1
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "ward.toml")
			if err := os.WriteFile(configPath, []byte(tt.tomlContent), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			_, err := LoadConfig(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadConfig() error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("skipping test; home directory not available")
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~", home},
		{"~/sub/dir", filepath.Join(home, "sub/dir")},
	}

	for _, tt := range tests {
		got := expandPath(tt.input)
		if got != tt.expected {
			t.Errorf("expandPath(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConfig_EnvironmentExpansion(t *testing.T) {
	os.Setenv("TEST_SOCKET_PATH", "/tmp/ward_test.sock")
	os.Setenv("TEST_APP_DIR", "/var/tmp")
	defer func() {
		os.Unsetenv("TEST_SOCKET_PATH")
		os.Unsetenv("TEST_APP_DIR")
	}()

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "ward_env.toml")

	tomlContent := `
[settings]
socket_path = "$TEST_SOCKET_PATH"

[[process]]
name = "api"
command = "go"
args = ["version"]
dir = "$TEST_APP_DIR"
stdout = "$TEST_APP_DIR/stdout.log"
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Settings.SocketPath != "/tmp/ward_test.sock" {
		t.Errorf("expected expanded socket path '/tmp/ward_test.sock', got %q", cfg.Settings.SocketPath)
	}

	p := cfg.Process[0]
	if p.Dir != "/var/tmp" {
		t.Errorf("expected expanded dir '/var/tmp', got %q", p.Dir)
	}
	if p.Stdout != "/var/tmp/stdout.log" {
		t.Errorf("expected expanded stdout '/var/tmp/stdout.log', got %q", p.Stdout)
	}
}

func TestConfig_DependencyCycles(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		wantErrMsg  string
	}{
		{
			name: "self dependency",
			tomlContent: `
[settings]
socket_path = "/tmp/ward.sock"

[[process]]
name = "a"
command = "go"
args = ["version"]
depends_on = ["a"]
`,
			wantErrMsg: "cannot depend on itself",
		},
		{
			name: "direct cycle",
			tomlContent: `
[settings]
socket_path = "/tmp/ward.sock"

[[process]]
name = "a"
command = "go"
args = ["version"]
depends_on = ["b"]

[[process]]
name = "b"
command = "go"
args = ["version"]
depends_on = ["a"]
`,
			wantErrMsg: "cyclic dependency detected",
		},
		{
			name: "missing dependency",
			tomlContent: `
[settings]
socket_path = "/tmp/ward.sock"

[[process]]
name = "a"
command = "go"
args = ["version"]
depends_on = ["c"]
`,
			wantErrMsg: "does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "ward_dep.toml")
			_ = os.WriteFile(configPath, []byte(tt.tomlContent), 0644)

			_, err := LoadConfig(configPath)
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("expected error containing %q, got %q", tt.wantErrMsg, err.Error())
			}
		})
	}
}

