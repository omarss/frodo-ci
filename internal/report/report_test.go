package report

import (
	"strings"
	"testing"
	"time"
)

func TestBuildCommentFailure(t *testing.T) {
	in := Input{
		SHA: "abc1234def567",
		Stages: []StageReport{
			{Module: "cards", Stage: "validate", Status: "success", Duration: 3 * time.Second},
			{
				Module: "cards", Stage: "test", Status: "failure",
				FailedStep: "Run integration tests", FailReason: "exit 1",
				Summary:      "Maven tests failed: 1 failure(s)",
				Hint:         "Fix the reported test and re-push.",
				Stack:        "maven",
				Output:       "CardServiceTest.transfer:42 expected:<200> but was:<500>",
				Reasons:      []string{"dependency money changed (affects test)"},
				Owners:       []string{"@cards-team"},
				ReproduceDir: "services/cards",
				ReproduceCmd: "../../mvnw -pl services/cards -am verify",
				Duration:     62 * time.Second,
			},
		},
	}
	out := BuildComment(in)

	for _, want := range []string{
		Marker,
		"1 of 2 check(s) failed",
		"`cards` · test",
		"What failed:** Maven tests failed: 1 failure(s)",
		"Failed step:** Run integration tests (exit 1)",
		"Why it ran:** dependency money changed",
		"Owner:** @cards-team",
		"How to fix:** Fix the reported test",
		"Error (maven):",
		"Reproduce locally:",
		"cd services/cards",
		"../../mvnw -pl services/cards -am verify",
		"expected:<200> but was:<500>",
		"abc1234def56", // truncated commit
		"| cards | validate | ✅",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("comment missing %q\n---\n%s", want, out)
		}
	}
}

func TestBuildCommentSuccess(t *testing.T) {
	in := Input{Stages: []StageReport{
		{Module: "a", Stage: "validate", Status: "success"},
		{Module: "a", Stage: "test", Status: "success"},
		{Module: "b", Stage: "build", Status: "skipped"},
	}}
	out := BuildComment(in)
	if !strings.Contains(out, "✅ Frodo CI — all 2 check(s) passed") {
		t.Errorf("expected all-passed header, got:\n%s", out)
	}
	if strings.Contains(out, "How to fix") {
		t.Error("success comment should not contain fix guidance")
	}
}

func TestBuildCommentSeparatesBlockedFromFailed(t *testing.T) {
	in := Input{
		SHA: "fc821134fe3d",
		Stages: []StageReport{
			{Module: "nestjs-grpc-clients", Stage: "validate", Status: "failure",
				FailedStep: "Typecheck", FailReason: "exit 254", Hint: "Run your formatter.",
				Duration: 234 * time.Second},
			{Module: "business-api", Stage: "build", Status: "cancelled"},
			{Module: "business-api", Stage: "test", Status: "cancelled"},
			{Module: "internal-api", Stage: "validate", Status: "cancelled"},
		},
	}
	out := BuildComment(in)
	for _, want := range []string{
		"1 of 4 check(s) failed",                 // only the real failure is counted
		"**1 failed · 3 blocked**",               // truthful breakdown
		"### ❌ `nestjs-grpc-clients` · validate", // root cause first, full detail
		"Typecheck (exit 254)",
		"Blocked (3)",                     // cascade collapsed into one section
		"depend on `nestjs-grpc-clients`", // pointing at the cause
		"`business-api` — build, test",    // grouped per module
		"`internal-api` — validate",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n---\n%s", want, out)
		}
	}
	if n := strings.Count(out, "### ❌"); n != 1 {
		t.Errorf("want exactly 1 root-cause block, got %d", n)
	}
}

func TestBuildCommentReviewsBlock(t *testing.T) {
	// Every stage is green, but a module's review requirement is unmet — the
	// single gate must still fail, prominently.
	in := Input{
		Stages: []StageReport{
			{Module: "internal-api", Stage: "validate", Status: "success"},
			{Module: "internal-api", Stage: "test", Status: "success"},
		},
		Reviews: []ReviewReport{
			{Module: "internal-api", OK: false, Missing: []string{"owner approval (1 of 1)", "expert approval"}},
			{Module: "common", OK: true},
		},
	}
	out := BuildComment(in)
	for _, want := range []string{
		"1 review requirement(s) not met",
		"🔒 Review required (1)",
		"`internal-api` — owner approval (1 of 1); expert approval",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q\n---\n%s", want, out)
		}
	}
	if strings.Contains(out, "all 2 check(s) passed") {
		t.Error("must not claim all passed when a review requirement is unmet")
	}
}

func TestCacheSummaryLine(t *testing.T) {
	in := Input{Stages: []StageReport{
		{Module: "a", Stage: "validate", Status: "success", Duration: 2 * time.Second},
		{Module: "a", Stage: "test", Status: "skipped", Cached: true, Saved: 90 * time.Second},
		{Module: "b", Stage: "build", Status: "skipped", Cached: true, Saved: 30 * time.Second},
		{Module: "b", Stage: "package", Status: "success", Duration: 5 * time.Second},
	}}
	out := BuildComment(in)
	for _, want := range []string{
		"⚡", "reused 2 of 4 stage(s) from cache", "saved ~2m0s",
		"slowest:", "`b` · package", "(5s)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("cache line missing %q\n---\n%s", want, out)
		}
	}
}

func TestReproduceIsRunnable(t *testing.T) {
	in := Input{Stages: []StageReport{{
		Module: "api", Stage: "package", Status: "failure",
		FailedStep: "Build image", FailReason: "exit 1",
		ReproduceDir: "apps/api", ReproduceCmd: `docker build -t "$FRODO_IMAGE" .`,
		Env: map[string]string{
			"FRODO_IMAGE": "api:abc1234", "FRODO_MODULE": "api", "FRODO_MODULE_PATH": "apps/api",
			"FRODO_STAGE": "package", "FRODO_ENVIRONMENT": "staging",
			"FRODO_REPO_ROOT": "/runner/work/x", "FRODO_MODULE_DIR": "/runner/work/x/apps/api",
		},
	}}}
	out := BuildComment(in)
	if !strings.Contains(out, "FRODO_IMAGE='api:abc1234'") {
		t.Errorf("expected the resolved FRODO_IMAGE to be exported, got:\n%s", out)
	}
	if !strings.Contains(out, `FRODO_REPO_ROOT="$(git rev-parse --show-toplevel)"`) {
		t.Error("repo root should be recomputed locally, not pinned to the CI path")
	}
	if strings.Contains(out, "/runner/work/x") {
		t.Error("machine-specific CI paths must not leak into the reproduce script")
	}
	if !strings.Contains(out, "cd apps/api") || !strings.Contains(out, `docker build -t "$FRODO_IMAGE" .`) {
		t.Errorf("missing cd + command, got:\n%s", out)
	}
}
