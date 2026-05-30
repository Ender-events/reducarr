package fsutil

import (
	"strings"
)

type PathMapping struct {
	Remote string
	Local  string
}

// MapPath rewrites a path based on a slice of remote->local mappings.
func MapPath(path string, mappings []PathMapping) string {
	for _, m := range mappings {
		if m.Remote != "" && strings.HasPrefix(path, m.Remote) {
			return strings.Replace(path, m.Remote, m.Local, 1)
		}
	}
	return path
}
