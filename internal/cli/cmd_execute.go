package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/omarss/frodo-ci/internal/cache"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/omarss/frodo-ci/internal/runner"
)

// newRunCommand executes the full plan (the final check).
func newRunCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Calculate the plan and execute all required stages (the final check)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runExecute(cmd.Context(), "all")
		},
	}
}

// newCICommand executes only the CI stages.
func newCICommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "ci",
		Short: "Execute only the CI stages (validate, test, build, package, scan)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runExecute(cmd.Context(), "ci")
		},
	}
}

// newCDCommand executes only the CD stages for the selected environment.
func newCDCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "cd",
		Short: "Execute only the CD stages (publish, deploy, verify)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runExecute(cmd.Context(), "cd")
		},
	}
}

func (a *App) runExecute(ctx context.Context, group string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	// Invalid configuration must fail before any expensive setup.
	if loaded.HasErrors() {
		fmt.Fprintln(a.Err, "Configuration is invalid; refusing to run:")
		printProblems(a.Err, loaded.Problems)
		return ErrExitQuiet
	}

	c, err := openCache(loaded.Root)
	if err != nil {
		return err
	}
	p, err := loaded.Plan(a.planContext(), c)
	if err != nil {
		return err
	}

	opts := a.runnerOptions(loaded, c)
	opts.Reporter = maybeReporter(ctx)
	opts.Scanner = a.buildScanner(loaded, p)
	result := runner.New(loaded, opts).Run(ctx, p, group)

	budgetExceeded := a.reportRun(loaded, p, result)
	a.publishReport(ctx, loaded, p, result)

	if a.JSON {
		if err := writeJSON(a.Out, result); err != nil {
			return err
		}
	} else {
		a.printResult(result)
		if budgetExceeded {
			fmt.Fprintln(a.Out, "performance budget exceeded")
		}
	}
	if !result.Success || budgetExceeded {
		return ErrExitQuiet
	}
	return nil
}

func (a *App) runnerOptions(loaded *plan.Loaded, c cache.Cache) runner.Options {
	ex := loaded.Root.Execution
	return runner.Options{
		Env:                  map[string]string{},
		MaxParallelModules:   ex.MaxParallelModules,
		MaxParallelExpensive: ex.MaxParallelExpensiveStages,
		FullRunTimeout:       time.Duration(ex.FullRunTimeoutMinutes) * time.Minute,
		NoProgressTimeout:    time.Duration(ex.NoProgressTimeoutMinutes) * time.Minute,
		StageDefaultTimeout:  30 * time.Minute,
		StopModuleOnFailure:  ex.StopModuleOnStageFailure,
		StopDependents:       ex.StopDependentsOnDependencyFailure,
		Cache:                c,
		ImagePrefix:          loaded.Root.Images.Prefix,
		Log:                  a.Log,
	}
}

func (a *App) printResult(result *runner.Result) {
	for _, s := range result.Stages {
		fmt.Fprintf(a.Out, "%-8s %s/%s/%s  (%s)\n", s.Status, s.Module, s.Group, s.Stage, s.Duration.Round(time.Millisecond))
		if s.Note != "" {
			fmt.Fprintf(a.Out, "         %s\n", s.Note)
		}
	}
	fmt.Fprintf(a.Out, "\n%s\n", result.Summary())
}
