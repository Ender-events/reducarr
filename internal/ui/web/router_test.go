package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
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
	assert.NotContains(t, wSettingsDisabled.Body.String(), "Troubleshooting Page")

	// Enable troubleshooting via viper
	viper.Set("webui.enableTroubleshooting", true)
	defer viper.Reset()

	// Check /settings when enabled
	reqSettingsEnabled, _ := http.NewRequest("GET", "/settings", nil)
	reqSettingsEnabled.AddCookie(cookie)
	wSettingsEnabled := httptest.NewRecorder()
	router.ServeHTTP(wSettingsEnabled, reqSettingsEnabled)
	assert.Equal(t, http.StatusOK, wSettingsEnabled.Code)
	assert.Contains(t, wSettingsEnabled.Body.String(), "Troubleshooting Page")

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

