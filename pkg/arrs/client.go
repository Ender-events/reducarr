package arrs

import (
	"context"

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
}

type RadarrInstance interface {
	Name() string
	ApiKey() string
	Api() *radarr.APIClient
	PathMappings() []fsutil.PathMapping
}

type TorrentInstance interface {
	Name() string
	Api() *qbittorrent.Client
	PathMappings() []fsutil.PathMapping
}

type Client struct {
	Sonarr   []SonarrInstance
	Radarr   []RadarrInstance
	Torrents []TorrentInstance
}

type sonarrInst struct {
	name     string
	apiKey   string
	api      *sonarr.APIClient
	mappings []fsutil.PathMapping
}

func (s *sonarrInst) Name() string                     { return s.name }
func (s *sonarrInst) ApiKey() string                   { return s.apiKey }
func (s *sonarrInst) Api() *sonarr.APIClient           { return s.api }
func (s *sonarrInst) PathMappings() []fsutil.PathMapping { return s.mappings }

type radarrInst struct {
	name     string
	apiKey   string
	api      *radarr.APIClient
	mappings []fsutil.PathMapping
}

func (r *radarrInst) Name() string                     { return r.name }
func (r *radarrInst) ApiKey() string                   { return r.apiKey }
func (r *radarrInst) Api() *radarr.APIClient           { return r.api }
func (r *radarrInst) PathMappings() []fsutil.PathMapping { return r.mappings }

type torrentInst struct {
	name     string
	api      *qbittorrent.Client
	mappings []fsutil.PathMapping
}

func (t *torrentInst) Name() string                     { return t.name }
func (t *torrentInst) Api() *qbittorrent.Client         { return t.api }
func (t *torrentInst) PathMappings() []fsutil.PathMapping { return t.mappings }

func NewClient(sonarrConfigs, radarrConfigs []ArrInstance, qbitConfigs []QBitConfig) *Client {
	c := &Client{}

	for _, cfg := range sonarrConfigs {
		sc := sonarr.NewConfiguration()
		sc.Servers = sonarr.ServerConfigurations{{URL: cfg.URL}}
		c.Sonarr = append(c.Sonarr, &sonarrInst{
			name:     cfg.Name,
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
