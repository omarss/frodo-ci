package config

// RootConfig models .github/frodo-ci.yml -- the repository-wide configuration.
type RootConfig struct {
	Version        int             `yaml:"version" json:"version"`
	ModuleFile     string          `yaml:"module_file,omitempty" json:"module_file,omitempty"`
	Scan           ScanConfig      `yaml:"scan,omitempty" json:"scan,omitempty"`
	Stages         StagesConfig    `yaml:"stages,omitempty" json:"stages,omitempty"`
	Modules        ModulesConfig   `yaml:"modules,omitempty" json:"modules,omitempty"`
	Templates      TemplatesConfig `yaml:"templates,omitempty" json:"templates,omitempty"`
	Execution      ExecutionConfig `yaml:"execution,omitempty" json:"execution,omitempty"`
	Minutes        MinutesConfig   `yaml:"minutes,omitempty" json:"minutes,omitempty"`
	Reviews        RootReviews     `yaml:"reviews,omitempty" json:"reviews,omitempty"`
	Experts        ExpertsConfig   `yaml:"experts,omitempty" json:"experts,omitempty"`
	Security       RootSecurity    `yaml:"security,omitempty" json:"security,omitempty"`
	Lint           RootLint        `yaml:"lint,omitempty" json:"lint,omitempty"`
	Performance    RootPerformance `yaml:"performance,omitempty" json:"performance,omitempty"`
	Slack          SlackConfig     `yaml:"slack,omitempty" json:"slack,omitempty"`
	ProtectedFiles []ProtectedFile `yaml:"protected_files,omitempty" json:"protected_files,omitempty"`
}

