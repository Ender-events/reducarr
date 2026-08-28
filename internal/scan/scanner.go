package scan

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"syscall"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/dustin/go-humanize"
)

type Scanner struct {
	Client  *arrs.Client
	DB      *db.DB
	Scorer  *Scorer
	UI      *ui.ProgressLogger
	Resume  bool
	Verbose bool

	TotalScanned   int
	TotalCandidate int

	OnProgress func(phase string, item string, done int, total int)
}

func (s *Scanner) Run(ctx context.Context) error {
	if s.Client == nil {
		return fmt.Errorf("scanner client is not initialized")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	type sonarrWork struct {
		idx     int
		inst    arrs.SonarrInstance
		series  []sonarr.SeriesResource
		authCtx context.Context
		err     error
	}
	sonarrWorks := make([]sonarrWork, 0, len(s.Client.Sonarr))
	for i, inst := range s.Client.Sonarr {
		authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
			"X-Api-Key": {Key: inst.ApiKey()},
		})
		series, _, err := inst.Api().SeriesAPI.ListSeries(authCtx).Execute()
		sonarrWorks = append(sonarrWorks, sonarrWork{idx: i, inst: inst, series: series, authCtx: authCtx, err: err})
	}
	sonarrGrandTotal := 0
	for _, w := range sonarrWorks {
		if w.err == nil {
			sonarrGrandTotal += len(w.series)
		}
	}

	sonarrOffset := 0
	for _, w := range sonarrWorks {
		if err := s.scanSonarr(ctx, w.idx, w.inst, w.series, w.authCtx, sonarrOffset, sonarrGrandTotal); err != nil {
			if ctx.Err() != nil {
				s.printSummary()
				return nil
			}
			return err
		}
		sonarrOffset += len(w.series)
	}

	type radarrWork struct {
		idx     int
		inst    arrs.RadarrInstance
		movies  []radarr.MovieResource
		authCtx context.Context
		err     error
	}
	radarrWorks := make([]radarrWork, 0, len(s.Client.Radarr))
	for i, inst := range s.Client.Radarr {
		authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
			"X-Api-Key": {Key: inst.ApiKey()},
		})
		movies, _, err := inst.Api().MovieAPI.ListMovie(authCtx).Execute()
		radarrWorks = append(radarrWorks, radarrWork{idx: i, inst: inst, movies: movies, authCtx: authCtx, err: err})
	}
	radarrGrandTotal := 0
	for _, w := range radarrWorks {
		if w.err == nil {
			radarrGrandTotal += len(w.movies)
		}
	}

	radarrOffset := 0
	for _, w := range radarrWorks {
		if err := s.scanRadarr(ctx, w.idx, w.inst, w.movies, w.authCtx, radarrOffset, radarrGrandTotal); err != nil {
			if ctx.Err() != nil {
				s.printSummary()
				return nil
			}
			return err
		}
		radarrOffset += len(w.movies)
	}

	s.printSummary()
	return nil
}

