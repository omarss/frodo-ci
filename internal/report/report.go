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
	Owners       []string          // mention strings, e.g. "@cards-team"
	ReproduceDir string            // module dir to cd into
	ReproduceCmd string            // the exact command to re-run
	Env          map[string]string // resolved FRODO_* env, so reproduce is runnable
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
		if hint := diagnose(s.Stage, s.Output); hint != "" {
			fmt.Fprintf(&b, "- **How to fix:** %s\n", hint)
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

// errorSignature maps substrings found in a step's output to a specific,
// actionable hint. Ordered most-specific first; the first match wins.
type errorSignature struct {
	patterns []string
	hint     string
}

var errorSignatures = []errorSignature{
	{[]string{"403 forbidden", "failed to authorize", "denied: ", "pull access denied",
		"401 unauthorized", "unauthorized:", "authentication required", "insufficient_scope",
		"requested access to the resource is denied"},
		"Registry/auth denied — the runner can't pull or push this image. Authenticate to the registry (for GitHub OIDC to a cloud registry, configure the workload-identity provider/service-account credentials) and retry."},
	{[]string{"failed to read dockerfile", "dockerfile: no such file", "open dockerfile:"},
		"Dockerfile not found in the build context — check the step's working directory or the `-f` path."},
	{[]string{"no space left on device"},
		"The runner ran out of disk — prune caches/images or use a larger runner."},
	{[]string{"out of memory", "oomkilled", "cannot allocate memory"},
		"The step ran out of memory — use a larger runner or reduce parallelism."},
	{[]string{"could not resolve host", "temporary failure in name resolution", "connection refused",
		"connection timed out", "i/o timeout", "dial tcp", "network is unreachable", "tls handshake timeout"},
		"Network error reaching a remote — check connectivity, a proxy, or the dependency mirror."},
	{[]string{"npm err! code e401", "npm err! code e403"},
		"npm registry authentication failed — configure the registry token."},
	{[]string{"command not found", "executable file not found", ": not found"},
		"A required tool isn't installed on the runner — add the matching setup step (e.g. setup-java / setup-node + corepack) or install it."},
	{[]string{"permission denied"},
		"Permission denied — check file modes or the credentials this step uses."},
	{[]string{"build failure", "cannot find symbol", "compilation failure", "error ts", "cannot find module"},
		"Compilation/build error — see the referenced file and line above."},
	{[]string{"assertionerror", "there were failing tests", "tests failed", "--- fail", "✗", "not ok "},
		"A test failed — see the assertion above; fix the test or the code."},
}

// diagnose derives a hint from the actual error output, falling back to
// stage-type guidance when nothing specific matches.
func diagnose(stage, output string) string {
	low := strings.ToLower(output)
	for _, sig := range errorSignatures {
		for _, p := range sig.patterns {
			if strings.Contains(low, p) {
				return sig.hint
			}
		}
	}
	return fixHint(stage)
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
