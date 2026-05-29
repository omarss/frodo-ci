package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/omarss/frodo-ci/internal/logging"
	"github.com/omarss/frodo-ci/internal/version"
)

// NewRootCommand builds the full frodo-ci command tree.
func NewRootCommand() *cobra.Command {
	app := &App{Out: os.Stdout, Err: os.Stderr}

	root := &cobra.Command{
		Use:   "frodo-ci",
		Short: "Opinionated modular CI/CD framework for monorepos",
		Long: "Frodo CI lets every module own its local automation under .ci, while the\n" +
			"platform keeps the whole repository fast, secure, reviewable, and predictable\n" +
			"through one final merge check.",
		Version:       version.Info(),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return app.resolve()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&app.RepoRoot, "repo", "C", "", "repository root (default: current directory)")
	pf.StringVar(&app.Base, "base", "", "base git ref for change detection (default: auto-detected)")
	pf.StringVar(&app.Head, "head", "", "head git ref for change detection (default: HEAD)")
	pf.StringVar(&app.Environment, "environment", "staging", "target environment for CD stages")
	pf.StringVar(&app.LogLevel, "log-level", "info", "log verbosity: trace|debug|info|warn|error")
	pf.BoolVar(&app.JSON, "json", false, "emit machine-readable JSON where supported")

	root.AddCommand(
		newInitCommand(app),
		newInitModuleCommand(app),
		newScaffoldCommand(app),
		newValidateConfigCommand(app),
		newLintConfigCommand(app),
		newPlanCommand(app),
		newRunCommand(app),
		newCICommand(app),
		newCDCommand(app),
		newReviewCommand(app),
		newExplainCommand(app),
		newDoctorCommand(app),
		newSchemasCommand(app),
		newTemplatesCommand(app),
		newFingerprintCommand(app),
	)
	return root
}

// resolve normalizes global options and initializes the logger. It runs before
// every command via PersistentPreRunE.
func (a *App) resolve() error {
	if a.RepoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("determine working directory: %w", err)
		}
		a.RepoRoot = wd
	}
	abs, err := filepath.Abs(a.RepoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root %q: %w", a.RepoRoot, err)
	}
	a.RepoRoot = abs
	if a.Out == nil {
		a.Out = os.Stdout
	}
	if a.Err == nil {
		a.Err = os.Stderr
	}
	a.Log = logging.New(a.LogLevel, !a.JSON, a.Err)
	return nil
}
