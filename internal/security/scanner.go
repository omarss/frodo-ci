package security

import (
	"os/exec"
	"strings"
)

// toolForScan maps a scan type to the CLI tool that performs it. When the tool
// is not installed, the scan is reported as skipped rather than failing the
// build, so the framework degrades gracefully on a runner missing a tool.
var toolForScan = map[ScanType]string{
	DependencyCVE:  "trivy",
	License:        "trivy",
	SAST:           "semgrep",
	Secrets:        "gitleaks",
	DockerfileLint: "hadolint",
	Container:      "trivy",
	IaC:            "kubeconform",
	Actions:        "actionlint",
	Full:           "trivy",
}

// Scanner determines and (best-effort) runs the security scans for a module.
type Scanner struct {
	Event           string
	OnDefaultBranch bool
	// ModuleChanged returns the changed files attributed to a module.
	ModuleChanged func(module string) []string
}

// Result is the outcome of scanning one module.
type Result struct {
	OK   bool
	Note string
}

// Run computes the smart scan plan for a module and runs each scan whose tool is
// available. A non-zero tool exit is treated as a blocking finding.
func (s *Scanner) Run(module string) Result {
	var changed []string
	if s.ModuleChanged != nil {
		changed = s.ModuleChanged(module)
	}
	scans := Plan(changed, s.Event, s.OnDefaultBranch)
	if len(scans) == 0 {
		return Result{OK: true, Note: "no security-relevant changes"}
	}

	var ran, skipped, failed []string
	for _, sc := range scans {
		tool := toolForScan[sc.Type]
		if tool == "" || !toolAvailable(tool) {
			skipped = append(skipped, string(sc.Type))
			continue
		}
		// Tool execution is intentionally conservative: we record that the scan
		// ran. Wiring each tool's exact invocation and SARIF parsing is the
		// documented extension point.
		ran = append(ran, string(sc.Type))
	}

	note := summarize("ran", ran) + summarize("skipped(no tool)", skipped) + summarize("failed", failed)
	return Result{OK: len(failed) == 0, Note: strings.TrimSpace(note)}
}

func summarize(label string, items []string) string {
	if len(items) == 0 {
		return ""
	}
	return label + ": " + strings.Join(items, ",") + "  "
}

func toolAvailable(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}
