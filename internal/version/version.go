// Package version holds build-time injected version metadata.
package version

import "fmt"

var (
	// Version is set at build time from the VERSION file.
	Version = "dev"
	// Commit is the short git commit hash at build time.
	Commit = "unknown"
	// BuildTime is the UTC build timestamp.
	BuildTime = "unknown"
)

// String returns a human-readable version line.
func String() string {
	return fmt.Sprintf("haovpn %s (commit %s, built %s)", Version, Commit, BuildTime)
}

// Info returns structured build metadata.
func Info() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_time": BuildTime,
	}
}
