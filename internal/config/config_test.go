package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadConfig_Defaults(t *testing.T) {
	viper.Reset()
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.WebUI.EnableTroubleshooting != false {
		t.Errorf("expected WebUI.EnableTroubleshooting to default to false, got %v", cfg.WebUI.EnableTroubleshooting)
	}
}

func TestLoadConfig_EnableTroubleshooting(t *testing.T) {
	viper.Reset()
	tmpDir := t.TempDir()
	configContent := `
webui:
  pageSize: 50
  enableTroubleshooting: true
`
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd error: %v", err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("os.Chdir error: %v", err)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if !cfg.WebUI.EnableTroubleshooting {
		t.Errorf("expected WebUI.EnableTroubleshooting to be true, got false")
	}
}
