package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check connectivity to Arrs and Torrent Client",
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

		client := arrs.NewClient(sonarrInstances, radarrInstances, qbitCfg)
		results := client.HealthCheck(context.Background())

		hasError := false
		for _, res := range results {
			status := "HEALTHY"
			if !res.Healthy {
				status = "FAILED"
				hasError = true
			}
			fmt.Printf("[%s] %-15s: %s", status, res.Type, res.Name)
			if res.Error != nil {
				fmt.Printf(" (Error: %v)", res.Error)
			}
			fmt.Println()
		}

		if hasError {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
