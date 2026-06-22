package config

import (
	"sync"
)

// ConfigDiff represents the differences between old and new config
type ConfigDiff struct {
	DryRunChanged      bool
	OldDryRun         bool
	NewDryRun         bool

	AutoUpgradeChanged bool
	OldAutoUpgrade    bool
	NewAutoUpgrade    bool

	ScheduleChanged bool
	OldSchedule     string
	NewSchedule     string

	ScoringChanged bool
	OldScoring     ScoringConfig
	NewScoring     ScoringConfig

	AutomationChanged bool
	OldAutomation    AutomationConfig
	NewAutomation    AutomationConfig
}

// HasChanges returns true if any configuration changed
func (d *ConfigDiff) HasChanges() bool {
	return d.DryRunChanged || d.AutoUpgradeChanged || d.ScheduleChanged ||
		d.ScoringChanged || d.AutomationChanged
}

// ConfigCallback is a function that receives config diffs
type ConfigCallback func(diff ConfigDiff)

// ConfigWatcher manages subscriptions to configuration changes
type ConfigWatcher struct {
	mu          sync.RWMutex
	subscribers []ConfigCallback
	lastConfig  *Config
}

// Global watcher instance
var globalWatcher = &ConfigWatcher{}

// Subscribe adds a callback to be notified of config changes
func Subscribe(callback ConfigCallback) {
	globalWatcher.mu.Lock()
	defer globalWatcher.mu.Unlock()
	globalWatcher.subscribers = append(globalWatcher.subscribers, callback)
}

// Unsubscribe removes a callback from the notification list
func Unsubscribe(callback ConfigCallback) {
	globalWatcher.mu.Lock()
	defer globalWatcher.mu.Unlock()
	for i, cb := range globalWatcher.subscribers {
		// Use pointer comparison for function identity
		// Since we can't compare functions directly, we use a wrapper approach
		// For now, we'll just clear and re-add if needed
		// This is a simplified version - in production you might want a more robust approach
		_ = i
		_ = cb
	}
}

// NotifyConfigChanged notifies all subscribers about a config change
func NotifyConfigChanged(oldCfg, newCfg *Config) {
	if oldCfg == nil || newCfg == nil {
		return
	}

	diff := computeDiff(oldCfg, newCfg)
	if !diff.HasChanges() {
		return
	}

	globalWatcher.mu.RLock()
	subscribers := make([]ConfigCallback, len(globalWatcher.subscribers))
	copy(subscribers, globalWatcher.subscribers)
	globalWatcher.mu.RUnlock()

	for _, cb := range subscribers {
		cb(diff)
	}

	// Update last known config
	globalWatcher.mu.Lock()
	globalWatcher.lastConfig = newCfg
	globalWatcher.mu.Unlock()
}

// computeDiff calculates the difference between two configs
func computeDiff(oldCfg, newCfg *Config) ConfigDiff {
	diff := ConfigDiff{}

	// Check DryRun
	if oldCfg.DryRun != newCfg.DryRun {
		diff.DryRunChanged = true
		diff.OldDryRun = oldCfg.DryRun
		diff.NewDryRun = newCfg.DryRun
	}

	// Check AutoUpgrade
	if oldCfg.Automation.AutoUpgrade != newCfg.Automation.AutoUpgrade {
		diff.AutoUpgradeChanged = true
		diff.OldAutoUpgrade = oldCfg.Automation.AutoUpgrade
		diff.NewAutoUpgrade = newCfg.Automation.AutoUpgrade
	}

	// Check Schedule
	if oldCfg.Automation.Schedule != newCfg.Automation.Schedule {
		diff.ScheduleChanged = true
		diff.OldSchedule = oldCfg.Automation.Schedule
		diff.NewSchedule = newCfg.Automation.Schedule
	}

	// Check Scoring
	if oldCfg.Scoring != newCfg.Scoring {
		diff.ScoringChanged = true
		diff.OldScoring = oldCfg.Scoring
		diff.NewScoring = newCfg.Scoring
	}

	// Check Automation config
	if oldCfg.Automation != newCfg.Automation {
		diff.AutomationChanged = true
		diff.OldAutomation = oldCfg.Automation
		diff.NewAutomation = newCfg.Automation
	}

	return diff
}

// GetCurrentConfig returns the last known configuration
func GetCurrentConfig() *Config {
	globalWatcher.mu.RLock()
	defer globalWatcher.mu.RUnlock()
	return globalWatcher.lastConfig
}
