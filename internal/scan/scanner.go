package scan

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
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
}

func (s *Scanner) Run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	for i, instance := range s.Client.Sonarr {
		if err := s.scanSonarr(ctx, i, instance); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}

	for i, instance := range s.Client.Radarr {
		if err := s.scanRadarr(ctx, i, instance); err != nil {
			if ctx.Err() != nil {
				s.printSummary()
				return nil
			}
			return err
		}
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

func getString(n sonarr.NullableString) string {
	if n.Get() == nil {
		return ""
	}
	return *n.Get()
}

func getStringRadarr(n radarr.NullableString) string {
	if n.Get() == nil {
		return ""
	}
	return *n.Get()
}

func getBoolRadarr(n radarr.NullableBool) bool {
	if n.Get() == nil {
		return false
	}
	return *n.Get()
}

func (s *Scanner) scanSonarr(ctx context.Context, idx int, inst arrs.SonarrInstance) error {
	instanceID := fmt.Sprintf("sonarr_%d", idx)
	lastID := ""
	if s.Resume {
		var err error
		lastID, err = s.DB.GetLastItemID(instanceID)
		if err != nil {
			return fmt.Errorf("get last id: %w", err)
		}
	}

	authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
		"X-Api-Key": {Key: inst.ApiKey()},
	})

	seriesList, _, err := inst.Api().SeriesAPI.ListSeries(authCtx).Execute()
	if err != nil {
		return fmt.Errorf("get series: %w", err)
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

	for _, series := range seriesList {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		idStr := fmt.Sprintf("%d", *series.Id)
		if s.Resume && lastID != "" && idStr <= lastID {
			continue
		}

		title := getString(series.Title)

		files, _, err := inst.Api().EpisodeFileAPI.ListEpisodeFile(authCtx).SeriesId(*series.Id).Execute()
		if err != nil {
			return fmt.Errorf("get episode files for series %d: %w", *series.Id, err)
		}

		for _, file := range files {
			duration := 0.0
			if file.MediaInfo != nil {
				d, _ := ParseDuration(getString(file.MediaInfo.RunTime))
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
			relPath := getString(file.RelativePath)
			sizeStr := humanize.Bytes(uint64(info.Size))

			s.TotalScanned++
			if isCand {
				s.TotalCandidate++
				s.UI.LogPermanent(fmt.Sprintf("\033[31m✘\033[0m [%s: %s] %s - %s (%s) - %s", "Sonarr", inst.Name(), title, relPath, sizeStr, reason))
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

func (s *Scanner) scanRadarr(ctx context.Context, idx int, inst arrs.RadarrInstance) error {
	instanceID := fmt.Sprintf("radarr_%d", idx)
	lastID := ""
	if s.Resume {
		var err error
		lastID, err = s.DB.GetLastItemID(instanceID)
		if err != nil {
			return fmt.Errorf("get last id: %w", err)
		}
	}

	authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
		"X-Api-Key": {Key: inst.ApiKey()},
	})

	movies, _, err := inst.Api().MovieAPI.ListMovie(authCtx).Execute()
	if err != nil {
		return fmt.Errorf("get movies: %w", err)
	}

	sort.Slice(movies, func(i, j int) bool {
		return *movies[i].Id < *movies[j].Id
	})

	if s.Verbose {
		s.UI.LogPermanent(fmt.Sprintf("\n--- Scanning Radarr Instance: %s ---", inst.Name()))
	}

	for _, movie := range movies {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		idStr := fmt.Sprintf("%d", *movie.Id)
		if s.Resume && lastID != "" && idStr <= lastID {
			continue
		}

		title := getStringRadarr(movie.Title)

		if getBoolRadarr(movie.HasFile) && movie.MovieFile != nil {
			duration := 0.0
			if movie.MovieFile.MediaInfo != nil {
				d, _ := ParseDuration(getStringRadarr(movie.MovieFile.MediaInfo.RunTime))
				duration = d
			}
			if duration == 0 && movie.Runtime != nil {
				duration = float64(*movie.Runtime) * 60
			}

			info := FileInfo{
				Size:     *movie.MovieFile.Size,
				Duration: duration,
			}

			isCand, reason := s.Scorer.IsCandidate(info)
			sizeStr := humanize.Bytes(uint64(info.Size))

			s.TotalScanned++
			if isCand {
				s.TotalCandidate++
				s.UI.LogPermanent(fmt.Sprintf("\033[31m✘\033[0m [%s: %s] %s (%s) - %s", "Radarr", inst.Name(), title, sizeStr, reason))
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
