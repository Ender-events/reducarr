// Package sorting provides shared sorting utilities for release-like structures.
package sorting

import (
	"math"
	"sort"
	"strings"
)

// ReleaseLike defines the interface for types that can be sorted like releases.
// This allows sharing sort logic between different release representations.
type ReleaseLike interface {
	GetRejections() []string
	GetScore() int32
	GetSize() int64
}

// lessFunc is the core comparison logic for release sorting.
// It returns true if i should come before j.
func lessFunc(iRejections []string, iScore int32, iSize int64,
	jRejections []string, jScore int32, jSize int64) bool {
	// 1. No rejections first
	iApproved := len(iRejections) == 0
	jApproved := len(jRejections) == 0
	if iApproved != jApproved {
		return iApproved
	}

	// 2. Lower rejection severity
	si := RejectionSeverity(iRejections)
	sj := RejectionSeverity(jRejections)
	if si != sj {
		return si < sj
	}

	// 3. Higher Custom Format Score
	if iScore != jScore {
		return iScore > jScore
	}

	// 4. Smaller size
	return iSize < jSize
}

// Sort sorts a slice of ReleaseLike items using the standard release sorting criteria:
// 1. No rejections first
// 2. Lower rejection severity
// 3. Higher Custom Format Score
// 4. Smaller size
func Sort[T ReleaseLike](items []T) {
	sort.Slice(items, func(i, j int) bool {
		return lessFunc(
			items[i].GetRejections(), items[i].GetScore(), items[i].GetSize(),
			items[j].GetRejections(), items[j].GetScore(), items[j].GetSize(),
		)
	})
}

// RejectionSeverity returns the severity level of rejections.
func RejectionSeverity(rejections []string) int {
	if len(rejections) == 0 {
		return 0 // Approved
	}

	hasGeneral := false
	for _, r := range rejections {
		if strings.Contains(r, "Unknown Movie") || strings.Contains(r, "Wrong episode") {
			return 3 // Absolute worst
		}
		if !strings.Contains(r, "does not allow upgrades") && !strings.Contains(r, "equal or higher preference") {
			hasGeneral = true
		}
	}

	if hasGeneral {
		return 2
	}
	return 1
}

// GetScore returns the score value, treating nil/zero as MinInt32.
// This is a helper for types that have nullable scores.
func GetScore(s *int32) int32 {
	if s == nil {
		return math.MinInt32
	}
	return *s
}
