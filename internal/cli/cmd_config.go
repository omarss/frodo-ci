package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/frodo-ci/frodo-ci/internal/configlint"
	"github.com/frodo-ci/frodo-ci/internal/plan"
	"github.com/frodo-ci/frodo-ci/internal/schema"
)

// newValidateConfigCommand validates configuration against the JSON Schemas.
func newValidateConfigCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "validate-config",
		Short: "Validate all Frodo CI configuration against the JSON Schemas",
		Long: "Discovers the root config, framework catalog files, and every module and\n" +
			"stage file under .ci directories, then validates each against its schema with\n" +
			"human-friendly diagnostics.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runValidateConfig()
		},
	}
}

// validationResult is the machine-readable result for one file (--json mode).
type validationResult struct {
	File     string   `json:"file"`
	Kind     string   `json:"kind"`
	OK       bool     `json:"ok"`
	Messages []string `json:"messages,omitempty"`
}

func (a *App) runValidateConfig() error {
	files, err := discoverConfigFiles(a.RepoRoot)
	if err != nil {
		return fmt.Errorf("discover config files: %w", err)
	}
	if len(files) == 0 {
		return fmt.Errorf("no Frodo CI configuration found under %s (expected .github/frodo-ci.yml)", a.RepoRoot)
	}

	v := schema.NewValidator()
	results := make([]validationResult, 0, len(files))
	failed := 0
	for _, f := range files {
		res := validationResult{File: f.Rel, Kind: string(f.Kind), OK: true}
		data, readErr := os.ReadFile(f.Path)
		if readErr != nil {
			res.OK = false
			res.Messages = []string{fmt.Sprintf("could not read file: %v", readErr)}
		} else if vErr := v.ValidateBytes(f.Kind, f.Rel, data); vErr != nil {
			res.OK = false
			res.Messages = messagesOf(vErr)
		}
		if !res.OK {
			failed++
		}
		results = append(results, res)
	}

	if a.JSON {
		return a.emitValidationJSON(results, failed)
	}
	return a.emitValidationText(results, failed)
}

func (a *App) emitValidationJSON(results []validationResult, failed int) error {
	enc := json.NewEncoder(a.Out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(map[string]any{
		"checked": len(results),
		"failed":  failed,
		"results": results,
	}); err != nil {
		return err
	}
	if failed > 0 {
		return ErrExitQuiet
	}
	return nil
}

func (a *App) emitValidationText(results []validationResult, failed int) error {
	for _, r := range results {
		if r.OK {
			fmt.Fprintf(a.Out, "ok    %s\n", r.File)
			continue
		}
		fmt.Fprintf(a.Out, "FAIL  %s\n", r.File)
		for _, m := range r.Messages {
			fmt.Fprintf(a.Out, "      %s\n", strings.ReplaceAll(m, "\n", "\n      "))
		}
	}
	fmt.Fprintf(a.Out, "\n%d file(s) checked, %d failed\n", len(results), failed)
	if failed > 0 {
		return ErrExitQuiet
	}
	return nil
}

// messagesOf extracts the friendly messages from a validation error, falling
// back to the raw error text for non-friendly errors.
func messagesOf(err error) []string {
	var fe *schema.FriendlyError
	if errors.As(err, &fe) {
		return fe.Messages
	}
	return []string{err.Error()}
}

// newLintConfigCommand runs semantic linting beyond structural schema checks.
func newLintConfigCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "lint-config",
		Short: "Lint configuration for semantic problems (cycles, broad inputs, weakening, ...)",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return app.runLintConfig()
		},
	}
}

func (a *App) runLintConfig() error {
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	problems := loaded.SemanticProblems
	if a.JSON {
		if err := writeJSON(a.Out, problems); err != nil {
			return err
		}
		if configlint.HasErrors(problems) {
			return ErrExitQuiet
		}
		return nil
	}
	if len(problems) == 0 {
		fmt.Fprintln(a.Out, "no semantic problems found")
		return nil
	}
	printProblems(a.Out, problems)
	errs, warns := 0, 0
	for _, p := range problems {
		if p.Severity == configlint.Error {
			errs++
		} else {
			warns++
		}
	}
	fmt.Fprintf(a.Out, "\n%d error(s), %d warning(s)\n", errs, warns)
	if errs > 0 {
		return ErrExitQuiet
	}
	return nil
}
