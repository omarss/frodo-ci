// Package graph builds the module dependency graph from depends_on/affects and
// rejects the integrity violations the requirements call out: missing modules,
// duplicate names, cycles, excessive fan-out, broad inputs, and repo-escaping
// paths.
package graph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/discover"
	"github.com/omarss/frodo-ci/internal/match"
)

// Edge is a dependency: From depends on To, and the listed Affects stages of
// From re-run when To changes.
type Edge struct {
	From    string
	To      string
	Affects []string
}

// Graph is the resolved module dependency graph.
type Graph struct {
	Modules  map[string]*discover.Module
	names    []string
	outEdges map[string][]Edge // From -> dependencies
	inEdges  map[string][]Edge // To   -> dependents
	Order    []string          // topological order, dependencies first
	HasCycle bool
}

// Problem is a human-friendly integrity violation tied to a config file.
type Problem struct {
	Module  string
	Path    string
	Message string
}

func (p Problem) String() string {
	if p.Path != "" {
		return fmt.Sprintf("%s: %s", p.Path, p.Message)
	}
	return p.Message
}

// Limits bounds graph shape. Defaults are generous so legitimate shared
// libraries (depended on by many modules) are never rejected; only pathological
// configs trip the guard.
type Limits struct {
	MaxDirectDependencies int
}

// DefaultLimits returns the standard graph limits.
func DefaultLimits() Limits { return Limits{MaxDirectDependencies: 50} }

// Build constructs the dependency graph and reports integrity problems. It
// returns a best-effort graph even when problems exist so callers can still
// inspect what resolved.
func Build(modules []*discover.Module, limits Limits) (*Graph, []Problem) {
	var problems []Problem
	byName := indexByName(modules, &problems)

	g := &Graph{
		Modules:  byName,
		outEdges: map[string][]Edge{},
		inEdges:  map[string][]Edge{},
	}
	for name := range byName {
		g.names = append(g.names, name)
	}
	sort.Strings(g.names)

	for _, name := range g.names {
		m := byName[name]
		if len(m.Config.DependsOn) > limits.MaxDirectDependencies {
			problems = append(problems, Problem{
				Module: name, Path: m.ConfigPath,
				Message: fmt.Sprintf("declares %d dependencies (max %d): likely excessive fan-out",
					len(m.Config.DependsOn), limits.MaxDirectDependencies),
			})
		}
		seen := map[string]bool{}
		for _, dep := range m.Config.DependsOn {
			switch {
			case dep.Module == "":
				continue
			case dep.Module == name:
				problems = append(problems, Problem{Module: name, Path: m.ConfigPath, Message: "module depends on itself"})
			case seen[dep.Module]:
				problems = append(problems, Problem{Module: name, Path: m.ConfigPath,
					Message: fmt.Sprintf("duplicate dependency on %q", dep.Module)})
			case byName[dep.Module] == nil:
				problems = append(problems, Problem{Module: name, Path: m.ConfigPath,
					Message: fmt.Sprintf("depends on unknown module %q", dep.Module)})
			default:
				seen[dep.Module] = true
				e := Edge{From: name, To: dep.Module, Affects: dep.Affects}
				g.outEdges[name] = append(g.outEdges[name], e)
				g.inEdges[dep.Module] = append(g.inEdges[dep.Module], e)
			}
		}
	}

	order, cyclic := topoSort(g)
	g.Order = order
	if len(cyclic) > 0 {
		g.HasCycle = true
		problems = append(problems, Problem{
			Message: fmt.Sprintf("dependency cycle detected involving: %s", strings.Join(cyclic, ", ")),
		})
	}

	sortProblems(problems)
	return g, problems
}

func indexByName(modules []*discover.Module, problems *[]Problem) map[string]*discover.Module {
	byName := make(map[string]*discover.Module, len(modules))
	for _, m := range modules {
		if m.Name == "" {
			*problems = append(*problems, Problem{Path: m.ConfigPath, Message: "module has no name"})
			continue
		}
		if prev, ok := byName[m.Name]; ok {
			*problems = append(*problems, Problem{
				Module: m.Name, Path: m.ConfigPath,
				Message: fmt.Sprintf("duplicate module name %q (already defined at %s)", m.Name, prev.ConfigPath),
			})
			continue
		}
		byName[m.Name] = m
	}
	return byName
}

