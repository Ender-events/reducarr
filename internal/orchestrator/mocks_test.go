package orchestrator

import (
	"context"

	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/autobrr/go-qbittorrent"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
)

// MockSonarrInstance is a mock implementation of arrs.SonarrInstance for testing
type MockSonarrInstance struct {
	name         string
	apiKey       string
	pathMappings []fsutil.PathMapping

	// Configurable function fields for testing
	listReleasesFunc      func(ctx context.Context, episodeId *int32, seriesId *int32, seasonNumber *int32) ([]sonarr.ReleaseResource, error)
	downloadReleaseFunc   func(ctx context.Context, release *sonarr.ReleaseResource) error
	getEpisodeFileFunc    func(ctx context.Context, fileId int32) (*sonarr.EpisodeFileResource, error)
	listEpisodesFunc      func(ctx context.Context, seriesId int32) ([]sonarr.EpisodeResource, error)
	deleteEpisodeFileFunc func(ctx context.Context, fileId int32) error
	listHistoryFunc       func(ctx context.Context, pageSize int32) ([]sonarr.HistoryResource, error)
}

// Name returns the instance name
func (m *MockSonarrInstance) Name() string {
	return m.name
}

// ApiKey returns the API key
func (m *MockSonarrInstance) ApiKey() string {
	return m.apiKey
}

// Api returns the API client (returns nil for mock)
func (m *MockSonarrInstance) Api() *sonarr.APIClient {
	return nil
}

// PathMappings returns the path mappings
func (m *MockSonarrInstance) PathMappings() []fsutil.PathMapping {
	return m.pathMappings
}

// ListReleases returns releases for the given criteria
func (m *MockSonarrInstance) ListReleases(ctx context.Context, episodeId *int32, seriesId *int32, seasonNumber *int32) ([]sonarr.ReleaseResource, error) {
	if m.listReleasesFunc != nil {
		return m.listReleasesFunc(ctx, episodeId, seriesId, seasonNumber)
	}
	return nil, nil
}

// DownloadRelease downloads the specified release
func (m *MockSonarrInstance) DownloadRelease(ctx context.Context, release *sonarr.ReleaseResource) error {
	if m.downloadReleaseFunc != nil {
		return m.downloadReleaseFunc(ctx, release)
	}
	return nil
}

// GetEpisodeFile retrieves an episode file by ID
func (m *MockSonarrInstance) GetEpisodeFile(ctx context.Context, fileId int32) (*sonarr.EpisodeFileResource, error) {
	if m.getEpisodeFileFunc != nil {
		return m.getEpisodeFileFunc(ctx, fileId)
	}
	return nil, nil
}

// ListEpisodes lists all episodes for a series
func (m *MockSonarrInstance) ListEpisodes(ctx context.Context, seriesId int32) ([]sonarr.EpisodeResource, error) {
	if m.listEpisodesFunc != nil {
		return m.listEpisodesFunc(ctx, seriesId)
	}
	return nil, nil
}

// DeleteEpisodeFile deletes an episode file
func (m *MockSonarrInstance) DeleteEpisodeFile(ctx context.Context, fileId int32) error {
	if m.deleteEpisodeFileFunc != nil {
		return m.deleteEpisodeFileFunc(ctx, fileId)
	}
	return nil
}

// ListHistory retrieves history
func (m *MockSonarrInstance) ListHistory(ctx context.Context, pageSize int32) ([]sonarr.HistoryResource, error) {
	if m.listHistoryFunc != nil {
		return m.listHistoryFunc(ctx, pageSize)
	}
	return nil, nil
}

// MockRadarrInstance is a mock implementation of arrs.RadarrInstance for testing
type MockRadarrInstance struct {
	name         string
	apiKey       string
	pathMappings []fsutil.PathMapping

	// Configurable function fields for testing
	listReleasesFunc       func(ctx context.Context, movieId int32) ([]radarr.ReleaseResource, error)
	downloadReleaseFunc    func(ctx context.Context, release *radarr.ReleaseResource) error
	triggerMovieSearchFunc func(ctx context.Context, movieId int32) error
	deleteMovieFileFunc    func(ctx context.Context, fileId int32) error
	listHistoryFunc        func(ctx context.Context, pageSize int32) ([]radarr.HistoryResource, error)
	getMovieFunc           func(ctx context.Context, movieId int32) (*radarr.MovieResource, error)
	getMovieFileFunc       func(ctx context.Context, fileId int32) (*radarr.MovieFileResource, error)
}

// Name returns the instance name
func (m *MockRadarrInstance) Name() string {
	return m.name
}

// ApiKey returns the API key
func (m *MockRadarrInstance) ApiKey() string {
	return m.apiKey
}

// Api returns the API client (returns nil for mock)
func (m *MockRadarrInstance) Api() *radarr.APIClient {
	return nil
}

// PathMappings returns the path mappings
func (m *MockRadarrInstance) PathMappings() []fsutil.PathMapping {
	return m.pathMappings
}

// ListReleases returns releases for the given movie
func (m *MockRadarrInstance) ListReleases(ctx context.Context, movieId int32) ([]radarr.ReleaseResource, error) {
	if m.listReleasesFunc != nil {
		return m.listReleasesFunc(ctx, movieId)
	}
	return nil, nil
}

// DownloadRelease downloads the specified release
func (m *MockRadarrInstance) DownloadRelease(ctx context.Context, release *radarr.ReleaseResource) error {
	if m.downloadReleaseFunc != nil {
		return m.downloadReleaseFunc(ctx, release)
	}
	return nil
}

