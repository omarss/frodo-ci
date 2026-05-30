package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/omarss/frodo-ci/internal/antiweaken"
	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/omarss/frodo-ci/internal/protected"
	"github.com/omarss/frodo-ci/internal/runner"
	"github.com/omarss/frodo-ci/internal/security"
	"github.com/omarss/frodo-ci/internal/vcs"
)

// securityAdapter bridges security.Scanner to the runner's SecurityScanner hook
// so a scan stage with no steps runs the smart scan plan.
type securityAdapter struct{ s *security.Scanner }

func (a securityAdapter) Scan(ctx context.Context, module string) (runner.Status, string) {
	res := a.s.Run(ctx, module)
	if res.OK {
		return runner.StatusSuccess, res.Note
	}
	return runner.StatusFailure, res.Note
}

// buildScanner wires a smart security scanner: it attributes changed files to
// modules by directory prefix, resolves each module's security profile, and
// passes the suppressions and rulesets so findings are gated, not just listed.
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
	moduleDir := func(module string) string {
		m := loaded.FindModule(module)
		if m == nil {
			return loaded.RepoRoot
		}
		return filepath.Join(loaded.RepoRoot, filepath.FromSlash(m.Dir))
	}
	var sups []config.Suppression
	if loaded.Catalog.Suppressions != nil {
		sups = loaded.Catalog.Suppressions.Suppressions
	}
	return securityAdapter{&security.Scanner{
		Event:           p.Context.Event,
		OnDefaultBranch: p.Context.OnDefaultBranch,
		ModuleChanged:   changedByModule,
		ModuleDir:       moduleDir,
		ProfileFor:      func(module string) config.SecurityProfile { return securityProfile(loaded, module) },
		Suppressions:    sups,
		Rulesets:        loaded.Catalog.Rulesets,
	}}
}

// securityProfile resolves the security posture for a module: its
// quality.security profile if set, else the repo-wide blocking_profile, else a
// safe default (block new criticals and any secret).
func securityProfile(loaded *plan.Loaded, module string) config.SecurityProfile {
	name := ""
	if m := loaded.FindModule(module); m != nil && m.Config != nil {
		name = m.Config.Quality.Security
	}
	if name == "" {
		name = loaded.Root.Security.BlockingProfile
	}
	if sb := loaded.Catalog.SecurityBaseline; sb != nil && name != "" {
		if prof, ok := sb.Profiles[name]; ok {
			return prof
		}
	}
	return config.SecurityProfile{
		FailOnNewCritical:             loaded.Root.Security.FailOnNewCritical,
		FailOnNewHighWhenFixAvailable: loaded.Root.Security.FailOnNewHighWhenFixAvailable,
		FailOnSecrets:                 true,
	}
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
