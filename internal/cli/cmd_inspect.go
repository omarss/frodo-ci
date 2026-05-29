package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/schema"
	"github.com/frodo-ci/frodo-ci/internal/templates"
)

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
func newDoctorCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check the environment and configuration for common problems",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runDoctor()
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
func newSchemasExportCommand(app *App) *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "export",
		Short: "Write JSON Schemas for all config files to a directory",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runSchemasExport(out)
		},
	}
	c.Flags().StringVar(&out, "out", ".github/frodo-ci/schemas", "output directory for schema files")
	return c
}

func (a *App) runSchemasExport(out string) error {
	dir := out
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.RepoRoot, dir)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create schema directory %s: %w", dir, err)
	}
	for _, k := range schema.ExportedKinds() {
		data, err := schema.JSON(k)
		if err != nil {
			return fmt.Errorf("generate %s schema: %w", k, err)
		}
		path := filepath.Join(dir, k.FileName())
		if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
		fmt.Fprintf(a.Out, "wrote %s\n", filepath.Join(out, k.FileName()))
	}
	return nil
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
func newTemplatesListCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available module templates",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runTemplatesList()
		},
	}
}

// newTemplatesExplainCommand explains one template's stages and defaults.
func newTemplatesExplainCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "explain <template>",
		Short: "Explain a template's stages and defaults",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return app.runTemplatesExplain(args[0])
		},
	}
}

func (a *App) templatesDir() string {
	dir := ".github/frodo-ci/templates"
	if root, _, err := config.LoadRoot(filepath.Join(a.RepoRoot, ".github", "frodo-ci.yml")); err == nil && root.Templates.Path != "" {
		dir = root.Templates.Path
	}
	return filepath.Join(a.RepoRoot, filepath.FromSlash(dir))
}

func availableTemplates(dir string) []string {
	set := map[string]bool{}
	for _, n := range templates.DefaultNames() {
		set[n] = true
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".yml") {
				set[strings.TrimSuffix(e.Name(), ".yml")] = true
			}
		}
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (a *App) runTemplatesList() error {
	dir := a.templatesDir()
	loader := templates.NewLoader(dir)
	names := availableTemplates(dir)
	if a.JSON {
		return writeJSON(a.Out, names)
	}
	fmt.Fprintln(a.Out, "Available templates:")
	for _, name := range names {
		t, _ := loader.Get(name)
		stages := 0
		if t != nil {
			stages = len(t.CI) + len(t.CD)
		}
		fmt.Fprintf(a.Out, "  %-16s %d stage(s)\n", name, stages)
	}
	return nil
}

func (a *App) runTemplatesExplain(name string) error {
	dir := a.templatesDir()
	loader := templates.NewLoader(dir)
	t, err := loader.Get(name)
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("unknown template %q (available: %s)", name, strings.Join(availableTemplates(dir), ", "))
	}
	if a.JSON {
		return writeJSON(a.Out, t)
	}
	fmt.Fprintf(a.Out, "Template: %s\n", name)
	if t.Type != "" {
		fmt.Fprintf(a.Out, "Type:     %s\n", t.Type)
	}
	q := t.Quality
	fmt.Fprintf(a.Out, "Quality:  format=%s lint=%s security=%s performance=%s\n",
		orDash(q.Format), orDash(q.Lint), orDash(q.Security), orDash(q.Performance))
	printTemplateStages(a.Out, "CI", t.CI)
	printTemplateStages(a.Out, "CD", t.CD)
	return nil
}

func printTemplateStages(w interface{ Write([]byte) (int, error) }, group string, stages map[string]templates.Stage) {
	if len(stages) == 0 {
		return
	}
	fmt.Fprintf(w, "%s stages:\n", group)
	names := make([]string, 0, len(stages))
	for n := range stages {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := stages[n]
		var when []string
		for _, m := range s.When {
			when = append(when, m.String())
		}
		fmt.Fprintf(w, "  %-9s %d step(s)", n, len(s.Steps))
		if len(when) > 0 {
			fmt.Fprintf(w, "  when: %s", strings.Join(when, ", "))
		}
		fmt.Fprintln(w)
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
