package orchestrator

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/autobrr/go-qbittorrent"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	require.NoError(t, err)
	return database
}

func createTestClient() *arrs.Client {
	return &arrs.Client{
		Sonarr:   make([]arrs.SonarrInstance, 0),
		Radarr:   make([]arrs.RadarrInstance, 0),
		Torrents: make([]arrs.TorrentInstance, 0),
	}
}

func TestDeleteCandidate_Sonarr(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	var deleteCalled bool
	var deleteFileID int32
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		deleteCalled = true
		deleteFileID = fileId
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance:  "test-sonarr",
			ArrType:      "sonarr",
			ItemID:       123,
			FileID:       456,
			Path:         "/path/to/file.mkv",
			Title:        "Test Episode",
			Inode:        789,
			Size:         1000000000,
			Duration:     1800,
			Quality:      "1080p",
			SeasonNumber: 1,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)
	err = database.UpsertCandidate(candidate.ArrInstance, candidate.FileID, candidate.Reason)
	require.NoError(t, err)

	err = orch.DeleteCandidate(ctx, candidate)
	require.NoError(t, err)

	assert.True(t, deleteCalled)
	assert.Equal(t, int32(456), deleteFileID)

}

func TestDeleteCandidate_Radarr(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockRadarr := NewMockRadarrInstance("test-radarr", "test-api-key")
	var deleteCalled bool
	var deleteFileID int32
	mockRadarr.deleteMovieFileFunc = func(ctx context.Context, fileId int32) error {
		deleteCalled = true
		deleteFileID = fileId
		return nil
	}
	client.Radarr = append(client.Radarr, mockRadarr)

	orch := New(database, client, false, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-radarr",
			ArrType:     "radarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/movie.mkv",
			Title:       "Test Movie",
			Inode:       789,
			Size:        2000000000,
			Duration:    7200,
			Quality:     "1080p",
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)
	err = database.UpsertCandidate(candidate.ArrInstance, candidate.FileID, candidate.Reason)
	require.NoError(t, err)

	err = orch.DeleteCandidate(ctx, candidate)
	require.NoError(t, err)

	assert.True(t, deleteCalled)
	assert.Equal(t, int32(456), deleteFileID)
}

func TestDeleteCandidate_WithError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	expectedErr := errors.New("delete failed")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return expectedErr
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(ctx, candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete sonarr episode file")
}

func TestDeleteCandidate_DryRun(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	var deleteCalled bool
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		deleteCalled = true
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, true, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(ctx, candidate)
	require.NoError(t, err)

	assert.False(t, deleteCalled)

	mediaFile, err := database.GetMediaFile(candidate.ArrInstance, candidate.FileID)
	assert.NoError(t, err)
	assert.NotNil(t, mediaFile)
}

func TestDeleteCandidate_InstanceNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	orch := New(database, client, false, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "non-existent-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := orch.DeleteCandidate(ctx, candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sonarr instance non-existent-sonarr not found")
}

func TestUpgradeCandidate_Sonarr(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	var deleteCalled bool
	var downloadCalled bool

	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		deleteCalled = true
		return nil
	}

	mockSonarr.downloadReleaseFunc = func(ctx context.Context, release *sonarr.ReleaseResource) error {
		downloadCalled = true
		return nil
	}

	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
			Size:        1000000000,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)
	err = database.UpsertCandidate(candidate.ArrInstance, candidate.FileID, candidate.Reason)
	require.NoError(t, err)

	release := &sonarr.ReleaseResource{}

	err = orch.UpgradeCandidate(ctx, candidate, release)
	require.NoError(t, err)

	assert.True(t, deleteCalled)
	assert.True(t, downloadCalled)
}

func TestUpgradeCandidate_DryRun(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	var deleteCalled bool
	var downloadCalled bool

	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		deleteCalled = true
		return nil
	}

	mockSonarr.downloadReleaseFunc = func(ctx context.Context, release *sonarr.ReleaseResource) error {
		downloadCalled = true
		return nil
	}

	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, true, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	release := &sonarr.ReleaseResource{}

	err = orch.UpgradeCandidate(ctx, candidate, release)
	require.NoError(t, err)

	assert.False(t, deleteCalled)
	assert.False(t, downloadCalled)
}

