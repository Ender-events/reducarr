package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/pkg/arrs"
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

func (o *Orchestrator) deleteCandidateInternal(ctx context.Context, item db.CandidateRecord, actionType string) (db.ReportRecord, error) {
	report := db.ReportRecord{
		ActionType:   actionType,
		ArrInstance:  item.ArrInstance,
		ArrType:      item.ArrType,
		ItemTitle:    item.Title,
		MainFileID:   item.FileID,
		MainFilePath: item.Path,
		Status:       "SUCCESS",
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
				if o.dryRun {
					fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would remove torrent: %s (Client: %s)\n", t.InfoHash[:8], t.ClientName)
					continue
				}

				if o.verbose {
					fmt.Printf("  Removing torrent: %s (Client: %s)...\n", t.InfoHash[:8], t.ClientName)
				}
				if err := tInst.Api().LoginCtx(ctx); err != nil {
					return report, o.failReport(report, fmt.Errorf("login to client %s: %w", t.ClientName, err))
				}
				if err := tInst.DeleteTorrent(ctx, t.InfoHash, true); err != nil {
					return report, o.failReport(report, fmt.Errorf("delete torrent %s: %w", t.InfoHash, err))
				}

				if err := o.db.DeleteTorrentByHash(t.ClientName, t.InfoHash); err != nil {
					fmt.Printf("\033[31m✘\033[0m Warning: failed to delete torrent %s from DB: %v\n", t.InfoHash, err)
				}
			}
		}
	}

	// 3. Delete from Arr
	if o.dryRun {
		fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would delete %s file %d from instance %s\n", item.ArrType, item.FileID, item.ArrInstance)
	} else {
		if o.verbose {
			fmt.Printf("  Deleting file %d from %s (Instance: %s)...\n", item.FileID, item.ArrType, item.ArrInstance)
		}
		if item.ArrType == "sonarr" {
			inst := o.client.FindSonarr(item.ArrInstance)
			if inst == nil {
				return report, o.failReport(report, fmt.Errorf("sonarr instance %s not found", item.ArrInstance))
			}
			if err := inst.DeleteEpisodeFile(ctx, item.FileID); err != nil {
				return report, o.failReport(report, fmt.Errorf("delete sonarr episode file: %w", err))
			}
		} else {
			inst := o.client.FindRadarr(item.ArrInstance)
			if inst == nil {
				return report, o.failReport(report, fmt.Errorf("radarr instance %s not found", item.ArrInstance))
			}
			if err := inst.DeleteMovieFile(ctx, item.FileID); err != nil {
				return report, o.failReport(report, fmt.Errorf("delete radarr movie file: %w", err))
			}
		}
	}

	// 4. Clean up Local DB
	if !o.dryRun {
		// Clean up all affected files from our DB
		for _, m := range affectedFiles {
			if err := o.db.DeleteMediaFile(m.ArrInstance, m.FileID); err != nil {
				fmt.Printf("\033[31m✘\033[0m Warning: failed to delete file %d from DB: %v\n", m.FileID, err)
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
