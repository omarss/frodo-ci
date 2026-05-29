// Package discover finds and loads the modules in a repository: directories
// that contain a .ci/module.yml, honoring the root config's scan include and
// exclude rules.
package discover

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/match"
)

// Module is a discovered module: its location, parsed config, and any present
// stage-override files.
type Module struct {
	Name       string
	Dir        string // repo-relative module directory, e.g. "services/cards"
	AbsDir     string
	ConfigPath string // repo-relative path to .ci/module.yml
	Config     *config.ModuleConfig
	Source     *config.Source
	Stages     map[string]*StageRef // present stage files, keyed by stage name
}

// StageRef is a loaded stage-override file.
type StageRef struct {
	Stage  string
	Path   string // repo-relative
	File   *config.StageFile
	Source *config.Source
}

// heavyDirs are skipped during the walk regardless of config.
var heavyDirs = map[string]bool{
	".git": true, "node_modules": true, "target": true, "dist": true,
	"build": true, "vendor": true, ".next": true, ".gradle": true, "out": true,
}

// Discover walks root and returns the modules it finds, sorted by directory for
// deterministic planning. scan.exclude prunes the walk; scan.include (when set)
// restricts which discovered modules are kept.
func Discover(root string, rootCfg *config.RootConfig) ([]*Module, error) {
	moduleBase := moduleFileBase(rootCfg.ModuleFile) // "module.yml"
	stageNames := rootCfg.AllStages()
	excludes := rootCfg.Scan.Exclude
	includes := rootCfg.Scan.Include

	var modules []*Module
	walkErr := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable entries
		}
		if d.IsDir() {
			rel := relOf(root, p)
			if rel != "." && (heavyDirs[d.Name()] || match.GlobAny(excludes, rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != moduleBase || filepath.Base(filepath.Dir(p)) != ".ci" {
			return nil
		}
		absDir := filepath.Dir(filepath.Dir(p)) // parent of the .ci directory
		dir := relOf(root, absDir)
		if len(includes) > 0 && !match.GlobAny(includes, dir) {
			return nil
		}
		m, loadErr := loadModule(root, absDir, dir, stageNames)
		if loadErr != nil {
			return loadErr
		}
		modules = append(modules, m)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	sort.Slice(modules, func(i, j int) bool { return modules[i].Dir < modules[j].Dir })
	return modules, nil
}

func loadModule(root, absDir, dir string, stageNames []string) (*Module, error) {
	cfgPath := filepath.Join(absDir, ".ci", "module.yml")
	cfg, src, err := config.LoadModule(cfgPath)
	if err != nil {
		return nil, err
	}
	m := &Module{
		Name:       cfg.Name,
		Dir:        dir,
		AbsDir:     absDir,
		ConfigPath: relOf(root, cfgPath),
		Config:     cfg,
		Source:     src,
		Stages:     map[string]*StageRef{},
	}
	for _, stage := range stageNames {
		sp := filepath.Join(absDir, ".ci", stage+".yml")
		if !isFile(sp) {
			continue
		}
		sf, ssrc, err := config.LoadStage(sp)
		if err != nil {
			return nil, err
		}
		m.Stages[stage] = &StageRef{Stage: stage, Path: relOf(root, sp), File: sf, Source: ssrc}
	}
	return m, nil
}

// moduleFileBase extracts the file name from the configured module_file path
// (".ci/module.yml" -> "module.yml").
func moduleFileBase(moduleFile string) string {
	if moduleFile == "" {
		return "module.yml"
	}
	return filepath.Base(moduleFile)
}

func relOf(root, p string) string {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ByName indexes modules by name. Names that appear more than once map to the
// first occurrence; duplicate detection is the dependency graph's job.
func ByName(modules []*Module) map[string]*Module {
	out := make(map[string]*Module, len(modules))
	for _, m := range modules {
		if m.Name == "" {
			continue
		}
		if _, exists := out[m.Name]; !exists {
			out[m.Name] = m
		}
	}
	return out
}
