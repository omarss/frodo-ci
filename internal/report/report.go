// Package report renders a human-friendly run summary for a PR comment and the
// GitHub step summary. Its goal is that a failing check tells a developer
// exactly what failed, why it ran, and how to fix it -- without digging through
// raw logs. Rendering is pure so it is easy to test; callers assemble the Input.
package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Marker is an invisible tag used to find and update Frodo CI's own PR comment,
// so re-runs update one comment instead of spamming new ones.
const Marker = "<!-- frodo-ci -->"

// StageReport is one stage's outcome, pre-assembled by the caller.
type StageReport struct {
	Module       string
	Stage        string
	Status       string // success | failure | skipped | cancelled | timed_out
	Note         string
	FailedStep   string // name of the step that failed (if any)
	FailReason   string // e.g. "exit 1", "timed out"
	Summary      string // stack-aware one-line "what failed" (from diag)
	Hint         string // stack-aware "how to fix" (from diag)
	Stack        string // detected tech stack (from diag)
	Output       string // the extracted, high-signal error snippet
	Reasons      []string
	Owners       []string          // mention strings, e.g. "@cards-team"
	ReproduceDir string            // module dir to cd into
	ReproduceCmd string            // the exact command to re-run
	Env          map[string]string // resolved FRODO_* env, so reproduce is runnable
	Duration     time.Duration
	Cached       bool          // skipped via an exact fingerprint cache hit
	Saved        time.Duration // wall-clock the cache hit avoided
}

// Input is everything needed to render the summary.
type Input struct {
	SHA     string
	Stages  []StageReport
	Reviews []ReviewReport
}

// ReviewReport is one module's review-gate status (owner/expert/team approvals).
type ReviewReport struct {
	Module    string
	OK        bool
	Missing   []string // human-readable unmet requirements
	Reviewers []string // handles who should review (e.g. @platform, @charlie)
}

func (s StageReport) ok() bool      { return s.Status == "success" }
func (s StageReport) skipped() bool { return s.Status == "skipped" }
func (s StageReport) blocked() bool { return s.Status == "cancelled" }
func (s StageReport) failed() bool  { return !s.ok() && !s.skipped() }

// breakdown renders the per-status tally, omitting empty buckets.
func breakdown(failed, blocked, skipped, passed, reviewsUnmet int) string {
	var parts []string
	add := func(n int, label string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, label))
		}
	}
	add(failed, "failed")
	add(blocked, "blocked")
	add(skipped, "skipped")
	add(passed, "passed")
	add(reviewsUnmet, "review unmet")
	return "**" + strings.Join(parts, " · ") + "**"
}

// renderReviews lists the modules whose review requirements are unmet. It is
// shown prominently (not collapsed): an unmet review is a merge blocker.
func renderReviews(b *strings.Builder, unmet []ReviewReport) {
	fmt.Fprintf(b, "\n### 🔒 Review required (%d)\n\n", len(unmet))
	b.WriteString("Checks can pass, but merging is blocked until these approvals land:\n\n")
	for _, r := range unmet {
		miss := strings.Join(r.Missing, "; ")
		if miss == "" {
			miss = "approval required"
		}
		fmt.Fprintf(b, "- `%s` — %s\n", r.Module, miss)
		if len(r.Reviewers) > 0 {
			fmt.Fprintf(b, "  ↳ should be reviewed by %s _(review requested)_\n", strings.Join(r.Reviewers, ", "))
		}
	}
}

// renderBlocked lists the cancelled (cascade) stages compactly, grouped by
// module and pointing at the root failure — instead of repeating a full block
// (with owner and why-it-ran) for each downstream cancellation.
func renderBlocked(b *strings.Builder, blocked []StageReport, failedModules []string) {
	byModule := map[string][]string{}
	var order []string
	for _, s := range blocked {
		if _, ok := byModule[s.Module]; !ok {
			order = append(order, s.Module)
		}
		byModule[s.Module] = append(byModule[s.Module], s.Stage)
	}
	sort.Strings(order)
	fmt.Fprintf(b, "\n<details><summary>⏸️ Blocked (%d) — cancelled by the failure above</summary>\n\n", len(blocked))
	if len(failedModules) > 0 {
		fmt.Fprintf(b, "These depend on %s and were cancelled when it failed; they'll run once the root cause is fixed.\n\n", joinCode(failedModules))
	} else {
		b.WriteString("These were cancelled because a dependency failed or the run was cancelled.\n\n")
	}
	for _, mod := range order {
		stages := byModule[mod]
		sort.Strings(stages)
		fmt.Fprintf(b, "- `%s` — %s\n", mod, strings.Join(stages, ", "))
	}
	b.WriteString("\n</details>\n")
}

