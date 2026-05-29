// Package configlint performs semantic configuration linting: the "meaning"
// checks that schema validation cannot catch (unknown stages, cycles, broad
// inputs, expired suppressions, weakening, ...). Schema validation catches
// structure; this catches meaning.
package configlint

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/discover"
	"github.com/frodo-ci/frodo-ci/internal/graph"
	"github.com/frodo-ci/frodo-ci/internal/schema"
)

// Severity classifies a problem.
type Severity string

const (
	// Error marks a problem that must block before any expensive setup.
	Error Severity = "error"
	// Warning marks a problem worth surfacing that does not block.
	Warning Severity = "warning"
)

// Problem is a human-friendly semantic finding tied to a config file.
type Problem struct {
	Severity Severity
	Path     string
	Message  string
}

func (p Problem) String() string {
	loc := p.Path
	if loc == "" {
		loc = "(config)"
	}
	return fmt.Sprintf("%s: %s: %s", loc, p.Severity, p.Message)
}

// Input bundles everything the semantic checks need. The graph and its build
// problems are passed in so we do not rebuild them.
type Input struct {
	Root             *config.RootConfig
	Modules          []*discover.Module
	Graph            *graph.Graph
	GraphProblems    []graph.Problem
	Toolchains       *config.Toolchains
	Suppressions     *config.Suppressions
	SuppressionsPath string
	SecurityBaseline *config.SecurityBaseline
	LintRules        *config.LintRules
	Now              time.Time
}

// Check runs every semantic rule and returns the problems found, sorted by path.
func Check(in Input) []Problem {
	var ps []Problem

	for _, gp := range in.GraphProblems {
		ps = append(ps, Problem{Severity: Error, Path: gp.Path, Message: gp.Message})
	}
	for _, gp := range graph.PathProblems(in.Modules) {
		ps = append(ps, Problem{Severity: Error, Path: gp.Path, Message: gp.Message})
	}

	for _, m := range in.Modules {
		ps = append(ps, checkModule(in, m)...)
	}
	ps = append(ps, checkSuppressions(in)...)
	ps = append(ps, checkSlack(in)...)

	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Path != ps[j].Path {
			return ps[i].Path < ps[j].Path
		}
		return ps[i].Message < ps[j].Message
	})
	return ps
}

// HasErrors reports whether any problem is an error.
func HasErrors(ps []Problem) bool {
	for _, p := range ps {
		if p.Severity == Error {
			return true
		}
	}
	return false
}

func checkModule(in Input, m *discover.Module) []Problem {
	var ps []Problem
	cfg := m.Config

	if len(cfg.Owners.Teams) == 0 && len(cfg.Owners.Users) == 0 {
		ps = append(ps, Problem{Warning, m.ConfigPath, "module has no owners (owners.teams or owners.users)"})
	}

	ps = append(ps, checkStageNames(in.Root, m, "ci", cfg.CI)...)
	ps = append(ps, checkStageNames(in.Root, m, "cd", cfg.CD)...)
	ps = append(ps, checkStageFiles(m)...)
	ps = append(ps, checkProfiles(in, m)...)

	maxTimeout := in.Root.Execution.FullRunTimeoutMinutes
	for _, sr := range m.Stages {
		if maxTimeout > 0 && sr.File.TimeoutMinutes > maxTimeout {
			ps = append(ps, Problem{Error, sr.Path,
				fmt.Sprintf("stage timeout %dm exceeds the run maximum of %dm", sr.File.TimeoutMinutes, maxTimeout)})
		}
	}
	return ps
}

func checkStageNames(root *config.RootConfig, m *discover.Module, group string, stages map[string]config.ModuleStage) []Problem {
	var ps []Problem
	allowed := root.Stages.CI
	if group == "cd" {
		allowed = root.Stages.CD
	}
	for _, name := range sortedStageKeys(stages) {
		if contains(allowed, name) {
			continue
		}
		msg := fmt.Sprintf("unknown %s stage %q", strings.ToUpper(group), name)
		if s := schema.Suggest(name, allowed); s != "" {
			msg += fmt.Sprintf("; did you mean %q?", s)
		}
		msg += "\nallowed: " + strings.Join(allowed, ", ")
		ps = append(ps, Problem{Error, m.ConfigPath, msg})
	}
	return ps
}

