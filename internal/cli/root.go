package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

var (
	cfgPath    string
	socketPath string
)

var RootCmd = &cobra.Command{
	Use:   "ward",
	Short: "Ward guards process lifecycles",
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	home, _ := os.UserHomeDir()
	defaultCfg := filepath.Join(home, ".config", "ward", "ward.toml")

	RootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", defaultCfg, "path to config file")
	RootCmd.PersistentFlags().StringVarP(&socketPath, "socket", "s", "", "override socket path")
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

func resolveSocketPath() (string, error) {
	cPath := expandPath(cfgPath)
	if socketPath != "" {
		return expandPath(socketPath), nil
	}

	type miniSettings struct {
		SocketPath string `toml:"socket_path"`
	}
	type miniConfig struct {
		Settings miniSettings `toml:"settings"`
	}

	home, _ := os.UserHomeDir()
	defaultCfg := filepath.Join(home, ".config", "ward", "ward.toml")
	isDefault := cPath == defaultCfg

	data, err := os.ReadFile(cPath)
	if err != nil {
		if isDefault {
			return "/tmp/ward.sock", nil
		}
		return "", fmt.Errorf("failed to read config file %q: %w", cPath, err)
	}

	var mCfg miniConfig
	if _, err := toml.Decode(string(data), &mCfg); err != nil {
		if isDefault {
			return "/tmp/ward.sock", nil
		}
		return "", fmt.Errorf("failed to parse config file %q: %w", cPath, err)
	}

	if mCfg.Settings.SocketPath != "" {
		return expandPath(mCfg.Settings.SocketPath), nil
	}

	return "/tmp/ward.sock", nil
}
