package cli

import "github.com/spf13/cobra"

// newRunCommand executes the full plan (CI and, when applicable, CD).
func newRunCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Calculate the plan and execute all required stages (the final check)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci run")
		},
	}
}

// newCICommand executes only the CI stages.
func newCICommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "ci",
		Short: "Execute only the CI stages (validate, test, build, package, scan)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci ci")
		},
	}
}

// newCDCommand executes only the CD stages for the selected environment.
func newCDCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "cd",
		Short: "Execute only the CD stages (publish, deploy, verify)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci cd")
		},
	}
}
