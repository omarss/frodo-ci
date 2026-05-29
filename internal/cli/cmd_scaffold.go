package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/omarss/frodo-ci/internal/assets"
	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/templates"
)

// newInitCommand bootstraps Frodo CI in a repository.
func newInitCommand(app *App) *cobra.Command {
	var force bool
	var actionRef string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap Frodo CI in the current repository",
		Long: "Creates the root workflow, root config, JSON Schemas, templates,\n" +
			"toolchains, security/lint/performance config, and VS Code settings.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runInit(force, actionRef)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	cmd.Flags().StringVar(&actionRef, "action-ref", assets.DefaultActionRef,
		"GitHub Action reference written into the workflow (for forks/mirrors)")
	return cmd
}

func (a *App) runInit(force bool, actionRef string) error {
	res, err := assets.Init(a.RepoRoot, force, actionRef)
	if err != nil {
		return err
	}
	for _, f := range res.Written {
		fmt.Fprintf(a.Out, "  created  %s\n", f)
	}
	for _, f := range res.Skipped {
		fmt.Fprintf(a.Out, "  exists   %s (use --force to overwrite)\n", f)
	}
	fmt.Fprintf(a.Out, "\nFrodo CI initialized: %d written, %d skipped.\n", len(res.Written), len(res.Skipped))
	fmt.Fprintln(a.Out, "Next: add modules with `frodo-ci init-module`, then require only")
	fmt.Fprintln(a.Out, "the \"Frodo CI / final\" status check in branch protection.")
	return nil
}

// newInitModuleCommand scaffolds a new module's .ci/module.yml.
func newInitModuleCommand(app *App) *cobra.Command {
	var name, typ, path, owner string
	var dependsOn []string
	var force bool
	cmd := &cobra.Command{
		Use:   "init-module",
		Short: "Scaffold a new module's .ci/module.yml from a template",
		Args:  cobra.NoArgs,
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "module name (required)")
	f.StringVar(&typ, "type", "", "module type / template profile (required)")
	f.StringVar(&path, "path", "", "module path relative to the repo root (required)")
	f.StringVar(&owner, "owner", "", "owning team (required)")
	f.StringArrayVar(&dependsOn, "depends-on", nil,
		"dependency edge, repeatable: --depends-on money[:affects=test,build,scan]")
	f.BoolVar(&force, "force", false, "overwrite an existing module.yml")
	for _, req := range []string{"name", "type", "path", "owner"} {
		_ = cmd.MarkFlagRequired(req)
	}
	cmd.RunE = func(*cobra.Command, []string) error {
		return app.runInitModule(name, typ, path, owner, parseDependsOn(dependsOn), force)
	}
	return cmd
}

func (a *App) runInitModule(name, typ, path, owner string, deps []config.Dependency, force bool) error {
	dest := filepath.Join(a.RepoRoot, filepath.FromSlash(path), ".ci", "module.yml")
	if !force {
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("%s already exists (use --force to overwrite)", filepath.Join(path, ".ci", "module.yml"))
		}
	}
	if !slices.Contains(templates.DefaultNames(), typ) {
		fmt.Fprintf(a.Err, "warning: %q is not a built-in template; available: %s\n",
			typ, strings.Join(templates.DefaultNames(), ", "))
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, []byte(renderModule(name, typ, owner, deps)), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "created %s\n", filepath.Join(path, ".ci", "module.yml"))
	return nil
}

// renderModule produces a module.yml body, including any dependency edges.
func renderModule(name, typ, owner string, deps []config.Dependency) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\ntype: %s\n\nuse:\n  profile: %s\n\nowners:\n  teams:\n    - %s\n",
		name, typ, typ, owner)
	if len(deps) > 0 {
		b.WriteString("\ndepends_on:\n")
		for _, d := range deps {
			fmt.Fprintf(&b, "  - module: %s\n", d.Module)
			if len(d.Affects) > 0 {
				fmt.Fprintf(&b, "    affects: [%s]\n", strings.Join(d.Affects, ", "))
			}
		}
	}
	return b.String()
}

// parseDependsOn parses "--depends-on module[:affects=a,b,c]" specs. When the
// affects list is omitted it defaults to the post-validate CI stages.
func parseDependsOn(specs []string) []config.Dependency {
	var out []config.Dependency
	for _, s := range specs {
		mod, rest, hasRest := strings.Cut(strings.TrimSpace(s), ":")
		mod = strings.TrimSpace(mod)
		if mod == "" {
			continue
		}
		d := config.Dependency{Module: mod}
		if hasRest {
			if csv, ok := strings.CutPrefix(strings.TrimSpace(rest), "affects="); ok {
				for _, a := range strings.Split(csv, ",") {
					if a = strings.TrimSpace(a); a != "" {
						d.Affects = append(d.Affects, a)
					}
				}
			}
		}
		if len(d.Affects) == 0 {
			d.Affects = []string{"test", "build", "package", "scan"}
		}
		out = append(out, d)
	}
	return out
}
