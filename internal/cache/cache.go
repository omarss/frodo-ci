// Package cache stores fingerprint markers so an expensive stage can be skipped
// on an exact fingerprint match. The cache is only ever an optimization: it can
// skip expensive work, never reviews, security governance, protected-file
// checks, or anti-weakening checks.
//
// On GitHub Actions the local cache directory is persisted across runs by the
// workflow's actions/cache step (pointed at FRODO_CI_CACHE_DIR), so we get
// native Actions caching without re-implementing the cache service protocol.
package cache

import (
	"os"
	"path/filepath"
)

// Entry records that a stage completed with a given fingerprint.
type Entry struct {
	Fingerprint string   `json:"fingerprint"`
	Module      string   `json:"module"`
	Stage       string   `json:"stage"`
	Conclusion  string   `json:"conclusion"`
	SavedAtUnix int64    `json:"saved_at_unix"`
	Outputs     []string `json:"outputs,omitempty"`
	// DurationMs is how long the stage took when it last ran, so a later cache
	// hit can report the time it saved.
	DurationMs int64 `json:"duration_ms,omitempty"`
}

// Cache stores and looks up fingerprint entries.
type Cache interface {
	// Has reports whether an exact fingerprint match exists.
	Has(fingerprint string) (bool, error)
	// Restore returns the cached entry for a fingerprint, if present.
	Restore(fingerprint string) (Entry, bool, error)
	// Save records an entry. Saving is best-effort and must not be required for
	// correctness.
	Save(entry Entry) error
	// SaveOutputs archives the given paths (relative to baseDir) under the
	// fingerprint so a later hit restores them instead of rebuilding. Best-effort.
	SaveOutputs(fingerprint, baseDir string, paths []string) error
	// RestoreOutputs extracts a fingerprint's archived outputs into baseDir,
	// reporting whether an archive was present.
	RestoreOutputs(fingerprint, baseDir string) (bool, error)
}

// Noop is a cache that never hits; used when caching is disabled.
type Noop struct{}

func (Noop) Has(string) (bool, error)                    { return false, nil }
func (Noop) Restore(string) (Entry, bool, error)         { return Entry{}, false, nil }
func (Noop) Save(Entry) error                            { return nil }
func (Noop) SaveOutputs(string, string, []string) error  { return nil }
func (Noop) RestoreOutputs(string, string) (bool, error) { return false, nil }

// Open selects a cache backend. When caching is disabled it returns Noop.
func Open(enabled bool, dir string) (Cache, error) {
	if !enabled {
		return Noop{}, nil
	}
	if dir == "" {
		dir = DefaultDir()
	}
	return NewLocalCache(dir)
}

// DefaultDir picks a cache directory, preferring one that a CI workflow can
// persist with actions/cache.
func DefaultDir() string {
	if d := os.Getenv("FRODO_CI_CACHE_DIR"); d != "" {
		return d
	}
	if rt := os.Getenv("RUNNER_TEMP"); rt != "" {
		return filepath.Join(rt, "frodo-ci-cache")
	}
	if uc, err := os.UserCacheDir(); err == nil {
		return filepath.Join(uc, "frodo-ci")
	}
	return filepath.Join(os.TempDir(), "frodo-ci-cache")
}
