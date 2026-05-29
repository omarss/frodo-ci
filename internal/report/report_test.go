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
