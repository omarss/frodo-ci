package config

// PerformanceBudgets models .github/frodo-ci/performance/budgets.yml.
//
// Budget keys may be specific stages (validate, test, ...) or the special keys
// ci_total, module_default, and <stage>_default that the requirements use.
type PerformanceBudgets struct {
	Version    int                    `yaml:"version,omitempty" json:"version,omitempty"`
	Budgets    map[string]Duration    `yaml:"budgets,omitempty" json:"budgets,omitempty"`
	Regression *PerformanceRegression `yaml:"regression,omitempty" json:"regression,omitempty"`
	Reporting  *PerformanceReporting  `yaml:"reporting,omitempty" json:"reporting,omitempty"`
}

// PerformanceRegression configures slowdown detection against a baseline ref.
type PerformanceRegression struct {
	CompareAgainst       string `yaml:"compare_against,omitempty" json:"compare_against,omitempty"`
	MaxSlowdownPercent   int    `yaml:"max_slowdown_percent,omitempty" json:"max_slowdown_percent,omitempty"`
	RequireLabelToExceed string `yaml:"require_label_to_exceed,omitempty" json:"require_label_to_exceed,omitempty"`
}

// PerformanceReporting configures where performance results are surfaced.
type PerformanceReporting struct {
	GitHubStepSummary bool `yaml:"github_step_summary,omitempty" json:"github_step_summary,omitempty"`
	PRComment         bool `yaml:"pr_comment,omitempty" json:"pr_comment,omitempty"`
	SlackOnRegression bool `yaml:"slack_on_regression,omitempty" json:"slack_on_regression,omitempty"`
}

// LintRules models .github/frodo-ci/lint/rules.yml: the named lint/format
// quality profiles modules select via quality.lint / quality.format.
type LintRules struct {
	Version  int                    `yaml:"version" json:"version"`
	Profiles map[string]LintProfile `yaml:"profiles,omitempty" json:"profiles,omitempty"`
}

// LintProfile defines the enforcement level of a named lint profile. Values are
// "required", "report", or "off".
type LintProfile struct {
	Format               string `yaml:"format,omitempty" json:"format,omitempty"`
	DeterministicLint    string `yaml:"deterministic_lint,omitempty" json:"deterministic_lint,omitempty"`
	NonDeterministicLint string `yaml:"non_deterministic_lint,omitempty" json:"non_deterministic_lint,omitempty"`
}
