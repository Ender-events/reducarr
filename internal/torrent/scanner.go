package torrent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/autobrr/go-qbittorrent"
)

type Scanner struct {
	Client   *arrs.Client
	DB       *db.DB
	UI       *ui.ProgressLogger
	Mappings map[string][]fsutil.PathMapping // client name -> mappings
	Verbose  bool

	TotalTorrents int
	TotalClients  int
}

func NewScanner(client *arrs.Client, database *db.DB, logger *ui.ProgressLogger, mappings map[string][]fsutil.PathMapping) *Scanner {
	return &Scanner{
		Client:   client,
		DB:       database,
		UI:       logger,
		Mappings: mappings,
	}
}

func (s *Scanner) ScanAll(ctx context.Context) error {
	s.TotalTorrents = 0
	s.TotalClients = 0

	for _, t := range s.Client.Torrents {
		s.TotalClients++
		if err := s.ScanClient(ctx, t); err != nil {
			return fmt.Errorf("scan client %s: %w", t.Name(), err)
		}
	}

	s.printSummary()
	return nil
}

func (s *Scanner) printSummary() {
	s.UI.LogPermanent("\nTorrent Scan Summary:")
	s.UI.LogPermanent(fmt.Sprintf("  Total Clients:  %d", s.TotalClients))
	s.UI.LogPermanent(fmt.Sprintf("  Total Torrents: %d", s.TotalTorrents))
}

func (s *Scanner) ScanClient(ctx context.Context, inst arrs.TorrentInstance) error {
	if s.Verbose {
		s.UI.LogPermanent(fmt.Sprintf("\n--- Scanning Torrent Client: %s ---", inst.Name()))
	} else {
		s.UI.UpdateTruncate(fmt.Sprintf("Fetching torrents from %s...", inst.Name()))
	}

	torrents, err := inst.Api().GetTorrentsCtx(ctx, qbittorrent.TorrentFilterOptions{})
	if err != nil {
		return err
	}

	for _, t := range torrents {
		s.TotalTorrents++
		msg := fmt.Sprintf("[%s] %s (%s)", inst.Name(), t.Name, t.Hash)
		if s.Verbose {
			s.UI.LogPermanent(msg)
		} else {
			s.UI.UpdateTruncate(fmt.Sprintf("Scanning torrent: %s", msg))
		}

		// Fetch files for this torrent
		files, err := inst.GetFiles(ctx, t.Hash)
		if err != nil {
			// If we fail to get files, at least map the content path
			if s.Verbose {
				s.UI.LogPermanent(fmt.Sprintf("Failed to get files for torrent %s: %v", t.Name, err))
			}
			files = []qbittorrent.TorrentFile{{Name: ""}}
		}

		for _, f := range files {
			relPath := f.Name
			fullRemotePath := t.ContentPath
			if relPath != "" {
				fullRemotePath = filepath.Join(t.SavePath, relPath)
			}

			localPath := fsutil.MapPath(fullRemotePath, inst.PathMappings())
			inode, err := fsutil.GetInode(localPath)
			if inode == 0 || err != nil {
				fmt.Fprintf(os.Stderr, "Error getting inode: %v\n", err)
				continue
			}

			isSeeding := t.State == "uploading" || t.State == "stalledUP" || t.State == "forcedUP" || t.State == "queuedUP"

			_, err = s.DB.Exec(`
				INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
				ON CONFLICT(client_name, info_hash, file_path) DO UPDATE SET
					inode = excluded.inode,
					is_seeding = excluded.is_seeding,
					added_at = excluded.added_at,
					updated_at = excluded.updated_at
			`, inst.Name(), t.Hash, fullRemotePath, inode, isSeeding, t.AddedOn)
			if err != nil {
				return fmt.Errorf("insert torrent file: %w", err)
			}
		}
	}

	if !s.Verbose {
		s.UI.LogPermanent(fmt.Sprintf("\033[32m✔\033[0m Scanned %d torrents from %s", len(torrents), inst.Name()))
	}
	return nil
}
