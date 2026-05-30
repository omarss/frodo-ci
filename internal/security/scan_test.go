package security

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/omarss/frodo-ci/internal/config"
)

const sampleSARIF = `{"runs":[{"tool":{"driver":{"name":"semgrep","rules":[
  {"id":"rules.sql-injection","defaultConfiguration":{"level":"error"},"properties":{"security-severity":"9.5"}},
  {"id":"rules.weak-hash","defaultConfiguration":{"level":"warning"},"properties":{"security-severity":"5.0"}}
]}},"results":[
  {"ruleId":"rules.sql-injection","level":"error","message":{"text":"SQL injection sink"},
   "locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/db.ts"},"region":{"startLine":42}}}]},
  {"ruleId":"rules.weak-hash","level":"warning","message":{"text":"weak hash"},
   "locations":[{"physicalLocation":{"artifactLocation":{"uri":"src/h.ts"},"region":{"startLine":7}}}]}
]}]}`

func TestParseSARIF(t *testing.T) {
	got, err := parseSARIF([]byte(sampleSARIF), SAST, "semgrep")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 findings, got %d", len(got))
	}
	if got[0].Severity != "critical" || got[0].Path != "src/db.ts" || got[0].Line != 42 {
		t.Errorf("finding[0] = %+v, want critical src/db.ts:42", got[0])
	}
	if got[1].Severity != "medium" {
		t.Errorf("finding[1] severity = %q, want medium (security-severity 5.0)", got[1].Severity)
	}
	if got[0].Kind != SAST || got[0].Tool != "semgrep" {
		t.Errorf("kind/tool not attributed: %+v", got[0])
	}
}

func TestGateClassifies(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	profile := config.SecurityProfile{FailOnNewCritical: true, FailOnSecrets: true}
	rs := &config.Rulesets{ReportOnly: []string{"noisy"}, Blocking: []string{"always-block"}}
	sups := []config.Suppression{{ID: "accepted", Path: "**", Expiry: config.Date{Time: now.AddDate(0, 1, 0)}}}

	findings := []Finding{
		{RuleID: "crit", Severity: "critical", Kind: SAST, Path: "a.ts"},     // blocks (profile)
		{RuleID: "med", Severity: "medium", Kind: SAST, Path: "b.ts"},        // reported
		{RuleID: "leak", Severity: "high", Kind: Secrets, Path: "c.ts"},      // blocks (secret)
		{RuleID: "noisy", Severity: "critical", Kind: SAST, Path: "d.ts"},    // report-only ruleset
		{RuleID: "always-block", Severity: "low", Kind: SAST, Path: "e.ts"},  // blocking ruleset
		{RuleID: "accepted", Severity: "critical", Kind: SAST, Path: "f.ts"}, // suppressed
	}
	d := Gate(findings, profile, sups, rs, now)
	if len(d.Blocking) != 3 {
		t.Errorf("blocking = %d (%v), want 3 (crit, leak, always-block)", len(d.Blocking), ruleIDs(d.Blocking))
	}
	if len(d.Reported) != 2 {
		t.Errorf("reported = %d (%v), want 2 (med, noisy)", len(d.Reported), ruleIDs(d.Reported))
	}
	if len(d.Suppressed) != 1 {
		t.Errorf("suppressed = %d, want 1 (accepted)", len(d.Suppressed))
	}
}

func ruleIDs(fs []Finding) []string {
	var out []string
	for _, f := range fs {
		out = append(out, f.RuleID)
	}
	return out
}

func scannerWith(execOut string, available bool) *Scanner {
	return &Scanner{
		Event:         "pull_request",
		ModuleChanged: func(string) []string { return []string{"m/src/x.ts"} },
		ModuleDir:     func(string) string { return "." },
		ProfileFor: func(string) config.SecurityProfile {
			return config.SecurityProfile{FailOnNewCritical: true, FailOnSecrets: true}
		},
		available: func(string) bool { return available },
		exec: func(_ context.Context, _, name string, _ ...string) ([]byte, int, error) {
			if name == "semgrep" {
				return []byte(execOut), 1, nil
			}
			return nil, 0, nil // gitleaks et al.: nothing written -> no findings
		},
	}
}

func TestScannerRunBlocksOnCritical(t *testing.T) {
	res := scannerWith(sampleSARIF, true).Run(context.Background(), "m")
	if res.OK {
		t.Error("a critical SAST finding must fail the gate")
	}
	if !strings.Contains(res.Note, "BLOCKED") || !strings.Contains(res.Note, "sql-injection") {
		t.Errorf("note should name the blocking finding, got: %q", res.Note)
	}
}

func TestScannerRunCleanPasses(t *testing.T) {
	res := scannerWith(`{"runs":[]}`, true).Run(context.Background(), "m")
	if !res.OK {
		t.Errorf("no findings should pass, got note: %q", res.Note)
	}
}

func TestScannerRunMissingToolFails(t *testing.T) {
	res := scannerWith(sampleSARIF, false).Run(context.Background(), "m")
	if res.OK {
		t.Error("a triggered scan whose tool is missing must fail (non-bypassable)")
	}
	if !strings.Contains(res.Note, "not installed") {
		t.Errorf("note should flag the missing tool, got: %q", res.Note)
	}
}
