package cli

import "github.com/spf13/cobra"

// newInitCommand bootstraps Frodo CI in a repository.
func newInitCommand(_ *App) *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap Frodo CI in the current repository",
		Long: "Creates or updates the root workflow, root config, JSON Schemas,\n" +
			"templates, toolchains, and VS Code settings.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci init")
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
}

// newInitModuleCommand scaffolds a new module's .ci/module.yml.
func newInitModuleCommand(_ *App) *cobra.Command {
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
	return cmd
}
