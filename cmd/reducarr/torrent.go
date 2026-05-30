package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/torrent"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/spf13/cobra"
)

var torrentCmd = &cobra.Command{
	Use:   "torrent",
	Short: "Manage torrent clients and cache",
}

var torrentScanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan torrent clients and update the local inode cache",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		qbitConfigs := make([]arrs.QBitConfig, len(cfg.QBittorrent))
		mappings := make(map[string][]fsutil.PathMapping)
		for i, q := range cfg.QBittorrent {
			qbitConfigs[i] = arrs.QBitConfig{
				Name:         q.Name,
				URL:          q.URL,
				Username:     q.Username,
				Password:     q.Password,
				PathMappings: q.PathMappings,
			}
			mappings[q.Name] = q.PathMappings
		}

		client := arrs.NewClient(nil, nil, qbitConfigs)
		scanner := torrent.NewScanner(client, database, ui.NewProgressLogger(), mappings)
		scanner.Verbose = verbose
		if err := scanner.ScanAll(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Torrent scan failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Torrent scan complete.")
	},
}

var torrentListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all torrents in the local cache",
	Run: func(cmd *cobra.Command, args []string) {
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		records, err := database.GetAllTorrents()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching torrents: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("%-15s %-40s %s\n", "CLIENT", "HASH", "PATH")
		for _, r := range records {
			fmt.Printf("%-15s %-40s %s\n", r.ClientName, r.InfoHash, r.FilePath)
		}
	},
}

var torrentCheckCmd = &cobra.Command{
	Use:   "check [file]",
	Short: "Check a local file for hardlinks and torrent associations",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		inode, err := fsutil.GetInode(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting inode: %v\n", err)
			os.Exit(1)
		}

		if inode == 0 {
			fmt.Println("Inode is 0 (unsupported on this OS or file not found).")
			return
		}

		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		records, err := database.GetTorrentsByInode(inode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error querying DB: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("File:  %s\n", filePath)
		fmt.Printf("Inode: %d\n", inode)
		fmt.Printf("Associations found: %d\n", len(records))

		for _, r := range records {
			status := "Seeding"
			if !r.IsSeeding {
				status = "Other"
			}
			fmt.Printf("  - [%s] %s (%s)\n", status, r.ClientName, r.InfoHash)
		}
	},
}

func init() {
	torrentCmd.AddCommand(torrentScanCmd)
	torrentCmd.AddCommand(torrentListCmd)
	torrentCmd.AddCommand(torrentCheckCmd)
	rootCmd.AddCommand(torrentCmd)
}
