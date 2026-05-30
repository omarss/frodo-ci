package security

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/omarss/frodo-ci/internal/config"
)

// toolForScan maps a scan type to the CLI tool that performs it. The generated
// workflow installs these; a triggered scan whose tool is missing fails the gate
// (it must not be silently bypassable).
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

// Scanner determines and runs the security scans for a module, then applies the
// gate (suppressions, rulesets, and the module's security profile).
type Scanner struct {
	Event           string
	OnDefaultBranch bool
	// ModuleChanged returns the changed files attributed to a module.
	ModuleChanged func(module string) []string
	// ModuleDir returns the absolute directory a module's scans run in.
	ModuleDir func(module string) string
	// ProfileFor returns the resolved security profile for a module.
	ProfileFor   func(module string) config.SecurityProfile
	Suppressions []config.Suppression
	Rulesets     *config.Rulesets
	Now          time.Time
	// exec is injectable so tests can supply canned tool output; nil uses the real
	// command runner. available is likewise injectable; nil uses PATH lookup.
	exec      ExecFunc
	available func(tool string) bool
}

// Result is the outcome of scanning one module.
type Result struct {
	OK       bool
	Note     string
	Decision Decision
}

// Run computes the smart scan plan for a module, runs each scan, and gates the
// findings. The scan fails when any finding blocks, any scan errors, or a
// triggered scan's tool is missing.
func (s *Scanner) Run(ctx context.Context, module string) Result {
	if s.exec == nil {
		s.exec = execCommand
	}
	if s.available == nil {
		s.available = toolAvailable
	}
	var changed []string
	if s.ModuleChanged != nil {
		changed = s.ModuleChanged(module)
	}
	scans := Plan(changed, s.Event, s.OnDefaultBranch)
	if len(scans) == 0 {
		return Result{OK: true, Note: "no security-relevant changes"}
	}

	dir := module
	if s.ModuleDir != nil {
		dir = s.ModuleDir(module)
	}
	profile := config.SecurityProfile{}
	if s.ProfileFor != nil {
		profile = s.ProfileFor(module)
	}

	var findings []Finding
	var ran, missing, failed []string
	for _, sc := range scans {
		tool := toolForScan[sc.Type]
		if tool == "" {
			continue
		}
		if !s.available(tool) {
			missing = append(missing, string(sc.Type)+" ("+tool+")")
			continue
		}
		fs, err := s.scanModule(ctx, sc, tool, dir)
		if err != nil {
			failed = append(failed, string(sc.Type)+": "+err.Error())
			continue
		}
		ran = append(ran, string(sc.Type))
		findings = append(findings, fs...)
	}

	d := Gate(findings, profile, s.Suppressions, s.Rulesets, s.now())
	ok := len(d.Blocking) == 0 && len(failed) == 0 && len(missing) == 0
	return Result{OK: ok, Note: buildNote(d, ran, missing, failed), Decision: d}
}

func (s *Scanner) now() time.Time {
	if s.Now.IsZero() {
		return time.Now()
	}
	return s.Now
}

// buildNote renders a human-readable scan summary for the PR comment / step
// summary: what blocked (with rule, severity, and location), what is missing,
// what errored, and the counts of reported/suppressed findings.
func buildNote(d Decision, ran, missing, failed []string) string {
	var b strings.Builder
	if len(d.Blocking) > 0 {
		fmt.Fprintf(&b, "BLOCKED by %d finding(s): ", len(d.Blocking))
		var parts []string
		for i, f := range d.Blocking {
			if i == 3 {
				parts = append(parts, fmt.Sprintf("…+%d more", len(d.Blocking)-3))
				break
			}
			parts = append(parts, describe(f))
		}
		b.WriteString(strings.Join(parts, "; "))
		b.WriteString(". ")
	}
	if len(missing) > 0 {
		fmt.Fprintf(&b, "Required tool not installed for: %s — install it to enforce. ", strings.Join(missing, ", "))
	}
	if len(failed) > 0 {
		fmt.Fprintf(&b, "Scan errored: %s. ", strings.Join(failed, "; "))
	}
	if len(ran) > 0 {
		fmt.Fprintf(&b, "Ran: %s. ", strings.Join(ran, ","))
	}
	if len(d.Reported) > 0 || len(d.Suppressed) > 0 {
		fmt.Fprintf(&b, "Reported: %d, suppressed: %d.", len(d.Reported), len(d.Suppressed))
	}
	note := strings.TrimSpace(b.String())
	if note == "" {
		return "no findings"
	}
	return note
}

func describe(f Finding) string {
	loc := f.Path
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", f.Path, f.Line)
	}
	rule := f.RuleID
	if rule == "" {
		rule = string(f.Kind)
	}
	sev := f.Severity
	if sev == "" {
		sev = "finding"
	}
	if loc == "" {
		return fmt.Sprintf("%s (%s)", rule, sev)
	}
	return fmt.Sprintf("%s (%s) at %s", rule, sev, loc)
}

func toolAvailable(tool string) bool {
	_, err := exec.LookPath(tool)
	return err == nil
}
