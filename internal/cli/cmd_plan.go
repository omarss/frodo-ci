package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/frodo-ci/frodo-ci/internal/configlint"
	"github.com/frodo-ci/frodo-ci/internal/fingerprint"
	"github.com/frodo-ci/frodo-ci/internal/plan"
)

// newPlanCommand prints the execution plan calculated at startup.
func newPlanCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Calculate and print the execution plan without running it",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runPlan()
		},
	}
}

func (a *App) runPlan() error {
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	c, err := openCache(loaded.Root)
	if err != nil {
		return err
	}
	p, err := loaded.Plan(a.planContext(), c)
	if err != nil {
		return err
	}
	if a.JSON {
		if err := writeJSON(a.Out, p); err != nil {
			return err
		}
		if p.HasErrors() {
			return ErrExitQuiet
		}
		return nil
	}
	return a.printPlan(loaded, p)
}

func (a *App) printPlan(loaded *plan.Loaded, p *plan.Plan) error {
	if len(p.Problems) > 0 {
		fmt.Fprintln(a.Out, "Configuration problems:")
		printProblems(a.Out, p.Problems)
		fmt.Fprintln(a.Out)
	}
	fmt.Fprintf(a.Out, "Plan (event=%s", p.Context.Event)
	if p.Context.Base != "" {
		fmt.Fprintf(a.Out, ", base=%s", p.Context.Base)
	}
	fmt.Fprintf(a.Out, "): %d changed file(s)\n\n", len(p.Changes))

	for _, m := range p.Modules {
		fmt.Fprintf(a.Out, "%s (%s)  fp=%s\n", m.Name, m.Dir, fingerprint.Short(m.Fingerprint))
		for _, s := range m.Stages {
			status := "run"
			switch {
			case s.Skipped:
				status = "skip (cached)"
			case s.Cached:
				status = "run (cache hit)"
			}
			fmt.Fprintf(a.Out, "  %-4s %-9s %-14s %s\n", s.Group, s.Stage, status, strings.Join(s.Reasons, "; "))
		}
	}

	c := p.Summary()
	fmt.Fprintf(a.Out, "\n%d module(s), %d stage(s) planned, %d skipped via cache\n",
		c.Modules, c.RequiredStages, c.SkippedStages)
	a.printGovernance(loaded, p)
	if p.HasErrors() {
		fmt.Fprintln(a.Out, "\nplan is invalid: fix the configuration errors above")
		return ErrExitQuiet
	}
	return nil
}

// newExplainCommand explains why a given file triggers modules and stages.
func newExplainCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <file>",
		Short: "Explain which modules and stages a file affects, and why",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.runExplain(args[0])
		},
	}
}

func (a *App) runExplain(file string) error {
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	exps := loaded.Explain(file)
	if a.JSON {
		return writeJSON(a.Out, exps)
	}
	if len(exps) == 0 {
		fmt.Fprintf(a.Out, "%s does not affect any module stage\n", file)
		return nil
	}
	fmt.Fprintf(a.Out, "%s affects:\n", file)
	for _, e := range exps {
		fmt.Fprintf(a.Out, "  %s %s/%s  (%s)\n", e.Module, e.Group, e.Stage, e.Reason)
	}
	return nil
}

// newFingerprintCommand prints the deterministic fingerprint for a stage.
func newFingerprintCommand(app *App) *cobra.Command {
	var showInputs bool
	cmd := &cobra.Command{
		Use:   "fingerprint <module.stage>",
		Short: "Print the deterministic fingerprint for a module stage (e.g. cards.test)",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.runFingerprint(args[0], showInputs)
		},
	}
	cmd.Flags().BoolVar(&showInputs, "inputs", false, "also list the input files that fed the fingerprint")
	return cmd
}

func (a *App) runFingerprint(ref string, showInputs bool) error {
	moduleName, stage, ok := strings.Cut(ref, ".")
	if !ok || moduleName == "" || stage == "" {
		return fmt.Errorf("expected <module>.<stage>, e.g. cards.test")
	}
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	m := loaded.FindModule(moduleName)
	if m == nil {
		return fmt.Errorf("unknown module %q", moduleName)
	}
	fp, files, err := loaded.StageFingerprint(m, stage)
	if err != nil {
		return err
	}
	if a.JSON {
		return writeJSON(a.Out, map[string]any{"module": moduleName, "stage": stage, "fingerprint": fp, "inputs": files})
	}
	fmt.Fprintln(a.Out, fp)
	if showInputs {
		for _, f := range files {
			fmt.Fprintf(a.Out, "  %s\n", f)
		}
	}
	return nil
}

func writeJSON(w interface{ Write([]byte) (int, error) }, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printProblems(w interface{ Write([]byte) (int, error) }, problems []configlint.Problem) {
	for _, p := range problems {
		loc := p.Path
		if loc == "" {
			loc = "(config)"
		}
		fmt.Fprintf(w, "  [%s] %s\n", p.Severity, loc)
		for _, line := range strings.Split(p.Message, "\n") {
			fmt.Fprintf(w, "      %s\n", line)
		}
	}
}
