package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/Ender-events/reducarr/internal/buildinfo"
	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/orchestrator"
	"github.com/Ender-events/reducarr/internal/scan"
	"github.com/Ender-events/reducarr/internal/sorting"
	"github.com/Ender-events/reducarr/internal/torrent"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/dustin/go-humanize"
)

type ScanManager struct {
	mu        sync.Mutex
	isRunning bool
}

// HealthResult holds health check information for a single service.
type HealthResult struct {
	Name    string
	Type    string
	Healthy bool
	Error   string
}

func (m *ScanManager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isRunning
}

func (m *ScanManager) SetRunning(val bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isRunning = val
}

var globalScanManager = &ScanManager{}
var startTime time.Time

func getUser(r *http.Request) string {
	u, _ := r.Context().Value(UserContextKey).(string)
	return u
}

func NewRouter(database *db.DB, client *arrs.Client, verbose bool) http.Handler {
	startTime = time.Now()
	mux := http.NewServeMux()

	vlog := func(format string, v ...any) {
		if verbose {
			log.Printf("[WEB] "+format, v...)
		}
	}

	// Health check - Simple liveness probe
	mux.HandleFunc("GET /health/simple", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	// Health check - Detailed health information
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		vlog("Detailed health check requested")
		HealthCheckHandler(w, r, database, client)
	})

	// Login page
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("reducarr_session")
		if err == nil {
			_, err = database.GetSession(cookie.Value)
			if err == nil {
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}
		if err := LoginPage("").Render(r.Context(), w); err != nil {
			vlog("Failed to render login page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Login action
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		user := r.FormValue("username")
		pass := r.FormValue("password")

		// Pure Database Auth with Bcrypt
		ok, err := database.AuthenticateUser(user, pass)
		if err != nil || !ok {
			vlog("Failed login attempt for user: %s", user)
			if err := LoginPage("Invalid username or password").Render(r.Context(), w); err != nil {
				vlog("Failed to render login page: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		token := GenerateToken()
		expiresAt := time.Now().Add(24 * 7 * time.Hour)
		if err := database.CreateSession(token, user, expiresAt); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		vlog("User logged in: %s", user)
		SetSessionCookie(w, token, expiresAt)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})

	// Logout action
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("reducarr_session")
		if err == nil {
			vlog("User logging out: %s", getUser(r))
			_ = database.DeleteSession(cookie.Value)
		}
		ClearSessionCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})

	// Dashboard
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		vlog("Accessing Dashboard")
		stats, err := database.GetDashboardStats()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		webStats := DashboardStats{
			TotalSpaceSaved:   stats.TotalSpaceSaved,
			PendingCandidates: stats.PendingCandidates,
			IgnoredFiles:      stats.IgnoredFiles,
			FailedActions:     stats.FailedActions,
			LastScanTime:      stats.LastScanTime,
		}
		if err := IndexPage(getUser(r), webStats).Render(r.Context(), w); err != nil {
			vlog("Failed to render index page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Candidates
	mux.HandleFunc("GET /candidates", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Candidates page")
		instanceFilter := r.URL.Query().Get("instance")
		candidates, err := database.GetCandidatesWithMediaFiltered(instanceFilter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		instances := buildInstanceInfos()
		if err := CandidatesPage(getUser(r), candidates, instanceFilter, instances).Render(r.Context(), w); err != nil {
			vlog("Failed to render candidates page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Reports
	mux.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Reports page")
		reports, err := database.GetReports(100, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := ReportsPage(getUser(r), reports).Render(r.Context(), w); err != nil {
			vlog("Failed to render reports page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	mux.HandleFunc("GET /reports/{id}", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, _ := strconv.Atoi(idStr)

		vlog("Accessing Report detail for ID: %d", id)
		report, err := database.GetReportByID(id)
		if err != nil || report == nil {
			http.Error(w, "Report not found", http.StatusNotFound)
			return
		}

		if err := ReportDetailPage(getUser(r), *report).Render(r.Context(), w); err != nil {
			vlog("Failed to render report detail page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Search
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Search page")
		if err := SearchPage(getUser(r)).Render(r.Context(), w); err != nil {
			vlog("Failed to render search page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Series Page
	mux.HandleFunc("GET /shows/{instance}/{id}", func(w http.ResponseWriter, r *http.Request) {
		instanceName := r.PathValue("instance")
		idStr := r.PathValue("id")
		seriesID, err := strconv.ParseInt(idStr, 10, 32)
		if err != nil {
			http.Error(w, "Invalid series ID", http.StatusBadRequest)
			return
		}

		inst := client.FindSonarr(instanceName)
		if inst == nil {
			if len(client.Sonarr) > 0 {
				inst = client.Sonarr[0]
			} else {
				http.Error(w, "No Sonarr instance configured", http.StatusNotFound)
				return
			}
		}

		vlog("Fetching series %d from Sonarr instance %s", seriesID, inst.Name())
		series, err := inst.GetSeriesByID(r.Context(), int32(seriesID))
		if err != nil || series == nil {
			vlog("Failed to fetch series %d: %v", seriesID, err)
			http.Error(w, "Series not found in Sonarr", http.StatusNotFound)
			return
		}

		seasonFiles := make(map[int32][]db.MediaFileRecord)
		for _, season := range series.Seasons {
			num := season.GetSeasonNumber()
			files, err := database.GetMediaFilesBySeason(inst.Name(), int32(seriesID), num)
			if err == nil {
				seasonFiles[num] = files
			}
		}

		if err := SeriesPage(getUser(r), inst.Name(), series, seasonFiles).Render(r.Context(), w); err != nil {
			vlog("Failed to render series page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Shows Search API
	mux.HandleFunc("GET /api/search/shows", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if len(query) < 2 {
			return
		}
		if len(client.Sonarr) == 0 {
			http.Error(w, "No Sonarr instance configured", http.StatusBadRequest)
			return
		}
		inst := client.Sonarr[0]
		vlog("Searching Sonarr series for: %s", query)
		results, err := inst.LookupSeries(r.Context(), query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var known []sonarr.SeriesResource
		for _, s := range results {
			if s.Id != nil && *s.Id > 0 {
				known = append(known, s)
			}
		}
		if err := ShowSearchResults(getUser(r), inst.Name(), known).Render(r.Context(), w); err != nil {
			vlog("Failed to render show search results: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Settings
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Settings page")
		content, _ := config.GetConfigContent()
		info := BuildInfo{
			Version:   buildinfo.Version,
			Commit:    buildinfo.Commit,
			GoVersion: buildinfo.GoVersion(),
			BuildTime: buildinfo.BuildTime,
		}
		if err := SettingsPage(getUser(r), content, globalScanManager.IsRunning(), info).Render(r.Context(), w); err != nil {
			vlog("Failed to render settings page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Optimization Page
	mux.HandleFunc("GET /optimize/files/{instance}/{id}", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Accessing Optimization page for: %s:%d", instance, id)

		media, err := database.GetMediaFile(instance, id)
		if err != nil || media == nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		torrents, _ := database.GetTorrentsByInode(media.Inode)

		autoSearch := r.URL.Query().Get("search") == "1"
		if err := OptimizationFilePage(getUser(r), *media, torrents, autoSearch).Render(r.Context(), w); err != nil {
			vlog("Failed to render optimization page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Optimization Page
	mux.HandleFunc("GET /optimize/shows/{instance}/{seriesId}/{seasonNum}", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		seriesIdStr := r.PathValue("seriesId")
		seriesId64, _ := strconv.ParseInt(seriesIdStr, 10, 32)
		seriesId := int32(seriesId64)
		seasonNumStr := r.PathValue("seasonNum")
		seasonNum64, _ := strconv.ParseInt(seasonNumStr, 10, 32)
		seasonNum := int32(seasonNum64)

		vlog("Accessing Optimization page for: %s:%d:%d", instance, seriesId, seasonNum)

		inst := client.FindSonarr(instance)
		if inst == nil {
			if len(client.Sonarr) > 0 {
				inst = client.Sonarr[0]
			} else {
				http.Error(w, "No Sonarr instance configured", http.StatusNotFound)
				return
			}
		}

		vlog("Fetching series %d from Sonarr instance %s", seriesId, inst.Name())
		series, err := inst.GetSeriesByID(r.Context(), seriesId)
		if err != nil || series == nil {
			vlog("Failed to fetch series %d: %v", seriesId, err)
			http.Error(w, "Series not found in Sonarr", http.StatusNotFound)
			return
		}

		autoSearch := r.URL.Query().Get("search") == "1"
		if err := OptimizationShowsPage(getUser(r), series, seasonNum, instance, autoSearch).Render(r.Context(), w); err != nil {
			vlog("Failed to render optimization page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})
	// --- API Endpoints for HTMX ---

	// Save Config
	mux.HandleFunc("POST /api/config", func(w http.ResponseWriter, r *http.Request) {
		vlog("Saving configuration")
		content := r.FormValue("content")
		if err := config.SaveConfigContent(content); err != nil {
			vlog("ERROR saving config: %v", err)
			fmt.Fprintf(w, "<span class='text-error text-xs font-bold font-mono'>Error: %v</span>", err)
			return
		}
		vlog("Configuration saved successfully")
		fmt.Fprintf(w, "<span class='text-success text-xs font-bold font-mono'>Saved at %s</span>", time.Now().Format("15:04:05"))
	})

	// Health Check API
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		vlog("Getting health status")
		if client == nil {
			fmt.Fprint(w, "<span class='text-error text-xs'>Client not initialized</span>")
			return
		}
		results := client.HealthCheck(r.Context())
		// Convert to web.HealthResult for templ
		webResults := make([]HealthResult, len(results))
		for i, res := range results {
			webResults[i] = HealthResult{
				Name:    res.Name,
				Type:    res.Type,
				Healthy: res.Healthy,
				Error: func() string {
					if res.Error != nil {
						return res.Error.Error()
					}
					return ""
				}(),
			}
		}
		if err := HealthInfo(webResults).Render(r.Context(), w); err != nil {
			vlog("Failed to render health info: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Trigger Scan
	triggerScan := func(w http.ResponseWriter, r *http.Request, isIncremental bool) {
		if globalScanManager.IsRunning() {
			http.Error(w, "Scan already in progress", http.StatusConflict)
			return
		}

		vlog("Starting manual %s scan", func() string {
			if isIncremental {
				return "incremental"
			}
			return "full"
		}())

		globalScanManager.SetRunning(true)

		go func() {
			defer globalScanManager.SetRunning(false)

			cfg, _ := config.LoadConfig()
			scorer := &scan.Scorer{}
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

			uiLogger := ui.NewProgressLogger()

			tScanner := torrent.NewScanner(client, database, uiLogger, nil)
			tScanner.Verbose = verbose
			tScanner.Incremental = isIncremental
			_ = tScanner.ScanAll(context.Background())

			scanner := &scan.Scanner{
				Client:  client,
				DB:      database,
				Scorer:  scorer,
				UI:      uiLogger,
				Verbose: verbose,
			}

			if isIncremental {
				_ = scanner.Incremental(context.Background())
			} else {
				_ = scanner.Run(context.Background())
			}
			vlog("Manual scan complete")
		}()

		if err := ScanControls(true).Render(r.Context(), w); err != nil {
			vlog("Failed to render scan controls: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}

	mux.HandleFunc("POST /api/scan/full", func(w http.ResponseWriter, r *http.Request) {
		triggerScan(w, r, false)
	})

	mux.HandleFunc("POST /api/scan/incremental", func(w http.ResponseWriter, r *http.Request) {
		triggerScan(w, r, true)
	})

	// Manual Search API
	mux.HandleFunc("GET /api/search", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		if len(query) < 2 {
			return
		}
		vlog("Searching library for: %s", query)
		results, err := database.SearchMediaFiles(query, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := SearchResults(getUser(r), results).Render(r.Context(), w); err != nil {
			vlog("Failed to render search results: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Profile Modal
	mux.HandleFunc("GET /api/user/password", func(w http.ResponseWriter, r *http.Request) {
		vlog("Opening profile modal")
		if err := ChangePasswordModal(getUser(r), "", false).Render(r.Context(), w); err != nil {
			vlog("Failed to render profile modal: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Change Password Action
	mux.HandleFunc("POST /api/user/password", func(w http.ResponseWriter, r *http.Request) {
		vlog("Updating password for user: %s", getUser(r))
		pass := r.FormValue("password")
		confirm := r.FormValue("confirm")

		if pass != confirm {
			if err := ChangePasswordModal(getUser(r), "Passwords do not match.", false).Render(r.Context(), w); err != nil {
				vlog("Failed to render password mismatch modal: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if len(pass) < 8 {
			if err := ChangePasswordModal(getUser(r), "Password must be at least 8 characters.", false).Render(r.Context(), w); err != nil {
				vlog("Failed to render password length modal: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if err := database.UpsertUser(getUser(r), pass); err != nil {
			if err := ChangePasswordModal(getUser(r), "Failed to update password in database.", false).Render(r.Context(), w); err != nil {
				vlog("Failed to render password update error modal: %v", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		if err := ChangePasswordModal(getUser(r), "", true).Render(r.Context(), w); err != nil {
			vlog("Failed to render password update success modal: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Ignore Candidate
	mux.HandleFunc("POST /api/candidates/{instance}/{id}/ignore", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Ignoring candidate: %s:%d", instance, id)
		if err := database.SetIgnoreCandidate(instance, id, true); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Delete Candidate
	mux.HandleFunc("DELETE /api/candidates/{instance}/{id}", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Deleting candidate: %s:%d", instance, id)
		target, _ := database.GetCandidate(instance, id)
		if target == nil {
			m, _ := database.GetMediaFile(instance, id)
			if m != nil {
				target = &db.CandidateRecord{MediaFileRecord: *m}
			}
		}

		if target == nil {
			http.Error(w, "Record not found", http.StatusNotFound)
			return
		}

		orch := orchestrator.New(database, client, false, verbose)
		if err := orch.DeleteCandidate(r.Context(), *target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Fetch Releases for Optimization
	mux.HandleFunc("GET /api/candidates/{instance}/{id}/releases", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Fetching releases for: %s:%d", instance, id)

		target, _ := database.GetMediaFile(instance, id)
		if target == nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		var releaseInfos []ReleaseInfo
		if target.ArrType == "radarr" {
			inst := client.FindRadarr(instance)
			_ = inst.TriggerMovieSearch(r.Context(), target.ItemID)
			releases, _ := inst.ListReleases(r.Context(), target.ItemID)
			for _, rl := range releases {
				score := int32(0)
				if rl.CustomFormatScore != nil {
					score = *rl.CustomFormatScore
				}
				seeders := int32(0)
				if rl.Seeders.Get() != nil {
					seeders = *rl.Seeders.Get()
				}
				releaseInfos = append(releaseInfos, ReleaseInfo{
					GUID:       arrs.GetStringRadarr(rl.Guid),
					Title:      arrs.GetStringRadarr(rl.Title),
					Size:       *rl.Size,
					Indexer:    arrs.GetStringRadarr(rl.Indexer),
					Seeders:    seeders,
					Quality:    arrs.GetStringRadarr(rl.Quality.Quality.Name),
					Score:      score,
					Rejections: rl.Rejections,
				})
			}
		} else {
			inst := client.FindSonarr(instance)
			episodes, _ := inst.ListEpisodes(r.Context(), target.ItemID)
			var epID int32
			for _, ep := range episodes {
				if ep.EpisodeFileId != nil && *ep.EpisodeFileId == target.FileID {
					epID = *ep.Id
					break
				}
			}
			if epID != 0 {
				releases, _ := inst.ListReleases(r.Context(), &epID, nil, nil)
				for _, rl := range releases {
					score := int32(0)
					if rl.CustomFormatScore != nil {
						score = *rl.CustomFormatScore
					}
					seeders := int32(0)
					if rl.Seeders.Get() != nil {
						seeders = *rl.Seeders.Get()
					}
					releaseInfos = append(releaseInfos, ReleaseInfo{
						GUID:       arrs.GetString(rl.Guid),
						Title:      arrs.GetString(rl.Title),
						Size:       *rl.Size,
						Indexer:    arrs.GetString(rl.Indexer),
						Seeders:    seeders,
						Quality:    arrs.GetString(rl.Quality.Quality.Name),
						Score:      score,
						Rejections: rl.Rejections,
					})
				}
			}
		}

		sorting.Sort(releaseInfos)

		vlog("Found %d releases for %s", len(releaseInfos), target.Title)
		if err := ReleaseList(getUser(r), instance, id, releaseInfos).Render(r.Context(), w); err != nil {
			vlog("Failed to render release list: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Grab Release
	mux.HandleFunc("POST /api/candidates/{instance}/{id}/grab", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)
		guid := r.FormValue("guid")

		vlog("Grabbing release with GUID %s for %s:%d", guid, instance, id)

		targetRecord, _ := database.GetCandidate(instance, id)
		if targetRecord == nil {
			m, _ := database.GetMediaFile(instance, id)
			if m != nil {
				targetRecord = &db.CandidateRecord{MediaFileRecord: *m}
			}
		}

		if targetRecord == nil || guid == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		orch := orchestrator.New(database, client, false, verbose)
		var err error
		if targetRecord.ArrType == "radarr" {
			inst := client.FindRadarr(instance)
			releases, _ := inst.ListReleases(r.Context(), targetRecord.ItemID)
			var selected *radarr.ReleaseResource
			for i := range releases {
				if arrs.GetStringRadarr(releases[i].Guid) == guid {
					selected = &releases[i]
					break
				}
			}
			if selected != nil {
				err = orch.UpgradeCandidate(r.Context(), *targetRecord, selected)
			}
		} else {
			inst := client.FindSonarr(instance)
			episodes, _ := inst.ListEpisodes(r.Context(), targetRecord.ItemID)
			var epID int32
			for _, ep := range episodes {
				if ep.EpisodeFileId != nil && *ep.EpisodeFileId == targetRecord.FileID {
					epID = *ep.Id
					break
				}
			}
			releases, _ := inst.ListReleases(r.Context(), &epID, nil, nil)
			var selected *sonarr.ReleaseResource
			for i := range releases {
				if arrs.GetString(releases[i].Guid) == guid {
					selected = &releases[i]
					break
				}
			}
			if selected != nil {
				err = orch.UpgradeCandidate(r.Context(), *targetRecord, selected)
			}
		}

		if err != nil {
			vlog("ERROR grabbing release: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		vlog("Successfully triggered upgrade for: %s", targetRecord.Title)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("GET /api/shows/candidates/{instance}/{showId}/{season}/releases", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		showIdStr := r.PathValue("showId")
		showId64, _ := strconv.ParseInt(showIdStr, 10, 32)
		showId := int32(showId64)
		seasonStr := r.PathValue("season")
		season64, _ := strconv.ParseInt(seasonStr, 10, 32)
		season := int32(season64)
		inst := client.FindSonarr(instance)

		series, err := inst.GetSeriesByID(r.Context(), showId)
		if err != nil || series == nil {
			vlog("Failed to fetch series %d: %v", showId, err)
			http.Error(w, "Series not found in Sonarr", http.StatusNotFound)
			return
		}

		vlog("Fetching releases for: %s:%d", instance, showId)

		var releaseInfos []ReleaseInfo
		releases, err := inst.ListReleases(r.Context(), nil, &showId, &season)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error fetching releases: %v\n", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		for _, rl := range releases {
			score := int32(0)
			if rl.CustomFormatScore != nil {
				score = *rl.CustomFormatScore
			}
			seeders := int32(0)
			if rl.Seeders.Get() != nil {
				seeders = *rl.Seeders.Get()
			}
			releaseInfos = append(releaseInfos, ReleaseInfo{
				GUID:       arrs.GetString(rl.Guid),
				Title:      arrs.GetString(rl.Title),
				Size:       *rl.Size,
				Indexer:    arrs.GetString(rl.Indexer),
				Seeders:    seeders,
				Quality:    arrs.GetString(rl.Quality.Quality.Name),
				Score:      score,
				Rejections: rl.Rejections,
			})
		}

		sorting.Sort(releaseInfos)

		vlog("Found %d releases for %s", len(releaseInfos), series.Title)
		if err := SeasonReleaseList(getUser(r), instance, showId, season, releaseInfos).Render(r.Context(), w); err != nil {
			vlog("Failed to render release list: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Grab Season Release
	mux.HandleFunc("POST /api/shows/candidates/{instance}/{showId}/{season}/grab", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		showIdStr := r.PathValue("showId")
		showId64, _ := strconv.ParseInt(showIdStr, 10, 32)
		showId := int32(showId64)
		seasonStr := r.PathValue("season")
		season64, _ := strconv.ParseInt(seasonStr, 10, 32)
		seasonNum := int32(season64)
		guid := r.FormValue("guid")

		vlog("Grabbing season release with GUID %s for %s:%d s%02d", guid, instance, showId, seasonNum)

		if guid == "" {
			http.Error(w, "Missing guid", http.StatusBadRequest)
			return
		}

		inst := client.FindSonarr(instance)
		if inst == nil {
			http.Error(w, "Sonarr instance not found", http.StatusNotFound)
			return
		}

		releases, err := inst.ListReleases(r.Context(), nil, &showId, &seasonNum)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var selected *sonarr.ReleaseResource
		for i := range releases {
			if arrs.GetString(releases[i].Guid) == guid {
				selected = &releases[i]
				break
			}
		}
		if selected == nil {
			http.Error(w, "Release not found", http.StatusNotFound)
			return
		}

		series, _ := inst.GetSeriesByID(r.Context(), showId)
		seriesTitle := ""
		if series != nil {
			seriesTitle = series.GetTitle()
		}

		orch := orchestrator.New(database, client, false, verbose)
		if err := orch.GrabSeasonRelease(r.Context(), instance, seriesTitle, selected); err != nil {
			vlog("ERROR grabbing season release: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		vlog("Successfully triggered season grab for: %s", seriesTitle)
		w.WriteHeader(http.StatusOK)
	})

	return SessionAuth(database)(mux)
}
