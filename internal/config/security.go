package config

// SecurityBaseline models .github/frodo-ci/security/baseline.yml: the named
// security profiles modules select via quality.security or scan.security.profile.
type SecurityBaseline struct {
	Version  int                        `yaml:"version" json:"version"`
	Profiles map[string]SecurityProfile `yaml:"profiles,omitempty" json:"profiles,omitempty"`
}

// SecurityProfile defines what a named security posture enforces.
type SecurityProfile struct {
	FailOnNewCritical             bool `yaml:"fail_on_new_critical,omitempty" json:"fail_on_new_critical,omitempty"`
	FailOnNewHighWhenFixAvailable bool `yaml:"fail_on_new_high_when_fix_available,omitempty" json:"fail_on_new_high_when_fix_available,omitempty"`
	FailOnSecrets                 bool `yaml:"fail_on_secrets,omitempty" json:"fail_on_secrets,omitempty"`
	RequireIaCScan                bool `yaml:"require_iac_scan,omitempty" json:"require_iac_scan,omitempty"`
	RequireContainerScan          bool `yaml:"require_container_scan,omitempty" json:"require_container_scan,omitempty"`
	RequireLicenseScan            bool `yaml:"require_license_scan,omitempty" json:"require_license_scan,omitempty"`
}

// Suppressions models .github/frodo-ci/security/suppressions.yml. Every
// suppression must carry an expiry; permanent suppressions are not allowed.
type Suppressions struct {
	Version      int           `yaml:"version" json:"version"`
	Suppressions []Suppression `yaml:"suppressions,omitempty" json:"suppressions,omitempty"`
}

// Suppression silences a specific finding for a bounded time.
type Suppression struct {
	ID       string `yaml:"id" json:"id"`
	Path     string `yaml:"path" json:"path"`
	Reason   string `yaml:"reason" json:"reason"`
	Owner    string `yaml:"owner" json:"owner"`
	Expiry   Date   `yaml:"expiry" json:"expiry"`
	Approver string `yaml:"approver" json:"approver"`
}

// Rulesets models .github/frodo-ci/security/rulesets.yml: the classification of
// finding rule IDs into blocking vs report-only buckets.
type Rulesets struct {
	Version    int      `yaml:"version" json:"version"`
	Blocking   []string `yaml:"blocking,omitempty" json:"blocking,omitempty"`
	ReportOnly []string `yaml:"report_only,omitempty" json:"report_only,omitempty"`
}
