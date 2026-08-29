package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
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

type ScanProgress struct {
	IsRunning      bool      `json:"is_running"`
	ScanType       string    `json:"scan_type"` // "full" or "incremental"
	Phase          string    `json:"phase"`     // "torrents", "sonarr", "radarr", "completed", "idle"
	CurrentItem    string    `json:"current_item"`
	TorrentsTotal  int       `json:"torrents_total"`
	TorrentsDone   int       `json:"torrents_done"`
	SonarrTotal    int       `json:"sonarr_total"`
	SonarrDone     int       `json:"sonarr_done"`
	RadarrTotal    int       `json:"radarr_total"`
	RadarrDone     int       `json:"radarr_done"`
	TotalScanned   int       `json:"total_scanned"`
	TotalCandidate int       `json:"total_candidate"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
}

func (p ScanProgress) TorrentsPercent() int {
	if p.TorrentsTotal <= 0 {
		if p.Phase != "torrents" && p.Phase != "idle" {
			return 100
		}
		return 0
	}
	pct := (p.TorrentsDone * 100) / p.TorrentsTotal
	if pct > 100 {
		return 100
	}
	return pct
}

func (p ScanProgress) SonarrPercent() int {
	if p.SonarrTotal <= 0 {
		if p.Phase == "radarr" || p.Phase == "completed" {
			return 100
		}
		return 0
	}
	pct := (p.SonarrDone * 100) / p.SonarrTotal
	if pct > 100 {
		return 100
	}
	return pct
}

func (p ScanProgress) RadarrPercent() int {
	if p.RadarrTotal <= 0 {
		if p.Phase == "completed" {
			return 100
		}
		return 0
	}
	pct := (p.RadarrDone * 100) / p.RadarrTotal
	if pct > 100 {
		return 100
	}
	return pct
}

func (p ScanProgress) OverallPercent() int {
	if !p.IsRunning && p.Phase == "completed" {
		return 100
	}
	if !p.IsRunning {
		return 0
	}

	var pct float64
	switch p.Phase {
	case "torrents":
		tPct := float64(p.TorrentsPercent()) / 100.0
		pct = tPct * 25.0
	case "sonarr":
		sPct := float64(p.SonarrPercent()) / 100.0
		pct = 25.0 + (sPct * 40.0)
	case "radarr":
		rPct := float64(p.RadarrPercent()) / 100.0
		pct = 65.0 + (rPct * 35.0)
	case "completed":
		pct = 100.0
	default:
		pct = 0.0
	}
	if pct > 100.0 {
		return 100
	}
	return int(pct)
}

type ScanManager struct {
	mu       sync.Mutex
	progress ScanProgress
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
	return m.progress.IsRunning
}

func (m *ScanManager) StartScan(scanType string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = ScanProgress{
		IsRunning: true,
		ScanType:  scanType,
		Phase:     "torrents",
		StartTime: time.Now(),
	}
}

func (m *ScanManager) SetPhase(phase string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress.Phase = phase
}

func (m *ScanManager) UpdateTorrentProgress(client string, item string, done int, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress.Phase = "torrents"
	m.progress.CurrentItem = fmt.Sprintf("[%s] %s", client, item)
	m.progress.TorrentsDone = done
	m.progress.TorrentsTotal = total
}

func (m *ScanManager) UpdateScanProgress(phase string, item string, done int, total int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress.Phase = phase
	m.progress.CurrentItem = item
	if phase == "sonarr" {
		m.progress.SonarrDone = done
		m.progress.SonarrTotal = total
	} else if phase == "radarr" {
		m.progress.RadarrDone = done
		m.progress.RadarrTotal = total
	}
}

func (m *ScanManager) UpdateSummary(scanned int, candidate int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress.TotalScanned = scanned
	m.progress.TotalCandidate = candidate
}

func (m *ScanManager) FinishScan() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress.IsRunning = false
	m.progress.Phase = "completed"
	m.progress.EndTime = time.Now()
}

func (m *ScanManager) GetProgress() ScanProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.progress
}

var globalScanManager = &ScanManager{}
var startTime time.Time

func getUser(r *http.Request) string {
	u, _ := r.Context().Value(UserContextKey).(string)
	return u
}

const redirectCookieName = "reducarr_redirect_back"
const toastCookieName = "reducarr_toast"

func setRedirectCookie(w http.ResponseWriter, urlStr string) {
	http.SetCookie(w, &http.Cookie{
		Name:     redirectCookieName,
		Value:    urlStr,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getAndClearRedirectCookie(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(redirectCookieName)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{
		Name:     redirectCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return c.Value
}

func setToastCookie(w http.ResponseWriter, msg string, toastType string) {
	val := fmt.Sprintf("%s:%s", toastType, msg)
	http.SetCookie(w, &http.Cookie{
		Name:     toastCookieName,
		Value:    url.QueryEscape(val),
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func getRedirectPath(referer string) string {
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil {
		return ""
	}
	if strings.HasPrefix(u.Path, "/candidates") || strings.HasPrefix(u.Path, "/shows/") || strings.HasPrefix(u.Path, "/search") {
		reqPath := u.Path
		if u.RawQuery != "" {
			reqPath += "?" + u.RawQuery
		}
		return reqPath
	}
	return ""
}

func NewRouter(database *db.DB, initialClient *arrs.Client, verbose bool) http.Handler {
	startTime = time.Now()
	mux := http.NewServeMux()

	var clientMu sync.RWMutex
	currentClient := initialClient

	getClient := func() *arrs.Client {
		clientMu.RLock()
		defer clientMu.RUnlock()
		return currentClient
	}

	setClient := func(c *arrs.Client) {
		clientMu.Lock()
		defer clientMu.Unlock()
		currentClient = c
	}

	vlog := func(format string, v ...any) {
		if verbose {
			log.Printf("[WEB] "+format, v...)
		}
	}

	config.Subscribe(func(diff config.ConfigDiff) {
		if !diff.InstancesChanged {
			return
		}
		vlog("Detected instance configuration changes, reloading clients...")
		cfg := diff.NewConfig
		if cfg == nil {
			var err error
			cfg, err = config.LoadConfig()
			if err != nil {
				vlog("ERROR loading config on reload: %v", err)
				return
			}
		}
		newClient, err := arrs.GetClient(context.Background(), cfg)
		if err != nil {
			vlog("ERROR reinitializing arrs client: %v", err)
			return
		}
		setClient(newClient)
		vlog("Client instances successfully updated")
	})

	// Health check - Simple liveness probe
	mux.HandleFunc("GET /health/simple", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	// Health check - Detailed health information
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		vlog("Detailed health check requested")
		HealthCheckHandler(w, r, database, getClient())
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
		navStats := buildNavStats(r.Context(), database, getClient())
		if err := IndexPage(getUser(r), webStats, navStats).Render(r.Context(), w); err != nil {
			vlog("Failed to render index page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Candidates
	mux.HandleFunc("GET /candidates", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Candidates page")
		instanceFilter := r.URL.Query().Get("instance")
		arrTypeFilter := r.URL.Query().Get("arr_type")
		showIgnored := r.URL.Query().Get("show_ignored") == "1" || r.URL.Query().Get("show_ignored") == "true"
		pageStr := r.URL.Query().Get("page")
		page, _ := strconv.Atoi(pageStr)
		if page < 1 {
			page = 1
		}
		cfg, _ := config.LoadConfig()
		pageSize := cfg.WebUI.PageSize
		if pageSize <= 0 {
			pageSize = 25
		}
		offset := (page - 1) * pageSize

		total, err := database.CountCandidatesFiltered(instanceFilter, showIgnored, arrTypeFilter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		candidates, err := database.GetCandidatesWithMediaPaginated(instanceFilter, showIgnored, arrTypeFilter, pageSize, offset)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		instances := buildInstanceInfos()
		navStats := buildNavStats(r.Context(), database, getClient())
		if err := CandidatesPage(getUser(r), candidates, instanceFilter, arrTypeFilter, instances, page, pageSize, total, showIgnored, navStats).Render(r.Context(), w); err != nil {
			vlog("Failed to render candidates page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Reports
	mux.HandleFunc("GET /reports", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Reports page")
		statusFilter := r.URL.Query().Get("status")
		actionFilter := r.URL.Query().Get("action")
		sortBy := r.URL.Query().Get("sort")
		sortOrder := r.URL.Query().Get("order")

		reports, err := database.GetReportsAdvanced(db.ReportFilter{
			Status:    statusFilter,
			Action:    actionFilter,
			SortBy:    sortBy,
			SortOrder: sortOrder,
			Limit:     100,
			Offset:    0,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		actions, _ := database.GetDistinctReportActions()
		navStats := buildNavStats(r.Context(), database, getClient())
		if err := ReportsPage(getUser(r), reports, statusFilter, actionFilter, sortBy, sortOrder, actions, navStats).Render(r.Context(), w); err != nil {
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

		navStats := buildNavStats(r.Context(), database, getClient())
		if err := ReportDetailPage(getUser(r), *report, navStats).Render(r.Context(), w); err != nil {
			vlog("Failed to render report detail page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Search
	mux.HandleFunc("GET /search", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Search page")
		navStats := buildNavStats(r.Context(), database, getClient())
		if err := SearchPage(getUser(r), navStats).Render(r.Context(), w); err != nil {
			vlog("Failed to render search page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Settings
	mux.HandleFunc("GET /settings", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Settings page")
		cfg, _ := config.LoadConfig()
		content, _ := config.GetConfigContent()
		info := BuildInfo{
			Version:   buildinfo.Version,
			Commit:    buildinfo.Commit,
			GoVersion: buildinfo.GoVersion(),
			BuildTime: buildinfo.BuildTime,
		}
		navStats := buildNavStats(r.Context(), database, getClient())
		if err := SettingsPage(getUser(r), content, globalScanManager.GetProgress(), info, cfg.WebUI.EnableTroubleshooting, navStats).Render(r.Context(), w); err != nil {
			vlog("Failed to render settings page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Troubleshooting Page
	mux.HandleFunc("GET /troubleshooting", func(w http.ResponseWriter, r *http.Request) {
		vlog("Accessing Troubleshooting page")
		cfg, _ := config.LoadConfig()
		if !cfg.WebUI.EnableTroubleshooting {
			http.NotFound(w, r)
			return
		}
		counts, err := database.GetTableCounts()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := TroubleshootingPage(getUser(r), counts).Render(r.Context(), w); err != nil {
			vlog("Failed to render troubleshooting page: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Clear Table API
	mux.HandleFunc("POST /api/troubleshooting/clear-table", func(w http.ResponseWriter, r *http.Request) {
		vlog("Executing clear-table action")
		cfg, _ := config.LoadConfig()
		if !cfg.WebUI.EnableTroubleshooting {
			http.NotFound(w, r)
			return
		}
		table := r.FormValue("table")
		if table == "all" {
			for _, t := range db.AllowedTables {
				_, _ = database.ClearTable(t)
			}
			setToastCookie(w, "All database tables cleared successfully", "success")
		} else {
			rows, err := database.ClearTable(table)
			if err != nil {
				vlog("Error clearing table %s: %v", table, err)
				http.Error(w, fmt.Sprintf("Failed to clear table %s: %v", table, err), http.StatusBadRequest)
				return
			}
			setToastCookie(w, fmt.Sprintf("Table '%s' cleared (%d rows deleted)", table, rows), "success")
		}
		counts, err := database.GetTableCounts()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := TroubleshootingTableList(counts).Render(r.Context(), w); err != nil {
			vlog("Failed to render troubleshooting table list: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Scan Status API for HTMX Live Polling
	mux.HandleFunc("GET /api/scan/status", func(w http.ResponseWriter, r *http.Request) {
		if err := ScanControls(globalScanManager.GetProgress()).Render(r.Context(), w); err != nil {
			vlog("Failed to render scan status: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Optimization Page
	mux.HandleFunc("GET /optimize/{instance}/{id}", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Accessing Optimization page for: %s:%d", instance, id)

		if refPath := getRedirectPath(r.Referer()); refPath != "" {
			setRedirectCookie(w, refPath)
		}

		media, err := database.GetMediaFile(instance, id)
		if err != nil || media == nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		torrents, _ := database.GetTorrentsByInode(media.Inode)

		autoSearch := r.URL.Query().Get("search") == "1"
		navStats := buildNavStats(r.Context(), database, getClient())
		if err := OptimizationPage(getUser(r), *media, torrents, autoSearch, navStats).Render(r.Context(), w); err != nil {
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
		c := getClient()
		if c == nil {
			fmt.Fprint(w, "<span class='text-error text-xs'>Client not initialized</span>")
			return
		}
		results := c.HealthCheck(r.Context())
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
		c := getClient()
		if c == nil {
			vlog("Cannot trigger scan: client is not initialized")
			http.Error(w, "Client not initialized", http.StatusInternalServerError)
			return
		}

		if globalScanManager.IsRunning() {
			http.Error(w, "Scan already in progress", http.StatusConflict)
			return
		}

		scanType := "full"
		if isIncremental {
			scanType = "incremental"
		}

		vlog("Starting manual %s scan", scanType)

		globalScanManager.StartScan(scanType)

		go func() {
			defer globalScanManager.FinishScan()

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

			tScanner := torrent.NewScanner(c, database, uiLogger, nil)
			tScanner.Verbose = verbose
			tScanner.Incremental = isIncremental
			tScanner.OnProgress = func(clientName string, item string, done int, total int) {
				globalScanManager.UpdateTorrentProgress(clientName, item, done, total)
			}

			globalScanManager.SetPhase("torrents")
			if err := tScanner.ScanAll(context.Background()); err != nil {
				vlog("Torrent scan failed: %v", err)
			}

			scanner := &scan.Scanner{
				Client:  c,
				DB:      database,
				Scorer:  scorer,
				UI:      uiLogger,
				Verbose: verbose,
			}
			scanner.OnProgress = func(phase string, item string, done int, total int) {
				globalScanManager.UpdateScanProgress(phase, item, done, total)
				globalScanManager.UpdateSummary(scanner.TotalScanned, scanner.TotalCandidate)
			}

			globalScanManager.SetPhase("sonarr")
			var scanErr error
			if isIncremental {
				scanErr = scanner.Incremental(context.Background())
			} else {
				scanErr = scanner.Run(context.Background())
			}
			if scanErr != nil {
				vlog("Scan failed: %v", scanErr)
			}
			globalScanManager.UpdateSummary(scanner.TotalScanned, scanner.TotalCandidate)
			vlog("Manual scan complete")
		}()

		if err := ScanControls(globalScanManager.GetProgress()).Render(r.Context(), w); err != nil {
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
		// Return updated CandidateItem for HTMX swap
		candidate, err := database.GetCandidate(instance, id)
		if err != nil || candidate == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := CandidateItem(getUser(r), *candidate).Render(r.Context(), w); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Unignore Candidate
	mux.HandleFunc("POST /api/candidates/{instance}/{id}/unignore", func(w http.ResponseWriter, r *http.Request) {
		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)

		vlog("Unignoring candidate: %s:%d", instance, id)
		if err := database.SetIgnoreCandidate(instance, id, false); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Return updated CandidateItem for HTMX swap
		candidate, err := database.GetCandidate(instance, id)
		if err != nil || candidate == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := CandidateItem(getUser(r), *candidate).Render(r.Context(), w); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Mark Report as Read
	mux.HandleFunc("POST /api/reports/{id}/read", func(w http.ResponseWriter, r *http.Request) {
		idStr := r.PathValue("id")
		id, _ := strconv.Atoi(idStr)
		vlog("Marking report %d as read", id)
		if err := database.MarkReportAsRead(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		report, err := database.GetReportByID(id)
		if err != nil || report == nil {
			w.WriteHeader(http.StatusOK)
			return
		}
		if err := ReportItem(getUser(r), *report).Render(r.Context(), w); err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	})

	// Delete Candidate
	mux.HandleFunc("DELETE /api/candidates/{instance}/{id}", func(w http.ResponseWriter, r *http.Request) {
		c := getClient()
		if c == nil {
			http.Error(w, "Client not initialized", http.StatusInternalServerError)
			return
		}

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

		orch := orchestrator.New(database, c, false, verbose)
		if err := orch.DeleteCandidate(r.Context(), *target); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	})

	// Fetch Releases for Optimization
	mux.HandleFunc("GET /api/candidates/{instance}/{id}/releases", func(w http.ResponseWriter, r *http.Request) {
		c := getClient()
		if c == nil {
			http.Error(w, "Client not initialized", http.StatusInternalServerError)
			return
		}

		instance := r.PathValue("instance")
		idStr := r.PathValue("id")
		id64, _ := strconv.ParseInt(idStr, 10, 32)
		id := int32(id64)
		cfg, err := config.LoadConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		vlog("Fetching releases for: %s:%d", instance, id)

		target, _ := database.GetMediaFile(instance, id)
		if target == nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		// TODO: move this logic into orchestrator ?
		var releaseInfos []ReleaseInfo
		if target.ArrType == "radarr" {
			inst := c.FindRadarr(instance)
			_ = inst.TriggerMovieSearch(r.Context(), target.ItemID)
			releases, _ := inst.ListReleases(r.Context(), target.ItemID)
			for _, rl := range releases {
				if sorting.RejectionSeverity(rl.GetRejections()) >= cfg.WebUI.MinRejectionSeverity {
					continue
				}
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
			inst := c.FindSonarr(instance)
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
					if sorting.RejectionSeverity(rl.GetRejections()) >= cfg.WebUI.MinRejectionSeverity {
						continue
					}
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
		c := getClient()
		if c == nil {
			http.Error(w, "Client not initialized", http.StatusInternalServerError)
			return
		}
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

		orch := orchestrator.New(database, c, false, verbose)
		var err error
		if targetRecord.ArrType == "radarr" {
			inst := c.FindRadarr(instance)
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
			inst := c.FindSonarr(instance)
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
		redirectTo := getAndClearRedirectCookie(w, r)
		if redirectTo == "" {
			redirectTo = "/candidates"
		}
		toastMsg := fmt.Sprintf("Release '%s' grabbed successfully!", targetRecord.Title)
		setToastCookie(w, toastMsg, "success")
		w.Header().Set("HX-Redirect", redirectTo)
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast": {"msg": "Release '%s' grabbed successfully!", "type": "success"}}`, targetRecord.Title))
		w.WriteHeader(http.StatusOK)
	})

	return SessionAuth(database)(mux)
}
