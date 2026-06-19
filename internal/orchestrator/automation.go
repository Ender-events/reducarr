package orchestrator

import (
	"context"
	"fmt"
	"log"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/scan"
	"github.com/Ender-events/reducarr/internal/torrent"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

type AutomationManager struct {
	DB        *db.DB
	Verbose   bool
	scheduler gocron.Scheduler
}

func NewAutomationManager(database *db.DB, verbose bool) (*AutomationManager, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}

	return &AutomationManager{
		DB:        database,
		Verbose:   verbose,
		scheduler: s,
	}, nil
}

func (m *AutomationManager) Start(ctx context.Context) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if cfg.Automation.Schedule == "" {
		if m.Verbose {
			log.Printf("\033[34m[AUTO]\033[0m Automation disabled (no schedule provided).")
		}
		return nil
	}

	_, err = m.scheduler.NewJob(
		gocron.CronJob(cfg.Automation.Schedule, false),
		gocron.NewTask(func() {
			m.runIncrementalScan()
		}),
		gocron.WithEventListeners(
			gocron.BeforeJobRuns(func(jobID uuid.UUID, jobName string) {
				if m.Verbose {
					log.Printf("\033[34m[AUTO]\033[0m Starting scheduled incremental scan...")
				}
			}),
			gocron.AfterJobRuns(func(jobID uuid.UUID, jobName string) {
				if m.Verbose {
					log.Printf("\033[34m[AUTO]\033[0m Scheduled incremental scan complete.")
				}
			}),
		),
	)
	if err != nil {
		return fmt.Errorf("register job: %w", err)
	}

	m.scheduler.Start()
	log.Printf("\033[32m✔\033[0m Automation started with schedule: %s", cfg.Automation.Schedule)

	// Keep running until context is cancelled
	<-ctx.Done()
	return m.scheduler.Shutdown()
}

func (m *AutomationManager) runIncrementalScan() {
	// Reload config to get latest settings
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to reload config: %v", err)
		return
	}

	// Initialize dependencies
	client := m.getArrsClient(cfg)
	scorer := m.getScorer(cfg)
	uiLogger := ui.NewProgressLogger()

	// 1. Refresh Torrent Cache (Incremental)
	tScanner := torrent.NewScanner(client, m.DB, uiLogger, nil)
	tScanner.Verbose = m.Verbose
	tScanner.Incremental = true
	_ = tScanner.ScanAll(context.Background())

	// 2. Run Incremental Scan
	scanner := &scan.Scanner{
		Client:  client,
		DB:      m.DB,
		Scorer:  scorer,
		UI:      uiLogger,
		Verbose: m.Verbose,
	}

	if err := scanner.Incremental(context.Background()); err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Incremental scan failed: %v", err)
	}
}

func (m *AutomationManager) getArrsClient(cfg *config.Config) *arrs.Client {
	var sonarrInstances []arrs.ArrInstance
	for _, s := range cfg.Sonarr {
		sonarrInstances = append(sonarrInstances, arrs.ArrInstance{
			Name:         s.Name,
			URL:          s.URL,
			APIKey:       s.APIKey,
			PathMappings: s.PathMappings,
		})
	}

	var radarrInstances []arrs.ArrInstance
	for _, r := range cfg.Radarr {
		radarrInstances = append(radarrInstances, arrs.ArrInstance{
			Name:         r.Name,
			URL:          r.URL,
			APIKey:       r.APIKey,
			PathMappings: r.PathMappings,
		})
	}

	var qbitInstances []arrs.QBitConfig
	for _, q := range cfg.QBittorrent {
		qbitInstances = append(qbitInstances, arrs.QBitConfig{
			Name:         q.Name,
			URL:          q.URL,
			Username:     q.Username,
			Password:     q.Password,
			PathMappings: q.PathMappings,
			ReadOnly:     q.ReadOnly,
		})
	}

	return arrs.NewClient(sonarrInstances, radarrInstances, qbitInstances)
}

func (m *AutomationManager) getScorer(cfg *config.Config) *scan.Scorer {
	scorer := &scan.Scorer{}
	if cfg.Scoring.MaxRatio != "" {
		val, err := scan.ParseRatio(cfg.Scoring.MaxRatio)
		if err == nil {
			scorer.MaxRatio = val
		}
	}
	return scorer
}
