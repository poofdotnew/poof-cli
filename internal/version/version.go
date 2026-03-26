package version

import (
	"fmt"
	"runtime"
)

// These are set at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info returns a formatted multi-line version string.
func Info() string {
	return fmt.Sprintf(
		"poof %s\n  commit:  %s\n  built:   %s\n  go:      %s\n  os/arch: %s/%s",
		Version, Commit, Date, runtime.Version(), runtime.GOOS, runtime.GOARCH,
	)
}

// Short returns just the version string.
func Short() string {
	return Version
}
