package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/frodo-ci/frodo-ci/internal/assets"
	"github.com/frodo-ci/frodo-ci/internal/templates"
)

// newInitCommand bootstraps Frodo CI in a repository.
func newInitCommand(app *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap Frodo CI in the current repository",
		Long: "Creates the root workflow, root config, JSON Schemas, templates,\n" +
			"toolchains, security/lint/performance config, and VS Code settings.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runInit(force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
}

func (a *App) runInit(force bool) error {
	res, err := assets.Init(a.RepoRoot, force)
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
	cmd := &cobra.Command{
		Use:   "init-module",
		Short: "Scaffold a new module's .ci/module.yml from a template",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci init-module")
		},
	}
	f := cmd.Flags()
	f.StringVar(&name, "name", "", "module name (required)")
	f.StringVar(&typ, "type", "", "module type / template profile (required)")
	f.StringVar(&path, "path", "", "module path relative to the repo root (required)")
	f.StringVar(&owner, "owner", "", "owning team (required)")
	for _, req := range []string{"name", "type", "path", "owner"} {
		_ = cmd.MarkFlagRequired(req)
	}
	cmd.RunE = func(*cobra.Command, []string) error {
		return app.runInitModule(name, typ, path, owner)
	}
	return cmd
}

func (a *App) runInitModule(name, typ, path, owner string) error {
	dest := filepath.Join(a.RepoRoot, filepath.FromSlash(path), ".ci", "module.yml")
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", filepath.Join(path, ".ci", "module.yml"))
	}
	if !slices.Contains(templates.DefaultNames(), typ) {
		fmt.Fprintf(a.Err, "warning: %q is not a built-in template; available: %s\n",
			typ, strings.Join(templates.DefaultNames(), ", "))
	}
	content := fmt.Sprintf("name: %s\ntype: %s\n\nuse:\n  profile: %s\n\nowners:\n  teams:\n    - %s\n",
		name, typ, typ, owner)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "created %s\n", filepath.Join(path, ".ci", "module.yml"))
	return nil
}
