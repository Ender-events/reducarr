package orchestrator

import (
	"context"
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
	// 1. Fetch associated torrents by Inode
	torrents, err := o.db.GetTorrentsByInode(item.Inode)
	if err != nil {
		return fmt.Errorf("fetch associated torrents: %w", err)
	}

	// 2. Delete Torrents (without deleting files, Arr will handle that)
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
					return fmt.Errorf("login to client %s: %w", t.ClientName, err)
				}
				if err := tInst.DeleteTorrent(ctx, t.InfoHash, true); err != nil {
					return fmt.Errorf("delete torrent %s: %w", t.InfoHash, err)
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
				return fmt.Errorf("sonarr instance %s not found", item.ArrInstance)
			}
			if err := inst.DeleteEpisodeFile(ctx, item.FileID); err != nil {
				return fmt.Errorf("delete sonarr episode file: %w", err)
			}
		} else {
			inst := o.client.FindRadarr(item.ArrInstance)
			if inst == nil {
				return fmt.Errorf("radarr instance %s not found", item.ArrInstance)
			}
			if err := inst.DeleteMovieFile(ctx, item.FileID); err != nil {
				return fmt.Errorf("delete radarr movie file: %w", err)
			}
		}
	}

	// 4. Clean up Local DB
	if !o.dryRun {
		if err := o.db.DeleteMediaFile(item.ArrInstance, item.FileID); err != nil {
			return fmt.Errorf("delete media file from DB: %w", err)
		}
	}

	return nil
}

// UpgradeCandidate deletes the old candidate and triggers a grab for the new release.
func (o *Orchestrator) UpgradeCandidate(ctx context.Context, item db.CandidateRecord, release any) error {
	// 1. Delete old
	if o.verbose {
		fmt.Printf("Cleaning up current files and torrents for: %s...\n", item.Title)
	}
	if err := o.DeleteCandidate(ctx, item); err != nil {
		return fmt.Errorf("cleanup old candidate: %w", err)
	}

	// 2. Grab new
	if o.dryRun {
		fmt.Printf("  \033[33m[DRY-RUN]\033[0m Would grab new release\n")
		return nil
	}

	if item.ArrType == "sonarr" {
		inst := o.client.FindSonarr(item.ArrInstance)
		if inst == nil {
			return fmt.Errorf("sonarr instance %s not found", item.ArrInstance)
		}
		r := release.(*sonarr.ReleaseResource)
		if o.verbose {
			fmt.Printf("Grabbing release: %s...\n", arrs.GetString(r.Title))
		}
		if err := inst.DownloadRelease(ctx, r); err != nil {
			return fmt.Errorf("grab sonarr release: %w", err)
		}
	} else {
		inst := o.client.FindRadarr(item.ArrInstance)
		if inst == nil {
			return fmt.Errorf("radarr instance %s not found", item.ArrInstance)
		}
		r := release.(*radarr.ReleaseResource)
		if o.verbose {
			fmt.Printf("Grabbing release: %s...\n", arrs.GetStringRadarr(r.Title))
		}
		if err := inst.DownloadRelease(ctx, r); err != nil {
			return fmt.Errorf("grab radarr release: %w", err)
		}
	}

	return nil
}