// TriggerMovieSearch triggers a movie search
func (m *MockRadarrInstance) TriggerMovieSearch(ctx context.Context, movieId int32) error {
	if m.triggerMovieSearchFunc != nil {
		return m.triggerMovieSearchFunc(ctx, movieId)
	}
	return nil
}

// DeleteMovieFile deletes a movie file
func (m *MockRadarrInstance) DeleteMovieFile(ctx context.Context, fileId int32) error {
	if m.deleteMovieFileFunc != nil {
		return m.deleteMovieFileFunc(ctx, fileId)
	}
	return nil
}

// ListHistory retrieves history
func (m *MockRadarrInstance) ListHistory(ctx context.Context, pageSize int32) ([]radarr.HistoryResource, error) {
	if m.listHistoryFunc != nil {
		return m.listHistoryFunc(ctx, pageSize)
	}
	return nil, nil
}

// GetMovie retrieves a movie by ID
func (m *MockRadarrInstance) GetMovie(ctx context.Context, movieId int32) (*radarr.MovieResource, error) {
	if m.getMovieFunc != nil {
		return m.getMovieFunc(ctx, movieId)
	}
	return nil, nil
}

// GetMovieFile retrieves a movie file by ID
func (m *MockRadarrInstance) GetMovieFile(ctx context.Context, fileId int32) (*radarr.MovieFileResource, error) {
	if m.getMovieFileFunc != nil {
		return m.getMovieFileFunc(ctx, fileId)
	}
	return nil, nil
}

// MockTorrentInstance is a mock implementation of arrs.TorrentInstance for testing
type MockTorrentInstance struct {
	name         string
	pathMappings []fsutil.PathMapping
	readOnly     bool
	apiClient    *qbittorrent.Client

	// Configurable function fields for testing
	getFilesFunc      func(ctx context.Context, hash string) ([]qbittorrent.TorrentFile, error)
	deleteTorrentFunc func(ctx context.Context, hash string, deleteFiles bool) error
	loginFunc         func(ctx context.Context) error
}

// Name returns the instance name
func (m *MockTorrentInstance) Name() string {
	return m.name
}

// Api returns the qBittorrent client
func (m *MockTorrentInstance) Api() *qbittorrent.Client {
	if m.apiClient != nil {
		return m.apiClient
	}
	// Create a fake client that won't panic on method calls
	// but will fail on actual operations. For mocking, we override deleteTorrentFunc.
	config := qbittorrent.Config{
		Host:     "http://fake-host:8080",
		Username: "fake",
		Password: "fake",
	}
	m.apiClient = qbittorrent.NewClient(config)
	return m.apiClient
}

// PathMappings returns the path mappings
func (m *MockTorrentInstance) PathMappings() []fsutil.PathMapping {
	return m.pathMappings
}

// IsReadOnly returns whether the instance is read-only
func (m *MockTorrentInstance) IsReadOnly() bool {
	return m.readOnly
}

// GetFiles retrieves files for a torrent
func (m *MockTorrentInstance) GetFiles(ctx context.Context, hash string) ([]qbittorrent.TorrentFile, error) {
	if m.getFilesFunc != nil {
		return m.getFilesFunc(ctx, hash)
	}
	return nil, nil
}

// DeleteTorrent deletes a torrent
func (m *MockTorrentInstance) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	if m.deleteTorrentFunc != nil {
		return m.deleteTorrentFunc(ctx, hash, deleteFiles)
	}
	return nil
}

// MockClient is a mock implementation of arrs.Client for testing
type MockClient struct {
	Sonarr   []arrs.SonarrInstance
	Radarr   []arrs.RadarrInstance
	Torrents []arrs.TorrentInstance
}

// FindSonarr finds a Sonarr instance by name
func (m *MockClient) FindSonarr(name string) arrs.SonarrInstance {
	for _, s := range m.Sonarr {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

// FindRadarr finds a Radarr instance by name
func (m *MockClient) FindRadarr(name string) arrs.RadarrInstance {
	for _, r := range m.Radarr {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

// NewMockClient creates a new mock client
func NewMockClient() *MockClient {
	return &MockClient{
		Sonarr:   make([]arrs.SonarrInstance, 0),
		Radarr:   make([]arrs.RadarrInstance, 0),
		Torrents: make([]arrs.TorrentInstance, 0),
	}
}

// NewMockSonarrInstance creates a new mock Sonarr instance
func NewMockSonarrInstance(name, apiKey string) *MockSonarrInstance {
	return &MockSonarrInstance{
		name:         name,
		apiKey:       apiKey,
		pathMappings: make([]fsutil.PathMapping, 0),
	}
}

// NewMockRadarrInstance creates a new mock Radarr instance
func NewMockRadarrInstance(name, apiKey string) *MockRadarrInstance {
	return &MockRadarrInstance{
		name:         name,
		apiKey:       apiKey,
		pathMappings: make([]fsutil.PathMapping, 0),
	}
}

// NewMockTorrentInstance creates a new mock Torrent instance
func NewMockTorrentInstance(name string, readOnly bool) *MockTorrentInstance {
	return &MockTorrentInstance{
		name:         name,
		pathMappings: make([]fsutil.PathMapping, 0),
		readOnly:     readOnly,
	}
}

// AddSonarr adds a Sonarr instance to the mock client
func (m *MockClient) AddSonarr(inst arrs.SonarrInstance) {
	m.Sonarr = append(m.Sonarr, inst)
}

// AddRadarr adds a Radarr instance to the mock client
func (m *MockClient) AddRadarr(inst arrs.RadarrInstance) {
	m.Radarr = append(m.Radarr, inst)
}

// AddTorrent adds a Torrent instance to the mock client
func (m *MockClient) AddTorrent(inst arrs.TorrentInstance) {
	m.Torrents = append(m.Torrents, inst)
}