// topoSort returns a deterministic topological order (dependencies first). Any
// nodes left unordered are part of a cycle and returned separately.
func topoSort(g *Graph) (order, cyclic []string) {
	remaining := make(map[string]int, len(g.names))
	for _, n := range g.names {
		remaining[n] = len(g.outEdges[n])
	}
	var queue []string
	for _, n := range g.names {
		if remaining[n] == 0 {
			queue = append(queue, n)
		}
	}
	for len(queue) > 0 {
		sort.Strings(queue)
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		for _, e := range g.inEdges[n] { // dependents of n
			remaining[e.From]--
			if remaining[e.From] == 0 {
				queue = append(queue, e.From)
			}
		}
	}
	if len(order) < len(g.names) {
		inOrder := make(map[string]bool, len(order))
		for _, n := range order {
			inOrder[n] = true
		}
		for _, n := range g.names {
			if !inOrder[n] {
				cyclic = append(cyclic, n)
			}
		}
	}
	return order, cyclic
}

func sortProblems(p []Problem) {
	sort.Slice(p, func(i, j int) bool {
		if p[i].Path != p[j].Path {
			return p[i].Path < p[j].Path
		}
		return p[i].Message < p[j].Message
	})
}

// Names returns module names in sorted order.
func (g *Graph) Names() []string { return g.names }

// Dependencies returns the edges from a module to its dependencies.
func (g *Graph) Dependencies(name string) []Edge { return g.outEdges[name] }

// DependentsOf returns the edges from modules that depend on name.
func (g *Graph) DependentsOf(name string) []Edge { return g.inEdges[name] }

// TransitiveDependents returns every module that directly or transitively
// depends on name (the change blast radius), sorted.
func (g *Graph) TransitiveDependents(name string) []string {
	seen := map[string]bool{}
	var stack []string
	for _, e := range g.inEdges[name] {
		stack = append(stack, e.From)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[n] {
			continue
		}
		seen[n] = true
		for _, e := range g.inEdges[n] {
			stack = append(stack, e.From)
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// PathProblems reports broad-glob and repo-escaping when/inputs across modules.
func PathProblems(modules []*discover.Module) []Problem {
	var problems []Problem
	for _, m := range modules {
		problems = append(problems, modulePathProblems(m, "ci", m.Config.CI)...)
		problems = append(problems, modulePathProblems(m, "cd", m.Config.CD)...)
	}
	sortProblems(problems)
	return problems
}

func modulePathProblems(m *discover.Module, group string, stages map[string]config.ModuleStage) []Problem {
	var problems []Problem
	for _, stage := range sortedKeys(stages) {
		ms := stages[stage]
		for _, w := range ms.When {
			if w.IsRegex() {
				continue
			}
			if match.IsBroadGlob(w.Glob) {
				problems = append(problems, Problem{Module: m.Name, Path: m.ConfigPath,
					Message: fmt.Sprintf("%s.%s.when has a broad pattern %q that matches most of the repository", group, stage, w.Glob)})
			}
			if _, esc := match.Resolve(m.Dir, w.Glob); esc {
				problems = append(problems, Problem{Module: m.Name, Path: m.ConfigPath,
					Message: fmt.Sprintf("%s.%s.when path %q escapes the repository", group, stage, w.Glob)})
			}
		}
		for _, in := range ms.Inputs {
			if match.IsBroadGlob(in) {
				problems = append(problems, Problem{Module: m.Name, Path: m.ConfigPath,
					Message: fmt.Sprintf("%s.%s.inputs has a broad pattern %q that matches most of the repository", group, stage, in)})
			}
			if _, esc := match.Resolve(m.Dir, in); esc {
				problems = append(problems, Problem{Module: m.Name, Path: m.ConfigPath,
					Message: fmt.Sprintf("%s.%s.inputs path %q escapes the repository", group, stage, in)})
			}
		}
	}
	return problems
}

func sortedKeys(m map[string]config.ModuleStage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
