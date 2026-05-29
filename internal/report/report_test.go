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
				Output:       "FAIL CardServiceTest.transfer\nexpected 200 got 500",
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
		"Failed step:** Run integration tests (exit 1)",
		"Why it ran:** dependency money changed",
		"Owner:** @cards-team",
		"How to fix:",
		"Reproduce locally:",
		"cd services/cards",
		"../../mvnw -pl services/cards -am verify",
		"expected 200 got 500",
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

func TestDiagnoseFromOutput(t *testing.T) {
	// A registry 403 yields an auth hint, not the generic package template.
	in := Input{Stages: []StageReport{{
		Module: "api", Stage: "package", Status: "failure",
		FailedStep: "Build image", FailReason: "exit 1",
		Output: "#10 [runner 1/4] FROM registry/base:latest\n#10 ERROR: failed to authorize: 403 Forbidden",
	}}}
	out := BuildComment(in)
	if !strings.Contains(out, "Registry/auth denied") {
		t.Errorf("expected auth diagnosis derived from output, got:\n%s", out)
	}
	if strings.Contains(out, "missing Dockerfile") {
		t.Error("must not fall back to the generic package hint when auth is the real cause")
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