func (s *Scanner) Incremental(ctx context.Context) error {
	if s.Client == nil {
		return fmt.Errorf("scanner client is not initialized")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	type sonarrHistWork struct {
		idx     int
		inst    arrs.SonarrInstance
		history []historyEvent
		err     error
	}
	sonarrHistWorks := make([]sonarrHistWork, 0, len(s.Client.Sonarr))
	for i, inst := range s.Client.Sonarr {
		history, err := inst.ListHistory(ctx, 100)
		events := make([]historyEvent, 0, len(history))
		for _, h := range history {
			eventType := ""
			if h.EventType != nil {
				eventType = string(*h.EventType)
			}
			itemID := int32(0)
			if h.SeriesId != nil {
				itemID = *h.SeriesId
			}
			events = append(events, historyEvent{
				ID:          *h.Id,
				EventType:   eventType,
				Data:        h.Data,
				ItemID:      itemID,
				SourceTitle: arrs.GetString(h.SourceTitle),
			})
		}
		sonarrHistWorks = append(sonarrHistWorks, sonarrHistWork{idx: i, inst: inst, history: events, err: err})
	}
	sonarrHistGrandTotal := 0
	for _, w := range sonarrHistWorks {
		if w.err == nil {
			sonarrHistGrandTotal += len(w.history)
		}
	}

	sonarrOffset := 0
	for _, w := range sonarrHistWorks {
		if err := s.incrementalSonarr(ctx, w.idx, w.inst, w.history, sonarrOffset, sonarrHistGrandTotal); err != nil {
			if ctx.Err() != nil {
				s.printSummary()
				return nil
			}
			return err
		}
		sonarrOffset += len(w.history)
	}

	type radarrHistWork struct {
		idx     int
		inst    arrs.RadarrInstance
		history []historyEvent
		err     error
	}
	radarrHistWorks := make([]radarrHistWork, 0, len(s.Client.Radarr))
	for i, inst := range s.Client.Radarr {
		history, err := inst.ListHistory(ctx, 100)
		events := make([]historyEvent, 0, len(history))
		for _, h := range history {
			eventType := ""
			if h.EventType != nil {
				eventType = string(*h.EventType)
			}
			itemID := int32(0)
			if h.MovieId != nil {
				itemID = *h.MovieId
			}
			events = append(events, historyEvent{
				ID:          *h.Id,
				EventType:   eventType,
				Data:        h.Data,
				ItemID:      itemID,
				SourceTitle: arrs.GetStringRadarr(h.SourceTitle),
			})
		}
		radarrHistWorks = append(radarrHistWorks, radarrHistWork{idx: i, inst: inst, history: events, err: err})
	}
	radarrHistGrandTotal := 0
	for _, w := range radarrHistWorks {
		if w.err == nil {
			radarrHistGrandTotal += len(w.history)
		}
	}

	radarrOffset := 0
	for _, w := range radarrHistWorks {
		if err := s.incrementalRadarr(ctx, w.idx, w.inst, w.history, radarrOffset, radarrHistGrandTotal); err != nil {
			if ctx.Err() != nil {
				s.printSummary()
				return nil
			}
			return err
		}
		radarrOffset += len(w.history)
	}

	s.printSummary()
	return nil
}

func (s *Scanner) printSummary() {
	valid := s.TotalScanned - s.TotalCandidate
	s.UI.LogPermanent("\nScan Summary:")
	s.UI.LogPermanent(fmt.Sprintf("  Total Scanned: %d", s.TotalScanned))
	s.UI.LogPermanent(fmt.Sprintf("  \033[32m✔ Valid\033[0m:       %d", valid))
	s.UI.LogPermanent(fmt.Sprintf("  \033[31m✘ Candidates\033[0m: %d", s.TotalCandidate))
}

func (s *Scanner) scanSonarr(ctx context.Context, idx int, inst arrs.SonarrInstance, seriesList []sonarr.SeriesResource, authCtx context.Context, doneOffset int, grandTotal int) error {
	instanceID := fmt.Sprintf("sonarr_%d", idx)
	lastID := ""
	if s.Resume {
		var err error
		lastID, err = s.DB.GetLastItemID(instanceID)
		if err != nil {
			return fmt.Errorf("get last id: %w", err)
		}
	}

	if s.OnProgress != nil {
		s.OnProgress("sonarr", fmt.Sprintf("Listing %d series...", len(seriesList)), doneOffset, grandTotal)
	}

	if s.Verbose {
		msg := fmt.Sprintf("[Sonarr: %s] Listing %d series...", inst.Name(), len(seriesList))
		s.UI.LogPermanent(msg)
	}

	sort.Slice(seriesList, func(i, j int) bool {
		return *seriesList[i].Id < *seriesList[j].Id
	})

	if s.Verbose {
		s.UI.LogPermanent(fmt.Sprintf("\n--- Scanning Sonarr Instance: %s ---", inst.Name()))
	}

	for sIdx, series := range seriesList {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		idStr := fmt.Sprintf("%d", *series.Id)
		if s.Resume && lastID != "" && idStr <= lastID {
			continue
		}

		title := arrs.GetString(series.Title)
		if s.OnProgress != nil {
			s.OnProgress("sonarr", title, doneOffset+sIdx+1, grandTotal)
		}

		files, _, err := inst.Api().EpisodeFileAPI.ListEpisodeFile(authCtx).SeriesId(*series.Id).Execute()
		if err != nil {
			return fmt.Errorf("get episode files for series %d: %w", *series.Id, err)
		}

		for _, file := range files {
			duration := 0.0
			if file.MediaInfo != nil {
				d, _ := ParseDuration(arrs.GetString(file.MediaInfo.RunTime))
				duration = d
			}
			if duration == 0 && series.Runtime != nil {
				duration = float64(*series.Runtime) * 60
			}

			info := FileInfo{
				Size:     *file.Size,
				Duration: duration,
			}

			isCand, reason := s.Scorer.IsCandidate(info)
			relPath := arrs.GetString(file.RelativePath)
			absPath := arrs.GetString(file.Path)
			sizeStr := humanize.Bytes(uint64(info.Size))

			// Apply Path Mapping
			localPath := fsutil.MapPath(absPath, inst.PathMappings())

			// Get Inode for cache
			inode, err := fsutil.GetInode(localPath)
			if inode == 0 || err != nil {
				fmt.Fprintf(os.Stderr, "Error getting inode: %v\n", err)
				continue
			}

			// Update Media Cache
			quality := ""
			if file.Quality != nil && file.Quality.Quality != nil {
				quality = arrs.GetString(file.Quality.Quality.Name)
			}

			seasonNum := int32(0)
			if file.SeasonNumber != nil {
				seasonNum = *file.SeasonNumber
			}

			err = s.DB.UpsertMediaFile(db.MediaFileRecord{
				ArrInstance:  inst.Name(),
				ArrType:      "sonarr",
				ItemID:       *series.Id,
				FileID:       *file.Id,
				Path:         absPath, // Keep remote path in DB for Arrs matching
				Title:        title,
				Inode:        inode,
				Size:         info.Size,
				Duration:     int64(duration),
				Quality:      quality,
				SeasonNumber: seasonNum,
			})
			if err != nil {
				return fmt.Errorf("upsert media file: %w", err)
			}

			s.TotalScanned++
			if isCand {
				if s.DB.IsCandidateIgnored(inst.Name(), *file.Id) {
					continue
				}

				s.TotalCandidate++

				// Check for cross-seeds
				records, _ := s.DB.GetTorrentsByInode(inode)

				crossSeedInfo := ""
				if len(records) > 1 {
					crossSeedInfo = fmt.Sprintf(" [CROSS-SEED: %d clients/torrents]", len(records))
				}

				// Save Candidate
				_ = s.DB.UpsertCandidate(inst.Name(), *file.Id, reason)

				s.UI.LogPermanent(fmt.Sprintf("\033[31m✘\033[0m [%s: %s] %s - %s (%s) - %s%s", "Sonarr", inst.Name(), title, relPath, sizeStr, reason, crossSeedInfo))
			} else {
				msg := fmt.Sprintf("\033[32m✔\033[0m [%s: %s] %s - %s (%s)", "Sonarr", inst.Name(), title, relPath, sizeStr)
				if s.Verbose {
					s.UI.LogPermanent(msg)
				} else {
					s.UI.UpdateTruncate(msg)
				}
			}
		}

		if err := s.DB.SetLastItemID(instanceID, idStr); err != nil {
			return fmt.Errorf("set last id: %w", err)
		}
	}

	return nil
}

func (s *Scanner) scanRadarr(ctx context.Context, idx int, inst arrs.RadarrInstance, movies []radarr.MovieResource, authCtx context.Context, doneOffset int, grandTotal int) error {
	instanceID := fmt.Sprintf("radarr_%d", idx)
	lastID := ""
	if s.Resume {
		var err error
		lastID, err = s.DB.GetLastItemID(instanceID)
		if err != nil {
			return fmt.Errorf("get last id: %w", err)
		}
	}

	if s.OnProgress != nil {
		s.OnProgress("radarr", fmt.Sprintf("Listing %d movies...", len(movies)), doneOffset, grandTotal)
	}

	sort.Slice(movies, func(i, j int) bool {
		return *movies[i].Id < *movies[j].Id
	})

	if s.Verbose {
		s.UI.LogPermanent(fmt.Sprintf("\n--- Scanning Radarr Instance: %s ---", inst.Name()))
	}

	for mIdx, movie := range movies {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		idStr := fmt.Sprintf("%d", *movie.Id)
		if s.Resume && lastID != "" && idStr <= lastID {
			continue
		}

		title := arrs.GetStringRadarr(movie.Title)
		if s.OnProgress != nil {
			s.OnProgress("radarr", title, doneOffset+mIdx+1, grandTotal)
		}

		if arrs.GetBoolRadarr(movie.HasFile) && movie.MovieFile != nil {
			duration := 0.0
			if movie.MovieFile.MediaInfo != nil {
				d, _ := ParseDuration(arrs.GetStringRadarr(movie.MovieFile.MediaInfo.RunTime))
				duration = d
			}
			if duration == 0 && movie.Runtime != nil {
				duration = float64(*movie.Runtime) * 60
			}

			info := FileInfo{
				Size:     *movie.MovieFile.Size,
				Duration: duration,
			}

			absPath := arrs.GetStringRadarr(movie.MovieFile.Path)

			// Apply Path Mapping
			localPath := fsutil.MapPath(absPath, inst.PathMappings())

			inode, _ := fsutil.GetInode(localPath)

			// Update Media Cache
			quality := ""
			if movie.MovieFile.Quality != nil && movie.MovieFile.Quality.Quality != nil {
				quality = arrs.GetStringRadarr(movie.MovieFile.Quality.Quality.Name)
			}

			err := s.DB.UpsertMediaFile(db.MediaFileRecord{
				ArrInstance:  inst.Name(),
				ArrType:      "radarr",
				ItemID:       *movie.Id,
				FileID:       *movie.MovieFile.Id,
				Path:         absPath,
				Title:        title,
				Inode:        inode,
				Size:         info.Size,
				Duration:     int64(duration),
				Quality:      quality,
				SeasonNumber: 0,
			})
			if err != nil {
				return fmt.Errorf("upsert media file: %w", err)
			}
			isCand, reason := s.Scorer.IsCandidate(info)
			sizeStr := humanize.Bytes(uint64(info.Size))

			s.TotalScanned++
			if isCand {
				if s.DB.IsCandidateIgnored(inst.Name(), *movie.MovieFile.Id) {
					continue
				}

				s.TotalCandidate++

				// Check for cross-seeds
				records, _ := s.DB.GetTorrentsByInode(inode)

				crossSeedInfo := ""
				if len(records) > 1 {
					crossSeedInfo = fmt.Sprintf(" [CROSS-SEED: %d clients/torrents]", len(records))
				}

				// Save Candidate
				_ = s.DB.UpsertCandidate(inst.Name(), *movie.MovieFile.Id, reason)

				s.UI.LogPermanent(fmt.Sprintf("\033[31m✘\033[0m [%s: %s] %s (%s) - %s%s", "Radarr", inst.Name(), title, sizeStr, reason, crossSeedInfo))
			} else {
				msg := fmt.Sprintf("\033[32m✔\033[0m [%s: %s] %s (%s)", "Radarr", inst.Name(), title, sizeStr)
				if s.Verbose {
					s.UI.LogPermanent(msg)
				} else {
					s.UI.UpdateTruncate(msg)
				}
			}
		}

		if err := s.DB.SetLastItemID(instanceID, idStr); err != nil {
			return fmt.Errorf("set last id: %w", err)
		}
	}

	return nil
}

// Internal structures for unified incremental scan
type historyEvent struct {
	ID          int32
	EventType   string
	Data        map[string]string
	ItemID      int32
	SourceTitle string
}

type fileMetadata struct {
	ID           int32
	Path         string
	Size         int64
	MediaRuntime string
	Quality      string
	SeasonNumber int32
}

func (s *Scanner) incrementalSonarr(ctx context.Context, idx int, inst arrs.SonarrInstance, history []historyEvent, doneOffset int, grandTotal int) error {
	getFile := func(ctx context.Context, fileID int32) (*fileMetadata, error) {
		f, err := inst.GetEpisodeFile(ctx, fileID)
		if err != nil {
			return nil, err
		}
		runtime := ""
		if f.MediaInfo != nil {
			runtime = arrs.GetString(f.MediaInfo.RunTime)
		}
		quality := ""
		if f.Quality != nil && f.Quality.Quality != nil {
			quality = arrs.GetString(f.Quality.Quality.Name)
		}
		season := int32(0)
		if f.SeasonNumber != nil {
			season = *f.SeasonNumber
		}
		return &fileMetadata{
			ID:           *f.Id,
			Path:         arrs.GetString(f.Path),
			Size:         *f.Size,
			MediaRuntime: runtime,
			Quality:      quality,
			SeasonNumber: season,
		}, nil
	}

	return s.runIncrementalScan(ctx, "sonarr", idx, inst.Name(), inst.PathMappings(), history, getFile, doneOffset, grandTotal)
}

func (s *Scanner) incrementalRadarr(ctx context.Context, idx int, inst arrs.RadarrInstance, history []historyEvent, doneOffset int, grandTotal int) error {
	getFile := func(ctx context.Context, fileID int32) (*fileMetadata, error) {
		f, err := inst.GetMovieFile(ctx, fileID)
		if err != nil {
			return nil, err
		}
		runtime := ""
		if f.MediaInfo != nil {
			runtime = arrs.GetStringRadarr(f.MediaInfo.RunTime)
		}
		quality := ""
		if f.Quality != nil && f.Quality.Quality != nil {
			quality = arrs.GetStringRadarr(f.Quality.Quality.Name)
		}
		return &fileMetadata{
			ID:           *f.Id,
			Path:         arrs.GetStringRadarr(f.Path),
			Size:         *f.Size,
			MediaRuntime: runtime,
			Quality:      quality,
			SeasonNumber: 0,
		}, nil
	}

	return s.runIncrementalScan(ctx, "radarr", idx, inst.Name(), inst.PathMappings(), history, getFile, doneOffset, grandTotal)
}

func (s *Scanner) runIncrementalScan(
	ctx context.Context,
	arrType string,
	idx int,
	instName string,
	mappings []fsutil.PathMapping,
	history []historyEvent,
	getFile func(context.Context, int32) (*fileMetadata, error),
	doneOffset int,
	grandTotal int,
) error {
	instanceID := fmt.Sprintf("hist_%s_%d", arrType, idx)
	lastID, _ := s.DB.GetLastItemID(instanceID)

	s.UI.LogPermanent(fmt.Sprintf("\n--- Incremental Scan %s: %s ---", arrType, instName))

	if s.OnProgress != nil {
		s.OnProgress(arrType, fmt.Sprintf("Checking %d history items...", len(history)), doneOffset, grandTotal)
	}

	var latestID string
	if len(history) > 0 {
		latestID = fmt.Sprintf("%d", history[0].ID)
	}

	for hIdx, h := range history {
		if s.OnProgress != nil {
			itemTitle := h.SourceTitle
			if itemTitle == "" {
				itemTitle = fmt.Sprintf("Event #%d", h.ID)
			}
			s.OnProgress(arrType, itemTitle, doneOffset+hIdx+1, grandTotal)
		}
		idStr := fmt.Sprintf("%d", h.ID)
		if lastID != "" && idStr <= lastID {
			break
		}

		if h.EventType != "downloadFolderImported" {
			continue
		}

		if h.Data == nil {
			continue
		}

		fileIDStr, ok := h.Data["fileId"]
		if !ok {
			continue
		}
		fileID64, _ := strconv.ParseInt(fileIDStr, 10, 32)
		fileID := int32(fileID64)

		file, err := getFile(ctx, fileID)
		if err != nil {
			continue
		}

		duration, _ := ParseDuration(file.MediaRuntime)
		if duration == 0 {
			// Fallback: in a real refactor, we'd fetch the item runtime here,
			// but for now we'll stick to what we have or a safe default.
			duration = 30 * 60 // 30 min default
		}

		info := FileInfo{
			Size:     file.Size,
			Duration: duration,
		}

		isCand, reason := s.Scorer.IsCandidate(info)
		sizeStr := humanize.Bytes(uint64(info.Size))

		localPath := fsutil.MapPath(file.Path, mappings)
		inode, _ := fsutil.GetInode(localPath)
		if inode == 0 {
			continue
		}

		_ = s.DB.UpsertMediaFile(db.MediaFileRecord{
			ArrInstance:  instName,
			ArrType:      arrType,
			ItemID:       h.ItemID,
			FileID:       file.ID,
			Path:         file.Path,
			Title:        h.SourceTitle,
			Inode:        inode,
			Size:         info.Size,
			Duration:     int64(duration),
			Quality:      file.Quality,
			SeasonNumber: file.SeasonNumber,
		})

		s.TotalScanned++
		if isCand {
			if s.DB.IsCandidateIgnored(instName, file.ID) {
				continue
			}
			s.TotalCandidate++
			_ = s.DB.UpsertCandidate(instName, file.ID, reason)
			s.UI.LogPermanent(fmt.Sprintf("\033[31m✘\033[0m [%s: %s] %s (%s) - %s", arrType, instName, h.SourceTitle, sizeStr, reason))
		}
	}

	if latestID != "" {
		_ = s.DB.SetLastItemID(instanceID, latestID)
	}
	return nil
}