func TestDeleteCandidate_WithTorrentsStandard(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", false)
	mockTorrent.apiClient = qbittorrent.NewClient(qbittorrent.Config{
		Host:     ts.URL,
		Username: "admin",
		Password: "admin",
	})

	var deleteTorrentCalled bool
	var deleteTorrentHash string
	var deleteFilesBool bool
	mockTorrent.deleteTorrentFunc = func(ctx context.Context, hash string, deleteFiles bool) error {
		deleteTorrentCalled = true
		deleteTorrentHash = hash
		deleteFilesBool = deleteFiles
		return nil
	}
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	// Create a candidate
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
			Size:        1000,
		},
		Reason: "Test",
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)
	err = database.UpsertCandidate(candidate.ArrInstance, candidate.FileID, candidate.Reason)
	require.NoError(t, err)

	// Insert torrent associated with this Inode
	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", "/path/to/file.mkv", 789, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)

	assert.True(t, deleteTorrentCalled)
	assert.Equal(t, "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", deleteTorrentHash)
	assert.True(t, deleteFilesBool)

	// Verify torrent and media file are removed from local DB
	torrents, err := database.GetTorrentsByInode(789)
	assert.NoError(t, err)
	assert.Empty(t, torrents)

	media, err := database.GetMediaFile(candidate.ArrInstance, candidate.FileID)
	assert.NoError(t, err)
	assert.Nil(t, media)
}

func TestDeleteCandidate_WithTorrentsReadOnly(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", true) // read-only = true
	mockTorrent.apiClient = qbittorrent.NewClient(qbittorrent.Config{
		Host:     ts.URL,
		Username: "admin",
		Password: "admin",
	})

	var deleteTorrentCalled bool
	var deleteTorrentHash string
	var deleteFilesBool bool
	mockTorrent.deleteTorrentFunc = func(ctx context.Context, hash string, deleteFiles bool) error {
		deleteTorrentCalled = true
		deleteTorrentHash = hash
		deleteFilesBool = deleteFiles
		return nil
	}
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	// Create temp file to verify manual deletion on disk
	tempFile, err := os.CreateTemp("", "reducarr-test-*.mkv")
	require.NoError(t, err)
	tempFilePath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempFilePath) // fallback cleanup

	// Create a candidate
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        tempFilePath,
			Title:       "Test Episode",
			Inode:       789,
			Size:        1000,
		},
		Reason: "Test",
	}

	err = database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)
	err = database.UpsertCandidate(candidate.ArrInstance, candidate.FileID, candidate.Reason)
	require.NoError(t, err)

	// Insert torrent associated with this Inode
	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", tempFilePath, 789, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)

	assert.True(t, deleteTorrentCalled)
	assert.Equal(t, "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", deleteTorrentHash)
	assert.False(t, deleteFilesBool) // standard client delete files is false for read-only mode

	// Verify the file was manually deleted from disk
	_, err = os.Stat(tempFilePath)
	assert.True(t, os.IsNotExist(err), "expected file to be manually deleted from disk")
}

func TestDeleteCandidate_WithTorrentsReadOnly_DeleteError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", true)
	mockTorrent.apiClient = qbittorrent.NewClient(qbittorrent.Config{
		Host:     ts.URL,
		Username: "admin",
		Password: "admin",
	})
	mockTorrent.deleteTorrentFunc = func(ctx context.Context, hash string, deleteFiles bool) error {
		return errors.New("delete API failed")
	}
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", "/path/to/file.mkv", 789, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete torrent entry cb1382490ec9ca81014e6b12a8497d337d1cfcb1")
}

func TestUpgradeCandidate_Radarr(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()

	mockRadarr := NewMockRadarrInstance("test-radarr", "test-api-key")
	var deleteCalled bool
	var downloadCalled bool

	mockRadarr.deleteMovieFileFunc = func(ctx context.Context, fileId int32) error {
		deleteCalled = true
		return nil
	}

	mockRadarr.downloadReleaseFunc = func(ctx context.Context, release *radarr.ReleaseResource) error {
		downloadCalled = true
		return nil
	}

	client.Radarr = append(client.Radarr, mockRadarr)

	orch := New(database, client, false, false)

	ctx := context.Background()
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-radarr",
			ArrType:     "radarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/movie.mkv",
			Title:       "Test Movie",
			Inode:       789,
			Size:        1000000000,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)
	err = database.UpsertCandidate(candidate.ArrInstance, candidate.FileID, candidate.Reason)
	require.NoError(t, err)

	release := &radarr.ReleaseResource{}

	err = orch.UpgradeCandidate(ctx, candidate, release)
	require.NoError(t, err)

	assert.True(t, deleteCalled)
	assert.True(t, downloadCalled)
}

