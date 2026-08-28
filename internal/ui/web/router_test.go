package web

import (
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
	defer viper.Reset()

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
	assert.NoError(t, database.InsertReport(r1))
	assert.NoError(t, database.InsertReport(r2))

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
}
