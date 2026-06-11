package main

import (
	"fmt"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
	verbose bool
	dryRun  bool
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

		// If dry-run flag exists on this command and was NOT explicitly set, use the config value
		f := cmd.Flags().Lookup("dry-run")
		if f != nil && !f.Changed {
			dryRun = cfg.DryRun
		} else if f == nil {
			// If flag doesn't exist (read-only command), always use config or default
			dryRun = cfg.DryRun
		}

		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
}
