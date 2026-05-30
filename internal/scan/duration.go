package scan

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseDuration parses a string like "00:45:00" or "45:00" into seconds.
func ParseDuration(s string) (float64, error) {
	if s == "" {
		return 0, nil
	}
	parts := strings.Split(s, ":")
	var totalSeconds float64

	switch len(parts) {
	case 1: // seconds only?
		val, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		totalSeconds = val
	case 2: // MM:SS
		m, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		s, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		totalSeconds = m*60 + s
	case 3: // HH:MM:SS
		h, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, err
		}
		m, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, err
		}
		s, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, err
		}
		totalSeconds = h*3600 + m*60 + s
	default:
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	return totalSeconds, nil
}