func TestDeleteCandidate_DBError(t *testing.T) {
	database := setupTestDB(t)
	// Create candidate first
	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	client := createTestClient()
	orch := New(database, client, false, false)

	// Close database to force error on GetTorrentsByInode
	database.Close()

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetch associated torrents")
}

func TestDeleteCandidate_WithTorrentsStandardDryRun(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", false)
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, true, false) // dryRun = true

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", "/path/to/file.mkv", 789, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)
}

func TestDeleteCandidate_RadarrInstanceNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-radarr",
			ArrType:     "radarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Movie",
			Inode:       789,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "radarr instance test-radarr not found")
}

func TestDeleteCandidate_RadarrDeleteFileError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockRadarr := NewMockRadarrInstance("test-radarr", "test-api-key")
	mockRadarr.deleteMovieFileFunc = func(ctx context.Context, fileId int32) error {
		return errors.New("radarr delete failed")
	}
	client.Radarr = append(client.Radarr, mockRadarr)

	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-radarr",
			ArrType:     "radarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Movie",
			Inode:       789,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete radarr movie file")
}

func TestUpgradeCandidate_SonarrDownloadError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	mockSonarr.downloadReleaseFunc = func(ctx context.Context, release *sonarr.ReleaseResource) error {
		return errors.New("download failed")
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	release := &sonarr.ReleaseResource{}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "grab sonarr release")
}

func TestUpgradeCandidate_RadarrDownloadError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockRadarr := NewMockRadarrInstance("test-radarr", "test-api-key")
	mockRadarr.deleteMovieFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	mockRadarr.downloadReleaseFunc = func(ctx context.Context, release *radarr.ReleaseResource) error {
		return errors.New("download failed")
	}
	client.Radarr = append(client.Radarr, mockRadarr)

	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-radarr",
			ArrType:     "radarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/movie.mkv",
			Title:       "Test Movie",
			Inode:       789,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	release := &radarr.ReleaseResource{}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "grab radarr release")
}

func TestDeleteCandidate_SonarrInstanceNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "missing-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sonarr instance missing-sonarr not found")
}

func TestDeleteCandidate_SonarrEpisodeFileError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return errors.New("sonarr API error")
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete sonarr episode file")
}

func TestDeleteCandidate_Sonarr404Warning(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	notFoundErr := errors.New("404 Not Found: Episode file not found")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return notFoundErr
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      123,
			FileID:      456,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       789,
			Size:        1000000000,
		},
		Reason:    "Test reason",
		IsIgnored: false,
	}

	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)

	allReports, err := database.GetReports(10, 0)
	require.NoError(t, err)
	require.Len(t, allReports, 1)

	report := allReports[0]
	assert.Equal(t, "WARNING", report.Status)
	assert.Len(t, report.WarningMessages, 1)
	assert.Contains(t, report.WarningMessages[0], "sonarr episode file not found (already deleted?)")
	assert.Equal(t, int32(456), report.MainFileID)
	assert.Equal(t, "Test Episode", report.ItemTitle)
}

func TestUpgradeCandidate_SonarrInstanceNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "missing-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	release := &sonarr.ReleaseResource{}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sonarr instance missing-sonarr not found")
}

func TestUpgradeCandidate_RadarrInstanceNotFound(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "missing-radarr",
			ArrType:     "radarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/movie.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	release := &radarr.ReleaseResource{}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "radarr instance missing-radarr not found")
}

func TestDeleteCandidate_VerboseStandard(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", false)
	mockTorrent.apiClient = qbittorrent.NewClient(qbittorrent.Config{
		Host:     ts.URL,
		Username: "admin",
		Password: "admin",
	})
	mockTorrent.deleteTorrentFunc = func(ctx context.Context, hash string, deleteFiles bool) error {
		return nil
	}
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, true) // verbose = true

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", "/path/to/file.mkv", 99, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)
}

