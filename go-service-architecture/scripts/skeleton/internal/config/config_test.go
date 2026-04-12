package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// No config file, no env vars — should succeed with defaults.
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := K.String("bind"); got != "" {
		t.Errorf("default bind = %q, want empty string", got)
	}
}

func TestLoadConfigFromYAML(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config", "notifier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("port: 9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := K.Int("port"); got != 9090 {
		t.Errorf("port = %d, want 9090", got)
	}
}

func TestLoadConfigEnvOverridesYAML(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config", "notifier")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configFile := filepath.Join(configDir, "config.yaml")
	if err := os.WriteFile(configFile, []byte("port: 9090\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))
	t.Setenv("NOTIFIER_PORT", "3000")

	if err := Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if got := K.Int("port"); got != 3000 {
		t.Errorf("port = %d, want 3000 (env should override YAML)", got)
	}
}

func TestInitDirsCreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	if err := InitDirs(); err != nil {
		t.Fatalf("InitDirs() error: %v", err)
	}

	configDir := filepath.Join(tmp, "config", "notifier")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Errorf("config dir not created: %s", configDir)
	}
	stateDir := filepath.Join(tmp, "state", "notifier")
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Errorf("state dir not created: %s", stateDir)
	}
}

func TestConfigPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))

	got := ConfigPath()
	want := filepath.Join(tmp, "config", "notifier", "config.yaml")
	if got != want {
		t.Errorf("ConfigPath() = %q, want %q", got, want)
	}
}

func TestStatePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state"))

	got := StatePath()
	want := filepath.Join(tmp, "state", "notifier")
	if got != want {
		t.Errorf("StatePath() = %q, want %q", got, want)
	}
}
