package arrs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Ender-events/reducarr/pkg/fsutil"
	"github.com/autobrr/go-qbittorrent"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
)

type ArrInstance struct {
	Name         string
	URL          string
	APIKey       string
	PathMappings []fsutil.PathMapping
}

type QBitConfig struct {
	Name         string
	URL          string
	Username     string
	Password     string
	PathMappings []fsutil.PathMapping
	ReadOnly     bool
}

type HealthResult struct {
	Name    string
	Type    string
	Healthy bool
	Error   error
}

type SonarrInstance interface {
	Name() string
	ApiKey() string
	Api() *sonarr.APIClient
	PathMappings() []fsutil.PathMapping
	ListReleases(ctx context.Context, episodeId *int32, seriesId *int32, seasonNumber *int32) ([]sonarr.ReleaseResource, error)
	DownloadRelease(ctx context.Context, release *sonarr.ReleaseResource) error
	GetEpisodeFile(ctx context.Context, fileId int32) (*sonarr.EpisodeFileResource, error)
	ListEpisodes(ctx context.Context, seriesId int32) ([]sonarr.EpisodeResource, error)
	DeleteEpisodeFile(ctx context.Context, fileId int32) error
}

type RadarrInstance interface {
	Name() string
	ApiKey() string
	Api() *radarr.APIClient
	PathMappings() []fsutil.PathMapping
	ListReleases(ctx context.Context, movieId int32) ([]radarr.ReleaseResource, error)
	DownloadRelease(ctx context.Context, release *radarr.ReleaseResource) error
	TriggerMovieSearch(ctx context.Context, movieId int32) error
	DeleteMovieFile(ctx context.Context, fileId int32) error
}

type TorrentInstance interface {
	Name() string
	Api() *qbittorrent.Client
	PathMappings() []fsutil.PathMapping
	GetFiles(ctx context.Context, hash string) ([]qbittorrent.TorrentFile, error)
	DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error
	IsReadOnly() bool
}

type Client struct {
	Sonarr   []SonarrInstance
	Radarr   []RadarrInstance
	Torrents []TorrentInstance
}

type sonarrInst struct {
	name     string
	url      string
	apiKey   string
	api      *sonarr.APIClient
	mappings []fsutil.PathMapping
}

func (s *sonarrInst) Name() string                       { return s.name }
func (s *sonarrInst) ApiKey() string                     { return s.apiKey }
func (s *sonarrInst) Api() *sonarr.APIClient             { return s.api }
func (s *sonarrInst) PathMappings() []fsutil.PathMapping { return s.mappings }

func (s *sonarrInst) ListReleases(ctx context.Context, episodeId *int32, seriesId *int32, seasonNumber *int32) ([]sonarr.ReleaseResource, error) {
	authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
		"X-Api-Key": {Key: s.apiKey},
	})
	req := s.api.ReleaseAPI.ListRelease(authCtx)
	if episodeId != nil {
		req = req.EpisodeId(*episodeId)
	}
	if seriesId != nil {
		req = req.SeriesId(*seriesId)
	}
	if seasonNumber != nil {
		req = req.SeasonNumber(*seasonNumber)
	}
	releases, _, err := req.Execute()
	return releases, err
}

func (s *sonarrInst) DownloadRelease(ctx context.Context, release *sonarr.ReleaseResource) error {
	authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
		"X-Api-Key": {Key: s.apiKey},
	})

	resp, err := s.api.ReleaseAPI.CreateRelease(authCtx).ReleaseResource(*release).Execute()
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (s *sonarrInst) GetEpisodeFile(ctx context.Context, fileId int32) (*sonarr.EpisodeFileResource, error) {
	authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
		"X-Api-Key": {Key: s.apiKey},
	})
	file, _, err := s.api.EpisodeFileAPI.GetEpisodeFileById(authCtx, fileId).Execute()
	return file, err
}

func (s *sonarrInst) ListEpisodes(ctx context.Context, seriesId int32) ([]sonarr.EpisodeResource, error) {
	authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
		"X-Api-Key": {Key: s.apiKey},
	})
	episodes, _, err := s.api.EpisodeAPI.ListEpisode(authCtx).SeriesId(seriesId).Execute()
	return episodes, err
}

func (s *sonarrInst) DeleteEpisodeFile(ctx context.Context, fileId int32) error {
	authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
		"X-Api-Key": {Key: s.apiKey},
	})
	_, err := s.api.EpisodeFileAPI.DeleteEpisodeFile(authCtx, fileId).Execute()
	return err
}

type radarrInst struct {
	name     string
	url      string
	apiKey   string
	api      *radarr.APIClient
	mappings []fsutil.PathMapping
}

func (r *radarrInst) Name() string                       { return r.name }
func (r *radarrInst) ApiKey() string                     { return r.apiKey }
func (r *radarrInst) Api() *radarr.APIClient             { return r.api }
func (r *radarrInst) PathMappings() []fsutil.PathMapping { return r.mappings }

func (r *radarrInst) ListReleases(ctx context.Context, movieId int32) ([]radarr.ReleaseResource, error) {
	authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
		"X-Api-Key": {Key: r.apiKey},
	})
	releases, _, err := r.api.ReleaseAPI.ListRelease(authCtx).MovieId(movieId).Execute()
	return releases, err
}

