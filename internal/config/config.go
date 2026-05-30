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

type PathMapping struct {
	Remote string `mapstructure:"remote"`
	Local  string `mapstructure:"local"`
}

type QBittorrentConfig struct {
	Name         string        `mapstructure:"name"`
	URL          string        `mapstructure:"url"`
	Username     string        `mapstructure:"username"`
	Password     string        `mapstructure:"password"`
	PathMappings []PathMapping `mapstructure:"pathMappings"`
}

type ScoringConfig struct {
	MaxSize    string `mapstructure:"maxSize"`
	MaxRatio   string `mapstructure:"maxRatio"`
	MaxBitrate string `mapstructure:"maxBitrate"`
}

type Config struct {
	Sonarr      []ArrInstance       `mapstructure:"sonarr"`
	Radarr      []ArrInstance       `mapstructure:"radarr"`
	QBittorrent []QBittorrentConfig `mapstructure:"qbittorrent"`
	Scoring     ScoringConfig       `mapstructure:"scoring"`
	RateLimit   int                 `mapstructure:"rateLimit"` // searches per hour
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
	viper.SetDefault("scoring.maxSize", "")
	viper.SetDefault("scoring.maxRatio", "100MiB/min")
	viper.SetDefault("scoring.maxBitrate", "")

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
