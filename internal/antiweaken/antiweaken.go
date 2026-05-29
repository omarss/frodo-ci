// Package antiweaken compares base-branch config with PR config and reports
// changes that weaken governance. Weakening changes require higher approval.
package antiweaken

import (
	"fmt"

	"github.com/frodo-ci/frodo-ci/internal/config"
)

// Weakening is a single governance-weakening change.
type Weakening struct {
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

// Root reports weakenings between two root configs.
func Root(path string, base, head *config.RootConfig) []Weakening {
	var w []Weakening
	add := func(detail string) { w = append(w, Weakening{path, detail}) }

	flag := func(was, now bool, name string) {
		if was && !now {
			add("disabled " + name)
		}
	}
	flag(base.Reviews.IgnoreAuthorApproval, head.Reviews.IgnoreAuthorApproval, "reviews.ignore_author_approval")
	flag(base.Reviews.IgnoreBotApproval, head.Reviews.IgnoreBotApproval, "reviews.ignore_bot_approval")
	flag(base.Reviews.RequireApprovalAfterLatestCommit, head.Reviews.RequireApprovalAfterLatestCommit, "reviews.require_approval_after_latest_commit")
	flag(base.Experts.Enabled, head.Experts.Enabled, "experts.enabled")
	flag(base.Experts.RequireWriteAccess, head.Experts.RequireWriteAccess, "experts.require_write_access")
	flag(base.Security.FailOnNewCritical, head.Security.FailOnNewCritical, "security.fail_on_new_critical")
	flag(base.Security.FailOnNewHighWhenFixAvailable, head.Security.FailOnNewHighWhenFixAvailable, "security.fail_on_new_high_when_fix_available")
	flag(base.Security.AllowSuppressionOnlyWithExpiry, head.Security.AllowSuppressionOnlyWithExpiry, "security.allow_suppression_only_with_expiry")
	flag(base.Lint.FailOnFormatting, head.Lint.FailOnFormatting, "lint.fail_on_formatting")
	flag(base.Lint.FailOnDeterministicLint, head.Lint.FailOnDeterministicLint, "lint.fail_on_deterministic_lint")

	if base.Execution.FullRunTimeoutMinutes > 0 && head.Execution.FullRunTimeoutMinutes > base.Execution.FullRunTimeoutMinutes {
		add(fmt.Sprintf("increased execution.full_run_timeout_minutes from %d to %d",
			base.Execution.FullRunTimeoutMinutes, head.Execution.FullRunTimeoutMinutes))
	}
	if len(head.Scan.Exclude) > len(base.Scan.Exclude) {
		add("broadened scan.exclude (added excludes)")
	}
	w = append(w, protectedWeakenings(path, base.ProtectedFiles, head.ProtectedFiles)...)
	return w
}

func protectedWeakenings(path string, base, head []config.ProtectedFile) []Weakening {
	headByName := map[string]config.ProtectedFile{}
	for _, p := range head {
		headByName[p.Name] = p
	}
	var w []Weakening
	for _, b := range base {
		h, ok := headByName[b.Name]
		if !ok {
			w = append(w, Weakening{path, "removed protected-file rule " + b.Name})
			continue
		}
		for team, n := range b.Require.Teams {
			if h.Require.Teams[team] < n {
				w = append(w, Weakening{path, fmt.Sprintf("reduced required approvals for %s in protected rule %s", team, b.Name)})
			}
		}
	}
	return w
}

// Module reports weakenings between two module configs.
func Module(path string, base, head *config.ModuleConfig) []Weakening {
	var w []Weakening
	add := func(detail string) { w = append(w, Weakening{path, detail}) }

	// A required scan stage was removed.
	if _, ok := base.CI["scan"]; ok {
		if _, still := head.CI["scan"]; !still {
			add("removed the scan stage")
		}
	}
	// Quality security profile removed.
	if base.Quality.Security != "" && head.Quality.Security == "" {
		add("removed quality.security")
	}

	// Review rules removed or reduced.
	for name, b := range base.Reviews {
		h, ok := head.Reviews[name]
		if !ok {
			add("removed review rule " + name)
			continue
		}
		w = append(w, reviewWeakenings(path, name, b.Require, h.Require)...)
		// Narrowing matched files (fewer when patterns) for a rule.
		if len(h.When) < len(b.When) {
			add("narrowed matched files for review rule " + name)
		}
	}

	// Narrowing a stage's when patterns.
	for stage, b := range base.CI {
		if h, ok := head.CI[stage]; ok && len(h.When) < len(b.When) {
			add("narrowed matched files for ci." + stage)
		}
	}
	return w
}

func reviewWeakenings(path, rule string, base, head config.ReviewRequire) []Weakening {
	var w []Weakening
	add := func(detail string) { w = append(w, Weakening{path, detail}) }
	if intval(base.Owners) > intval(head.Owners) {
		add("reduced required owners for review rule " + rule)
	}
	if intval(base.Expert) > intval(head.Expert) {
		add("reduced required expert approvals for review rule " + rule)
	}
	for team, n := range base.Teams {
		if head.Teams[team] < n {
			add(fmt.Sprintf("reduced required %s approvals for review rule %s", team, rule))
		}
	}
	return w
}

func intval(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
