package security

import (
	"time"

	"github.com/omarss/frodo-ci/internal/config"
)

// severityRank orders severities so thresholds can be compared. Unknown ("")
// ranks lowest so an unclassified finding never blocks on severity alone.
func severityRank(s string) int {
	switch s {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	}
	return 0
}

// Decision is the gate outcome for a module's scan: which findings block the
// build, which are reported only, and which were silenced by an active
// suppression. A build fails when Blocking is non-empty.
type Decision struct {
	Blocking   []Finding
	Reported   []Finding
	Suppressed []Finding
}

// Gate classifies findings against the active suppressions, the rulesets
// (explicit blocking / report-only), and the module's security profile. The
// precedence is: an active suppression silences a finding; an explicit
// report-only rule never blocks; an explicit blocking rule always blocks;
// otherwise the profile's severity/secret policy decides.
func Gate(findings []Finding, profile config.SecurityProfile, sups []config.Suppression, rs *config.Rulesets, now time.Time) Decision {
	var d Decision
	for _, f := range findings {
		switch {
		case IsSuppressed(f, sups, now):
			d.Suppressed = append(d.Suppressed, f)
		case rs != nil && contains(rs.ReportOnly, f.RuleID):
			d.Reported = append(d.Reported, f)
		case IsBlocking(f.RuleID, rs):
			d.Blocking = append(d.Blocking, f)
		case blocksByProfile(f, profile):
			d.Blocking = append(d.Blocking, f)
		default:
			d.Reported = append(d.Reported, f)
		}
	}
	return d
}

// blocksByProfile applies the profile's severity/secret policy to a finding.
func blocksByProfile(f Finding, p config.SecurityProfile) bool {
	if f.Kind == Secrets {
		return p.FailOnSecrets
	}
	switch f.Severity {
	case "critical":
		return p.FailOnNewCritical
	case "high":
		return p.FailOnNewHighWhenFixAvailable && f.FixAvailable
	}
	return false
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
