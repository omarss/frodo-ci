package diag

import (
	"fmt"
	"regexp"
	"strings"
)

// extractors maps a stack to its salient-error extractor. Each returns ok=false
// when it can't find a recognizable error, so Analyze falls back to concise().
var extractors = map[string]func(string) (Result, bool){
	"maven":      extractMaven,
	"gradle":     extractGradle,
	"go":         extractGo,
	"typescript": extractTypeScript,
	"node":       extractNode,
	"python":     extractPython,
	"docker":     extractDocker,
	"terraform":  extractTerraform,
	"kubernetes": extractKubernetes,
	"dotnet":     extractDotnet,
	"rust":       extractRust,
	"ruby":       extractRuby,
	"shell":      extractShell,
}

var (
	reMavenErr     = regexp.MustCompile(`(?m)^\s*\[ERROR\]`)
	reMavenTests   = regexp.MustCompile(`Tests run: \d+, Failures: (\d+), Errors: (\d+)`)
	reMavenCompile = regexp.MustCompile(`\.java:\[\d+,\d+\]`)
	reGoFail       = regexp.MustCompile(`^--- FAIL: |^\s+[^ ].*\.go:\d+:|^FAIL\b|^# |^\s+.*\.go:\d+:\d+:`)
	reGoTestName   = regexp.MustCompile(`--- FAIL: (\S+)`)
	reTSErr        = regexp.MustCompile(`error TS\d+:`)
	reNodeErr      = regexp.MustCompile(`npm ERR!|ERR_PNPM|ELIFECYCLE|Command failed`)
	rePnpmCode     = regexp.MustCompile(`(ERR_PNPM_[A-Z_]+)`)
	reJest         = regexp.MustCompile(`(?m)✕|✗|^FAIL |^\s*●`)
	rePyLine       = regexp.MustCompile(`(?m)^E\s{2,}|^FAILED |^\s*assert|^\w+Error:`)
	rePyCount      = regexp.MustCompile(`(\d+) failed`)
	reDocker       = regexp.MustCompile(`failed to solve|^ERROR: |Dockerfile:\d+|error: |failed to `)
	reDockerLine   = regexp.MustCompile(`Dockerfile:(\d+)`)
	reTfErr        = regexp.MustCompile(`(?m)^Error: `)
	reTfLines      = regexp.MustCompile(`(?m)^(Error: |\s+on .* line \d+| +\d+:)`)
	reTfFirst      = regexp.MustCompile(`(?m)^Error: (.+)`)
	reK8s          = regexp.MustCompile(`is invalid|failed validation|error validating|Error from server|invalid:|must be `)
	reDotnet       = regexp.MustCompile(`error (CS|MSB|NU)\d+`)
	reRust         = regexp.MustCompile(`error\[E\d+\]|^error: `)
	reRuby         = regexp.MustCompile(`Failure/Error|examples?, \d+ failures?|\)\s*$`)
	reShell        = regexp.MustCompile(`line \d+: |command not found|syntax error|No such file or directory`)
)

func extractMaven(out string) (Result, bool) {
	raw := grep(out, reMavenErr, 18)
	cleaned := nonEmpty(stripPrefixes(raw, "[ERROR]"))
	if len(cleaned) == 0 {
		return Result{}, false
	}
	summary := "Maven build failed"
	if m := reMavenTests.FindStringSubmatch(out); m != nil && (m[1] != "0" || m[2] != "0") {
		summary = fmt.Sprintf("Maven tests failed: %s failure(s), %s error(s)", m[1], m[2])
	} else if reMavenCompile.MatchString(out) {
		summary = "Maven compilation failed"
	}
	return Result{
		Summary: summary,
		Snippet: strings.Join(cleaned, "\n"),
		Hint:    "Fix the reported file(s)/test(s) and re-push; reproduce with the command below. Formatters/linters run via the central quality profile, not ad hoc commands.",
	}, true
}

func extractGradle(out string) (Result, bool) {
	if seg := blockFrom(out, "* What went wrong:", 14); seg != "" {
		return Result{Summary: "Gradle build failed", Snippet: seg,
			Hint: "See 'What went wrong' above; reproduce with the command below."}, true
	}
	f := grep(out, regexp.MustCompile(`> Task .* FAILED|BUILD FAILED|FAILURE: `), 12)
	if len(f) == 0 {
		return Result{}, false
	}
	return Result{Summary: "Gradle build failed", Snippet: strings.Join(f, "\n"),
		Hint: "See the failed task above; reproduce with the command below."}, true
}

func extractGo(out string) (Result, bool) {
	if strings.Contains(out, "panic:") {
		blk := blockFrom(out, "panic:", 12)
		return Result{Summary: "Go panic: " + strings.TrimSpace(strings.TrimPrefix(firstLine(blk), "panic:")),
			Snippet: blk, Hint: "A panic crashed the run — see the stack trace and guard the offending input."}, true
	}
	f := grep(out, reGoFail, 18)
	if len(f) == 0 {
		return Result{}, false
	}
	summary := "Go build failed"
	if m := reGoTestName.FindStringSubmatch(out); m != nil {
		summary = "Go test failed: " + m[1]
	}
	return Result{Summary: summary, Snippet: strings.Join(f, "\n"),
		Hint: "Fix the test or compile error above; reproduce with the command below."}, true
}

