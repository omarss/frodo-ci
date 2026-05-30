package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"

	"github.com/omarss/frodo-ci/internal/config"
)

// newSuppressCommand manages security finding suppressions in
// .github/frodo-ci/security/suppressions.yml entirely via the CLI, so accepting
// a false positive never means hand-editing YAML. Every suppression must carry a
// future expiry, enforced at write time, so a real finding re-surfaces later.
func newSuppressCommand(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "suppress",
		Short: "Manage time-bounded security finding suppressions",
	}
	cmd.AddCommand(newSuppressAddCommand(app), newSuppressListCommand(app), newSuppressPruneCommand(app))
	return cmd
}

func newSuppressAddCommand(app *App) *cobra.Command {
	var id, path, reason, owner, approver, expiry string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a suppression (requires a future expiry)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runSuppressAdd(id, path, reason, owner, approver, expiry, time.Now())
		},
	}
	f := cmd.Flags()
	f.StringVar(&id, "id", "", "finding rule id to suppress (required)")
	f.StringVar(&path, "path", "", "path glob to scope the suppression (optional)")
	f.StringVar(&reason, "reason", "", "why the finding is accepted (required)")
	f.StringVar(&owner, "owner", "", "owning team or user (required)")
	f.StringVar(&approver, "approver", "", "who approved the suppression (required)")
	f.StringVar(&expiry, "expiry", "", "expiry date YYYY-MM-DD; must be in the future (required)")
	for _, r := range []string{"id", "reason", "owner", "approver", "expiry"} {
		_ = cmd.MarkFlagRequired(r)
	}
	return cmd
}

func newSuppressListCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List suppressions and whether each is active or expired",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return app.runSuppressList(time.Now()) },
	}
}

func newSuppressPruneCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove expired suppressions",
		Args:  cobra.NoArgs,
		RunE:  func(*cobra.Command, []string) error { return app.runSuppressPrune(time.Now()) },
	}
}

func (a *App) suppressionsPath() string {
	return filepath.Join(a.RepoRoot, ".github", "frodo-ci", "security", "suppressions.yml")
}

func (a *App) loadSuppressions() (*config.Suppressions, error) {
	path := a.suppressionsPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &config.Suppressions{Version: 1}, nil
	}
	sup, _, err := config.LoadSuppressions(path)
	if err != nil {
		return nil, err
	}
	if sup.Version == 0 {
		sup.Version = 1
	}
	return sup, nil
}

func (a *App) writeSuppressions(s *config.Suppressions) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	path := a.suppressionsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	header := "# Security suppressions — every entry needs a future expiry.\n" +
		"# Managed by `frodo-ci suppress`; schema: security/suppressions.schema.json\n"
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

func (a *App) runSuppressAdd(id, path, reason, owner, approver, expiry string, now time.Time) error {
	exp, err := time.Parse("2006-01-02", expiry)
	if err != nil {
		return fmt.Errorf("--expiry %q must be YYYY-MM-DD", expiry)
	}
	if !exp.After(now) {
		return fmt.Errorf("--expiry %s must be in the future (a suppression must eventually re-surface)", expiry)
	}
	sup, err := a.loadSuppressions()
	if err != nil {
		return err
	}
	sup.Suppressions = append(sup.Suppressions, config.Suppression{
		ID: id, Path: path, Reason: reason, Owner: owner, Approver: approver,
		Expiry: config.Date{Time: exp},
	})
	if err := a.writeSuppressions(sup); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "added suppression %s (expires %s)\n", id, exp.Format("2006-01-02"))
	return nil
}

func (a *App) runSuppressList(now time.Time) error {
	sup, err := a.loadSuppressions()
	if err != nil {
		return err
	}
	if len(sup.Suppressions) == 0 {
		fmt.Fprintln(a.Out, "no suppressions")
		return nil
	}
	for _, s := range sup.Suppressions {
		status := "active"
		if !s.Expiry.After(now) {
			status = "EXPIRED"
		}
		fmt.Fprintf(a.Out, "%-7s  %-24s  path=%-18s  expiry=%s  owner=%s\n",
			status, s.ID, orDash(s.Path), s.Expiry.Format("2006-01-02"), s.Owner)
	}
	return nil
}

func (a *App) runSuppressPrune(now time.Time) error {
	sup, err := a.loadSuppressions()
	if err != nil {
		return err
	}
	kept := sup.Suppressions[:0:0]
	removed := 0
	for _, s := range sup.Suppressions {
		if s.Expiry.After(now) {
			kept = append(kept, s)
		} else {
			removed++
		}
	}
	sup.Suppressions = kept
	if err := a.writeSuppressions(sup); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "pruned %d expired suppression(s)\n", removed)
	return nil
}