func uniqueModules(stages []StageReport) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range stages {
		if !seen[s.Module] {
			seen[s.Module] = true
			out = append(out, s.Module)
		}
	}
	sort.Strings(out)
	return out
}

func joinCode(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = "`" + s + "`"
	}
	return strings.Join(q, ", ")
}

// BuildComment renders the Markdown summary.
func BuildComment(in Input) string {
	// Separate genuine failures from the cancellation cascade: when one stage
	// fails, every stage that depends on it is cancelled. Reporting those 14
	// cancellations as "failures" buries the one real cause, so they get their
	// own collapsed section and the headline counts only real failures.
	var hardFailed, blocked []StageReport
	skipped, pass := 0, 0
	for _, s := range in.Stages {
		switch {
		case s.blocked():
			blocked = append(blocked, s)
		case s.failed():
			hardFailed = append(hardFailed, s)
		case s.skipped():
			skipped++
		case s.ok():
			pass++
		}
	}
	var reviewsUnmet []ReviewReport
	for _, r := range in.Reviews {
		if !r.OK {
			reviewsUnmet = append(reviewsUnmet, r)
		}
	}

	var b strings.Builder
	b.WriteString(Marker + "\n")
	switch {
	case len(hardFailed) == 0 && len(blocked) == 0 && len(reviewsUnmet) == 0:
		fmt.Fprintf(&b, "## ✅ Frodo CI — all %d check(s) passed\n", pass)
	case len(hardFailed) > 0:
		fmt.Fprintf(&b, "## ❌ Frodo CI — %d of %d check(s) failed\n", len(hardFailed), len(in.Stages))
	case len(blocked) > 0:
		fmt.Fprintf(&b, "## ❌ Frodo CI — run cancelled, %d stage(s) blocked\n", len(blocked))
	default: // checks passed but review requirements are unmet
		fmt.Fprintf(&b, "## ❌ Frodo CI — %d review requirement(s) not met\n", len(reviewsUnmet))
	}
	if len(hardFailed) > 0 || len(blocked) > 0 || len(reviewsUnmet) > 0 {
		fmt.Fprintf(&b, "\n%s\n", breakdown(len(hardFailed), len(blocked), skipped, pass, len(reviewsUnmet)))
	}
	if in.SHA != "" {
		fmt.Fprintf(&b, "\n**Commit:** `%s`\n", short(in.SHA))
	}
	if line := cacheSummary(in.Stages); line != "" {
		b.WriteString("\n" + line)
	}

	for _, s := range hardFailed {
		fmt.Fprintf(&b, "\n### ❌ `%s` · %s\n\n", s.Module, s.Stage)
		if s.Summary != "" {
			fmt.Fprintf(&b, "- **What failed:** %s\n", s.Summary)
		}
		if s.FailedStep != "" {
			fmt.Fprintf(&b, "- **Failed step:** %s", s.FailedStep)
			if s.FailReason != "" {
				fmt.Fprintf(&b, " (%s)", s.FailReason)
			}
			b.WriteString("\n")
		} else if s.Note != "" {
			fmt.Fprintf(&b, "- **Result:** %s — %s\n", s.Status, s.Note)
		}
		if len(s.Reasons) > 0 {
			fmt.Fprintf(&b, "- **Why it ran:** %s\n", strings.Join(s.Reasons, "; "))
		}
		if len(s.Owners) > 0 {
			fmt.Fprintf(&b, "- **Owner:** %s\n", strings.Join(s.Owners, " "))
		}
		if s.Hint != "" {
			fmt.Fprintf(&b, "- **How to fix:** %s\n", s.Hint)
		}
		if s.ReproduceCmd != "" {
			dir := s.ReproduceDir
			if dir == "" {
				dir = "."
			}
			b.WriteString("- **Reproduce locally:**\n\n")
			b.WriteString(codeFence("bash", reproduceScript(s.Env, dir, s.ReproduceCmd), "  "))
		}
		if s.Output != "" {
			label := "Error output"
			if s.Stack != "" && s.Stack != "generic" {
				label = "Error (" + s.Stack + ")"
			}
			fmt.Fprintf(&b, "- **%s:**\n\n", label)
			b.WriteString(codeFence("", s.Output, "  "))
		}
	}

	if len(reviewsUnmet) > 0 {
		renderReviews(&b, reviewsUnmet)
	}
	if len(blocked) > 0 {
		renderBlocked(&b, blocked, uniqueModules(hardFailed))
	}

	b.WriteString("\n<details><summary>All checks</summary>\n\n")
	b.WriteString("| Module | Stage | Result | Duration |\n|---|---|---|---|\n")
	rows := append([]StageReport{}, in.Stages...)
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Module != rows[j].Module {
			return rows[i].Module < rows[j].Module
		}
		return rows[i].Stage < rows[j].Stage
	})
	for _, s := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", s.Module, s.Stage, emoji(s.Status), s.Duration.Round(time.Millisecond))
	}
	b.WriteString("\n</details>\n")
	b.WriteString("\n<sub>Frodo CI — updates on each push. Run <code>frodo-ci plan</code> to preview locally.</sub>\n")
	return b.String()
}

// cacheSummary renders the cache ROI line: how many stages a fingerprint hit
// skipped, the wall-clock that saved, and the slowest stage that actually ran (a
// free bottleneck pointer). Returns "" when nothing ran and nothing was cached.
func cacheSummary(stages []StageReport) string {
	var cached, total int
	var saved time.Duration
	var slowest StageReport
	for _, s := range stages {
		total++
		if s.Cached {
			cached++
			saved += s.Saved
			continue
		}
		if s.Duration > slowest.Duration {
			slowest = s
		}
	}
	if cached == 0 && slowest.Duration == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("⚡ **Cache:** ")
	if cached > 0 {
		fmt.Fprintf(&b, "reused %d of %d stage(s) from cache", cached, total)
		if saved > 0 {
			fmt.Fprintf(&b, " — saved ~%s", dur(saved))
		}
	} else {
		b.WriteString("none reused this run")
	}
	if slowest.Duration > 0 {
		fmt.Fprintf(&b, "; slowest: `%s` · %s (%s)", slowest.Module, slowest.Stage, dur(slowest.Duration))
	}
	b.WriteString("\n")
	return b.String()
}

// dur formats a duration for humans: minute-resolution stages round to seconds
// (e.g. 7m58s), shorter ones keep millisecond precision.
func dur(d time.Duration) string {
	if d >= time.Minute {
		return d.Round(time.Second).String()
	}
	return d.Round(time.Millisecond).String()
}

// reproduceScript builds a literally-runnable reproduce: it exports the resolved
// FRODO_* values (so e.g. $FRODO_IMAGE is set), recomputes the machine-specific
// path vars locally, cds into the module, and runs the failed command verbatim.
func reproduceScript(env map[string]string, dir, cmd string) string {
	var lines []string
	var parts []string
	for _, k := range sortedKeys(env) {
		if k == "FRODO_REPO_ROOT" || k == "FRODO_MODULE_DIR" || env[k] == "" {
			continue // path vars are machine-specific; recomputed below
		}
		parts = append(parts, k+"="+shellQuote(env[k]))
	}
	if len(parts) > 0 {
		lines = append(lines, "export "+strings.Join(parts, " "))
	}
	if _, ok := env["FRODO_REPO_ROOT"]; ok {
		lines = append(lines, `export FRODO_REPO_ROOT="$(git rev-parse --show-toplevel)"`)
	}
	if _, ok := env["FRODO_MODULE_DIR"]; ok {
		lines = append(lines, `export FRODO_MODULE_DIR="$FRODO_REPO_ROOT/$FRODO_MODULE_PATH"`)
	}
	lines = append(lines, "cd "+dir, cmd)
	return strings.Join(lines, "\n")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func emoji(status string) string {
	switch status {
	case "success":
		return "✅"
	case "skipped":
		return "⏭️"
	case "timed_out":
		return "⏱️ timed out"
	case "cancelled":
		return "🚫 cancelled"
	default:
		return "❌"
	}
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// codeFence renders body as a fenced code block, indenting every line by prefix
// so it nests correctly under a Markdown list item (and multi-line commands and
// output stay inside the fence).
func codeFence(lang, body, prefix string) string {
	var b strings.Builder
	b.WriteString(prefix + "```" + lang + "\n")
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		b.WriteString(prefix + line + "\n")
	}
	b.WriteString(prefix + "```\n")
	return b.String()
}
