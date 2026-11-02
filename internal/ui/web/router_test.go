package web

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatAction(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SEASON_UPGRADE", "SEASON UPGRADE"},
		{"SERIES_UPGRADE", "SERIES UPGRADE"},
		{"UPGRADE", "UPGRADE"},
		{"DELETE", "DELETE"},
		{"CUSTOM_ACTION_NAME", "CUSTOM ACTION NAME"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, FormatAction(tt.input))
			assert.Equal(t, tt.expected, formatAction(tt.input))
		})
	}
}

func TestGetRedirectPath(t *testing.T) {
	tests := []struct {
		name     string
		referer  string
		expected string
	}{
		{
			name:     "empty referer",
			referer:  "",
			expected: "",
		},
		{
			name:     "candidates path",
			referer:  "http://localhost:8080/candidates",
			expected: "/candidates",
		},
		{
			name:     "candidates with query",
			referer:  "http://localhost:8080/candidates?instance=sonarr-1",
			expected: "/candidates?instance=sonarr-1",
		},
		{
			name:     "shows path with params",
			referer:  "http://localhost:8080/shows/sonarr-main/42",
			expected: "/shows/sonarr-main/42",
		},
		{
			name:     "search path",
			referer:  "http://localhost:8080/search?q=movie",
			expected: "/search?q=movie",
		},
		{
			name:     "unrelated path",
			referer:  "http://localhost:8080/settings",
			expected: "",
		},
		{
			name:     "invalid URL",
			referer:  "://invalid-url",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRedirectPath(tt.referer)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRedirectCookieHelpers(t *testing.T) {
	w := httptest.NewRecorder()
	setRedirectCookie(w, "/candidates?instance=test")

	resp := w.Result()
	cookies := resp.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, redirectCookieName, cookies[0].Name)
	assert.Equal(t, "/candidates?instance=test", cookies[0].Value)

	// Test getting and clearing cookie
	req := httptest.NewRequest("POST", "/api/candidates/test/1/grab", nil)
	req.AddCookie(cookies[0])

	w2 := httptest.NewRecorder()
	val := getAndClearRedirectCookie(w2, req)
	assert.Equal(t, "/candidates?instance=test", val)

	clearedCookies := w2.Result().Cookies()
	assert.Len(t, clearedCookies, 1)
	assert.Equal(t, redirectCookieName, clearedCookies[0].Name)
	assert.Equal(t, -1, clearedCookies[0].MaxAge)
}

func TestSetToastCookie(t *testing.T) {
	w := httptest.NewRecorder()
	setToastCookie(w, "Release 'Movie' grabbed successfully!", "success")

	resp := w.Result()
	cookies := resp.Cookies()
	assert.Len(t, cookies, 1)
	assert.Equal(t, toastCookieName, cookies[0].Name)

	unescaped, err := url.QueryUnescape(cookies[0].Value)
	assert.NoError(t, err)
	assert.Equal(t, "success:Release 'Movie' grabbed successfully!", unescaped)
}

func TestTroubleshootingRoutes(t *testing.T) {
	t.Chdir(t.TempDir())
	viper.Reset()
	defer viper.Reset()

	database, err := db.Open(":memory:")
	assert.NoError(t, err)
	defer func() { _ = database.Close() }()

	err = database.UpsertUser("admin", "secret")
	assert.NoError(t, err)
	token := "test-session-token"
	err = database.CreateSession(token, "admin", time.Now().Add(time.Hour))
	assert.NoError(t, err)
	cookie := &http.Cookie{Name: "reducarr_session", Value: token}

	router := NewRouter(database, nil, false)

	// Disabled by default -> 404
	req, _ := http.NewRequest("GET", "/troubleshooting", nil)
	req.AddCookie(cookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)

	reqPost, _ := http.NewRequest("POST", "/api/troubleshooting/clear-table", strings.NewReader("table=scan_state"))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPost.AddCookie(cookie)
	wPost := httptest.NewRecorder()
	router.ServeHTTP(wPost, reqPost)
	assert.Equal(t, http.StatusNotFound, wPost.Code)

	// Check /settings when disabled
	reqSettingsDisabled, _ := http.NewRequest("GET", "/settings", nil)
	reqSettingsDisabled.AddCookie(cookie)
	wSettingsDisabled := httptest.NewRecorder()
	router.ServeHTTP(wSettingsDisabled, reqSettingsDisabled)
	assert.Equal(t, http.StatusOK, wSettingsDisabled.Code)
	assert.NotContains(t, wSettingsDisabled.Body.String(), `href="/troubleshooting"`)

	// Enable troubleshooting via viper
	viper.Set("webui.enableTroubleshooting", true)

	// Check /settings when enabled
	reqSettingsEnabled, _ := http.NewRequest("GET", "/settings", nil)
	reqSettingsEnabled.AddCookie(cookie)
	wSettingsEnabled := httptest.NewRecorder()
	router.ServeHTTP(wSettingsEnabled, reqSettingsEnabled)
	assert.Equal(t, http.StatusOK, wSettingsEnabled.Code)
	assert.Contains(t, wSettingsEnabled.Body.String(), `href="/troubleshooting"`)

	reqEnabled, _ := http.NewRequest("GET", "/troubleshooting", nil)
	reqEnabled.AddCookie(cookie)
	wEnabled := httptest.NewRecorder()
	router.ServeHTTP(wEnabled, reqEnabled)
	assert.Equal(t, http.StatusOK, wEnabled.Code)
	assert.Contains(t, wEnabled.Body.String(), "Troubleshooting & Maintenance")

	// Post clear table when enabled
	reqClear, _ := http.NewRequest("POST", "/api/troubleshooting/clear-table", strings.NewReader("table=scan_state"))
	reqClear.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqClear.AddCookie(cookie)
	wClear := httptest.NewRecorder()
	router.ServeHTTP(wClear, reqClear)
	assert.Equal(t, http.StatusOK, wClear.Code)
	assert.Contains(t, wClear.Body.String(), "Database Tables")
	assert.Contains(t, wClear.Header().Get("HX-Trigger"), "showToast")

	// Post wal-checkpoint when enabled
	reqWal, _ := http.NewRequest("POST", "/api/troubleshooting/wal-checkpoint", nil)
	reqWal.AddCookie(cookie)
	wWal := httptest.NewRecorder()
	router.ServeHTTP(wWal, reqWal)
	assert.Equal(t, http.StatusOK, wWal.Code)
	assert.Contains(t, wWal.Body.String(), "Database Tables")
	assert.Contains(t, wWal.Header().Get("HX-Trigger"), "showToast")
}

func TestRouter_NilClientHandling(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_nil_client.db")
	database, err := db.Open(dbPath)
	assert.NoError(t, err)
	defer db.Close(database)

	err = database.UpsertUser("admin", "secret")
	assert.NoError(t, err)
	token := "test-session-token-nil"
	err = database.CreateSession(token, "admin", time.Now().Add(time.Hour))
	assert.NoError(t, err)
	cookie := &http.Cookie{Name: "reducarr_session", Value: token}

	router := NewRouter(database, nil, false)

	// Triggering manual scan when client is nil should return 500 instead of panicking
	reqScan, _ := http.NewRequest("POST", "/api/scan/full", nil)
	reqScan.AddCookie(cookie)
	wScan := httptest.NewRecorder()
	router.ServeHTTP(wScan, reqScan)
	assert.Equal(t, http.StatusInternalServerError, wScan.Code)

	// Fetch releases when client is nil
	reqReleases, _ := http.NewRequest("GET", "/api/candidates/sonarr_0/1/releases", nil)
	reqReleases.AddCookie(cookie)
	wReleases := httptest.NewRecorder()
	router.ServeHTTP(wReleases, reqReleases)
	assert.Equal(t, http.StatusInternalServerError, wReleases.Code)
}

func TestRouter_DynamicClientReload(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_dynamic.db")
	database, err := db.Open(dbPath)
	assert.NoError(t, err)
	defer db.Close(database)

	err = database.UpsertUser("admin", "secret")
	assert.NoError(t, err)
	token := "test-session-token-dyn"
	err = database.CreateSession(token, "admin", time.Now().Add(time.Hour))
	assert.NoError(t, err)
	cookie := &http.Cookie{Name: "reducarr_session", Value: token}

	router := NewRouter(database, nil, false)

	// Before reload: client is nil -> /api/health should say "Client not initialized"
	reqHealth, _ := http.NewRequest("GET", "/api/health", nil)
	reqHealth.AddCookie(cookie)
	wHealth := httptest.NewRecorder()
	router.ServeHTTP(wHealth, reqHealth)
	assert.Contains(t, wHealth.Body.String(), "Client not initialized")

	// Notify config changed with instances
	oldCfg := &config.Config{}
	newCfg := &config.Config{
		Sonarr: []config.ArrInstance{
			{Name: "Sonarr-Test", URL: "http://127.0.0.1:8989", APIKey: "dummy"},
		},
	}
	config.NotifyConfigChanged(oldCfg, newCfg)

	// After reload: client is now initialized with Sonarr-Test
	wHealth2 := httptest.NewRecorder()
	router.ServeHTTP(wHealth2, reqHealth)
	assert.NotContains(t, wHealth2.Body.String(), "Client not initialized")
	assert.Contains(t, wHealth2.Body.String(), "Sonarr-Test")
}

func TestWebRouter_Dashboard_And_Filters(t *testing.T) {
	database, err := db.Open(":memory:")
	assert.NoError(t, err)
	defer func() { _ = database.Close() }()

	// Create user and session
	assert.NoError(t, database.UpsertUser("admin", "password123"))
	token := "test-token"
	assert.NoError(t, database.CreateSession(token, "admin", time.Now().Add(time.Hour)))

	// Insert test data: candidates & reports
	m1 := db.MediaFileRecord{ArrInstance: "Sonarr-1", ArrType: "sonarr", ItemID: 1, FileID: 10, Title: "Episode 1", Size: 3 * 1024 * 1024 * 1024}
	m2 := db.MediaFileRecord{ArrInstance: "Radarr-1", ArrType: "radarr", ItemID: 2, FileID: 20, Title: "Movie 1", Size: 6 * 1024 * 1024 * 1024}
	assert.NoError(t, database.UpsertMediaFile(m1))
	assert.NoError(t, database.UpsertMediaFile(m2))
	assert.NoError(t, database.UpsertCandidate(m1.ArrInstance, m1.FileID, "reason 1"))
	assert.NoError(t, database.UpsertCandidate(m2.ArrInstance, m2.FileID, "reason 2"))
	assert.NoError(t, database.SetIgnoreCandidate(m2.ArrInstance, m2.FileID, true))

	r1 := db.ReportRecord{ActionType: "UPGRADE", ItemTitle: "Movie Upgrade", Status: "SUCCESS"}
	r2 := db.ReportRecord{ActionType: "UPGRADE", ItemTitle: "Failed Action", Status: "FAILED", ErrorMessage: "Error occurred", IsRead: false}
	r3 := db.ReportRecord{
		ActionType:      "SEASON_UPGRADE",
		ItemTitle:       "Show - Season 1",
		Status:          "SUCCESS",
		TotalSizeBefore: 10000,
		TotalSizeAfter:  4000,
	}
	r4 := db.ReportRecord{
		ActionType:      "SERIES_UPGRADE",
		ItemTitle:       "Complete Series",
		Status:          "SUCCESS",
		TotalSizeBefore: 20000,
		TotalSizeAfter:  8000,
	}
	assert.NoError(t, database.InsertReport(r1))
	assert.NoError(t, database.InsertReport(r2))
	assert.NoError(t, database.InsertReport(r3))
	assert.NoError(t, database.InsertReport(r4))

	router := NewRouter(database, nil, false)

	// Test GET / (Dashboard)
	t.Run("GET / contains ignored and failed links", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "/candidates?show_ignored=1")
		assert.Contains(t, body, "/reports?status=FAILED")
		assert.Contains(t, body, "System Health")
		assert.Contains(t, body, `hx-get="/api/health"`)
	})

	// Test GET /candidates
	t.Run("GET /candidates default excludes ignored", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/candidates", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Episode 1")
		assert.NotContains(t, body, "Movie 1")
	})

	// Test GET /candidates?show_ignored=1
	t.Run("GET /candidates?show_ignored=1 includes ignored", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/candidates?show_ignored=1", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Episode 1")
		assert.Contains(t, body, "Movie 1")
		assert.Contains(t, body, "Unignore")
	})

	// Test POST /api/candidates/{instance}/{id}/unignore
	t.Run("POST /api/candidates/{instance}/{id}/unignore", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/candidates/Radarr-1/20/unignore", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.False(t, database.IsCandidateIgnored("Radarr-1", 20))
	})

	// Test GET /reports?status=FAILED
	t.Run("GET /reports?status=FAILED filters reports", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/reports?status=FAILED", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Failed Action")
		assert.NotContains(t, body, "Movie Upgrade")
	})

	// Test POST /api/reports/{id}/read
	t.Run("POST /api/reports/{id}/read marks report as read", func(t *testing.T) {
		reports, err := database.GetReportsFiltered("FAILED", 10, 0)
		assert.NoError(t, err)
		require.NotEmpty(t, reports)
		reportID := reports[0].ID

		req := httptest.NewRequest("POST", fmt.Sprintf("/api/reports/%d/read", reportID), nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		updated, err := database.GetReportByID(reportID)
		assert.NoError(t, err)
		assert.True(t, updated.IsRead)
	})

	// Test GET /reports?action=UPGRADE
	t.Run("GET /reports?action=UPGRADE filters by action", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/reports?action=UPGRADE", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Movie Upgrade")
		assert.Contains(t, body, "Reset")
	})

	// Test GET /reports?sort=item&order=asc
	t.Run("GET /reports?sort=item&order=asc preserves filters and renders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/reports?sort=item&order=asc&status=FAILED", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Failed Action")
		assert.Contains(t, body, "order=desc") // Next sort toggle link
	})

	// Test GET /reports renders SEASON UPGRADE and SERIES UPGRADE with title and truncate
	t.Run("GET /reports renders season and series upgrades with ellipsis badge and hover title", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/reports", nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `title="SEASON UPGRADE"`)
		assert.Contains(t, body, `title="SERIES UPGRADE"`)
		assert.Contains(t, body, "SEASON UPGRADE")
		assert.Contains(t, body, "SERIES UPGRADE")
		assert.Contains(t, body, "truncate max-w-full")
	})

	// Test GET /reports/{id} renders season upgrade detail
	t.Run("GET /reports/{id} renders season upgrade with title and net saved", func(t *testing.T) {
		reports, err := database.GetReports(100, 0)
		assert.NoError(t, err)
		var seasonRepID int
		for _, rep := range reports {
			if rep.ActionType == "SEASON_UPGRADE" {
				seasonRepID = rep.ID
				break
			}
		}
		require.NotZero(t, seasonRepID)

		req := httptest.NewRequest("GET", fmt.Sprintf("/reports/%d", seasonRepID), nil)
		req.AddCookie(&http.Cookie{Name: "reducarr_session", Value: token})
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `title="SEASON UPGRADE"`)
		assert.Contains(t, body, "SEASON UPGRADE")
		assert.Contains(t, body, "Net Saved")
	})
}

