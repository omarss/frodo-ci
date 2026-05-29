package cli

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/frodo-ci/frodo-ci/internal/cache"
	"github.com/frodo-ci/frodo-ci/internal/plan"
)

func exampleRoot(t *testing.T) string {
	t.Helper()
	root := "../../examples/monorepo"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("example monorepo not found: %v", err)
	}
	return root
}

// TestExampleValidatesAndPlans is the end-to-end guarantee: the shipped example
// monorepo validates, lints clean, discovers all modules with the expected
// dependency edge, and produces a sensible plan.
func TestExampleValidatesAndPlans(t *testing.T) {
	root := exampleRoot(t)

	var out bytes.Buffer
	app := &App{RepoRoot: root, Out: &out, Err: &out}
	if err := app.runValidateConfig(); err != nil {
		t.Fatalf("example failed validate-config:\n%s", out.String())
	}

	loaded, err := plan.Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HasErrors() {
		t.Fatalf("example has config errors: %v", loaded.Problems)
	}
	if len(loaded.Modules) != 5 {
		t.Errorf("expected 5 modules, got %d", len(loaded.Modules))
	}

	deps := loaded.Graph.Dependencies("cards")
	if len(deps) != 1 || deps[0].To != "vrtx-common" {
		t.Errorf("cards should depend on vrtx-common, got %+v", deps)
	}

	// A push to main runs a full vulnerability scan for every module.
	p, err := loaded.Plan(plan.Context{Event: "push", OnDefaultBranch: true, Environment: "staging"}, cache.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if !planHasStage(p, "cards", "scan") {
		t.Error("expected cards.scan on a push to main (full vulnerability scan)")
	}
}

func planHasStage(p *plan.Plan, module, stage string) bool {
	for _, m := range p.Modules {
		if m.Name != module {
			continue
		}
		for _, s := range m.Stages {
			if s.Stage == stage {
				return true
			}
		}
	}
	return false
}
