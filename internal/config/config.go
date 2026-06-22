package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/spf13/viper"
)

type ArrInstance struct {
	Name         string               `mapstructure:"name"`
	URL          string               `mapstructure:"url"`
	APIKey       string               `mapstructure:"apiKey"`
	PathMappings []fsutil.PathMapping `mapstructure:"pathMappings"`
}

type QBittorrentConfig struct {
	Name         string               `mapstructure:"name"`
	URL          string               `mapstructure:"url"`
	Username     string               `mapstructure:"username"`
	Password     string               `mapstructure:"password"`
	PathMappings []fsutil.PathMapping `mapstructure:"pathMappings"`
	ReadOnly     bool                 `mapstructure:"readOnly"`
}

type ScoringConfig struct {
	MaxSize         string `mapstructure:"maxSize"`
	MaxRatio        string `mapstructure:"maxRatio"`
	MaxBitrate      string `mapstructure:"maxBitrate"`
	MinSeedDuration string `mapstructure:"minSeedDuration"`
}

type AutomationConfig struct {
	Schedule         string `mapstructure:"schedule"`
	AutoUpgrade      bool   `mapstructure:"autoUpgrade"`
	MinSizeReduction string `mapstructure:"minSizeReduction"`
	MinSeeders       int32  `mapstructure:"minSeeders"`
	RateLimit        int    `mapstructure:"rateLimit"` // searches per hour
}

type Config struct {
	Sonarr      []ArrInstance       `mapstructure:"sonarr"`
	Radarr      []ArrInstance       `mapstructure:"radarr"`
	QBittorrent []QBittorrentConfig `mapstructure:"qbittorrent"`
	Scoring     ScoringConfig       `mapstructure:"scoring"`
	Automation  AutomationConfig    `mapstructure:"automation"`
	DryRun      bool                `mapstructure:"dryRun"`
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.SetEnvPrefix("REDUCARR")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults
	viper.SetDefault("dryRun", false)
	viper.SetDefault("scoring.maxSize", "")
	viper.SetDefault("scoring.maxRatio", "100MiB/min")
	viper.SetDefault("scoring.maxBitrate", "")
	viper.SetDefault("scoring.minSeedDuration", "336h") // 2 weeks
	viper.SetDefault("automation.schedule", "")
	viper.SetDefault("automation.autoUpgrade", false)
	viper.SetDefault("automation.minSizeReduction", "10%")
	viper.SetDefault("automation.minSeeders", 3)
	viper.SetDefault("automation.rateLimit", 6)

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}

func GetConfigPath() string {
	return "config.yaml"
}

func GetConfigContent() (string, error) {
	content, err := os.ReadFile(GetConfigPath())
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func SaveConfigContent(content string) error {
	oldCfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
	}

	// Write new config to file
	if err := os.WriteFile(GetConfigPath(), []byte(content), 0644); err != nil {
		return err
	}

	// Load new config to compare
	newCfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load new config after save: %v\n", err)
		return nil
	}

	// Notify subscribers about config changes
	NotifyConfigChanged(oldCfg, newCfg)

	return nil
}
