package config

import (
	"testing"

	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/stretchr/testify/assert"
)

func TestComputeDiff_Instances(t *testing.T) {
	oldCfg := &Config{
		Sonarr: []ArrInstance{
			{Name: "sonarr-1", URL: "http://localhost:8989", APIKey: "key1"},
		},
		Radarr: []ArrInstance{
			{Name: "radarr-1", URL: "http://localhost:7878", APIKey: "key2"},
		},
		QBittorrent: []QBittorrentConfig{
			{Name: "qbit-1", URL: "http://localhost:8080", Username: "admin", Password: "pwd"},
		},
	}

	t.Run("no changes", func(t *testing.T) {
		newCfg := &Config{
			Sonarr: []ArrInstance{
				{Name: "sonarr-1", URL: "http://localhost:8989", APIKey: "key1"},
			},
			Radarr: []ArrInstance{
				{Name: "radarr-1", URL: "http://localhost:7878", APIKey: "key2"},
			},
			QBittorrent: []QBittorrentConfig{
				{Name: "qbit-1", URL: "http://localhost:8080", Username: "admin", Password: "pwd"},
			},
		}

		diff := computeDiff(oldCfg, newCfg)
		assert.False(t, diff.HasChanges())
		assert.False(t, diff.InstancesChanged)
		assert.False(t, diff.SonarrChanged)
		assert.False(t, diff.RadarrChanged)
		assert.False(t, diff.QBittorrentChanged)
	})

	t.Run("sonarr changed", func(t *testing.T) {
		newCfg := &Config{
			Sonarr: []ArrInstance{
				{Name: "sonarr-1", URL: "http://localhost:8989", APIKey: "new-key"},
			},
			Radarr: []ArrInstance{
				{Name: "radarr-1", URL: "http://localhost:7878", APIKey: "key2"},
			},
			QBittorrent: []QBittorrentConfig{
				{Name: "qbit-1", URL: "http://localhost:8080", Username: "admin", Password: "pwd"},
			},
		}

		diff := computeDiff(oldCfg, newCfg)
		assert.True(t, diff.HasChanges())
		assert.True(t, diff.InstancesChanged)
		assert.True(t, diff.SonarrChanged)
		assert.False(t, diff.RadarrChanged)
		assert.False(t, diff.QBittorrentChanged)
	})

	t.Run("radarr added", func(t *testing.T) {
		newCfg := &Config{
			Sonarr: []ArrInstance{
				{Name: "sonarr-1", URL: "http://localhost:8989", APIKey: "key1"},
			},
			Radarr: []ArrInstance{
				{Name: "radarr-1", URL: "http://localhost:7878", APIKey: "key2"},
				{Name: "radarr-2", URL: "http://localhost:7879", APIKey: "key3"},
			},
			QBittorrent: []QBittorrentConfig{
				{Name: "qbit-1", URL: "http://localhost:8080", Username: "admin", Password: "pwd"},
			},
		}

		diff := computeDiff(oldCfg, newCfg)
		assert.True(t, diff.HasChanges())
		assert.True(t, diff.InstancesChanged)
		assert.False(t, diff.SonarrChanged)
		assert.True(t, diff.RadarrChanged)
		assert.False(t, diff.QBittorrentChanged)
	})

	t.Run("qbittorrent path mappings changed", func(t *testing.T) {
		newCfg := &Config{
			Sonarr: []ArrInstance{
				{Name: "sonarr-1", URL: "http://localhost:8989", APIKey: "key1"},
			},
			Radarr: []ArrInstance{
				{Name: "radarr-1", URL: "http://localhost:7878", APIKey: "key2"},
			},
			QBittorrent: []QBittorrentConfig{
				{
					Name:     "qbit-1",
					URL:      "http://localhost:8080",
					Username: "admin",
					Password: "pwd",
					PathMappings: []fsutil.PathMapping{
						{Remote: "/downloads", Local: "/data/downloads"},
					},
				},
			},
		}

		diff := computeDiff(oldCfg, newCfg)
		assert.True(t, diff.HasChanges())
		assert.True(t, diff.InstancesChanged)
		assert.False(t, diff.SonarrChanged)
		assert.False(t, diff.RadarrChanged)
		assert.True(t, diff.QBittorrentChanged)
	})
}
