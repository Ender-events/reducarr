package orchestrator

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/internal/release"
	"github.com/Ender-events/reducarr/internal/scan"
	"github.com/Ender-events/reducarr/internal/torrent"
	"github.com/Ender-events/reducarr/internal/ui"
	"github.com/Ender-events/reducarr/pkg/arrs"
	"github.com/dustin/go-humanize"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

type AutomationManager struct {
	DB              *db.DB
	scheduler       gocron.Scheduler
	Verbose         bool
	DryRun          bool
	AutoUpgrade     bool
	LoopBeforeRetry map[int32]struct{}
	Config          *config.Config

	rateLimitMu sync.Mutex
	tokens      float64
	lastRefill  time.Time
	cronJob     gocron.Job
	retryJob    gocron.Job
}

func NewAutomationManager(database *db.DB, verbose bool, dryRun bool, autoUpgrade bool) (*AutomationManager, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, fmt.Errorf("create scheduler: %w", err)
	}

	m := &AutomationManager{
		DB:              database,
		Verbose:         verbose,
		scheduler:       s,
		DryRun:          dryRun,
		AutoUpgrade:     autoUpgrade,
		LoopBeforeRetry: make(map[int32]struct{}),
	}

	// Subscribe to config changes
	config.Subscribe(m.handleConfigChange)

	return m, nil
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

	job, err := m.scheduler.NewJob(
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
	m.cronJob = job

	m.scheduler.Start()
	log.Printf("\033[32m✔\033[0m Automation started with schedule: %s", cfg.Automation.Schedule)
	nextRun, err := m.cronJob.NextRun()
	if err != nil {
		return fmt.Errorf("get next run: %w", err)
	} else {
		log.Printf("\033[34m[AUTO]\033[0m Next scheduled run: %s", nextRun.Format(time.RFC3339))
	}

	// Keep running until context is cancelled
	<-ctx.Done()
	return m.scheduler.Shutdown()
}

func (m *AutomationManager) runIncrementalScan() {
	m.rateLimitMu.Lock()
	if m.retryJob != nil {
		_ = m.scheduler.RemoveJob(m.retryJob.ID())
		m.retryJob = nil
	}
	m.rateLimitMu.Unlock()

	// Reload config to get latest settings
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to reload config: %v", err)
		return
	}

	// Acquire scan lock
	acquired, err := m.DB.AcquireScanLock("global_scan", os.Getpid(), "scheduler", 3600)
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to check/acquire scan lock: %v", err)
		return
	}
	if !acquired {
		log.Printf("\033[34m[AUTO]\033[0m Scan skipped: Another scan is currently in progress.")
		return
	}
	defer func() {
		_ = m.DB.ReleaseScanLock("global_scan")
	}()

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

	if m.AutoUpgrade {
		if m.Verbose {
			log.Printf("\033[34m[AUTO]\033[0m Auto-upgrade is enabled. Processing candidates...")
		}
		m.processAutoUpgrades(context.Background(), cfg, client)
	}
}

