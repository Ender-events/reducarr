package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDB_GetCandidatesWithMediaFiltered(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	// Insert media files
	m1 := MediaFileRecord{
		ArrInstance:  "Sonarr-HD",
		ArrType:      "sonarr",
		ItemID:       1,
		FileID:       101,
		Path:         "/media/tv/Show1/S01E01.mkv",
		Title:        "Show 1 S01E01",
		Inode:        1001,
		Size:         5000000000,
		Duration:     3600,
		Quality:      "HDTV-1080p",
		SeasonNumber: 1,
	}
	m2 := MediaFileRecord{
		ArrInstance:  "Radarr-HD",
		ArrType:      "radarr",
		ItemID:       2,
		FileID:       201,
		Path:         "/media/movies/Movie1.mkv",
		Title:        "Movie 1",
		Inode:        1002,
		Size:         15000000000,
		Duration:     7200,
		Quality:      "Bluray-1080p",
		SeasonNumber: 0,
	}
	m3 := MediaFileRecord{
		ArrInstance:  "Sonarr-4K",
		ArrType:      "sonarr",
		ItemID:       3,
		FileID:       301,
		Path:         "/media/tv/Show2/S01E01.mkv",
		Title:        "Show 2 S01E01",
		Inode:        1003,
		Size:         8000000000,
		Duration:     3600,
		Quality:      "WEBDL-1080p",
		SeasonNumber: 1,
	}

	require.NoError(t, d.UpsertMediaFile(m1))
	require.NoError(t, d.UpsertMediaFile(m2))
	require.NoError(t, d.UpsertMediaFile(m3))

	// Insert candidates
	require.NoError(t, d.UpsertCandidate(m1.ArrInstance, m1.FileID, "Oversized file"))
	require.NoError(t, d.UpsertCandidate(m2.ArrInstance, m2.FileID, "High bitrate"))
	require.NoError(t, d.UpsertCandidate(m3.ArrInstance, m3.FileID, "Oversized file"))

	t.Run("no filter returns all non-ignored candidates", func(t *testing.T) {
		candidates, err := d.GetCandidatesWithMediaFiltered("")
		assert.NoError(t, err)
		assert.Len(t, candidates, 3)
	})

	t.Run("filter by existing instance", func(t *testing.T) {
		candidates, err := d.GetCandidatesWithMediaFiltered("Sonarr-HD")
		assert.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, "Sonarr-HD", candidates[0].ArrInstance)
		assert.Equal(t, int32(101), candidates[0].FileID)
	})

	t.Run("filter by non-existent instance", func(t *testing.T) {
		candidates, err := d.GetCandidatesWithMediaFiltered("NonExistent")
		assert.NoError(t, err)
		assert.Empty(t, candidates)
	})

	t.Run("ignored candidates excluded even with instance filter", func(t *testing.T) {
		require.NoError(t, d.SetIgnoreCandidate(m1.ArrInstance, m1.FileID, true))

		candidates, err := d.GetCandidatesWithMediaFiltered("Sonarr-HD")
		assert.NoError(t, err)
		assert.Empty(t, candidates)

		allCandidates, err := d.GetCandidatesWithMediaFiltered("")
		assert.NoError(t, err)
		assert.Len(t, allCandidates, 2)
	})
}
