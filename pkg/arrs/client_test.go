package arrs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHealthCheck(t *testing.T) {
	// Mock Sonarr server
	sonarrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/system/status", r.URL.Path)
		assert.Equal(t, "sonarr-key", r.Header.Get("X-Api-Key"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"version": "3.0.0"}`))
	}))
	defer sonarrServer.Close()

	// Mock Radarr server
	radarrServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/system/status", r.URL.Path)
		assert.Equal(t, "radarr-key", r.Header.Get("X-Api-Key"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"version": "3.0.0"}`))
	}))
	defer radarrServer.Close()

	client := NewClient(
		[]ArrInstance{{Name: "Sonarr", URL: sonarrServer.URL, APIKey: "sonarr-key"}},
		[]ArrInstance{{Name: "Radarr", URL: radarrServer.URL, APIKey: "radarr-key"}},
		nil,
	)

	results := client.HealthCheck(context.Background())
	assert.Len(t, results, 2)
	assert.True(t, results[0].Healthy)
	assert.True(t, results[1].Healthy)
}

func TestHealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(
		[]ArrInstance{{Name: "Sonarr", URL: server.URL, APIKey: "wrong-key"}},
		nil,
		nil,
	)

	results := client.HealthCheck(context.Background())
	assert.Len(t, results, 1)
	assert.False(t, results[0].Healthy)
}
