// Package buildinfo holds build-time variables injected via -ldflags.
package buildinfo

import "runtime"

// These variables are set at build time via:
//
//	go build -ldflags="-X github.com/Ender-events/reducarr/internal/buildinfo.Version=..."
var (
	// Version is the git tag (e.g. v1.2.3 or v1.2.3-5-gabcdef).
	Version = "dev"

	// Commit is the short git commit SHA.
	Commit = "unknown"

	// BuildTime is the RFC3339 timestamp of the build.
	BuildTime = "unknown"
)

// GoVersion returns the Go runtime version used to compile the binary.
func GoVersion() string {
	return runtime.Version()
}
