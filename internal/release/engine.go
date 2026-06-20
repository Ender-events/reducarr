package release

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Release unifies Sonarr and Radarr release resources for scoring and sorting.
type Release struct {
	Title      string
	Size       int64
	Indexer    string
	Seeders    int32
	Quality    string
	Protocol   string
	Rejections []string
	Score      *int32
	Raw        any // Pointer to the original sonarr/radarr struct
}

// Engine handles the sorting and automated selection of releases.
type Engine struct {
	MinSizeReduction string
	MinSeeders       int32
}

// NewEngine creates a new Selection Engine.
func NewEngine(minSizeReduction string, minSeeders int32) *Engine {
	return &Engine{
		MinSizeReduction: minSizeReduction,
		MinSeeders:       minSeeders,
	}
}

// Sort sorts releases in-place, prioritizing:
// 1. No rejections first.
// 2. Higher Custom Format Score.
// 3. Lower rejection severity.
// 4. Smaller size (to achieve space reduction).
func (e *Engine) Sort(releases []Release) {
	sort.Slice(releases, func(i, j int) bool {
		iApproved := len(releases[i].Rejections) == 0
		jApproved := len(releases[j].Rejections) == 0
		if iApproved != jApproved {
			return iApproved
		}

		ci := getScore(releases[i].Score)
		cj := getScore(releases[j].Score)
		if ci != cj {
			return ci > cj
		}

		si := getRejectionSeverity(releases[i].Rejections)
		sj := getRejectionSeverity(releases[j].Rejections)
		if si != sj {
			return si < sj
		}

		return releases[i].Size < releases[j].Size
	})
}

// EvaluateUpgrade checks if the best candidate release qualifies as an upgrade over the current file.
func (e *Engine) EvaluateUpgrade(best Release, currentSize int64, currentScore int32) (bool, string) {
	// 1. Rejection check - absolute block for automated upgrade if has severity > 1
	severity := getRejectionSeverity(best.Rejections)
	if severity > 1 {
		return false, fmt.Sprintf("release has severe rejection(s): %s", strings.Join(best.Rejections, ", "))
	}

	// 2. Seeders check
	if best.Seeders < e.MinSeeders {
		return false, fmt.Sprintf("seeders (%d) is below minimum allowed (%d)", best.Seeders, e.MinSeeders)
	}

	// 4. Custom Format Score check - we should not downgrade Custom Format Score unless current score is MinInt32 (no score)
	bestScore := getScore(best.Score)
	if currentScore != math.MinInt32 && bestScore < currentScore {
		return false, fmt.Sprintf("release custom format score (%d) is lower than current score (%d)", bestScore, currentScore)
	}

	// 5. Size Reduction check
	qualifies, reason := e.checkSizeReduction(best.Size, currentSize)
	if !qualifies {
		return false, reason
	}

	return true, "qualifies for auto-upgrade"
}

func (e *Engine) checkSizeReduction(bestSize, currentSize int64) (bool, string) {
	if bestSize >= currentSize {
		return false, fmt.Sprintf("release size (%d bytes) is not smaller than current size (%d bytes)", bestSize, currentSize)
	}

	diffBytes := currentSize - bestSize
	spec := strings.TrimSpace(strings.ToLower(e.MinSizeReduction))
	if spec == "" || spec == "0" {
		return true, ""
	}

	// Percentage check
	if strings.HasSuffix(spec, "%") {
		pctStr := strings.TrimSuffix(spec, "%")
		pct, err := strconv.ParseFloat(pctStr, 64)
		if err != nil {
			return true, "" // Default to true if unparseable
		}
		actualPct := (float64(diffBytes) / float64(currentSize)) * 100
		if actualPct < pct {
			return false, fmt.Sprintf("size reduction (%.2f%%) is below minimum required (%s)", actualPct, spec)
		}
		return true, ""
	}

	// Absolute size check (e.g. 500mib, 1gib)
	requiredBytes, err := parseBytes(spec)
	if err != nil {
		return true, "" // Default to true if unparseable
	}

	if diffBytes < requiredBytes {
		return false, fmt.Sprintf("size reduction (%d bytes) is below minimum required (%d bytes)", diffBytes, requiredBytes)
	}

	return true, ""
}

func getScore(s *int32) int32 {
	if s == nil {
		return math.MinInt32
	}
	return *s
}

func getRejectionSeverity(rejections []string) int {
	if len(rejections) == 0 {
		return 0 // Approved
	}

	hasGeneral := false
	for _, r := range rejections {
		if strings.Contains(r, "Unknown Movie") {
			return 3 // Absolute worst
		}
		if !strings.Contains(r, "does not allow upgrades") {
			hasGeneral = true
		}
	}

	if hasGeneral {
		return 2
	}
	return 1
}

func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "gib") || strings.HasSuffix(s, "gb"):
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-3]
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "mib") || strings.HasSuffix(s, "mb"):
		multiplier = 1024 * 1024
		s = s[:len(s)-3]
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "kib") || strings.HasSuffix(s, "kb"):
		multiplier = 1024
		s = s[:len(s)-3]
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "b"):
		s = s[:len(s)-1]
	}

	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, err
	}
	return int64(val * float64(multiplier)), nil
}
