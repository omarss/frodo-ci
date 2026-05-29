package graph

import (
	"strings"
	"testing"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/discover"
)

func mod(name, dir string, deps ...config.Dependency) *discover.Module {
	return &discover.Module{
		Name:       name,
		Dir:        dir,
		ConfigPath: dir + "/.ci/module.yml",
		Config:     &config.ModuleConfig{Name: name, DependsOn: deps},
	}
}

func dep(name string, affects ...string) config.Dependency {
	return config.Dependency{Module: name, Affects: affects}
}

func hasMessage(problems []Problem, substr string) bool {
	for _, p := range problems {
		if strings.Contains(p.Message, substr) {
			return true
		}
	}
	return false
}

func TestBuildValid(t *testing.T) {
	mods := []*discover.Module{
		mod("cards", "services/cards", dep("money", "test", "build")),
		mod("money", "packages/money"),
	}
	g, problems := Build(mods, DefaultLimits())
	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
	if got := g.Order; len(got) != 2 || got[0] != "money" || got[1] != "cards" {
		t.Errorf("Order = %v, want [money cards]", got)
	}
	deps := g.Dependencies("cards")
	if len(deps) != 1 || deps[0].To != "money" || len(deps[0].Affects) != 2 {
		t.Errorf("cards dependencies = %+v", deps)
	}
	if d := g.DependentsOf("money"); len(d) != 1 || d[0].From != "cards" {
		t.Errorf("money dependents = %+v", d)
	}
}

func TestMissingModule(t *testing.T) {
	mods := []*discover.Module{mod("cards", "services/cards", dep("ghost"))}
	_, problems := Build(mods, DefaultLimits())
	if !hasMessage(problems, `unknown module "ghost"`) {
		t.Errorf("expected unknown-module problem, got %v", problems)
	}
}

func TestDuplicateName(t *testing.T) {
	mods := []*discover.Module{mod("x", "a"), mod("x", "b")}
	_, problems := Build(mods, DefaultLimits())
	if !hasMessage(problems, "duplicate module name") {
		t.Errorf("expected duplicate-name problem, got %v", problems)
	}
}

func TestCycle(t *testing.T) {
	mods := []*discover.Module{
		mod("a", "a", dep("b")),
		mod("b", "b", dep("a")),
	}
	g, problems := Build(mods, DefaultLimits())
	if !g.HasCycle || !hasMessage(problems, "cycle") {
		t.Errorf("expected a cycle, got problems %v hasCycle=%v", problems, g.HasCycle)
	}
}

func TestSelfDependency(t *testing.T) {
	mods := []*discover.Module{mod("a", "a", dep("a"))}
	_, problems := Build(mods, DefaultLimits())
	if !hasMessage(problems, "depends on itself") {
		t.Errorf("expected self-dependency problem, got %v", problems)
	}
}

func TestExcessiveFanOut(t *testing.T) {
	mods := []*discover.Module{
		mod("a", "a", dep("b"), dep("c"), dep("d")),
		mod("b", "b"), mod("c", "c"), mod("d", "d"),
	}
	_, problems := Build(mods, Limits{MaxDirectDependencies: 2})
	if !hasMessage(problems, "excessive fan-out") {
		t.Errorf("expected fan-out problem, got %v", problems)
	}
}

func TestTransitiveDependents(t *testing.T) {
	mods := []*discover.Module{
		mod("a", "a", dep("b")),
		mod("b", "b", dep("c")),
		mod("c", "c"),
	}
	g, _ := Build(mods, DefaultLimits())
	got := g.TransitiveDependents("c")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("TransitiveDependents(c) = %v, want [a b]", got)
	}
}

func TestPathProblems(t *testing.T) {
	m := mod("cards", "services/cards")
	m.Config.CI = map[string]config.ModuleStage{
		"scan": {
			When:   []config.Matcher{{Glob: "**"}},
			Inputs: []string{"../../../etc/passwd"},
		},
	}
	problems := PathProblems([]*discover.Module{m})
	if !hasMessage(problems, "broad pattern") {
		t.Errorf("expected broad-pattern problem, got %v", problems)
	}
	if !hasMessage(problems, "escapes the repository") {
		t.Errorf("expected repo-escape problem, got %v", problems)
	}
}
