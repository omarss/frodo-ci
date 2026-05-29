package cli

import "github.com/spf13/cobra"

// newPlanCommand prints the execution plan calculated at startup.
func newPlanCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Calculate and print the execution plan without running it",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci plan")
		},
	}
}

// newExplainCommand explains why a given file triggers modules and stages.
func newExplainCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <file>",
		Short: "Explain which modules and stages a file affects, and why",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci explain")
		},
	}
}

// newFingerprintCommand prints the deterministic fingerprint for a stage.
func newFingerprintCommand(_ *App) *cobra.Command {
	return &cobra.Command{
		Use:   "fingerprint <module.stage>",
		Short: "Print the deterministic fingerprint for a module stage (e.g. cards.test)",
		Args:  cobra.ExactArgs(1),
		RunE: func(*cobra.Command, []string) error {
			return notImplemented("frodo-ci fingerprint")
		},
	}
}
