package main

import (
	"fmt"

	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/spf13/cobra"
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan libraries for oversized files",
	Run: func(cmd *cobra.Command, args []string) {
		sonarrInstances := make([]arrs.ArrInstance, len(cfg.Sonarr))
		for i, s := range cfg.Sonarr {
			sonarrInstances[i] = arrs.ArrInstance{Name: s.Name, URL: s.URL, APIKey: s.APIKey}
		}

		radarrInstances := make([]arrs.ArrInstance, len(cfg.Radarr))
		for i, r := range cfg.Radarr {
			radarrInstances[i] = arrs.ArrInstance{Name: r.Name, URL: r.URL, APIKey: r.APIKey}
		}

		qbitCfg := &arrs.QBitConfig{
			URL:      cfg.QBittorrent.URL,
			Username: cfg.QBittorrent.Username,
			Password: cfg.QBittorrent.Password,
		}

		_ = arrs.NewClient(sonarrInstances, radarrInstances, qbitCfg)
		
		fmt.Println("Scanning libraries...")
		// Placeholder for actual scan logic
		fmt.Println("Scan complete (placeholder).")
	},
}

func init() {
	rootCmd.AddCommand(scanCmd)
}
