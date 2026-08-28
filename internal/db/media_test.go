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

func TestDB_GetMediaFilesBySeason(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	m1 := MediaFileRecord{
		ArrInstance:  "Sonarr-1",
		ArrType:      "sonarr",
		ItemID:       10,
		FileID:       101,
		Path:         "/tv/Show/S01E01.mkv",
		Title:        "Show",
		Inode:        1001,
		Size:         1000,
		SeasonNumber: 1,
	}
	m2 := MediaFileRecord{
		ArrInstance:  "Sonarr-1",
		ArrType:      "sonarr",
		ItemID:       10,
		FileID:       102,
		Path:         "/tv/Show/S01E02.mkv",
		Title:        "Show",
		Inode:        1002,
		Size:         1000,
		SeasonNumber: 1,
	}
	m3 := MediaFileRecord{
		ArrInstance:  "Sonarr-1",
		ArrType:      "sonarr",
		ItemID:       10,
		FileID:       201,
		Path:         "/tv/Show/S02E01.mkv",
		Title:        "Show",
		Inode:        2001,
		Size:         1000,
		SeasonNumber: 2,
	}

	require.NoError(t, d.UpsertMediaFile(m1))
	require.NoError(t, d.UpsertMediaFile(m2))
	require.NoError(t, d.UpsertMediaFile(m3))

	t.Run("returns files for existing season", func(t *testing.T) {
		files, err := d.GetMediaFilesBySeason("Sonarr-1", 10, 1)
		require.NoError(t, err)
		require.Len(t, files, 2)
		assert.Equal(t, int32(101), files[0].FileID)
		assert.Equal(t, int32(102), files[1].FileID)
	})

	t.Run("returns empty for non-existent season", func(t *testing.T) {
		files, err := d.GetMediaFilesBySeason("Sonarr-1", 10, 3)
		require.NoError(t, err)
		assert.Empty(t, files)
	})
}

func TestDB_GetOptimizableEstimatedSavings(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	// Film: 6 GB (> 4GB limit) -> excess: 6GB - 4GB = 2GB
	film := MediaFileRecord{
		ArrInstance: "Radarr-1",
		ArrType:     "radarr",
		ItemID:      1,
		FileID:      10,
		Title:       "Big Movie",
		Size:        6 * 1024 * 1024 * 1024,
	}
	// Small Film: 3 GB (<= 4GB limit) -> excess: 0
	smallFilm := MediaFileRecord{
		ArrInstance: "Radarr-1",
		ArrType:     "radarr",
		ItemID:      2,
		FileID:      20,
		Title:       "Small Movie",
		Size:        3 * 1024 * 1024 * 1024,
	}
	// Serie: 3 GB (> 2GB limit) -> excess: 3GB - 2GB = 1GB
	serie := MediaFileRecord{
		ArrInstance: "Sonarr-1",
		ArrType:     "sonarr",
		ItemID:      3,
		FileID:      30,
		Title:       "Big Episode",
		Size:        3 * 1024 * 1024 * 1024,
	}
	// Ignored Serie: 5 GB -> should NOT be included because is_ignored = 1
	ignoredSerie := MediaFileRecord{
		ArrInstance: "Sonarr-1",
		ArrType:     "sonarr",
		ItemID:      4,
		FileID:      40,
		Title:       "Ignored Episode",
		Size:        5 * 1024 * 1024 * 1024,
	}

	require.NoError(t, d.UpsertMediaFile(film))
	require.NoError(t, d.UpsertMediaFile(smallFilm))
	require.NoError(t, d.UpsertMediaFile(serie))
	require.NoError(t, d.UpsertMediaFile(ignoredSerie))

	require.NoError(t, d.UpsertCandidate(film.ArrInstance, film.FileID, "oversized"))
	require.NoError(t, d.UpsertCandidate(smallFilm.ArrInstance, smallFilm.FileID, "reason"))
	require.NoError(t, d.UpsertCandidate(serie.ArrInstance, serie.FileID, "oversized"))
	require.NoError(t, d.UpsertCandidate(ignoredSerie.ArrInstance, ignoredSerie.FileID, "ignored"))
	require.NoError(t, d.SetIgnoreCandidate(ignoredSerie.ArrInstance, ignoredSerie.FileID, true))

	savings, err := d.GetOptimizableEstimatedSavings()
	require.NoError(t, err)

	// Expected: 2GB (film) + 0 (small film) + 1GB (serie) = 3GB
	expected := int64(3 * 1024 * 1024 * 1024)
	assert.Equal(t, expected, savings)
}

func TestDB_Candidates_ShowIgnored(t *testing.T) {
	d, err := Open(":memory:")
	require.NoError(t, err)
	defer func() { _ = d.Close() }()

	m1 := MediaFileRecord{ArrInstance: "Sonarr-1", ArrType: "sonarr", ItemID: 1, FileID: 1, Title: "Ep 1", Size: 100}
	m2 := MediaFileRecord{ArrInstance: "Sonarr-1", ArrType: "sonarr", ItemID: 2, FileID: 2, Title: "Ep 2", Size: 200}

	require.NoError(t, d.UpsertMediaFile(m1))
	require.NoError(t, d.UpsertMediaFile(m2))

	require.NoError(t, d.UpsertCandidate(m1.ArrInstance, m1.FileID, "r1"))
	require.NoError(t, d.UpsertCandidate(m2.ArrInstance, m2.FileID, "r2"))
	require.NoError(t, d.SetIgnoreCandidate(m2.ArrInstance, m2.FileID, true))

	// When showIgnored = false
	count, err := d.CountCandidatesFiltered("", false, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	paginated, err := d.GetCandidatesWithMediaPaginated("", false, "", 10, 0)
	require.NoError(t, err)
	assert.Len(t, paginated, 1)
	assert.Equal(t, int32(1), paginated[0].FileID)

	// When showIgnored = true
	countAll, err := d.CountCandidatesFiltered("", true, "")
	require.NoError(t, err)
	assert.Equal(t, 2, countAll)

	paginatedAll, err := d.GetCandidatesWithMediaPaginated("", true, "", 10, 0)
	require.NoError(t, err)
	assert.Len(t, paginatedAll, 2)
}
