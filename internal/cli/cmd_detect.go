package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/scaffold"
)

// newScaffoldCommand detects modules from build metadata and proposes configs.
func newScaffoldCommand(app *App) *cobra.Command {
	var write, force bool
	var ownerFallback string
	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Detect modules from build metadata and propose .ci/module.yml files",
		Long: "Infers modules, types, owners, and coarse dependency edges from Maven\n" +
			"reactors, pnpm workspaces, and generic content (go.mod, Dockerfiles,\n" +
			"Helm/Kustomize/Terraform). Dry-run by default; --write to apply.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runScaffold(write, force, ownerFallback)
		},
	}
	f := cmd.Flags()
	f.BoolVar(&write, "write", false, "write the proposed module.yml files (default: dry run)")
	f.BoolVar(&force, "force", false, "overwrite existing module configs when writing")
	f.StringVar(&ownerFallback, "owner", "", "fallback owning team for modules with no CODEOWNERS match")
	return cmd
}

func (a *App) runScaffold(write, force bool, ownerFallback string) error {
	root := &config.RootConfig{}
	if rc, _, err := config.LoadRoot(filepath.Join(a.RepoRoot, ".github", "frodo-ci.yml")); err == nil {
		root = rc
	}
	root.ApplyDefaults()

	res, err := scaffold.Detect(a.RepoRoot, root)
	if err != nil {
		return err
	}
	if ownerFallback != "" {
		for i := range res.Modules {
			if len(res.Modules[i].Owners.Teams) == 0 && len(res.Modules[i].Owners.Users) == 0 {
				res.Modules[i].Owners.Teams = []string{ownerFallback}
			}
		}
	}
	// Warn only about modules that remain ownerless after any fallback.
	for _, m := range res.Modules {
		if len(m.Owners.Teams) == 0 && len(m.Owners.Users) == 0 {
			res.Warnings = append(res.Warnings,
				m.Path+": no owner (no CODEOWNERS match); set owners.teams or pass --owner")
		}
	}

	if a.JSON {
		return writeJSON(a.Out, res)
	}

	fmt.Fprintf(a.Out, "Detected %d module(s):\n", len(res.Modules))
	for _, m := range res.Modules {
		line := fmt.Sprintf("  %-30s %-14s owner=%s", m.Path, m.Type, scaffoldOwner(m.Owners))
		if len(m.DependsOn) > 0 {
			var ds []string
			for _, d := range m.DependsOn {
				ds = append(ds, d.Module)
			}
			line += "  depends_on: " + strings.Join(ds, ", ")
		}
		fmt.Fprintln(a.Out, line)
	}
	if len(res.Skipped) > 0 {
		fmt.Fprintf(a.Out, "\n%d already configured (skipped).\n", len(res.Skipped))
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(a.Out, "  ! %s\n", w)
	}

	if !write {
		fmt.Fprintf(a.Out, "\nDry run. Re-run with --write to create %d module.yml file(s).\n", len(res.Modules))
		return nil
	}

	written := 0
	for _, m := range res.Modules {
		dest := filepath.Join(a.RepoRoot, filepath.FromSlash(m.Path), filepath.FromSlash(root.ModuleFile))
		if !force && isFile(dest) {
			continue
		}
		data, err := scaffold.Render(m)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		written++
		fmt.Fprintf(a.Out, "wrote %s\n", filepath.ToSlash(filepath.Join(m.Path, root.ModuleFile)))
	}
	fmt.Fprintf(a.Out, "\nWrote %d module.yml file(s).\n", written)
	return nil
}

func scaffoldOwner(o scaffold.Owners) string {
	parts := append([]string{}, o.Teams...)
	parts = append(parts, o.Users...)
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ",")
}
