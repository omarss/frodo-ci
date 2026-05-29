// Package plan calculates the deterministic execution plan at startup: it loads
// and validates config, discovers modules, builds the dependency graph, detects
// changed files, maps them to module stages, propagates dependency-affected
// stages, computes fingerprints, and consults the cache. Given the same
// repository state, config, templates, toolchains, and refs, it always produces
// the same plan.
package plan

import (
	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/configlint"
)

// Context describes the run that the plan is being calculated for.
type Context struct {
	Base            string // base ref/sha (empty = working tree)
	Head            string // head ref/sha (empty = HEAD)
	ThreeDot        bool   // use merge-base diff (PRs)
	Event           string // pull_request | push | schedule | workflow_dispatch | local
	OnDefaultBranch bool   // push to the default branch (full vuln scan)
	Environment     string // target environment for CD stages
}

// Plan is the calculated execution plan.
type Plan struct {
	RepoRoot string
	Context  Context
	Changes  []string
	Modules  []*ModulePlan
	Problems []configlint.Problem
}

// ModulePlan is the planned work for one module.
type ModulePlan struct {
	Name        string
	Dir         string
	Owners      []string
	Fingerprint string
	Stages      []*StagePlan
}

// StagePlan is the planned work for one module stage.
type StagePlan struct {
	Module       string
	Stage        string
	Group        string // "ci" or "cd"
	Reasons      []string
	Fingerprint  string
	Cacheable    bool
	Cached       bool
	Skipped      bool // cached and skipped to save minutes
	Expensive    bool
	TimeoutMin   int
	Budget       config.Duration
	MatchedFiles []string
}

// expensiveStages are resource-heavy stages subject to bounded parallelism and
// the stop-after-validation-failure rule.
var expensiveStages = map[string]bool{
	"build": true, "package": true, "scan": true, "publish": true, "deploy": true,
}

// nonCacheable stages are never skipped by the cache: scan is security
// governance, and CD stages have external side effects.
var nonCacheable = map[string]bool{
	"scan": true, "publish": true, "deploy": true, "verify": true,
}

// IsExpensive reports whether a stage is resource-heavy.
func IsExpensive(stage string) bool { return expensiveStages[stage] }

// IsCacheable reports whether a stage may be skipped on an exact fingerprint
// match. Security and CD stages never are.
func IsCacheable(stage string) bool { return !nonCacheable[stage] }

// HasErrors reports whether the plan has any blocking configuration error.
func (p *Plan) HasErrors() bool { return configlint.HasErrors(p.Problems) }

// PlannedStages returns the stages that will actually execute (required and not
// skipped by the cache).
func (p *Plan) PlannedStages() []*StagePlan {
	var out []*StagePlan
	for _, m := range p.Modules {
		for _, s := range m.Stages {
			if !s.Skipped {
				out = append(out, s)
			}
		}
	}
	return out
}

// Counts summarizes the plan.
type Counts struct {
	Modules       int
	RequiredStages int
	SkippedStages int
}

// Summary returns aggregate counts for the plan.
func (p *Plan) Summary() Counts {
	c := Counts{Modules: len(p.Modules)}
	for _, m := range p.Modules {
		for _, s := range m.Stages {
			c.RequiredStages++
			if s.Skipped {
				c.SkippedStages++
			}
		}
	}
	return c
}
