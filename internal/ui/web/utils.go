package web

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Ender-events/reducarr/internal/config"
	"github.com/Ender-events/reducarr/internal/db"
	"github.com/Ender-events/reducarr/pkg/arrs"
)

func safeID(instance string, id int32) string {
	s := strings.ReplaceAll(instance, " ", "_")
	return fmt.Sprintf("candidate-%s-%d", s, id)
}

func extractSXXEXX(path string) string {
	filename := filepath.Base(path)
	re := regexp.MustCompile(`(?i)s(\d+)e(\d+)`)
	match := re.FindString(filename)
	if match != "" {
		return strings.ToUpper(match)
	}
	re2 := regexp.MustCompile(`(?i)(\d+)x(\d+)`)
	match2 := re2.FindString(filename)
	return strings.ToUpper(match2)
}

type ReleaseInfo struct {
	GUID       string
	Title      string
	Size       int64
	Indexer    string
	Seeders    int32
	Quality    string
	Score      int32
	Rejections []string
}

// getters for sorting compatibility.
func (r ReleaseInfo) GetRejections() []string {
	return r.Rejections
}
func (r ReleaseInfo) GetScore() int32 {
	return r.Score
}
func (r ReleaseInfo) GetSize() int64 {
	return r.Size
}

type InstanceInfo struct {
	Name             string
	ArrType          string
	OptimizableBytes int64 // estimated savings for this instance type (not per-instance, but grouped)
}

type NavStats struct {
	OptimizableBytes     int64 // total (all types)
	SonarrOptimizable    int64 // savings for sonarr episodes > 2GB
	RadarrOptimizable    int64 // savings for radarr films > 4GB
	UnreadErrors         int
	DownloadingCount     int
}

func buildNavStats(ctx context.Context, database *db.DB, client *arrs.Client) NavStats {
	var ns NavStats
	if database != nil {
		ns.OptimizableBytes, _ = database.GetOptimizableEstimatedSavings()
		ns.SonarrOptimizable, _ = database.GetOptimizableEstimatedSavingsByType("sonarr")
		ns.RadarrOptimizable, _ = database.GetOptimizableEstimatedSavingsByType("radarr")
		ns.UnreadErrors, _ = database.GetUnreadErrorsCount()
	}
	if client != nil {
		ns.DownloadingCount = client.GetTotalDownloadingCount(ctx)
	}
	return ns
}

func buildInstanceInfos() []InstanceInfo {
	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil {
		return nil
	}
	var out []InstanceInfo
	for _, s := range cfg.Sonarr {
		out = append(out, InstanceInfo{Name: s.Name, ArrType: "sonarr"})
	}
	for _, r := range cfg.Radarr {
		out = append(out, InstanceInfo{Name: r.Name, ArrType: "radarr"})
	}
	return out
}

// paginationURL builds a /candidates URL for the given page, optional instance filter, arrType, and showIgnored flag.
func paginationURL(page int, instance string, arrType string, showIgnored bool) string {
	u := fmt.Sprintf("/candidates?page=%d", page)
	if instance != "" {
		u += fmt.Sprintf("&instance=%s", instance)
	}
	if arrType != "" {
		u += fmt.Sprintf("&arr_type=%s", arrType)
	}
	if showIgnored {
		u += "&show_ignored=1"
	}
	return u
}

// paginationPages returns the list of page numbers to display, with 0 representing an ellipsis.
// Always shows first/last page and a window of ±2 around the current page.
func paginationPages(current, total int) []int {
	if total <= 7 {
		pages := make([]int, total)
		for i := range pages {
			pages[i] = i + 1
		}
		return pages
	}
	var pages []int
	addPage := func(p int) {
		if len(pages) > 0 && pages[len(pages)-1] == p {
			return
		}
		pages = append(pages, p)
	}
	addEllipsis := func() {
		if len(pages) > 0 && pages[len(pages)-1] != 0 {
			pages = append(pages, 0)
		}
	}
	addPage(1)
	if current > 4 {
		addEllipsis()
	}
	for p := current - 2; p <= current+2; p++ {
		if p > 1 && p < total {
			addPage(p)
		}
	}
	if current < total-3 {
		addEllipsis()
	}
	addPage(total)
	return pages
}
