package perf

import (
	"testing"
	"time"

	"github.com/omarss/frodo-ci/internal/config"
)

func dur(s string) config.Duration {
	d, _ := config.ParseDuration(s)
	return config.Duration(d)
}

func TestBudgetResolutionAndCheck(t *testing.T) {
	b := BuildBudgets(
		map[string]map[string]config.Duration{
			"cards": {"test": dur("20m")},
		},
		&config.PerformanceBudgets{Budgets: map[string]config.Duration{
			"ci_total":         dur("45m"),
			"module_default":   dur("20m"),
			"validate_default": dur("5m"),
		}},
	)

	if d, ok := b.For("cards", "test"); !ok || d != 20*time.Minute {
		t.Errorf("module budget = %s (ok=%v)", d, ok)
	}
	if d, ok := b.For("other", "validate"); !ok || d != 5*time.Minute {
		t.Errorf("stage default = %s (ok=%v)", d, ok)
	}
	if d, ok := b.For("other", "build"); !ok || d != 20*time.Minute {
		t.Errorf("module default = %s (ok=%v)", d, ok)
	}

	timings := []StageTiming{
		{Module: "cards", Stage: "test", Duration: 25 * time.Minute}, // over 20m
		{Module: "cards", Stage: "validate", Duration: 2 * time.Minute},
	}
	v := Check(timings, b)
	if len(v) == 0 || v[0].Module != "cards" || v[0].Stage != "test" {
		t.Errorf("expected cards/test violation, got %+v", v)
	}
}

func TestCITotalViolation(t *testing.T) {
	b := BudgetSet{CITotal: 10 * time.Minute}
	timings := []StageTiming{
		{Module: "a", Stage: "test", Duration: 7 * time.Minute},
		{Module: "b", Stage: "test", Duration: 6 * time.Minute},
	}
	v := Check(timings, b)
	found := false
	for _, x := range v {
		if x.Module == "(ci total)" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a CI-total violation, got %+v", v)
	}
}

func TestRegressions(t *testing.T) {
	baseline := map[string]time.Duration{"cards/test": 10 * time.Minute}
	current := []StageTiming{{Module: "cards", Stage: "test", Duration: 14 * time.Minute}} // +40%
	regs := Regressions(current, baseline, 30)
	if len(regs) != 1 || regs[0].Percent < 30 {
		t.Errorf("expected a >30%% regression, got %+v", regs)
	}
}
