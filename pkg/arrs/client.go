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

type Client struct {
	Sonarr []instance[sonarr.APIClient]
	Radarr []instance[radarr.APIClient]
	QBit   *qbittorrent.Client
}

type instance[T any] struct {
	name   string
	apiKey string
	api    *T
}

func NewClient(sonarrConfigs, radarrConfigs []ArrInstance, qbitConfig *QBitConfig) *Client {
	c := &Client{}

	for _, cfg := range sonarrConfigs {
		sc := sonarr.NewConfiguration()
		sc.Servers = sonarr.ServerConfigurations{{URL: cfg.URL}}
		c.Sonarr = append(c.Sonarr, instance[sonarr.APIClient]{
			name:   cfg.Name,
			apiKey: cfg.APIKey,
			api:    sonarr.NewAPIClient(sc),
		})
	}

	for _, cfg := range radarrConfigs {
		rc := radarr.NewConfiguration()
		rc.Servers = radarr.ServerConfigurations{{URL: cfg.URL}}
		c.Radarr = append(c.Radarr, instance[radarr.APIClient]{
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
			"X-Api-Key": {Key: s.apiKey},
		})
		_, _, err := s.api.SystemAPI.GetSystemStatus(authCtx).Execute()
		results = append(results, HealthResult{
			Name:    s.name,
			Type:    "Sonarr",
			Healthy: err == nil,
			Error:   err,
		})
	}

	// Check Radarr
	for _, r := range c.Radarr {
		authCtx := context.WithValue(ctx, radarr.ContextAPIKeys, map[string]radarr.APIKey{
			"X-Api-Key": {Key: r.apiKey},
		})
		_, _, err := r.api.SystemAPI.GetSystemStatus(authCtx).Execute()
		results = append(results, HealthResult{
			Name:    r.name,
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
