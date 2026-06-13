package web

import (
	"path/filepath"
	"regexp"
	"strings"
)

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
