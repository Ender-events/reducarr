package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/scan"
	"github.com/Ender-events/reducarr/internal/torrent"
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
	incremental     bool
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
		defer db.Close(database)
		client, err := arrs.GetClient(context.Background(), cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting client: %v\n", err)
			os.Exit(1)
		}

		// Acquire scan lock
		acquired, err := database.AcquireScanLock("global_scan", os.Getpid(), "cli", 3600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error acquiring scan lock: %v\n", err)
			os.Exit(1)
		}
		if !acquired {
			fmt.Fprintf(os.Stderr, "Error: A scan is already in progress (acquired by another CLI process or background scheduler).\n")
			os.Exit(1)
		}
		defer func() {
			_ = database.ReleaseScanLock("global_scan")
		}()

		uiLogger := ui.NewProgressLogger()

		// 4. Refresh Torrent Cache (Mandatory for cross-seed/deletion awareness)
		tScanner := torrent.NewScanner(client, database, uiLogger, nil)
		tScanner.Verbose = verbose
		tScanner.Incremental = incremental
		if err := tScanner.ScanAll(context.Background()); err != nil {
			fmt.Printf("Warning: torrent scan failed: %v\n", err)
		}

		// 5. Setup Scanner
		scanner := &scan.Scanner{
			Client:  client,
			DB:      database,
			Scorer:  scorer,
			UI:      uiLogger,
			Resume:  resume,
			Verbose: verbose,
		}

		ctx := context.Background()
		if incremental {
			err = scanner.Incremental(ctx)
		} else {
			err = scanner.Run(ctx)
		}

		if err != nil {
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
	scanCmd.Flags().BoolVarP(&resume, "resume", "r", false, "Resume scanning from the last saved checkpoint")
	scanCmd.Flags().BoolVarP(&incremental, "incremental", "n", false, "Scan only recently added files (incremental)")
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
