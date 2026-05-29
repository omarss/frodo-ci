package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

// Source captures a config file's path and raw bytes so later stages (schema
// validation, semantic linting) can produce position-aware diagnostics.
type Source struct {
	Path  string
	Bytes []byte
}

// ParseError wraps a YAML decode failure with a human-friendly, position-aware
// message (file path plus a source snippet with a caret at the offending token).
type ParseError struct {
	Path string
	Err  error
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("invalid file: %s\n\n%s", e.Path, yaml.FormatError(e.Err, false, true))
}

func (e *ParseError) Unwrap() error { return e.Err }

// loadYAML reads a file and decodes it into dst, returning the source bytes for
// later reuse even when decoding fails.
func loadYAML(path string, dst any) (*Source, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	src := &Source{Path: path, Bytes: b}
	if err := yaml.Unmarshal(b, dst); err != nil {
		return src, &ParseError{Path: path, Err: err}
	}
	return src, nil
}

// LoadRoot loads the repository root config (.github/frodo-ci.yml).
func LoadRoot(path string) (*RootConfig, *Source, error) {
	var c RootConfig
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadModule loads a module config (<module>/.ci/module.yml).
func LoadModule(path string) (*ModuleConfig, *Source, error) {
	var c ModuleConfig
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadStage loads a stage file (<module>/.ci/<stage>.yml).
func LoadStage(path string) (*StageFile, *Source, error) {
	var c StageFile
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadToolchains loads the toolchain catalog.
func LoadToolchains(path string) (*Toolchains, *Source, error) {
	var c Toolchains
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadSecurityBaseline loads the security baseline (profiles).
func LoadSecurityBaseline(path string) (*SecurityBaseline, *Source, error) {
	var c SecurityBaseline
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadSuppressions loads the security suppressions list.
func LoadSuppressions(path string) (*Suppressions, *Source, error) {
	var c Suppressions
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadRulesets loads the security ruleset classification.
func LoadRulesets(path string) (*Rulesets, *Source, error) {
	var c Rulesets
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadLintRules loads the lint/format profiles.
func LoadLintRules(path string) (*LintRules, *Source, error) {
	var c LintRules
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}

// LoadPerformanceBudgets loads the performance budgets file.
func LoadPerformanceBudgets(path string) (*PerformanceBudgets, *Source, error) {
	var c PerformanceBudgets
	src, err := loadYAML(path, &c)
	if err != nil {
		return nil, src, err
	}
	return &c, src, nil
}
