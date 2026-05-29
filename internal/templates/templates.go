// Package templates loads module templates (profiles) and merges them with a
// module's config and stage-override files to produce the effective, runnable
// stages. Templates centralize defaults so a module only declares what differs.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/discover"
)

//go:embed defaults/*.yml
var defaultsFS embed.FS

// Template is a reusable profile: default trigger patterns and runnable steps
// for each stage, plus default quality profiles.
type Template struct {
	Name    string                 `yaml:"name,omitempty" json:"name,omitempty"`
	Type    string                 `yaml:"type,omitempty" json:"type,omitempty"`
	Quality config.QualityProfiles `yaml:"quality,omitempty" json:"quality,omitempty"`
	CI      map[string]Stage       `yaml:"ci,omitempty" json:"ci,omitempty"`
	CD      map[string]Stage       `yaml:"cd,omitempty" json:"cd,omitempty"`
}

// Stage is a template stage: trigger plus execution.
type Stage struct {
	When           []config.Matcher            `yaml:"when,omitempty" json:"when,omitempty"`
	Inputs         []string                    `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	TimeoutMinutes int                         `yaml:"timeout_minutes,omitempty" json:"timeout_minutes,omitempty"`
	Setup          map[string]config.SetupTool `yaml:"setup,omitempty" json:"setup,omitempty"`
	Cache          *config.StageCache          `yaml:"cache,omitempty" json:"cache,omitempty"`
	Steps          []config.StageStep          `yaml:"steps,omitempty" json:"steps,omitempty"`
	Security       *config.StageSecurity       `yaml:"security,omitempty" json:"security,omitempty"`
	Environments   []string                    `yaml:"environments,omitempty" json:"environments,omitempty"`
}

// EffectiveStage is a fully resolved, runnable stage after merging template,
// module config, and any stage-override file.
type EffectiveStage struct {
	Stage          string
	Group          string // "ci" or "cd"
	When           []config.Matcher
	Inputs         []string
	TimeoutMinutes int
	Setup          map[string]config.SetupTool
	Cache          *config.StageCache
	Steps          []config.StageStep
	Security       *config.StageSecurity
	Environments   []string
}

// DefaultNames lists the names of the built-in templates.
func DefaultNames() []string {
	entries, _ := fs.ReadDir(defaultsFS, "defaults")
	var names []string
	for _, e := range entries {
		names = append(names, strings.TrimSuffix(e.Name(), ".yml"))
	}
	sort.Strings(names)
	return names
}

// Loader resolves templates by profile name, preferring a repository's template
// directory and falling back to the built-in defaults.
type Loader struct {
	dir   string // repo templates dir (may be empty)
	cache map[string]*Template
}

// NewLoader returns a Loader rooted at the repo's templates directory.
func NewLoader(dir string) *Loader {
	return &Loader{dir: dir, cache: map[string]*Template{}}
}

// Get returns the template for a profile name, or nil if none is found.
func (l *Loader) Get(name string) (*Template, error) {
	if name == "" {
		return nil, nil
	}
	if t, ok := l.cache[name]; ok {
		return t, nil
	}
	if l.dir != "" {
		if data, err := readFile(filepath.Join(l.dir, name+".yml")); err == nil {
			t, perr := parse(data)
			if perr != nil {
				return nil, fmt.Errorf("template %s: %w", name, perr)
			}
			l.cache[name] = t
			return t, nil
		}
	}
	data, err := defaultsFS.ReadFile("defaults/" + name + ".yml")
	if err != nil {
		return nil, nil // unknown profile; the linter reports this
	}
	t, perr := parse(data)
	if perr != nil {
		return nil, fmt.Errorf("built-in template %s: %w", name, perr)
	}
	l.cache[name] = t
	return t, nil
}

// LoadDefault parses a built-in template by name.
func LoadDefault(name string) (*Template, error) {
	data, err := defaultsFS.ReadFile("defaults/" + name + ".yml")
	if err != nil {
		return nil, fmt.Errorf("no built-in template %q", name)
	}
	return parse(data)
}

func parse(data []byte) (*Template, error) {
	var t Template
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Resolve merges a template (may be nil) with a module's config and stage files
// to produce the effective stages, keyed by stage name. Module config overrides
// template triggers; a stage-override file overrides template execution.
func Resolve(tmpl *Template, m *discover.Module) map[string]EffectiveStage {
	out := map[string]EffectiveStage{}
	merge := func(group string, tplStages map[string]Stage, modStages map[string]config.ModuleStage) {
		names := map[string]bool{}
		for s := range tplStages {
			names[s] = true
		}
		for s := range modStages {
			names[s] = true
		}
		for name := range names {
			out[name] = resolveStage(group, name, tplStages[name], modStages[name], m.Stages[name])
		}
	}
	var ci, cd map[string]Stage
	if tmpl != nil {
		ci, cd = tmpl.CI, tmpl.CD
	}
	merge("ci", ci, m.Config.CI)
	merge("cd", cd, m.Config.CD)
	return out
}

func resolveStage(group, name string, tpl Stage, mod config.ModuleStage, file *discover.StageRef) EffectiveStage {
	es := EffectiveStage{
		Stage:          name,
		Group:          group,
		When:           tpl.When,
		Inputs:         tpl.Inputs,
		TimeoutMinutes: tpl.TimeoutMinutes,
		Setup:          tpl.Setup,
		Cache:          tpl.Cache,
		Steps:          tpl.Steps,
		Security:       tpl.Security,
		Environments:   tpl.Environments,
	}
	// Module config overrides triggers and CD targeting.
	if len(mod.When) > 0 {
		es.When = mod.When
	}
	if len(mod.Inputs) > 0 {
		es.Inputs = mod.Inputs
	}
	if mod.Security != nil {
		es.Security = mod.Security
	}
	if len(mod.Environments) > 0 {
		es.Environments = mod.Environments
	}
	// A stage-override file overrides execution.
	if file != nil {
		sf := file.File
		if sf.TimeoutMinutes > 0 {
			es.TimeoutMinutes = sf.TimeoutMinutes
		}
		if len(sf.Setup) > 0 {
			es.Setup = sf.Setup
		}
		if sf.Cache != nil {
			es.Cache = sf.Cache
		}
		if len(sf.Steps) > 0 {
			es.Steps = sf.Steps
		}
	}
	return es
}

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }
