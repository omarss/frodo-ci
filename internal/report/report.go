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
	Output       string // tail of the failed step's output
	Reasons      []string
	Owners       []string // mention strings, e.g. "@cards-team"
	ReproduceDir string   // module dir to cd into
	ReproduceCmd string   // the exact command to re-run
	Duration     time.Duration
}

// Input is everything needed to render the summary.
type Input struct {
	SHA    string
	Stages []StageReport
}

func (s StageReport) ok() bool      { return s.Status == "success" }
func (s StageReport) skipped() bool { return s.Status == "skipped" }
func (s StageReport) failed() bool  { return !s.ok() && !s.skipped() }

// BuildComment renders the Markdown summary.
func BuildComment(in Input) string {
	var failed []StageReport
	pass := 0
	for _, s := range in.Stages {
		switch {
		case s.failed():
			failed = append(failed, s)
		case s.ok():
			pass++
		}
	}

	var b strings.Builder
	b.WriteString(Marker + "\n")
	if len(failed) == 0 {
		fmt.Fprintf(&b, "## ✅ Frodo CI — all %d check(s) passed\n", pass)
	} else {
		fmt.Fprintf(&b, "## ❌ Frodo CI — %d of %d check(s) failed\n", len(failed), len(in.Stages))
	}
	if in.SHA != "" {
		fmt.Fprintf(&b, "\n**Commit:** `%s`\n", short(in.SHA))
	}

	for _, s := range failed {
		fmt.Fprintf(&b, "\n### ❌ `%s` · %s\n\n", s.Module, s.Stage)
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
		if hint := fixHint(s.Stage); hint != "" {
			fmt.Fprintf(&b, "- **How to fix:** %s\n", hint)
		}
		if s.ReproduceCmd != "" {
			dir := s.ReproduceDir
			if dir == "" {
				dir = "."
			}
			b.WriteString("- **Reproduce locally:**\n\n")
			b.WriteString(codeFence("bash", "cd "+dir+"\n"+s.ReproduceCmd, "  "))
		}
		if s.Output != "" {
			b.WriteString("- **Error output:**\n\n")
			b.WriteString(codeFence("", s.Output, "  "))
		}
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

// fixHint returns stage-specific, actionable guidance.
func fixHint(stage string) string {
	switch stage {
	case "validate":
		return "A format/lint/schema check failed. Run your module's formatter and `frodo-ci validate-config`, then re-push."
	case "test":
		return "A test failed. Reproduce with the command below, fix the test or the code, and re-push."
	case "build":
		return "The build failed. See the compiler/build output below."
	case "package":
		return "Packaging failed — commonly a missing Dockerfile or image config. Confirm the module builds an image."
	case "scan":
		return "A blocking security finding. Review it; a suppression requires an owner, reason, approver, and a future expiry."
	case "publish", "deploy", "verify":
		return "A delivery step failed. Check the environment and credentials, then see the output below."
	default:
		return ""
	}
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
