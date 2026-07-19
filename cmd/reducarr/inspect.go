package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/scan"
	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect [PATH|INODE]",
	Short: "Deep inspection of a media file across all systems",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		input := args[0]
		database, err := db.Open("reducarr.db")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close(database)

		var media *db.MediaFileRecord

		// 1. Try to parse as Inode
		if inode, err := strconv.ParseUint(input, 10, 64); err == nil {
			media, err = database.GetMediaFileByInode(inode)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error querying DB by inode: %v\n", err)
				os.Exit(1)
			}
		}

		// 2. If not found or not an inode, try as path
		if media == nil {
			// Resolve absolute path if it exists on disk
			if _, err := os.Stat(input); err == nil {
				// Get inode from FS if possible to be more robust
				inode, _ := fsutil.GetInode(input)
				if inode > 0 {
					media, _ = database.GetMediaFileByInode(inode)
				}
			}

			if media == nil {
				media, err = database.GetMediaFileByPath(input)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error querying DB by path: %v\n", err)
					os.Exit(1)
				}
			}
		}

		if media == nil {
			fmt.Printf("File '%s' not found in media cache. Have you run 'reducarr scan'?\n", input)
			os.Exit(1)
		}

		// 3. Fetch torrents associated with this inode
		torrents, _ := database.GetTorrentsByInode(media.Inode)

		// 4. Setup Scorer to show current status
		scorer := &scan.Scorer{}
		// Load limits from config
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

		info := scan.FileInfo{
			Size:     media.Size,
			Duration: float64(media.Duration),
		}
		isCand, reason := scorer.IsCandidate(info)

		// 5. Display Report
		fmt.Printf("\n--- Inspection Report ---\n")
		fmt.Printf("Path:    %s\n", media.Path)
		fmt.Printf("Inode:   %d\n", media.Inode)
		fmt.Printf("Size:    %s\n", humanize.Bytes(uint64(media.Size)))
		fmt.Printf("Quality: %s\n", media.Quality)

		if media.Duration > 0 {
			minutes := float64(media.Duration) / 60
			mib := float64(media.Size) / (1024 * 1024)
			fmt.Printf("Ratio:   %.2f MiB/min\n", mib/minutes)
			bitrate := int64(float64(media.Size*8) / float64(media.Duration))
			fmt.Printf("Bitrate: %s\n", humanize.SIWithDigits(float64(bitrate), 2, "bps"))
		}

		fmt.Printf("\n[Arrs Association]\n")
		fmt.Printf("  Instance: %s (%s)\n", media.ArrInstance, media.ArrType)
		fmt.Printf("  Item ID:  %d (File ID: %d)\n", media.ItemID, media.FileID)

		fmt.Printf("\n[Torrent Association]\n")
		if len(torrents) == 0 {
			fmt.Println("  No active torrents found in cache. Run 'reducarr torrent scan'?")
		} else {
			// Deduplicate by hash to avoid showing same torrent multiple times
			seenHashes := make(map[string]bool)
			for _, t := range torrents {
				if seenHashes[t.InfoHash] {
					continue
				}
				seenHashes[t.InfoHash] = true

				status := "Seeding"
				if !t.IsSeeding {
					status = "Other"
				}
				addedStr := "unknown"
				if t.AddedAt > 0 {
					addedStr = time.Unix(t.AddedAt, 0).Format("2006-01-02 15:04")
				}
				fmt.Printf("  - [%s] %s (%s) - Added: %s\n", status, t.ClientName, t.InfoHash, addedStr)

				// Fetch all files for this torrent
				allFiles, _ := database.GetTorrentsByHash(t.InfoHash)
				if len(allFiles) > 0 {
					fmt.Printf("    Files in torrent:\n")
					for _, tf := range allFiles {
						// Check if this file is in media cache to check thresholds
						m, err := database.GetMediaFileByInode(tf.Inode)

						if err != nil {
							fmt.Fprintf(os.Stderr, "  - inode %d: %v\n", tf.Inode, err)
							continue
						}

						if m == nil {
							continue
						}

						fInfo := scan.FileInfo{
							Size:     m.Size,
							Duration: float64(m.Duration),
						}
						isCandidate, _ := scorer.IsCandidate(fInfo)
						var inodeStr string
						if isCandidate {
							inodeStr = fmt.Sprintf("\033[31m%d\033[0m", tf.Inode)
						} else {
							inodeStr = fmt.Sprintf("\033[32m%d\033[0m", tf.Inode)
						}

						fmt.Printf("    - [%s] %s\n", inodeStr, tf.FilePath)
					}
				}
			}
		}

		fmt.Printf("\n[Optimization Status]\n")
		if isCand {
			fmt.Printf("  \033[31m✘ CANDIDATE\033[0m: %s\n", reason)
		} else {
			fmt.Printf("  \033[32m✔ VALID\033[0m: Within all thresholds.\n")
		}
		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}
