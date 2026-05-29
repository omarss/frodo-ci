// Package scaffold infers Frodo CI module wiring from a repository's existing
// build metadata (Maven reactors, pnpm workspaces, and generic content), so
// adopting a large monorepo does not require running init-module by hand for
// every module. Detection is general-purpose and heuristic: it proposes modules,
// types, owners, and coarse dependency edges for human review, never silently
// committing to a guess.
package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/match"
)

// defaultAffects is the coarse set of dependent stages re-run when a dependency
// changes. It is intentionally broad and meant to be refined by the reviewer.
var defaultAffects = []string{"test", "build", "package", "scan"}

// detected is one module a detector found, before naming and edge resolution.
type detected struct {
	Path    string   // repo-relative module directory
	Type    string   // template profile name
	Key     string   // dependency identity (maven groupId:artifactId, node package name); "" if none
	DepKeys []string // identities of internal dependencies
}

// Module is a proposed module after naming, edge resolution, and ownership.
type Module struct {
	Name      string       `json:"name"`
	Path      string       `json:"path"`
	Type      string       `json:"type"`
	Owners    Owners       `json:"owners,omitempty"`
	DependsOn []Dependency `json:"depends_on,omitempty"`
}

// Owners holds the owning teams/users resolved from CODEOWNERS.
type Owners struct {
	Teams []string `json:"teams,omitempty"`
	Users []string `json:"users,omitempty"`
}

// Dependency is a proposed dependency edge.
type Dependency struct {
	Module  string   `json:"module"`
	Affects []string `json:"affects"`
}

// Result is the full proposal.
type Result struct {
	Modules  []Module `json:"modules"`
	Skipped  []string `json:"skipped,omitempty"` // dirs that already have a module config
	Warnings []string `json:"warnings,omitempty"`
}

// Detect inspects the repository and proposes modules. It never writes anything.
func Detect(root string, rootCfg *config.RootConfig) (*Result, error) {
	moduleFile := rootCfg.ModuleFile
	if moduleFile == "" {
		moduleFile = ".ci/module.yml"
	}
	excludes := rootCfg.Scan.Exclude
	includes := rootCfg.Scan.Include

	var all []detected
	all = append(all, detectMaven(root, excludes)...)
	all = append(all, detectNode(root, excludes)...)
	all = append(all, detectGeneric(root, excludes, covered(all))...)

	res := &Result{}
	owners := loadCodeowners(root)

	// Filter by include scope and existing module configs; de-duplicate by path.
	seenPath := map[string]bool{}
	var kept []detected
	for _, d := range all {
		if seenPath[d.Path] {
			continue
		}
		seenPath[d.Path] = true
		if len(includes) > 0 && !underAnyInclude(includes, d.Path) {
			continue
		}
		if hasModuleConfig(root, d.Path, moduleFile) {
			res.Skipped = append(res.Skipped, d.Path)
			continue
		}
		kept = append(kept, d)
	}

	names := assignNames(kept, &res.Warnings)
	keyToName := map[string]string{}
	for _, d := range kept {
		if d.Key != "" {
			keyToName[d.Key] = names[d.Path]
		}
	}

	for _, d := range kept {
		m := Module{Name: names[d.Path], Path: d.Path, Type: d.Type}
		// Owners come from CODEOWNERS when matched; otherwise they are left empty
		// for the caller to fill (e.g. a --owner fallback) and warn about.
		if team, user, ok := owners.Owner(d.Path); ok {
			if team != "" {
				m.Owners.Teams = []string{team}
			}
			if user != "" {
				m.Owners.Users = []string{user}
			}
		}
		m.DependsOn = resolveDeps(d, names[d.Path], keyToName)
		res.Modules = append(res.Modules, m)
	}

	sort.Slice(res.Modules, func(i, j int) bool { return res.Modules[i].Path < res.Modules[j].Path })
	sort.Strings(res.Skipped)
	sort.Strings(res.Warnings)
	return res, nil
}

func resolveDeps(d detected, self string, keyToName map[string]string) []Dependency {
	seen := map[string]bool{}
	var deps []Dependency
	for _, k := range d.DepKeys {
		name, ok := keyToName[k]
		if !ok || name == self || seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, Dependency{Module: name, Affects: append([]string{}, defaultAffects...)})
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].Module < deps[j].Module })
	return deps
}

// assignNames derives a module name from each path's basename, disambiguating
// collisions by prepending the parent segment and warning.
func assignNames(mods []detected, warnings *[]string) map[string]string {
	byName := map[string][]string{}
	for _, d := range mods {
		byName[sanitizeName(path.Base(d.Path))] = append(byName[sanitizeName(path.Base(d.Path))], d.Path)
	}
	names := map[string]string{}
	for base, paths := range byName {
		if len(paths) == 1 {
			names[paths[0]] = base
			continue
		}
		for _, p := range paths {
			parent := sanitizeName(path.Base(path.Dir(p)))
			disambiguated := sanitizeName(parent + "-" + base)
			names[p] = disambiguated
			*warnings = append(*warnings, fmt.Sprintf("%s: name %q collides; proposing %q (review)", p, base, disambiguated))
		}
	}
	return names
}

// Render returns the module.yml content for a proposed module. It marshals the
// real config type, so output is always schema-valid.
func Render(m Module) ([]byte, error) {
	mc := config.ModuleConfig{
		Name: m.Name,
		Type: m.Type,
		Use:  config.ModuleUse{Profile: m.Type},
	}
	mc.Owners.Teams = m.Owners.Teams
	mc.Owners.Users = m.Owners.Users
	for _, d := range m.DependsOn {
		mc.DependsOn = append(mc.DependsOn, config.Dependency{Module: d.Module, Affects: d.Affects})
	}
	return yaml.Marshal(mc)
}

func sanitizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == ' ', r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	// Names must start with a letter per the default naming pattern.
	if out == "" || !(out[0] >= 'a' && out[0] <= 'z') {
		out = "m-" + out
	}
	return out
}

func covered(ds []detected) map[string]bool {
	m := map[string]bool{}
	for _, d := range ds {
		m[d.Path] = true
	}
	return m
}

func underAnyInclude(includes []string, dir string) bool {
	for _, inc := range includes {
		if match.Glob(inc, dir) {
			return true
		}
	}
	return false
}

// heavyDirs are skipped during detection walks regardless of config.
var heavyDirs = map[string]bool{
	".git": true, "node_modules": true, "target": true, "dist": true,
	"build": true, "vendor": true, ".next": true, ".gradle": true, "out": true,
	".github": true,
}

// walkFiles invokes fn for each file under root (repo-relative path), skipping
// heavy directories and any directory matching an exclude glob.
func walkFiles(root string, excludes []string, fn func(rel string)) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := relOf(root, p)
		if d.IsDir() {
			if rel != "." && (heavyDirs[d.Name()] || match.GlobAny(excludes, rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		fn(rel)
		return nil
	})
}

func relOf(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func hasModuleConfig(root, dir, moduleFile string) bool {
	return isFile(filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(moduleFile)))
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

func readFile(root, rel string) ([]byte, error) {
	return os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
}

// absUnder builds an OS path for rel inside dir inside root.
func absUnder(root, dir, rel string) string {
	return filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(rel))
}
