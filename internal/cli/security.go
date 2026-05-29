package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/frodo-ci/frodo-ci/internal/antiweaken"
	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/plan"
	"github.com/frodo-ci/frodo-ci/internal/protected"
	"github.com/frodo-ci/frodo-ci/internal/runner"
	"github.com/frodo-ci/frodo-ci/internal/security"
	"github.com/frodo-ci/frodo-ci/internal/vcs"
)

// securityAdapter bridges security.Scanner to the runner's SecurityScanner hook
// so a scan stage with no steps runs the smart scan plan.
type securityAdapter struct{ s *security.Scanner }

func (a securityAdapter) Scan(_ context.Context, module string) (runner.Status, string) {
	res := a.s.Run(module)
	if res.OK {
		return runner.StatusSuccess, res.Note
	}
	return runner.StatusFailure, res.Note
}

// buildScanner wires a smart security scanner that attributes changed files to
// modules by directory prefix.
func (a *App) buildScanner(loaded *plan.Loaded, p *plan.Plan) runner.SecurityScanner {
	changedByModule := func(module string) []string {
		m := loaded.FindModule(module)
		if m == nil {
			return nil
		}
		var out []string
		for _, f := range p.Changes {
			if f == m.Dir || strings.HasPrefix(f, m.Dir+"/") {
				out = append(out, f)
			}
		}
		return out
	}
	return securityAdapter{&security.Scanner{
		Event:           p.Context.Event,
		OnDefaultBranch: p.Context.OnDefaultBranch,
		ModuleChanged:   changedByModule,
	}}
}

// printGovernance reports protected-file matches and anti-weakening findings for
// a plan. Anti-weakening needs the base revision of the root config, fetched via
// git when a base ref is known.
func (a *App) printGovernance(loaded *plan.Loaded, p *plan.Plan) {
	matches := protected.Matches(p.Changes, loaded.Root.ProtectedFiles)
	if len(matches) > 0 {
		fmt.Fprintln(a.Out, "\nProtected files changed (extra approval required):")
		for _, m := range matches {
			fmt.Fprintf(a.Out, "  %s -> require %v\n", m.Name, m.Require.Teams)
		}
	}

	if p.Context.Base == "" {
		return
	}
	g := vcs.New(a.RepoRoot)
	data, err := g.ShowFile(p.Context.Base, plan.RootConfigPath)
	if err != nil {
		return
	}
	baseRoot, err := config.ParseRoot(data)
	if err != nil {
		return
	}
	baseRoot.ApplyDefaults()
	weak := antiweaken.Root(plan.RootConfigPath, baseRoot, loaded.Root)
	if len(weak) > 0 {
		fmt.Fprintln(a.Out, "\nGovernance-weakening changes (require higher approval):")
		for _, w := range weak {
			fmt.Fprintf(a.Out, "  %s: %s\n", w.Path, w.Detail)
		}
	}
}
