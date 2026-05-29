package config

// ModuleConfig models <module>/.ci/module.yml.
type ModuleConfig struct {
	Name        string                 `yaml:"name" json:"name"`
	Type        string                 `yaml:"type,omitempty" json:"type,omitempty"`
	Use         ModuleUse              `yaml:"use,omitempty" json:"use,omitempty"`
	Owners      Owners                 `yaml:"owners,omitempty" json:"owners,omitempty"`
	DependsOn   []Dependency           `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Quality     QualityProfiles        `yaml:"quality,omitempty" json:"quality,omitempty"`
	CI          map[string]ModuleStage `yaml:"ci,omitempty" json:"ci,omitempty"`
	CD          map[string]ModuleStage `yaml:"cd,omitempty" json:"cd,omitempty"`
	Reviews     map[string]ReviewRule  `yaml:"reviews,omitempty" json:"reviews,omitempty"`
	Performance ModulePerformance      `yaml:"performance,omitempty" json:"performance,omitempty"`
}

// ModuleUse selects a template profile to merge into this module.
type ModuleUse struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
}

// Owners lists the teams and users accountable for a module.
type Owners struct {
	Teams []string `yaml:"teams,omitempty" json:"teams,omitempty"`
	Users []string `yaml:"users,omitempty" json:"users,omitempty"`
}

// Dependency declares that this module depends on another, and which of its
// stages should re-run when that dependency changes.
type Dependency struct {
	Module  string   `yaml:"module" json:"module"`
	Affects []string `yaml:"affects,omitempty" json:"affects,omitempty"`
}

// QualityProfiles selects centrally-defined quality profiles by name.
type QualityProfiles struct {
	Format      string `yaml:"format,omitempty" json:"format,omitempty"`
	Lint        string `yaml:"lint,omitempty" json:"lint,omitempty"`
	Security    string `yaml:"security,omitempty" json:"security,omitempty"`
	Performance string `yaml:"performance,omitempty" json:"performance,omitempty"`
}

// ModuleStage is a single entry under ci:/cd: in a module config.
//
//   - When lists module-local files that trigger the stage.
//   - Inputs lists raw files elsewhere in the repo that affect the stage.
//   - Security overrides the security profile for the scan stage.
//   - Environments lists target environments for CD stages.
type ModuleStage struct {
	When         []Matcher      `yaml:"when,omitempty" json:"when,omitempty"`
	Inputs       []string       `yaml:"inputs,omitempty" json:"inputs,omitempty"`
	Security     *StageSecurity `yaml:"security,omitempty" json:"security,omitempty"`
	Environments []string       `yaml:"environments,omitempty" json:"environments,omitempty"`
}

// StageSecurity overrides the security profile used for a stage.
type StageSecurity struct {
	Profile string `yaml:"profile,omitempty" json:"profile,omitempty"`
}

// ReviewRule is a named review requirement. The "default" rule applies to all
// changes in the module; named rules apply when their When matchers hit.
type ReviewRule struct {
	When    []Matcher     `yaml:"when,omitempty" json:"when,omitempty"`
	Request ReviewRequest `yaml:"request,omitempty" json:"request,omitempty"`
	Require ReviewRequire `yaml:"require,omitempty" json:"require,omitempty"`
}

// ReviewRequest lists reviewers to auto-request (not necessarily required).
type ReviewRequest struct {
	Teams []string `yaml:"teams,omitempty" json:"teams,omitempty"`
	Users []string `yaml:"users,omitempty" json:"users,omitempty"`
}

// ReviewRequire lists the approvals that must be present. Owners and Expert are
// pointers so "0 required" can be distinguished from "unset".
type ReviewRequire struct {
	Owners *int           `yaml:"owners,omitempty" json:"owners,omitempty"`
	Expert *int           `yaml:"expert,omitempty" json:"expert,omitempty"`
	Teams  map[string]int `yaml:"teams,omitempty" json:"teams,omitempty"`
	Users  map[string]int `yaml:"users,omitempty" json:"users,omitempty"`
}

// ModulePerformance holds per-stage performance budgets for a module.
type ModulePerformance struct {
	Budgets map[string]Duration `yaml:"budgets,omitempty" json:"budgets,omitempty"`
}

// Stage returns the configured module stage for the given CI or CD stage name,
// preferring CI when a name appears in both maps (it never should).
func (m *ModuleConfig) Stage(name string) (ModuleStage, bool) {
	if s, ok := m.CI[name]; ok {
		return s, true
	}
	s, ok := m.CD[name]
	return s, ok
}
