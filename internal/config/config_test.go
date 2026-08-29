package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Ender-events/reducarr/internal/db"
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
	if cfg.WebUI.RadarrTargetSize != "4GB" {
		t.Errorf("expected WebUI.RadarrTargetSize to default to '4GB', got %v", cfg.WebUI.RadarrTargetSize)
	}
	if cfg.WebUI.SonarrTargetSize != "2GB" {
		t.Errorf("expected WebUI.SonarrTargetSize to default to '2GB', got %v", cfg.WebUI.SonarrTargetSize)
	}
	if got := cfg.WebUI.GetRadarrTargetSizeBytes(); got != db.DefaultRadarrTargetSize && got != 4000000000 {
		t.Errorf("unexpected GetRadarrTargetSizeBytes: %v", got)
	}
	if got := cfg.WebUI.GetSonarrTargetSizeBytes(); got != db.DefaultSonarrTargetSize && got != 2000000000 {
		t.Errorf("unexpected GetSonarrTargetSizeBytes: %v", got)
	}
}

func TestWebUIConfig_TargetSizeBytes(t *testing.T) {
	// When empty, defaults to db.DefaultRadarrTargetSize and db.DefaultSonarrTargetSize
	emptyCfg := WebUIConfig{}
	if emptyCfg.GetRadarrTargetSizeBytes() != db.DefaultRadarrTargetSize {
		t.Errorf("expected %d, got %d", db.DefaultRadarrTargetSize, emptyCfg.GetRadarrTargetSizeBytes())
	}
	if emptyCfg.GetSonarrTargetSizeBytes() != db.DefaultSonarrTargetSize {
		t.Errorf("expected %d, got %d", db.DefaultSonarrTargetSize, emptyCfg.GetSonarrTargetSizeBytes())
	}

	// Custom valid strings
	customCfg := WebUIConfig{
		RadarrTargetSize: "5GB",
		SonarrTargetSize: "1.5GB",
	}
	if customCfg.GetRadarrTargetSizeBytes() != 5000000000 {
		t.Errorf("expected 5000000000, got %d", customCfg.GetRadarrTargetSizeBytes())
	}
	if customCfg.GetSonarrTargetSizeBytes() != 1500000000 {
		t.Errorf("expected 1500000000, got %d", customCfg.GetSonarrTargetSizeBytes())
	}

	// Invalid strings fallback to defaults
	invalidCfg := WebUIConfig{
		RadarrTargetSize: "invalid",
		SonarrTargetSize: "invalid",
	}
	if invalidCfg.GetRadarrTargetSizeBytes() != db.DefaultRadarrTargetSize {
		t.Errorf("expected %d, got %d", db.DefaultRadarrTargetSize, invalidCfg.GetRadarrTargetSizeBytes())
	}
	if invalidCfg.GetSonarrTargetSizeBytes() != db.DefaultSonarrTargetSize {
		t.Errorf("expected %d, got %d", db.DefaultSonarrTargetSize, invalidCfg.GetSonarrTargetSizeBytes())
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