func (r *radarrInst) DownloadRelease(ctx context.Context, release *radarr.ReleaseResource) error {
	authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
		"X-Api-Key": {Key: r.apiKey},
	})

	resp, err := r.api.ReleaseAPI.CreateRelease(authCtx).ReleaseResource(*release).Execute()
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

func (r *radarrInst) TriggerMovieSearch(ctx context.Context, movieId int32) error {
	body := map[string]any{
		"name":     "MovieSearch",
		"movieIds": []int32{movieId},
	}
	return r.rawPost(ctx, "/api/v3/command", body)
}

func (r *radarrInst) DeleteMovieFile(ctx context.Context, fileId int32) error {
	authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
		"X-Api-Key": {Key: r.apiKey},
	})
	_, err := r.api.MovieFileAPI.DeleteMovieFile(authCtx, fileId).Execute()
	return err
}

func (r *radarrInst) rawPost(ctx context.Context, endpoint string, body any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", r.url+endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", r.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

type torrentInst struct {
	name     string
	api      *qbittorrent.Client
	mappings []fsutil.PathMapping
	readOnly bool
}

func (t *torrentInst) Name() string                       { return t.name }
func (t *torrentInst) Api() *qbittorrent.Client           { return t.api }
func (t *torrentInst) PathMappings() []fsutil.PathMapping { return t.mappings }
func (t *torrentInst) IsReadOnly() bool                   { return t.readOnly }
func (t *torrentInst) GetFiles(ctx context.Context, hash string) ([]qbittorrent.TorrentFile, error) {
	res, err := t.api.GetFilesInformationCtx(ctx, hash)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return *res, nil
}

func (t *torrentInst) DeleteTorrent(ctx context.Context, hash string, deleteFiles bool) error {
	return t.api.DeleteTorrentsCtx(ctx, []string{hash}, deleteFiles)
}

func NewClient(sonarrConfigs, radarrConfigs []ArrInstance, qbitConfigs []QBitConfig) *Client {
	c := &Client{}

	for _, cfg := range sonarrConfigs {
		sc := sonarr.NewConfiguration()
		sc.Servers = sonarr.ServerConfigurations{{URL: cfg.URL}}
		c.Sonarr = append(c.Sonarr, &sonarrInst{
			name:     cfg.Name,
			url:      cfg.URL,
			apiKey:   cfg.APIKey,
			api:      sonarr.NewAPIClient(sc),
			mappings: cfg.PathMappings,
		})
	}

	for _, cfg := range radarrConfigs {
		rc := radarr.NewConfiguration()
		rc.Servers = radarr.ServerConfigurations{{URL: cfg.URL}}
		c.Radarr = append(c.Radarr, &radarrInst{
			name:     cfg.Name,
			url:      cfg.URL,
			apiKey:   cfg.APIKey,
			api:      radarr.NewAPIClient(rc),
			mappings: cfg.PathMappings,
		})
	}

	for _, cfg := range qbitConfigs {
		api := qbittorrent.NewClient(qbittorrent.Config{
			Host:     cfg.URL,
			Username: cfg.Username,
			Password: cfg.Password,
		})
		c.Torrents = append(c.Torrents, &torrentInst{
			name:     cfg.Name,
			api:      api,
			mappings: cfg.PathMappings,
			readOnly: cfg.ReadOnly,
		})
	}

	return c
}

func (c *Client) HealthCheck(ctx context.Context) []HealthResult {
	var results []HealthResult

	// Check Sonarr
	for _, s := range c.Sonarr {
		authCtx := context.WithValue(ctx, sonarr.ContextAPIKeys, map[string]sonarr.APIKey{
			"X-Api-Key": {Key: s.ApiKey()},
		})
		_, _, err := s.Api().SystemAPI.GetSystemStatus(authCtx).Execute()
		results = append(results, HealthResult{
			Name:    s.Name(),
			Type:    "Sonarr",
			Healthy: err == nil,
			Error:   err,
		})
	}

	// Check Radarr
	for _, r := range c.Radarr {
		authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
			"X-Api-Key": {Key: r.ApiKey()},
		})
		_, _, err := r.Api().SystemAPI.GetSystemStatus(authCtx).Execute()
		results = append(results, HealthResult{
			Name:    r.Name(),
			Type:    "Radarr",
			Healthy: err == nil,
			Error:   err,
		})
	}

	// Check qBittorrent
	for _, t := range c.Torrents {
		err := t.Api().LoginCtx(ctx)
		if err == nil {
			_, err = t.Api().GetAppVersionCtx(ctx)
		}
		results = append(results, HealthResult{
			Name:    t.Name(),
			Type:    "TorrentClient",
			Healthy: err == nil,
			Error:   err,
		})
	}

	return results
}

func (c *Client) FindSonarr(name string) SonarrInstance {
	for _, s := range c.Sonarr {
		if s.Name() == name {
			return s
		}
	}
	return nil
}

func (c *Client) FindRadarr(name string) RadarrInstance {
	for _, r := range c.Radarr {
		if r.Name() == name {
			return r
		}
	}
	return nil
}

func GetString(n sonarr.NullableString) string {
	if n.Get() == nil {
		return ""
	}
	return *n.Get()
}

func GetStringRadarr(n radarr.NullableString) string {
	if n.Get() == nil {
		return ""
	}
	return *n.Get()
}

func GetBoolRadarr(n radarr.NullableBool) bool {
	if n.Get() == nil {
		return false
	}
	return *n.Get()
}
