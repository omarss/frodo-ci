package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/frodo-ci/frodo-ci/internal/schema"
)

// configFile pairs a config file with the schema kind that validates it.
type configFile struct {
	Kind schema.Kind
	Path string // absolute path
	Rel  string // repo-relative path, for display
}

// standardStages is the set of stage names a .ci/<stage>.yml file may use.
var standardStages = map[string]bool{
	"validate": true, "test": true, "build": true, "package": true, "scan": true,
	"publish": true, "deploy": true, "verify": true,
}

// frameworkConfigCandidates are the fixed-location framework config files.
var frameworkConfigCandidates = []struct {
	kind schema.Kind
	rel  string
}{
	{schema.KindRoot, ".github/frodo-ci.yml"},
	{schema.KindToolchains, ".github/frodo-ci/toolchains.yml"},
	{schema.KindSecurity, ".github/frodo-ci/security/baseline.yml"},
	{schema.KindSuppressions, ".github/frodo-ci/security/suppressions.yml"},
	{schema.KindRulesets, ".github/frodo-ci/security/rulesets.yml"},
	{schema.KindLint, ".github/frodo-ci/lint/rules.yml"},
	{schema.KindPerformance, ".github/frodo-ci/performance/budgets.yml"},
}

// discoverConfigFiles returns every Frodo CI config file under root, paired with
// the schema kind that validates it: the framework files at their fixed
// locations plus all module/stage files found in .ci directories.
func discoverConfigFiles(root string) ([]configFile, error) {
	var files []configFile

	for _, c := range frameworkConfigCandidates {
		abs := filepath.Join(root, c.rel)
		if isFile(abs) {
			files = append(files, configFile{Kind: c.kind, Path: abs, Rel: c.rel})
		}
	}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // tolerate unreadable entries; validation covers what we can read
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(filepath.Dir(p)) != ".ci" {
			return nil
		}
		if ext := filepath.Ext(p); ext != ".yml" && ext != ".yaml" {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		base := filepath.Base(p)
		switch {
		case base == "module.yml" || base == "module.yaml":
			files = append(files, configFile{Kind: schema.KindModule, Path: p, Rel: rel})
		default:
			name := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
			if standardStages[name] {
				files = append(files, configFile{Kind: schema.KindStage, Path: p, Rel: rel})
			}
			// Non-stage .ci/*.yml files are left for the semantic linter to flag.
		}
		return nil
	})
	return files, err
}

// shouldSkipDir skips heavy or irrelevant directories during the walk.
func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "target", "dist", "build", "vendor", ".next", ".gradle", "bin", "out":
		return true
	default:
		return false
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
