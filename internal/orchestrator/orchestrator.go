package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
)

type Orchestrator struct {
	db      *db.DB
	client  *arrs.Client
	dryRun  bool
	verbose bool
}

func New(database *db.DB, client *arrs.Client, dryRun, verbose bool) *Orchestrator {
	return &Orchestrator{
		db:      database,
		client:  client,
		dryRun:  dryRun,
		verbose: verbose,
	}
}

// DeleteCandidate removes the candidate file from the Arr instance,
// all associated torrents from the torrent clients, and cleans up the local database.
func (o *Orchestrator) DeleteCandidate(ctx context.Context, item db.CandidateRecord) error {
	report, err := o.deleteCandidateInternal(ctx, item, "DELETE")
	if err != nil {
		return err
	}
	if !o.dryRun {
		_ = o.db.InsertReport(report)
	}
	return nil
}

// addWarning adds a warning message to the report and sets status to WARNING if there are warnings
func (o *Orchestrator) addWarning(report *db.ReportRecord, warning string) {
	report.WarningMessages = append(report.WarningMessages, warning)
	if report.Status == "SUCCESS" {
		report.Status = "WARNING"
	}
}

func (o *Orchestrator) deleteCandidateInternal(ctx context.Context, item db.CandidateRecord, actionType string) (db.ReportRecord, error) {
	report := db.ReportRecord{
		ActionType:      actionType,
		ArrInstance:     item.ArrInstance,
		ArrType:         item.ArrType,
		ItemTitle:       item.Title,
		MainFileID:      item.FileID,
		MainFilePath:    item.Path,
		Status:          "SUCCESS",
		WarningMessages: []string{},
	}

	// 1. Fetch associated torrents and ALL files they contain
	torrents, err := o.db.GetTorrentsByInode(item.Inode)
	if err != nil {
		return report, o.failReport(report, fmt.Errorf("fetch associated torrents: %w", err))
	}

	var deletedTorrents []map[string]string
	affectedFiles := make(map[string]db.MediaFileRecord)

	for _, t := range torrents {
		deletedTorrents = append(deletedTorrents, map[string]string{
			"info_hash":   t.InfoHash,
			"client_name": t.ClientName,
		})

		// Find all files in this torrent
		allTorrentFiles, _ := o.db.GetTorrentsByHash(t.InfoHash)
		for _, tf := range allTorrentFiles {
			m, _ := o.db.GetMediaFileByInode(tf.Inode)
			if m != nil {
				affectedFiles[m.Path] = *m
			}
		}
	}

	var deletedFilesList []map[string]any
	var totalSize int64
	for path, m := range affectedFiles {
		deletedFilesList = append(deletedFilesList, map[string]any{
			"path":    path,
			"file_id": m.FileID,
			"size":    m.Size,
			"inode":   m.Inode,
		})
		totalSize += m.Size
	}

	report.TotalSizeBefore = totalSize
	dtJSON, _ := json.Marshal(deletedTorrents)
	dfJSON, _ := json.Marshal(deletedFilesList)
	report.DeletedTorrents = string(dtJSON)
	report.DeletedFiles = string(dfJSON)

	// 2. Delete Torrents
	for _, t := range torrents {
		for _, tInst := range o.client.Torrents {
			if tInst.Name() == t.ClientName {
				if tInst.IsReadOnly() {
					// Read-Only mode: Get files from DB (which has full remote paths), remove from client (entry only), then delete manually
					allTorrentFiles, err := o.db.GetTorrentsByHash(t.InfoHash)
					var filesToDelete []string
					if err != nil {
						fmt.Fprintf(os.Stderr, "\033[31m✘\033[0m Warning: could not get file list from database for torrent %s: %v\n", t.InfoHash, err)
					} else {
						for _, tf := range allTorrentFiles {
							filesToDelete = append(filesToDelete, fsutil.MapPath(tf.FilePath, tInst.PathMappings()))
						}
					}

					if o.dryRun {
						fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would remove torrent from client %s (metadata only): %s\n", t.ClientName, t.InfoHash[:8])
						fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would manually delete %d files on disk:\n", len(filesToDelete))
						for _, p := range filesToDelete {
							fmt.Printf("  \033[33m[DRY-RUN]\033[0m   - %s\n", p)
						}
					} else {
						if o.verbose {
							fmt.Printf("  Removing torrent entry from client %s: %s...\n", t.ClientName, t.InfoHash[:8])
						}
						if err := tInst.DeleteTorrent(ctx, t.InfoHash, false); err != nil {
							return report, o.failReport(report, fmt.Errorf("delete torrent entry %s: %w", t.InfoHash, err))
						}

						if o.verbose {
							fmt.Printf("  Manually deleting %d files for torrent %s...\n", len(filesToDelete), t.InfoHash[:8])
						}
						for _, p := range filesToDelete {
							if err := os.Remove(p); err != nil {
								if !os.IsNotExist(err) {
									warningMsg := fmt.Sprintf("failed to manually delete %s: %v", p, err)
									o.addWarning(&report, warningMsg)
									if o.verbose {
										fmt.Printf("\033[33m⚠\033[0m %s\n", warningMsg)
									}
								}
							}
						}
					}
				} else {
					// Standard mode: Let the client delete everything
					if o.dryRun {
						fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would remove torrent and files from client %s: %s\n", t.ClientName, t.InfoHash[:8])
					} else {
						if o.verbose {
							fmt.Printf("  Removing torrent and files from client %s: %s...\n", t.ClientName, t.InfoHash[:8])
						}
						if err := tInst.DeleteTorrent(ctx, t.InfoHash, true); err != nil {
							return report, o.failReport(report, fmt.Errorf("delete torrent and files %s: %w", t.InfoHash, err))
						}
					}
				}

				if !o.dryRun {
					if err := o.db.DeleteTorrentByHash(t.ClientName, t.InfoHash); err != nil {
						warningMsg := fmt.Sprintf("failed to delete torrent %s from DB: %v", t.InfoHash[:8], err)
						o.addWarning(&report, warningMsg)
						if o.verbose {
							fmt.Printf("\033[33m⚠\033[0m %s\n", warningMsg)
						}
					}
				}
			}
		}
	}

	// 3. Delete from Arr
	if o.dryRun {
		fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would delete file (internal %s id %d) from instance %s\n", item.ArrType, item.FileID, item.ArrInstance)
	} else {
		if o.verbose {
			fmt.Printf("  Deleting file (internal %s id %d) from instance: %s\n", item.ArrType, item.FileID, item.ArrInstance)
		}
		if item.ArrType == "sonarr" {
			inst := o.client.FindSonarr(item.ArrInstance)
			if inst == nil {
				return report, o.failReport(report, fmt.Errorf("sonarr instance %s not found", item.ArrInstance))
			}
			if err := inst.DeleteEpisodeFile(ctx, item.FileID); err != nil {
				if strings.Contains(err.Error(), "404 Not Found") {
					o.addWarning(&report, fmt.Sprintf("sonarr episode file not found (already deleted?): %d", item.FileID))
				} else {
					return report, o.failReport(report, fmt.Errorf("delete sonarr episode file: %w", err))
				}
			}
		} else {
			inst := o.client.FindRadarr(item.ArrInstance)
			if inst == nil {
				return report, o.failReport(report, fmt.Errorf("radarr instance %s not found", item.ArrInstance))
			}
			if err := inst.DeleteMovieFile(ctx, item.FileID); err != nil {
				if strings.Contains(err.Error(), "404 Not Found") {
					o.addWarning(&report, fmt.Sprintf("radarr movie file not found (already deleted?): %d", item.FileID))
				} else {
					return report, o.failReport(report, fmt.Errorf("delete radarr movie file: %w", err))
				}
			}
		}
	}

	// 4. Clean up Local DB
	if !o.dryRun {
		// Clean up all affected files from our DB
		for _, m := range affectedFiles {
			if err := o.db.DeleteMediaFile(m.ArrInstance, m.FileID); err != nil {
				warningMsg := fmt.Sprintf("failed to delete file %d from DB: %v", m.FileID, err)
				o.addWarning(&report, warningMsg)
				if o.verbose {
					fmt.Printf("\033[33m⚠\033[0m %s\n", warningMsg)
				}
			}
		}
	}

	return report, nil
}

func (o *Orchestrator) failReport(r db.ReportRecord, err error) error {
	r.Status = "FAILED"
	r.ErrorMessage = err.Error()
	if !o.dryRun {
		_ = o.db.InsertReport(r)
	}
	return err
}

// UpgradeCandidate deletes the old candidate and triggers a grab for the new release.
func (o *Orchestrator) UpgradeCandidate(ctx context.Context, item db.CandidateRecord, release any) error {
	// 1. Delete old
	if o.verbose {
		fmt.Printf("Cleaning up current files and torrents for: %s...\n", item.Title)
	}
	report, err := o.deleteCandidateInternal(ctx, item, "UPGRADE")
	if err != nil {
		return err
	}

	// 2. Grab new
	if o.dryRun {
		fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would grab new release\n")
		return nil
	}

	if item.ArrType == "sonarr" {
		inst := o.client.FindSonarr(item.ArrInstance)
		if inst == nil {
			return o.failReport(report, fmt.Errorf("sonarr instance %s not found", item.ArrInstance))
		}
		r := release.(*sonarr.ReleaseResource)
		report.NewReleaseTitle = arrs.GetString(r.Title)
		report.NewIndexer = arrs.GetString(r.Indexer)
		if r.Size != nil {
			report.TotalSizeAfter = *r.Size
		}

		if o.verbose {
			fmt.Printf("Grabbing release: %s...\n", report.NewReleaseTitle)
		}
		if err := inst.DownloadRelease(ctx, r); err != nil {
			return o.failReport(report, fmt.Errorf("grab sonarr release: %w", err))
		}
	} else {
		inst := o.client.FindRadarr(item.ArrInstance)
		if inst == nil {
			return o.failReport(report, fmt.Errorf("radarr instance %s not found", item.ArrInstance))
		}
		r := release.(*radarr.ReleaseResource)
		report.NewReleaseTitle = arrs.GetStringRadarr(r.Title)
		report.NewIndexer = arrs.GetStringRadarr(r.Indexer)
		if r.Size != nil {
			report.TotalSizeAfter = *r.Size
		}

		if o.verbose {
			fmt.Printf("Grabbing release: %s...\n", report.NewReleaseTitle)
		}
		if err := inst.DownloadRelease(ctx, r); err != nil {
			return o.failReport(report, fmt.Errorf("grab radarr release: %w", err))
		}
	}

	_ = o.db.InsertReport(report)
	return nil
}
