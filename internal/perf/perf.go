// Package perf tracks stage timings and evaluates them against budgets and a
// baseline, and renders the results for the GitHub step summary / PR comment.
package perf

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/omarss/frodo-ci/internal/config"
)

// StageTiming is one stage's measured duration.
type StageTiming struct {
	Module   string
	Stage    string
	Duration time.Duration
}

// BudgetSet resolves a budget for a (module, stage), preferring the module's own
// budget, then a per-stage default, then a module default.
type BudgetSet struct {
	PerModuleStage map[string]map[string]time.Duration
	StageDefault   map[string]time.Duration
	ModuleDefault  time.Duration
	CITotal        time.Duration
}

// BuildBudgets assembles a BudgetSet from module configs and the budgets file.
func BuildBudgets(moduleBudgets map[string]map[string]config.Duration, file *config.PerformanceBudgets) BudgetSet {
	b := BudgetSet{
		PerModuleStage: map[string]map[string]time.Duration{},
		StageDefault:   map[string]time.Duration{},
	}
	for module, stages := range moduleBudgets {
		m := map[string]time.Duration{}
		for stage, d := range stages {
			m[stage] = d.Duration()
		}
		b.PerModuleStage[module] = m
	}
	if file != nil {
		for key, d := range file.Budgets {
			switch {
			case key == "ci_total":
				b.CITotal = d.Duration()
			case key == "module_default":
				b.ModuleDefault = d.Duration()
			case strings.HasSuffix(key, "_default"):
				b.StageDefault[strings.TrimSuffix(key, "_default")] = d.Duration()
			default:
				b.StageDefault[key] = d.Duration()
			}
		}
	}
	return b
}

// For returns the budget for a (module, stage), if one applies.
func (b BudgetSet) For(module, stage string) (time.Duration, bool) {
	if m, ok := b.PerModuleStage[module]; ok {
		if d, ok := m[stage]; ok && d > 0 {
			return d, true
		}
	}
	if d, ok := b.StageDefault[stage]; ok && d > 0 {
		return d, true
	}
	if b.ModuleDefault > 0 {
		return b.ModuleDefault, true
	}
	return 0, false
}

// Violation is a stage (or the CI total) exceeding its budget.
type Violation struct {
	Module string
	Stage  string
	Budget time.Duration
	Actual time.Duration
}

// Check returns budget violations for the given timings, including the CI total.
func Check(timings []StageTiming, b BudgetSet) []Violation {
	var v []Violation
	var total time.Duration
	for _, t := range timings {
		total += t.Duration
		if budget, ok := b.For(t.Module, t.Stage); ok && t.Duration > budget {
			v = append(v, Violation{t.Module, t.Stage, budget, t.Duration})
		}
	}
	if b.CITotal > 0 && total > b.CITotal {
		v = append(v, Violation{Module: "(ci total)", Budget: b.CITotal, Actual: total})
	}
	sort.Slice(v, func(i, j int) bool {
		if v[i].Module != v[j].Module {
			return v[i].Module < v[j].Module
		}
		return v[i].Stage < v[j].Stage
	})
	return v
}

// Regression is a stage slower than its baseline beyond the allowed percentage.
type Regression struct {
	Key      string
	Baseline time.Duration
	Current  time.Duration
	Percent  int
}

// Regressions compares current timings against a baseline keyed by
// "module/stage", flagging slowdowns beyond maxSlowdownPercent.
func Regressions(current []StageTiming, baseline map[string]time.Duration, maxSlowdownPercent int) []Regression {
	var out []Regression
	for _, t := range current {
		key := t.Module + "/" + t.Stage
		base, ok := baseline[key]
		if !ok || base <= 0 {
			continue
		}
		pct := int(float64(t.Duration-base) / float64(base) * 100)
		if pct > maxSlowdownPercent {
			out = append(out, Regression{Key: key, Baseline: base, Current: t.Duration, Percent: pct})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// StepSummary renders a Markdown summary of timings and violations for the
// GitHub step summary or a PR comment.
func StepSummary(timings []StageTiming, violations []Violation) string {
	var b strings.Builder
	b.WriteString("## Frodo CI performance\n\n")
	b.WriteString("| Module | Stage | Duration |\n|---|---|---|\n")
	sorted := append([]StageTiming{}, timings...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Duration > sorted[j].Duration })
	for _, t := range sorted {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", t.Module, t.Stage, t.Duration.Round(time.Millisecond))
	}
	if len(violations) > 0 {
		b.WriteString("\n### Budget violations\n\n")
		for _, v := range violations {
			fmt.Fprintf(&b, "- %s/%s took %s (budget %s)\n", v.Module, v.Stage,
				v.Actual.Round(time.Millisecond), v.Budget)
		}
	}
	return b.String()
}