func TestDeleteCandidate_VerboseReadOnly(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", true)
	mockTorrent.apiClient = qbittorrent.NewClient(qbittorrent.Config{
		Host:     ts.URL,
		Username: "admin",
		Password: "admin",
	})
	mockTorrent.deleteTorrentFunc = func(ctx context.Context, hash string, deleteFiles bool) error {
		return nil
	}
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, true) // verbose = true

	tempFile, err := os.CreateTemp("", "reducarr-verbose-ro-*.mkv")
	require.NoError(t, err)
	tempFilePath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempFilePath)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        tempFilePath,
			Title:       "Test Episode",
			Inode:       99,
		},
	}
	err = database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", tempFilePath, 99, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)
}

func TestUpgradeCandidate_VerboseSonarr(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error {
		return nil
	}
	mockSonarr.downloadReleaseFunc = func(ctx context.Context, release *sonarr.ReleaseResource) error {
		return nil
	}
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, false, true) // verbose = true

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test Episode",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	release := &sonarr.ReleaseResource{}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.NoError(t, err)
}

func TestUpgradeCandidate_SonarrWithSize(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	mockSonarr.deleteEpisodeFileFunc = func(ctx context.Context, fileId int32) error { return nil }
	mockSonarr.downloadReleaseFunc = func(ctx context.Context, release *sonarr.ReleaseResource) error { return nil }
	client.Sonarr = append(client.Sonarr, mockSonarr)
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	size := int64(1234567)
	release := &sonarr.ReleaseResource{Size: &size}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.NoError(t, err)
}

func TestUpgradeCandidate_RadarrWithSize(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockRadarr := NewMockRadarrInstance("test-radarr", "test-api-key")
	mockRadarr.deleteMovieFileFunc = func(ctx context.Context, fileId int32) error { return nil }
	mockRadarr.downloadReleaseFunc = func(ctx context.Context, release *radarr.ReleaseResource) error { return nil }
	client.Radarr = append(client.Radarr, mockRadarr)
	orch := New(database, client, false, true) // verbose = true to cover radarr verbose path

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-radarr",
			ArrType:     "radarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/movie.mkv",
			Title:       "Test Movie",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	size := int64(9876543)
	release := &radarr.ReleaseResource{Size: &size}
	err = orch.UpgradeCandidate(context.Background(), candidate, release)
	require.NoError(t, err)
}

func TestDeleteCandidate_ReadOnly_DryRunWithFiles(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", true)
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	client.Sonarr = append(client.Sonarr, mockSonarr)

	orch := New(database, client, true, false) // dryRun = true, read-only

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", "/path/to/file.mkv", 99, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.NoError(t, err)
}

func TestDeleteCandidate_StandardTorrentDeleteError(t *testing.T) {
	database := setupTestDB(t)
	defer database.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/auth/login" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Ok."))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := createTestClient()
	mockTorrent := NewMockTorrentInstance("test-torrent", false) // standard (not read-only)
	mockTorrent.apiClient = qbittorrent.NewClient(qbittorrent.Config{
		Host:     ts.URL,
		Username: "admin",
		Password: "admin",
	})
	mockTorrent.deleteTorrentFunc = func(ctx context.Context, hash string, deleteFiles bool) error {
		return errors.New("qbittorrent delete failed")
	}
	client.Torrents = append(client.Torrents, mockTorrent)

	mockSonarr := NewMockSonarrInstance("test-sonarr", "test-api-key")
	client.Sonarr = append(client.Sonarr, mockSonarr)
	orch := New(database, client, false, false)

	candidate := db.CandidateRecord{
		MediaFileRecord: db.MediaFileRecord{
			ArrInstance: "test-sonarr",
			ArrType:     "sonarr",
			ItemID:      1,
			FileID:      2,
			Path:        "/path/to/file.mkv",
			Title:       "Test",
			Inode:       99,
		},
	}
	err := database.UpsertMediaFile(candidate.MediaFileRecord)
	require.NoError(t, err)

	_, err = database.Exec("INSERT INTO torrents (client_name, info_hash, file_path, inode, is_seeding, added_at) VALUES (?, ?, ?, ?, ?, ?)",
		"test-torrent", "cb1382490ec9ca81014e6b12a8497d337d1cfcb1", "/path/to/file.mkv", 99, 1, 1000)
	require.NoError(t, err)

	err = orch.DeleteCandidate(context.Background(), candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete torrent and files")
}
