package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type ArrInstance struct {
	Name   string `mapstructure:"name"`
	URL    string `mapstructure:"url"`
	APIKey string `mapstructure:"apiKey"`
}

type QBittorrentConfig struct {
	URL      string `mapstructure:"url"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type ScoringConfig struct {
	MaxRatio float64 `mapstructure:"maxRatio"` // bytes per second
}

type Config struct {
	Sonarr      []ArrInstance     `mapstructure:"sonarr"`
	Radarr      []ArrInstance     `mapstructure:"radarr"`
	QBittorrent QBittorrentConfig `mapstructure:"qbittorrent"`
	Scoring     ScoringConfig     `mapstructure:"scoring"`
	RateLimit   int               `mapstructure:"rateLimit"` // searches per hour
}

func LoadConfig() (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.SetEnvPrefix("REDUCARR")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Defaults
	viper.SetDefault("rateLimit", 50)
	viper.SetDefault("scoring.maxRatio", 1024*1024) // 1MiB/s default? Need to refine this later.

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
