package arrs

import (
	"context"

	"github.com/autobrr/go-qbittorrent"
	"github.com/devopsarr/radarr-go/radarr"
	"github.com/devopsarr/sonarr-go/sonarr"
)

type ArrInstance struct {
	Name   string
	URL    string
	APIKey string
}

type QBitConfig struct {
	URL      string
	Username string
	Password string
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
}

type RadarrInstance interface {
	Name() string
	ApiKey() string
	Api() *radarr.APIClient
}

type Client struct {
	Sonarr []SonarrInstance
	Radarr []RadarrInstance
	QBit   *qbittorrent.Client
}

type sonarrInst struct {
	name   string
	apiKey string
	api    *sonarr.APIClient
}

func (s *sonarrInst) Name() string           { return s.name }
func (s *sonarrInst) ApiKey() string         { return s.apiKey }
func (s *sonarrInst) Api() *sonarr.APIClient { return s.api }

type radarrInst struct {
	name   string
	apiKey string
	api    *radarr.APIClient
}

func (r *radarrInst) Name() string           { return r.name }
func (r *radarrInst) ApiKey() string         { return r.apiKey }
func (r *radarrInst) Api() *radarr.APIClient { return r.api }

func NewClient(sonarrConfigs, radarrConfigs []ArrInstance, qbitConfig *QBitConfig) *Client {
	c := &Client{}

	for _, cfg := range sonarrConfigs {
		sc := sonarr.NewConfiguration()
		sc.Servers = sonarr.ServerConfigurations{{URL: cfg.URL}}
		c.Sonarr = append(c.Sonarr, &sonarrInst{
			name:   cfg.Name,
			apiKey: cfg.APIKey,
			api:    sonarr.NewAPIClient(sc),
		})
	}

	for _, cfg := range radarrConfigs {
		rc := radarr.NewConfiguration()
		rc.Servers = radarr.ServerConfigurations{{URL: cfg.URL}}
		c.Radarr = append(c.Radarr, &radarrInst{
			name:   cfg.Name,
			apiKey: cfg.APIKey,
			api:    radarr.NewAPIClient(rc),
		})
	}

	if qbitConfig != nil {
		c.QBit = qbittorrent.NewClient(qbittorrent.Config{
			Host:     qbitConfig.URL,
			Username: qbitConfig.Username,
			Password: qbitConfig.Password,
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
	if c.QBit != nil {
		err := c.QBit.LoginCtx(ctx)
		if err == nil {
			_, err = c.QBit.GetAppVersionCtx(ctx)
		}
		results = append(results, HealthResult{
			Name:    "qBittorrent",
			Type:    "TorrentClient",
			Healthy: err == nil,
			Error:   err,
		})
	}

	return results
}