func extractTypeScript(out string) (Result, bool) {
	errs := grep(out, reTSErr, 18)
	if len(errs) == 0 {
		return Result{}, false
	}
	return Result{Summary: fmt.Sprintf("%d TypeScript error(s)", len(errs)), Snippet: strings.Join(errs, "\n"),
		Hint: "Fix the type error(s) above; run the typecheck command below locally."}, true
}

func extractNode(out string) (Result, bool) {
	if reTSErr.MatchString(out) {
		return extractTypeScript(out)
	}
	errs := grep(out, reNodeErr, 18)
	if len(errs) > 0 {
		summary := "Node/pnpm step failed"
		if m := rePnpmCode.FindStringSubmatch(out); m != nil {
			summary = "pnpm: " + m[1]
		}
		return Result{Summary: summary, Snippet: strings.Join(errs, "\n"),
			Hint: "See the error above; reproduce with the command below. Dependencies install with --frozen-lockfile, so commit lockfile changes."}, true
	}
	if reJest.MatchString(out) {
		return Result{Summary: "Node tests failed", Snippet: strings.Join(grep(out, reJest, 18), "\n"),
			Hint: "A test failed — see the assertion above; reproduce with the command below."}, true
	}
	return Result{}, false
}

func extractPython(out string) (Result, bool) {
	if !strings.Contains(out, "Traceback") && !rePyLine.MatchString(out) {
		return Result{}, false
	}
	summary := "Python error"
	if m := rePyCount.FindStringSubmatch(out); m != nil {
		summary = "pytest: " + m[1] + " failed"
	}
	return Result{Summary: summary, Snippet: strings.Join(grep(out, rePyLine, 18), "\n"),
		Hint: "See the assertion/traceback above; reproduce with the command below."}, true
}

func extractDocker(out string) (Result, bool) {
	lines := grep(out, reDocker, 16)
	if len(lines) == 0 {
		return Result{}, false
	}
	summary := "Docker build failed"
	if m := reDockerLine.FindStringSubmatch(out); m != nil {
		summary = "Docker build failed at Dockerfile:" + m[1]
	}
	return Result{Summary: summary, Snippet: strings.Join(lines, "\n"),
		Hint: "See the failing instruction above; build locally with the command below."}, true
}

func extractTerraform(out string) (Result, bool) {
	if !reTfErr.MatchString(out) {
		return Result{}, false
	}
	summary := "Terraform error"
	if m := reTfFirst.FindStringSubmatch(out); m != nil {
		summary = "Terraform: " + strings.TrimSpace(m[1])
	}
	return Result{Summary: summary, Snippet: strings.Join(grep(out, reTfLines, 16), "\n"),
		Hint: "Fix the configuration at the indicated file/line; run `terraform validate` and `plan` locally."}, true
}

func extractKubernetes(out string) (Result, bool) {
	lines := grep(out, reK8s, 16)
	if len(lines) == 0 {
		return Result{}, false
	}
	return Result{Summary: "Kubernetes manifest validation failed", Snippet: strings.Join(lines, "\n"),
		Hint: "Fix the manifest field(s) above; run kubeconform / `kubectl apply --dry-run=server` locally."}, true
}

func extractDotnet(out string) (Result, bool) {
	lines := grep(out, reDotnet, 16)
	if len(lines) == 0 {
		return Result{}, false
	}
	return Result{Summary: ".NET build failed", Snippet: strings.Join(lines, "\n"),
		Hint: "Fix the reported error(s); reproduce with `dotnet build` locally."}, true
}

func extractRust(out string) (Result, bool) {
	lines := grep(out, reRust, 16)
	if len(lines) == 0 {
		return Result{}, false
	}
	return Result{Summary: "Cargo build failed", Snippet: strings.Join(lines, "\n"),
		Hint: "Fix the reported error(s); reproduce with `cargo build`/`cargo test` locally."}, true
}

func extractRuby(out string) (Result, bool) {
	if !reRuby.MatchString(out) {
		return Result{}, false
	}
	return Result{Summary: "Ruby test failed", Snippet: strings.Join(grep(out, regexp.MustCompile(`Failure/Error|expected|got:|rspec `), 16), "\n"),
		Hint: "See the failure above; reproduce with the command below."}, true
}

func extractShell(out string) (Result, bool) {
	lines := grep(out, reShell, 12)
	if len(lines) == 0 {
		return Result{}, false
	}
	return Result{Summary: "Shell step failed", Snippet: strings.Join(lines, "\n"),
		Hint: "See the failing command/line above."}, true
}

// --- helpers ---

func stripPrefixes(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		t := strings.TrimSpace(l)
		t = strings.TrimSpace(strings.TrimPrefix(t, prefix))
		out = append(out, t)
	}
	return out
}

func nonEmpty(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func blockFrom(out, marker string, max int) string {
	idx := strings.Index(out, marker)
	if idx < 0 {
		return ""
	}
	lines := strings.Split(out[idx:], "\n")
	if len(lines) > max {
		lines = lines[:max]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return strings.Join(lines, "\n")
}
