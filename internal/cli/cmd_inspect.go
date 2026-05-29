package cli

import "github.com/spf13/cobra"

// newReviewCommand evaluates review and approval requirements for the PR.
func newReviewCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Evaluate review, owner, and expert requirements for the current PR",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci review")
		},
	}
}

// newDoctorCommand performs environment and configuration health checks.
func newDoctorCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the environment and configuration for common problems",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci doctor")
		},
	}
}

// newSchemasCommand groups schema-related subcommands.
func newSchemasCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schemas",
		Short: "Work with Frodo CI JSON Schemas",
	}
	cmd.AddCommand(newSchemasExportCommand(app))
	return cmd
}

// newSchemasExportCommand writes all JSON Schemas to a directory.
func newSchemasExportCommand(_ *App) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "export",
		Short: "Write JSON Schemas for all config files to a directory",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci schemas export")
		},
	}
	c.Flags().StringVar(&out, "out", ".github/frodo-ci/schemas", "output directory for schema files")
	return c
}

// newTemplatesCommand groups template-related subcommands.
func newTemplatesCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "Inspect Frodo CI module templates",
	}
	cmd.AddCommand(newTemplatesListCommand(app), newTemplatesExplainCommand(app))
	return cmd
}

// newTemplatesListCommand lists available templates.
func newTemplatesListCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available module templates",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci templates list")
		},
	}
}

// newTemplatesExplainCommand explains one template's stages and defaults.
func newTemplatesExplainCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <template>",
		Short: "Explain a template's stages and defaults",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci templates explain")
		},
	}
}
