package security

import (
	"time"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/match"
)

// Finding is a single security finding to be classified or suppressed.
type Finding struct {
	RuleID       string
	Path         string
	Line         int
	Severity     string // critical | high | medium | low | "" (unknown)
	Message      string
	Tool         string
	Kind         ScanType
	FixAvailable bool
}

// IsSuppressed reports whether an active suppression covers the finding. A
// suppression with no expiry, or an expired one, never suppresses (the linter
// rejects those separately).
func IsSuppressed(f Finding, sups []config.Suppression, now time.Time) bool {
	for _, s := range sups {
		if s.Expiry.IsZero() || !s.Expiry.After(now) {
			continue
		}
		if s.ID != "" && s.ID != f.RuleID {
			continue
		}
		if s.Path != "" && !match.Glob(s.Path, f.Path) {
			continue
		}
		return true
	}
	return false
}

// IsBlocking reports whether a finding's rule is in the blocking ruleset.
func IsBlocking(ruleID string, rs *config.Rulesets) bool {
	if rs == nil {
		return false
	}
	for _, b := range rs.Blocking {
		if b == ruleID {
			return true
		}
	}
	return false
}
