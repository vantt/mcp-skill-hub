// Package version exposes build metadata populated through linker flags.
package version

import "strings"

var (
	// Version, Commit, Date, and Dirty may be overridden with -ldflags -X.
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
	Dirty   = "unknown"
)

// Info is the build metadata displayed by delivery adapters.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
	Dirty   string `json:"dirty"`
}

// Current returns build metadata with safe values for local builds.
func Current() Info {
	return Info{
		Version: valueOrDefault(Version, "dev"),
		Commit:  valueOrDefault(Commit, "none"),
		Date:    valueOrDefault(Date, "unknown"),
		Dirty:   valueOrDefault(Dirty, "unknown"),
	}
}

func valueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
