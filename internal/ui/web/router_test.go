package web

import (
	"net/http/httptest"
	"net/url"
	"testing"

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
