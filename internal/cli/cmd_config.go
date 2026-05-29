package cli

import "github.com/spf13/cobra"

// newValidateConfigCommand validates configuration against the JSON Schemas.
func newValidateConfigCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "validate-config",
		Short: "Validate all Frodo CI configuration against the JSON Schemas",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci validate-config")
		},
	}
}

// newLintConfigCommand runs semantic linting beyond structural schema checks.
func newLintConfigCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "lint-config",
		Short: "Lint configuration for semantic problems (cycles, broad inputs, weakening, ...)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci lint-config")
		},
	}
}
