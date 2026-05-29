// Package assets embeds the files that `frodo-ci init` writes into a repository:
// the root config, framework catalog, root workflow, VS Code settings, JSON
// Schemas, and module templates.
package assets

import (
	"embed"
	"os"
	"path/filepath"

	"github.com/omarss/frodo-ci/internal/schema"
	"github.com/omarss/frodo-ci/internal/templates"
)

//go:embed files
var filesFS embed.FS

type item struct{ embedded, dest string }

// manifest maps embedded asset paths to their destination repo-relative paths.
var manifest = []item{
	{"files/frodo-ci.yml", ".github/frodo-ci.yml"},
	{"files/toolchains.yml", ".github/frodo-ci/toolchains.yml"},
	{"files/security/baseline.yml", ".github/frodo-ci/security/baseline.yml"},
	{"files/security/suppressions.yml", ".github/frodo-ci/security/suppressions.yml"},
	{"files/security/rulesets.yml", ".github/frodo-ci/security/rulesets.yml"},
	{"files/lint/rules.yml", ".github/frodo-ci/lint/rules.yml"},
	{"files/performance/budgets.yml", ".github/frodo-ci/performance/budgets.yml"},
	{"files/workflow.yml", ".github/workflows/frodo-ci.yml"},
	{"files/vscode-settings.json", ".vscode/settings.json"},
}

// Result reports what Init wrote and what it left untouched.
type Result struct {
	Written []string
	Skipped []string
}

// Init scaffolds Frodo CI in repoRoot. Existing files are skipped unless force
// is set. It also exports the JSON Schemas and writes the module templates.
func Init(repoRoot string, force bool) (*Result, error) {
	res := &Result{}
	write := func(dest string, data []byte) error {
		abs := filepath.Join(repoRoot, filepath.FromSlash(dest))
		if !force {
			if _, err := os.Stat(abs); err == nil {
				res.Skipped = append(res.Skipped, dest)
				return nil
			}
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			return err
		}
		res.Written = append(res.Written, dest)
		return nil
	}

	for _, it := range manifest {
		data, err := filesFS.ReadFile(it.embedded)
		if err != nil {
			return nil, err
		}
		if err := write(it.dest, data); err != nil {
			return nil, err
		}
	}
	for _, k := range schema.ExportedKinds() {
		data, err := schema.JSON(k)
		if err != nil {
			return nil, err
		}
		if err := write(".github/frodo-ci/schemas/"+k.FileName(), append(data, '\n')); err != nil {
			return nil, err
		}
	}
	for _, name := range templates.DefaultNames() {
		data, err := templates.DefaultBytes(name)
		if err != nil {
			return nil, err
		}
		if err := write(".github/frodo-ci/templates/"+name+".yml", data); err != nil {
			return nil, err
		}
	}
	return res, nil
}