func (m *AutomationManager) processAutoUpgrades(ctx context.Context, cfg *config.Config, client *arrs.Client) {
	candidates, err := m.DB.GetCandidatesWithMedia()
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to fetch candidates: %v", err)
		return
	}

	if len(candidates) == 0 {
		if m.Verbose {
			log.Printf("\033[34m[AUTO]\033[0m No candidates found for upgrade.")
		}
		return
	}

	// Create selection engine with configuration
	engine := release.NewEngine(cfg.Automation.MinSizeReduction, cfg.Automation.MinSeeders)
	orch := New(m.DB, client, m.DryRun, m.Verbose)

	for _, cand := range candidates {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if cand.IsIgnored {
			continue
		}

		if m.Verbose {
			log.Printf("\033[34m[AUTO]\033[0m Evaluating candidate: %s (Current size: %s)", cand.Title, humanize.Bytes(uint64(cand.Size)))
		}

		// Check minSeedDuration
		// Fetch torrent info associated with this candidate's inode
		torrentRecords, err := m.DB.GetTorrentsByInode(cand.Inode)
		if err != nil {
			log.Printf("\033[31m✘\033[0m [AUTO] Failed to fetch torrents for %s: %v", cand.Title, err)
			continue
		}

		// Calculate min seed duration
		var minSeedDuration time.Duration
		if cfg.Scoring.MinSeedDuration != "" {
			minSeedDuration, err = time.ParseDuration(cfg.Scoring.MinSeedDuration)
			if err != nil {
				// Default fallback
				minSeedDuration = 336 * time.Hour // 2 weeks
			}
		}

		// Check if any associated torrent is still seeding and has not met the min seed duration
		tooYoung := false
		for _, t := range torrentRecords {
			if t.IsSeeding {
				// AddedAt is epoch seconds
				addedAt := time.Unix(t.AddedAt, 0)
				if time.Since(addedAt) < minSeedDuration {
					tooYoung = true
					log.Printf("\033[34m[AUTO]\033[0m Skipping candidate %s: Torrent %s has not reached min seed duration (%v remaining)", cand.Title, t.InfoHash[:8], minSeedDuration-time.Since(addedAt))
					break
				}
			}
		}

		if tooYoung {
			continue
		}

		if _, exists := m.LoopBeforeRetry[cand.ItemID]; exists {
			continue
		}

		// Check rate limit
		allowed, timeNeeded := m.takeToken(cfg.Automation.RateLimit)
		if !allowed {
			log.Printf("\033[33m⚠\033[0m [AUTO] Rate limit reached. Need to wait %v for next token. Exiting loop.", timeNeeded)
			m.scheduleRetry(timeNeeded)
			return
		}
		m.LoopBeforeRetry[cand.ItemID] = struct{}{}

		// Fetch releases for this candidate
		var releases []release.Release
		if cand.ArrType == "sonarr" {
			inst := client.FindSonarr(cand.ArrInstance)
			if inst == nil {
				log.Printf("\033[31m✘\033[0m [AUTO] Sonarr instance %s not found for candidate %s", cand.ArrInstance, cand.Title)
				continue
			}

			var fetchErr error
			releases, fetchErr = fetchSonarrReleases(ctx, inst, cand)
			if fetchErr != nil {
				log.Printf("\033[31m✘\033[0m [AUTO] Failed to fetch releases for %s: %v", cand.Title, fetchErr)
				continue
			}
		} else { // radarr
			inst := client.FindRadarr(cand.ArrInstance)
			if inst == nil {
				log.Printf("\033[31m✘\033[0m [AUTO] Radarr instance %s not found for candidate %s", cand.ArrInstance, cand.Title)
				continue
			}

			var fetchErr error
			releases, fetchErr = fetchRadarrReleases(ctx, inst, cand)
			if fetchErr != nil {
				log.Printf("\033[31m✘\033[0m [AUTO] Failed to fetch releases for %s: %v", cand.Title, fetchErr)
				continue
			}
		}

		if len(releases) == 0 {
			if m.Verbose {
				log.Printf("\033[34m[AUTO]\033[0m No releases found for candidate %s", cand.Title)
			}
			continue
		}

		engine.Sort(releases)
		best := releases[0]
		currentScore := int32(math.MinInt32)

		qualifies, reason := engine.EvaluateUpgrade(best, cand.Size, currentScore)
		if !qualifies {
			if m.Verbose {
				log.Printf("\033[34m[AUTO]\033[0m Release '%s' rejected: %s", best.Title, reason)
			}
			continue
		}

		log.Printf("\033[32m✔\033[0m [AUTO] Upgrading candidate %s to: %s (Size: %s, Saved: %s)", cand.Title, best.Title, humanize.Bytes(uint64(best.Size)), humanize.Bytes(uint64(cand.Size-best.Size)))

		// Execute upgrade
		if err := orch.UpgradeCandidate(ctx, cand, best.Raw); err != nil {
			log.Printf("\033[31m✘\033[0m [AUTO] Upgrade failed for %s: %v", cand.Title, err)
		} else {
			log.Printf("\033[32m✔\033[0m [AUTO] Upgrade succeeded for %s", cand.Title)
		}
	}
	clear(m.LoopBeforeRetry)
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

func fetchSonarrReleases(ctx context.Context, inst arrs.SonarrInstance, cand db.CandidateRecord) ([]release.Release, error) {
	episodes, err := inst.ListEpisodes(ctx, cand.ItemID)
	if err != nil {
		return nil, fmt.Errorf("list episodes: %w", err)
	}

	var episodeID int32
	for _, ep := range episodes {
		if ep.EpisodeFileId != nil && *ep.EpisodeFileId == cand.FileID {
			if ep.Id != nil {
				episodeID = *ep.Id
				break
			}
		}
	}

	if episodeID == 0 {
		return nil, fmt.Errorf("could not find episode ID for file ID %d", cand.FileID)
	}

	rawReleases, err := inst.ListReleases(ctx, &episodeID, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	releases := make([]release.Release, len(rawReleases))
	for i, r := range rawReleases {
		quality := ""
		if r.Quality != nil && r.Quality.Quality != nil {
			quality = arrs.GetString(r.Quality.Quality.Name)
		}
		seeders := int32(0)
		if r.Seeders.Get() != nil {
			seeders = *r.Seeders.Get()
		}
		protocol := "unknown"
		if r.Protocol != nil {
			protocol = string(*r.Protocol)
		}
		releases[i] = release.Release{
			Title:      arrs.GetString(r.Title),
			Size:       *r.Size,
			Indexer:    arrs.GetString(r.Indexer),
			Seeders:    seeders,
			Quality:    quality,
			Protocol:   protocol,
			Rejections: r.Rejections,
			Score:      r.CustomFormatScore,
			Raw:        &rawReleases[i],
		}
	}
	return releases, nil
}

func fetchRadarrReleases(ctx context.Context, inst arrs.RadarrInstance, cand db.CandidateRecord) ([]release.Release, error) {
	_ = inst.TriggerMovieSearch(ctx, cand.ItemID)

	rawReleases, err := inst.ListReleases(ctx, cand.ItemID)
	if err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	releases := make([]release.Release, len(rawReleases))
	for i, r := range rawReleases {
		quality := ""
		if r.Quality != nil && r.Quality.Quality != nil {
			quality = arrs.GetStringRadarr(r.Quality.Quality.Name)
		}
		seeders := int32(0)
		if r.Seeders.Get() != nil {
			seeders = *r.Seeders.Get()
		}
		protocol := "unknown"
		if r.Protocol != nil {
			protocol = string(*r.Protocol)
		}
		releases[i] = release.Release{
			Title:      arrs.GetStringRadarr(r.Title),
			Size:       *r.Size,
			Indexer:    arrs.GetStringRadarr(r.Indexer),
			Seeders:    seeders,
			Quality:    quality,
			Protocol:   protocol,
			Rejections: r.Rejections,
			Score:      r.CustomFormatScore,
			Raw:        &rawReleases[i],
		}
	}
	return releases, nil
}

func (m *AutomationManager) takeToken(rateLimit int) (bool, time.Duration) {
	if rateLimit <= 0 {
		return true, 0
	}

	m.rateLimitMu.Lock()
	defer m.rateLimitMu.Unlock()

	now := time.Now()
	if m.lastRefill.IsZero() {
		m.tokens = float64(rateLimit)
		m.lastRefill = now
	} else {
		elapsed := now.Sub(m.lastRefill)
		// Refill rate is rateLimit per hour (3600 seconds)
		refill := elapsed.Seconds() * (float64(rateLimit) / 3600.0)
		m.tokens += refill
		if m.tokens > float64(rateLimit) {
			m.tokens = float64(rateLimit)
		}
		m.lastRefill = now
	}

	if m.tokens >= 1.0 {
		m.tokens -= 1.0
		return true, 0
	}

	// Calculate time needed to get 1 token
	timeNeeded := (1.0 - m.tokens) / (float64(rateLimit) / 3600.0)
	return false, time.Duration(timeNeeded * float64(time.Second))
}

func (m *AutomationManager) scheduleRetry(timeNeeded time.Duration) {
	m.rateLimitMu.Lock()
	defer m.rateLimitMu.Unlock()

	// Clean up previous retry job if any
	if m.retryJob != nil {
		_ = m.scheduler.RemoveJob(m.retryJob.ID())
		m.retryJob = nil
	}

	if m.cronJob == nil {
		return
	}

	nextCron, err := m.cronJob.NextRun()
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to get next cron run time: %v", err)
		return
	}

	retryTime := time.Now().Add(timeNeeded)
	if nextCron.Before(retryTime) {
		if m.Verbose {
			log.Printf("\033[34m[AUTO]\033[0m Next cron run is at %v, which is before rate limit reset (%v). Skipping retry scheduling.", nextCron, retryTime)
		}
		return
	}

	log.Printf("\033[34m[AUTO]\033[0m Scheduling one-time retry in %v (at %v)", timeNeeded, retryTime)
	job, err := m.scheduler.NewJob(
		gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(retryTime)),
		gocron.NewTask(func() {
			m.runIncrementalScan()
		}),
	)
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to schedule retry job: %v", err)
		return
	}
	m.retryJob = job
}

