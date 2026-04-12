package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

const serviceName = "notifier"

// K is the global koanf instance. It is loaded once during startup
// (in PersistentPreRunE) and read during request handling. It is not
// safe for concurrent Load calls, but concurrent Get calls after a
// single Load are safe.
var K = koanf.New(".")

// Load reads configuration from the YAML config file and environment
// variables. A missing config file is not an error. Environment
// variables override file values. The prefix NOTIFIER_ is stripped,
// the remainder is lowercased, and underscores become dots.
func Load() error {
	// Reset for testability — allows calling Load() multiple times
	// in tests without accumulating state from prior calls.
	K = koanf.New(".")

	// Load from YAML file (missing file is ok).
	_ = K.Load(file.Provider(ConfigPath()), yaml.Parser())

	// Load from environment — strip prefix, lowercase, replace _ with .
	if err := K.Load(env.Provider("NOTIFIER_", ".", func(s string) string {
		return strings.ReplaceAll(
			strings.ToLower(strings.TrimPrefix(s, "NOTIFIER_")),
			"_", ".",
		)
	}), nil); err != nil {
		return fmt.Errorf("load env config: %w", err)
	}
	return nil
}

// ConfigPath returns the path to the YAML config file:
// $XDG_CONFIG_HOME/notifier/config.yaml
func ConfigPath() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, serviceName, "config.yaml")
}

// StatePath returns the XDG state directory for runtime data:
// $XDG_STATE_HOME/notifier/
func StatePath() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, serviceName)
}

// InitDirs creates the XDG config and state directories if they do
// not exist.
func InitDirs() error {
	dirs := []string{
		filepath.Dir(ConfigPath()),
		StatePath(),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", d, err)
		}
	}
	return nil
}