// checkStageFiles flags stages that cannot run (no stage file and no profile)
// and stage files that are never referenced.
func checkStageFiles(m *discover.Module) []Problem {
	var ps []Problem
	hasProfile := m.Config.Use.Profile != "" || m.Config.Type != ""

	declared := map[string]bool{}
	for name := range m.Config.CI {
		declared[name] = true
	}
	for name := range m.Config.CD {
		declared[name] = true
	}

	if !hasProfile {
		for _, name := range sortedStageKeys(m.Config.CI) {
			if _, ok := m.Stages[name]; !ok {
				ps = append(ps, Problem{Warning, m.ConfigPath,
					fmt.Sprintf("stage %q has no steps: no .ci/%s.yml and no template profile", name, name)})
			}
		}
	}
	for stage, sr := range m.Stages {
		if !declared[stage] && !hasProfile {
			ps = append(ps, Problem{Warning, sr.Path,
				fmt.Sprintf("stage file %q is never referenced by ci/cd and there is no profile", stage)})
		}
	}
	return ps
}

func checkProfiles(in Input, m *discover.Module) []Problem {
	var ps []Problem
	q := m.Config.Quality
	if in.LintRules != nil && len(in.LintRules.Profiles) > 0 {
		ps = appendUnknownProfile(ps, m.ConfigPath, "quality.format", q.Format, profileNames(in.LintRules.Profiles))
		ps = appendUnknownProfile(ps, m.ConfigPath, "quality.lint", q.Lint, profileNames(in.LintRules.Profiles))
	}
	if in.SecurityBaseline != nil && len(in.SecurityBaseline.Profiles) > 0 {
		known := securityProfileNames(in.SecurityBaseline.Profiles)
		ps = appendUnknownProfile(ps, m.ConfigPath, "quality.security", q.Security, known)
		for _, name := range sortedStageKeys(m.Config.CI) {
			st := m.Config.CI[name]
			if st.Security != nil && st.Security.Profile != "" && !contains(known, st.Security.Profile) {
				ps = append(ps, unknownProfileProblem(m.ConfigPath, "ci."+name+".security.profile", st.Security.Profile, known))
			}
		}
	}
	return ps
}

func checkSuppressions(in Input) []Problem {
	if in.Suppressions == nil {
		return nil
	}
	var ps []Problem
	path := in.SuppressionsPath
	for _, s := range in.Suppressions.Suppressions {
		switch {
		case s.Expiry.IsZero():
			ps = append(ps, Problem{Error, path,
				fmt.Sprintf("suppression %q has no expiry (permanent suppressions are not allowed)", s.ID)})
		case s.Expiry.Time.Before(in.Now):
			ps = append(ps, Problem{Error, path,
				fmt.Sprintf("suppression %q expired on %s", s.ID, s.Expiry.Time.Format("2006-01-02"))})
		}
		if s.Owner == "" || s.Approver == "" || s.Reason == "" {
			ps = append(ps, Problem{Error, path,
				fmt.Sprintf("suppression %q must set reason, owner, and approver", s.ID)})
		}
	}
	return ps
}

func checkSlack(in Input) []Problem {
	s := in.Root.Slack
	if !s.Enabled || s.Channel == "" {
		return nil
	}
	if !strings.HasPrefix(s.Channel, "#") && !isChannelID(s.Channel) {
		return []Problem{{Warning, ".github/frodo-ci.yml",
			fmt.Sprintf("slack channel %q should start with # or be a channel ID", s.Channel)}}
	}
	return nil
}

func appendUnknownProfile(ps []Problem, path, field, value string, known []string) []Problem {
	if value == "" || contains(known, value) {
		return ps
	}
	return append(ps, unknownProfileProblem(path, field, value, known))
}

func unknownProfileProblem(path, field, value string, known []string) Problem {
	msg := fmt.Sprintf("%s references unknown profile %q", field, value)
	if s := schema.Suggest(value, known); s != "" {
		msg += fmt.Sprintf("; did you mean %q?", s)
	}
	return Problem{Error, path, msg}
}

func isChannelID(s string) bool {
	return len(s) >= 9 && (s[0] == 'C' || s[0] == 'G') && strings.ToUpper(s) == s
}

func profileNames(m map[string]config.LintProfile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func securityProfileNames(m map[string]config.SecurityProfile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedStageKeys(m map[string]config.ModuleStage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
