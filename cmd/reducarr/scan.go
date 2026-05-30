package main

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/scan"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var (
	maxSize         string
	maxRatio        string
	maxBitrate      string
	resume          bool
	targetInstances []string
)

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan libraries for oversized files",
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Setup Scorer
		scorer := &scan.Scorer{}

		// Load from config first
		if cfg.Scoring.MaxSize != "" {
			val, _ := humanize.ParseBytes(cfg.Scoring.MaxSize)
			scorer.MaxSize = val
		}
		if cfg.Scoring.MaxRatio != "" {
			val, _ := scan.ParseRatio(cfg.Scoring.MaxRatio)
			scorer.MaxRatio = val
		}
		if cfg.Scoring.MaxBitrate != "" {
			val, _ := scan.ParseBitrate(cfg.Scoring.MaxBitrate)
			scorer.MaxBitrate = val
		}

		// Overwrite with flags if provided
		if maxSize != "" {
			bytes, err := humanize.ParseBytes(maxSize)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing max-size: %v\n", err)
				os.Exit(1)
			}
			scorer.MaxSize = bytes
		}

		if maxRatio != "" {
			ratio, err := scan.ParseRatio(maxRatio)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing max-ratio: %v\n", err)
				os.Exit(1)
			}
			scorer.MaxRatio = ratio
		}

		if maxBitrate != "" {
			bitrate, err := scan.ParseBitrate(maxBitrate)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing max-bitrate: %v\n", err)
				os.Exit(1)
			}
			scorer.MaxBitrate = bitrate
		}

		// 2. Setup DB
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		// 3. Setup Client
		var sonarrInstances []arrs.ArrInstance
		for _, s := range cfg.Sonarr {
			if len(targetInstances) > 0 && !slices.Contains(targetInstances, s.Name) {
				continue
			}
			sonarrInstances = append(sonarrInstances, arrs.ArrInstance{
				Name:         s.Name,
				URL:          s.URL,
				APIKey:       s.APIKey,
				PathMappings: s.PathMappings,
			})
		}

		var radarrInstances []arrs.ArrInstance
		for _, r := range cfg.Radarr {
			if len(targetInstances) > 0 && !slices.Contains(targetInstances, r.Name) {
				continue
			}
			radarrInstances = append(radarrInstances, arrs.ArrInstance{
				Name:         r.Name,
				URL:          r.URL,
				APIKey:       r.APIKey,
				PathMappings: r.PathMappings,
			})
		}

		qbitConfigs := make([]arrs.QBitConfig, len(cfg.QBittorrent))
		for i, q := range cfg.QBittorrent {
			qbitConfigs[i] = arrs.QBitConfig{
				Name:         q.Name,
				URL:          q.URL,
				Username: q.Username,
				Password: q.Password,
				PathMappings: q.PathMappings,
			}
		}

		client := arrs.NewClient(sonarrInstances, radarrInstances, qbitConfigs)

		// 4. Setup Scanner
		scanner := &scan.Scanner{
			Client:  client,
			DB:      database,
			Scorer:  scorer,
			UI:      ui.NewProgressLogger(),
			Resume:  resume,
			Verbose: verbose,
		}

		if err := scanner.Run(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Scan failed: %v\n", err)
			os.Exit(1)
		}

		scanner.UI.Done()
		fmt.Println("Scan complete.")
	},
}

func init() {
	scanCmd.Flags().StringVar(&maxSize, "max-size", "", "Maximum allowed file size (e.g., 10GB)")
	scanCmd.Flags().StringVar(&maxRatio, "max-ratio", "", "Maximum allowed Size/Duration ratio (e.g., 100MiB/min)")
	scanCmd.Flags().StringVar(&maxBitrate, "max-bitrate", "", "Maximum allowed bitrate (e.g., 10Mbit)")
	scanCmd.Flags().BoolVar(&resume, "resume", false, "Resume scanning from the last saved checkpoint")
	scanCmd.Flags().StringSliceVarP(&targetInstances, "instance", "i", []string{}, "Target specific instances to scan")

	_ = scanCmd.RegisterFlagCompletionFunc("instance", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := config.LoadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}

		var names []string
		for _, s := range cfg.Sonarr {
			names = append(names, s.Name)
		}
		for _, r := range cfg.Radarr {
			names = append(names, r.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	rootCmd.AddCommand(scanCmd)
}