func TestRouter_OptimizeRoutes(t *testing.T) {
	database, err := db.Open(":memory:")
	assert.NoError(t, err)
	defer func() { _ = database.Close() }()

	err = database.UpsertUser("admin", "secret")
	assert.NoError(t, err)
	token := "test-session-token"
	err = database.CreateSession(token, "admin", time.Now().Add(time.Hour))
	assert.NoError(t, err)
	sessionCookie := &http.Cookie{Name: "reducarr_session", Value: token}

	// Insert test media file
	m := db.MediaFileRecord{
		ArrInstance:  "sonarr-1",
		ArrType:      "sonarr",
		ItemID:       10,
		FileID:       101,
		Title:        "Test Series",
		Path:         "/media/test.mkv",
		SeasonNumber: 1,
		Inode:        12345,
		Size:         5000,
	}
	assert.NoError(t, database.UpsertMediaFile(m))

	router := NewRouter(database, nil, false)

	// 1. Legacy /optimize/{instance}/{id} redirect
	t.Run("Legacy optimize redirects to /optimize/files/", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/optimize/sonarr-1/101?search=1", nil)
		req.AddCookie(sessionCookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMovedPermanently, w.Code)
		assert.Equal(t, "/optimize/files/sonarr-1/101?search=1", w.Header().Get("Location"))
	})

	// 2. /optimize/files/{instance}/{id}
	t.Run("Optimize files page renders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/optimize/files/sonarr-1/101", nil)
		req.AddCookie(sessionCookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Test Series")
		assert.Contains(t, w.Body.String(), "Optimization Detail")
	})

	// 3. /optimize/shows/{instance}/{seriesId} without Sonarr -> 404
	t.Run("Optimize show without Sonarr returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/optimize/shows/sonarr-1/10", nil)
		req.AddCookie(sessionCookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// 4. /shows/{instance}/{seriesId} without Sonarr -> 404
	t.Run("Shows detail without Sonarr returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/shows/sonarr-1/10", nil)
		req.AddCookie(sessionCookie)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestSettingsPage_PreservesNonVisualOptions(t *testing.T) {
	database, err := db.Open(":memory:")
	assert.NoError(t, err)
	defer func() { _ = database.Close() }()

	err = database.UpsertUser("admin", "secret")
	assert.NoError(t, err)
	token := "test-settings-token"
	err = database.CreateSession(token, "admin", time.Now().Add(time.Hour))
	assert.NoError(t, err)
	sessionCookie := &http.Cookie{Name: "reducarr_session", Value: token}

	router := NewRouter(database, nil, false)

	configYAML := `scoring:
    maxSize: "15GB"
    maxRatio: "120MiB/min"
    maxBitrate: "8000k"
    minSeedDuration: "720h"
webui:
    pageSize: 50
    enableTroubleshooting: true
    radarrTargetSize: "4GB"
    sonarrTargetSize: "2GB"
automation:
    autoUpgrade: true
    customAutomationKey: "preserved"
dryRun: true
`

	// Test POST /api/config
	form := url.Values{}
	form.Set("content", configYAML)
	reqPost := httptest.NewRequest("POST", "/api/config", strings.NewReader(form.Encode()))
	reqPost.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqPost.AddCookie(sessionCookie)
	wPost := httptest.NewRecorder()
	router.ServeHTTP(wPost, reqPost)

	assert.Equal(t, http.StatusOK, wPost.Code)
	assert.Contains(t, wPost.Body.String(), "Saved at")

	// Test GET /settings renders the content in Ace editor preserving all non-visual options
	reqGet := httptest.NewRequest("GET", "/settings", nil)
	reqGet.AddCookie(sessionCookie)
	wGet := httptest.NewRecorder()
	router.ServeHTTP(wGet, reqGet)

	assert.Equal(t, http.StatusOK, wGet.Code)
	body := wGet.Body.String()
	assert.Contains(t, body, "id=\"ace-editor\"")
	assert.Contains(t, body, "enableTroubleshooting")
	assert.Contains(t, body, "radarrTargetSize")
	assert.Contains(t, body, "maxSize")
	assert.Contains(t, body, "customAutomationKey")
}

func TestGrabButtonsRenderSpinner(t *testing.T) {
	rel := ReleaseInfo{
		GUID:    "test-guid-123",
		Title:   "Movie.2024.1080p",
		Indexer: "Prowlarr",
		Size:    1024 * 1024 * 1024,
		Seeders: 10,
		Quality: "1080p",
		Score:   100,
	}

	t.Run("ReleaseItem has spinner", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := ReleaseItem("admin", rel, "radarr", 42).Render(context.Background(), buf)
		require.NoError(t, err)
		out := buf.String()
		assert.Contains(t, out, "loading loading-spinner")
		assert.Contains(t, out, "htmx-indicator")
		assert.Contains(t, out, "hx-disabled-elt=\"this\"")
		assert.Contains(t, out, "Grab")
	})

	t.Run("SeasonReleaseItem has spinner", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := SeasonReleaseItem("admin", rel, "sonarr", 10, 1).Render(context.Background(), buf)
		require.NoError(t, err)
		out := buf.String()
		assert.Contains(t, out, "loading loading-spinner")
		assert.Contains(t, out, "htmx-indicator")
		assert.Contains(t, out, "hx-disabled-elt=\"this\"")
		assert.Contains(t, out, "Grab Season")
	})

	t.Run("SeriesReleaseItem has spinner", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := SeriesReleaseItem("admin", rel, "sonarr", 10).Render(context.Background(), buf)
		require.NoError(t, err)
		out := buf.String()
		assert.Contains(t, out, "loading loading-spinner")
		assert.Contains(t, out, "htmx-indicator")
		assert.Contains(t, out, "hx-disabled-elt=\"this\"")
		assert.Contains(t, out, "Grab Show")
	})

	t.Run("SearchModal has spinner", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := SearchModal("Test Movie", "radarr", 42, []ReleaseInfo{rel}).Render(context.Background(), buf)
		require.NoError(t, err)
		out := buf.String()
		assert.Contains(t, out, "loading loading-spinner")
		assert.Contains(t, out, "htmx-indicator")
		assert.Contains(t, out, "hx-disabled-elt=\"this\"")
		assert.Contains(t, out, "Grab")
	})
}

func TestLayoutRendersErrorToastHandler(t *testing.T) {
	buf := new(bytes.Buffer)
	err := Layout("Test Page", "admin", LayoutOptions{}).Render(context.Background(), buf)
	require.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "id=\"toast-container\"")
	assert.Contains(t, out, "alert-error")
	assert.Contains(t, out, "htmx:responseError")
	assert.Contains(t, out, "htmx:sendError")
}

func TestGrabEndpointErrors(t *testing.T) {
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = database.Close() }()

	err = database.UpsertUser("admin", "secret")
	require.NoError(t, err)
	token := "grab-test-token"
	err = database.CreateSession(token, "admin", time.Now().Add(time.Hour))
	require.NoError(t, err)
	sessionCookie := &http.Cookie{Name: "reducarr_session", Value: token}

	router := NewRouter(database, nil, false)

	// Missing candidate / missing client should return error
	form := url.Values{}
	form.Set("guid", "some-guid")
	req := httptest.NewRequest("POST", "/api/candidates/test/9999/grab", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Since client is nil, should return 500 "Client not initialized"
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Client not initialized")
}
