package web

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Ender-events/reducarr/internal/config"
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
	Name    string
	ArrType string
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

// paginationURL builds a /candidates URL for the given page and optional instance filter.
func paginationURL(page int, instance string) string {
	if instance != "" {
		return fmt.Sprintf("/candidates?page=%d&instance=%s", page, instance)
	}
	return fmt.Sprintf("/candidates?page=%d", page)
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
