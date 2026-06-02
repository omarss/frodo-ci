package cli

import (
	"strings"
	"testing"

	"github.com/omarss/frodo-ci/internal/reviews"
)

func TestReviewDiagnostics(t *testing.T) {
	req := reviews.Requirement{Owners: 1, Teams: map[string]int{"security": 1, "qa": 1}}

	// security required-team resolved to no members; qa is fine.
	emptyReq := emptyTeams(req.Teams, map[string][]string{"security": {}, "qa": {"u1"}})
	if len(emptyReq) != 1 || emptyReq[0] != "security" {
		t.Fatalf("emptyTeams = %v, want [security]", emptyReq)
	}

	// owners unresolved (no members) + the empty security team -> two diagnoses.
	diag := reviewDiagnostics(req, false, []string{"platform"}, emptyReq)
	joined := strings.Join(diag, " | ")
	if !strings.Contains(joined, "@platform") || !strings.Contains(joined, "unsatisfiable") {
		t.Errorf("owner diagnosis missing or wrong: %q", joined)
	}
	if !strings.Contains(joined, "team @security") {
		t.Errorf("required-team diagnosis missing: %q", joined)
	}

	// When owners DO resolve, no owner diagnosis (it's a normal pending review).
	if d := reviewDiagnostics(req, true, []string{"platform"}, nil); len(d) != 0 {
		t.Errorf("resolved owners should yield no diagnosis, got %v", d)
	}
}