// handleConfigChange is called when configuration changes
func (m *AutomationManager) handleConfigChange(diff config.ConfigDiff) {
	if m.Verbose {
		log.Printf("\033[34m[CONFIG]\033[0m Configuration changed, applying updates...")
	}

	// Update internal state based on diff
	if diff.DryRunChanged {
		m.DryRun = diff.NewDryRun
		if m.Verbose {
			log.Printf("\033[34m[CONFIG]\033[0m DryRun changed from %v to %v", diff.OldDryRun, diff.NewDryRun)
		}
	}

	if diff.AutoUpgradeChanged {
		m.AutoUpgrade = diff.NewAutoUpgrade
		if m.Verbose {
			log.Printf("\033[34m[CONFIG]\033[0m AutoUpgrade changed from %v to %v", diff.OldAutoUpgrade, diff.NewAutoUpgrade)
		}
	}

	// Update cron job schedule if it changed
	if diff.ScheduleChanged {
		m.updateCronSchedule(diff.NewSchedule)
	}

	// Reload config reference
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("\033[31m✘\033[0m [CONFIG] Failed to reload config: %v", err)
		return
	}
	m.Config = cfg

	if m.Verbose {
		log.Printf("\033[32m✔\033[0m [CONFIG] Configuration update complete")
	}
}

// updateCronSchedule updates the cron job schedule
func (m *AutomationManager) updateCronSchedule(newSchedule string) {
	// Remove existing cron job if any
	if m.cronJob != nil {
		if err := m.scheduler.RemoveJob(m.cronJob.ID()); err != nil {
			log.Printf("\033[31m✘\033[0m [AUTO] Failed to remove old cron job: %v", err)
			return
		}
		m.cronJob = nil
	}

	// If new schedule is empty, automation is disabled
	if newSchedule == "" {
		log.Printf("\033[34m[AUTO]\033[0m Automation disabled (no schedule provided).")
		return
	}

	// Create new cron job with updated schedule
	job, err := m.scheduler.NewJob(
		gocron.CronJob(newSchedule, false),
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
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to create new cron job: %v", err)
		return
	}
	m.cronJob = job

	log.Printf("\033[32m✔\033[0m Automation schedule updated to: %s", newSchedule)
	nextRun, err := m.cronJob.NextRun()
	if err != nil {
		log.Printf("\033[31m✘\033[0m [AUTO] Failed to get next run: %v", err)
	} else {
		log.Printf("\033[34m[AUTO]\033[0m Next scheduled run: %s", nextRun.Format(time.RFC3339))
	}
}
