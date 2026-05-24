package main

import (
	"fmt"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "reducarr",
	Short: "reducarr is an automated media optimization tool",
	Long:  `reducarr monitors Sonarr and Radarr for oversized files and replaces them with more efficient versions.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.LoadConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
}