// ScanConfig bounds which paths Frodo CI considers when discovering modules.
type ScanConfig struct {
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

// StagesConfig declares the canonical CI and CD stage order.
type StagesConfig struct {
	CI []string `yaml:"ci,omitempty" json:"ci,omitempty"`
	CD []string `yaml:"cd,omitempty" json:"cd,omitempty"`
}

// ModulesConfig holds repository-wide module rules.
type ModulesConfig struct {
	Naming NamingConfig `yaml:"naming,omitempty" json:"naming,omitempty"`
}

// NamingConfig constrains module names with a regular expression.
type NamingConfig struct {
	Pattern string `yaml:"pattern,omitempty" json:"pattern,omitempty"`
}

// TemplatesConfig points at the directory holding module templates.
type TemplatesConfig struct {
	Path string `yaml:"path,omitempty" json:"path,omitempty"`
}

// ExecutionConfig controls how the final check orchestrates planned work.
type ExecutionConfig struct {
	FinalCheckName                            string `yaml:"final_check_name,omitempty" json:"final_check_name,omitempty"`
	DynamicChecks                             bool   `yaml:"dynamic_checks,omitempty" json:"dynamic_checks,omitempty"`
	CalculatePlanOnStartup                    bool   `yaml:"calculate_plan_on_startup,omitempty" json:"calculate_plan_on_startup,omitempty"`
	FailFast                                  bool   `yaml:"fail_fast,omitempty" json:"fail_fast,omitempty"`
	FullRunTimeoutMinutes                     int    `yaml:"full_run_timeout_minutes,omitempty" json:"full_run_timeout_minutes,omitempty"`
	NoProgressTimeoutMinutes                  int    `yaml:"no_progress_timeout_minutes,omitempty" json:"no_progress_timeout_minutes,omitempty"`
	MaxParallelModules                        int    `yaml:"max_parallel_modules,omitempty" json:"max_parallel_modules,omitempty"`
	MaxParallelExpensiveStages                int    `yaml:"max_parallel_expensive_stages,omitempty" json:"max_parallel_expensive_stages,omitempty"`
	StopModuleOnStageFailure                  bool   `yaml:"stop_module_on_stage_failure,omitempty" json:"stop_module_on_stage_failure,omitempty"`
	StopDependentsOnDependencyFailure         bool   `yaml:"stop_dependents_on_dependency_failure,omitempty" json:"stop_dependents_on_dependency_failure,omitempty"`
	StopExpensiveStagesAfterValidationFailure bool   `yaml:"stop_expensive_stages_after_validation_failure,omitempty" json:"stop_expensive_stages_after_validation_failure,omitempty"`
}

// MinutesConfig controls CI-minute optimizations.
type MinutesConfig struct {
	CancelPreviousCIRuns      bool `yaml:"cancel_previous_ci_runs,omitempty" json:"cancel_previous_ci_runs,omitempty"`
	SkipUnchanged             bool `yaml:"skip_unchanged,omitempty" json:"skip_unchanged,omitempty"`
	ExactFingerprintMatchOnly bool `yaml:"exact_fingerprint_match_only,omitempty" json:"exact_fingerprint_match_only,omitempty"`
}

// RootReviews holds repository-wide review governance toggles.
type RootReviews struct {
	IgnoreAuthorApproval             bool `yaml:"ignore_author_approval,omitempty" json:"ignore_author_approval,omitempty"`
	IgnoreBotApproval                bool `yaml:"ignore_bot_approval,omitempty" json:"ignore_bot_approval,omitempty"`
	RequireApprovalAfterLatestCommit bool `yaml:"require_approval_after_latest_commit,omitempty" json:"require_approval_after_latest_commit,omitempty"`
}

// ExpertsConfig controls expert-reviewer resolution.
type ExpertsConfig struct {
	Enabled            bool     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	Window             Duration `yaml:"window,omitempty" json:"window,omitempty"`
	ExcludeAuthor      bool     `yaml:"exclude_author,omitempty" json:"exclude_author,omitempty"`
	ExcludeBots        bool     `yaml:"exclude_bots,omitempty" json:"exclude_bots,omitempty"`
	RequireWriteAccess bool     `yaml:"require_write_access,omitempty" json:"require_write_access,omitempty"`
	FallbackToOwners   bool     `yaml:"fallback_to_owners,omitempty" json:"fallback_to_owners,omitempty"`
}

// RootSecurity holds repository-wide security scanning policy.
type RootSecurity struct {
	Mode                           string `yaml:"mode,omitempty" json:"mode,omitempty"`
	BlockingProfile                string `yaml:"blocking_profile,omitempty" json:"blocking_profile,omitempty"`
	ReportProfile                  string `yaml:"report_profile,omitempty" json:"report_profile,omitempty"`
	UpdateVulnerabilityDatabase    bool   `yaml:"update_vulnerability_database,omitempty" json:"update_vulnerability_database,omitempty"`
	FailOnNewCritical              bool   `yaml:"fail_on_new_critical,omitempty" json:"fail_on_new_critical,omitempty"`
	FailOnNewHighWhenFixAvailable  bool   `yaml:"fail_on_new_high_when_fix_available,omitempty" json:"fail_on_new_high_when_fix_available,omitempty"`
	AllowSuppressionOnlyWithExpiry bool   `yaml:"allow_suppression_only_with_expiry,omitempty" json:"allow_suppression_only_with_expiry,omitempty"`
}

// RootLint holds repository-wide lint/format policy.
type RootLint struct {
	Mode                       string `yaml:"mode,omitempty" json:"mode,omitempty"`
	FailOnFormatting           bool   `yaml:"fail_on_formatting,omitempty" json:"fail_on_formatting,omitempty"`
	FailOnDeterministicLint    bool   `yaml:"fail_on_deterministic_lint,omitempty" json:"fail_on_deterministic_lint,omitempty"`
	ReportNonDeterministicLint bool   `yaml:"report_non_deterministic_lint,omitempty" json:"report_non_deterministic_lint,omitempty"`
}

// RootPerformance toggles the performance-tracking subsystem.
type RootPerformance struct {
	Enabled              bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	CollectStageTimings  bool `yaml:"collect_stage_timings,omitempty" json:"collect_stage_timings,omitempty"`
	FailOnBudgetExceeded bool `yaml:"fail_on_budget_exceeded,omitempty" json:"fail_on_budget_exceeded,omitempty"`
	CompareWithBaseline  bool `yaml:"compare_with_baseline,omitempty" json:"compare_with_baseline,omitempty"`
}

// SlackConfig controls Slack notifications.
type SlackConfig struct {
	Enabled  bool         `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	NotifyOn []string     `yaml:"notify_on,omitempty" json:"notify_on,omitempty"`
	Channel  string       `yaml:"channel,omitempty" json:"channel,omitempty"`
	Mention  SlackMention `yaml:"mention,omitempty" json:"mention,omitempty"`
}

// SlackMention controls who gets mentioned in Slack notifications.
type SlackMention struct {
	ModuleOwners               bool `yaml:"module_owners,omitempty" json:"module_owners,omitempty"`
	PlatformOnFrameworkFailure bool `yaml:"platform_on_framework_failure,omitempty" json:"platform_on_framework_failure,omitempty"`
}

// ProtectedFile requires extra approvals when matching files change.
type ProtectedFile struct {
	Name    string           `yaml:"name" json:"name"`
	Files   []string         `yaml:"files" json:"files"`
	Require ProtectedRequire `yaml:"require" json:"require"`
}

// ProtectedRequire enumerates required approver teams/users and their counts.
type ProtectedRequire struct {
	Teams map[string]int `yaml:"teams,omitempty" json:"teams,omitempty"`
	Users map[string]int `yaml:"users,omitempty" json:"users,omitempty"`
}

// ApplyDefaults fills opinionated defaults for unset non-boolean fields so the
// planner can rely on them. Boolean toggles are intentionally left at their
// zero value: `frodo-ci init` ships a root config that sets them explicitly,
// and we never want to silently re-enable a check the user disabled.
func (c *RootConfig) ApplyDefaults() {
	if c.ModuleFile == "" {
		c.ModuleFile = ".ci/module.yml"
	}
	if len(c.Stages.CI) == 0 {
		c.Stages.CI = []string{"validate", "test", "build", "package", "scan"}
	}
	if len(c.Stages.CD) == 0 {
		c.Stages.CD = []string{"publish", "deploy", "verify"}
	}
	if c.Modules.Naming.Pattern == "" {
		c.Modules.Naming.Pattern = "^[a-z][a-z0-9-]*$"
	}
	if c.Templates.Path == "" {
		c.Templates.Path = ".github/frodo-ci/templates"
	}
	if c.Execution.FinalCheckName == "" {
		c.Execution.FinalCheckName = "Frodo CI / final"
	}
	if c.Execution.FullRunTimeoutMinutes == 0 {
		c.Execution.FullRunTimeoutMinutes = 90
	}
	if c.Execution.NoProgressTimeoutMinutes == 0 {
		c.Execution.NoProgressTimeoutMinutes = 10
	}
	if c.Execution.MaxParallelModules == 0 {
		c.Execution.MaxParallelModules = 4
	}
	if c.Execution.MaxParallelExpensiveStages == 0 {
		c.Execution.MaxParallelExpensiveStages = 1
	}
}

// AllStages returns the CI stages followed by the CD stages in canonical order.
func (c *RootConfig) AllStages() []string {
	out := make([]string, 0, len(c.Stages.CI)+len(c.Stages.CD))
	out = append(out, c.Stages.CI...)
	out = append(out, c.Stages.CD...)
	return out
}

// IsCDStage reports whether stage is one of the configured CD stages.
func (c *RootConfig) IsCDStage(stage string) bool {
	for _, s := range c.Stages.CD {
		if s == stage {
			return true
		}
	}
	return false
}
