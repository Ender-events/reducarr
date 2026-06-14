package web

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
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
