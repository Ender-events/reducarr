package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Ender-events/reducarr/internal/buildinfo"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/pkg/arrs"
)

// checkDatabase verifies database connectivity
func checkDatabase(database *db.DB) (healthy bool, errMsg string) {
	if database == nil {
		return false, "database not initialized"
	}
	err := database.Ping()
	if err != nil {
		return false, err.Error()
	}
	return true, ""
}

// formatHealthLine formats a single health status line
func formatHealthLine(status, name, value string, errMsg string) string {
	line := fmt.Sprintf("[%s] %-18s: %s", status, name, value)
	if errMsg != "" {
		line += fmt.Sprintf(" (Error: %s)", errMsg)
	}
	return line
}

// getUptime returns formatted uptime duration
func getUptime() string {
	if startTime.IsZero() {
		return "unknown"
	}
	duration := time.Since(startTime)

	// Format: 2d 3h 45m or 45m or 2h etc.
	days := int(duration.Hours()) / 24
	hours := int(duration.Hours()) % 24
	minutes := int(duration.Minutes()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}

	return strings.Join(parts, " ")
}

func HealthCheckHandler(w http.ResponseWriter, r *http.Request, database *db.DB, client *arrs.Client) {

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	var lines []string
	coreServicesHealthy := true

	// Database check (core service)
	dbHealthy, dbErr := checkDatabase(database)
	if dbHealthy {
		lines = append(lines, formatHealthLine("HEALTHY", "Database", "Connected", ""))
	} else {
		lines = append(lines, formatHealthLine("FAILED", "Database", "Disconnected", dbErr))
		coreServicesHealthy = false
	}

	// Config check (core service - always loaded if we got here)
	lines = append(lines, formatHealthLine("HEALTHY", "Config", "Loaded", ""))

	// Arr services check (core service if any are configured)
	if client != nil {
		hasArrServices := len(client.Sonarr) > 0 || len(client.Radarr) > 0 || len(client.Torrents) > 0
		arrHealthy := true

		results := client.HealthCheck(r.Context())
		for _, res := range results {
			status := "HEALTHY"
			errMsg := ""
			if !res.Healthy {
				status = "FAILED"
				arrHealthy = false
				if res.Error != nil {
					errMsg = res.Error.Error()
				}
			}
			lines = append(lines, formatHealthLine(status, res.Type, res.Name, errMsg))
		}

		// If we have Arr services configured and all failed, mark core as unhealthy
		if hasArrServices && !arrHealthy {
			coreServicesHealthy = false
		}
	}

	// Scan status (info only, doesn't affect health)
	if globalScanManager.IsRunning() {
		lines = append(lines, formatHealthLine("HEALTHY", "Scan Status", "Running", ""))
	} else {
		stats, _ := database.GetDashboardStats()
		if stats.LastScanTime != "" && stats.LastScanTime != "Never" {
			lines = append(lines, formatHealthLine("HEALTHY", "Scan Status", fmt.Sprintf("Idle (Last: %s)", stats.LastScanTime), ""))
		} else {
			lines = append(lines, formatHealthLine("HEALTHY", "Scan Status", "Idle (Never)", ""))
		}
	}

	// Queue status (info only)
	stats, _ := database.GetDashboardStats()
	lines = append(lines, formatHealthLine("HEALTHY", "Queue", fmt.Sprintf("%d candidates pending", stats.PendingCandidates), ""))

	// Uptime (info only)
	lines = append(lines, formatHealthLine("HEALTHY", "Uptime", getUptime(), ""))

	// Version info (info only)
	commitHash := buildinfo.Commit
	if len(commitHash) > 8 {
		commitHash = commitHash[:8]
	}
	lines = append(lines, formatHealthLine("HEALTHY", "Version", fmt.Sprintf("%s (%s)", buildinfo.Version, commitHash), ""))

	// Write all lines
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}

	// Set HTTP status code based on core services
	if !coreServicesHealthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
}
