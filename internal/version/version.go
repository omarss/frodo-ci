// Package version exposes build-time version metadata for the frodo-ci binary.
package version

import (
	"fmt"
	"runtime"
)

// These values are injected at build time via -ldflags (see the Makefile).
// They intentionally default to recognizable placeholders for `go run` builds.
var (
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

// Info returns a human-readable one-line version string.
func Info() string {
	return fmt.Sprintf("%s (commit %s, built %s, %s)",
		Version, Commit, BuildDate, runtime.Version())
}
