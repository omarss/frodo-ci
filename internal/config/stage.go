package config

// StageFile models <module>/.ci/<stage>.yml -- the deliberately small stage
// schema. It is NOT full GitHub Actions workflow syntax: only the fields below
// are supported, which keeps stage files reviewable and prevents ad hoc
// commands from bypassing central quality/security profiles.
type StageFile struct {
	Name           string               `yaml:"name,omitempty" json:"name,omitempty"`
	TimeoutMinutes int                  `yaml:"timeout_minutes,omitempty" json:"timeout_minutes,omitempty"`
	Env            map[string]FlexStr   `yaml:"env,omitempty" json:"env,omitempty"`
	Setup          map[string]SetupTool `yaml:"setup,omitempty" json:"setup,omitempty"`
	// Outputs are build artifacts (e.g. dist/, target/) this stage produces. They
	// are archived keyed by the stage fingerprint and restored on a hit -- for
	// this module and for dependents in the same run -- so unchanged work is
	// restored instead of rebuilt.
	Outputs []string    `yaml:"outputs,omitempty" json:"outputs,omitempty"`
	Cache   *StageCache `yaml:"cache,omitempty" json:"cache,omitempty"`
	Steps   []StageStep `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// SetupTool requests a toolchain (e.g. java, node, go) for a stage. Versions
// are pinned centrally in toolchains.yml; this only selects what to provision.
type SetupTool struct {
	Distribution string  `yaml:"distribution,omitempty" json:"distribution,omitempty"`
	Version      FlexStr `yaml:"version,omitempty" json:"version,omitempty"`
}

// StageCache declares incidental package-store directories (e.g. the pnpm or
// Maven store) that the CI workflow persists across runs. For build artifacts
// that dependent modules consume, use the stage's `outputs:` instead -- those
// are archived and restored keyed by the stage fingerprint.
type StageCache struct {
	Paths []string `yaml:"paths,omitempty" json:"paths,omitempty"`
	Key   string   `yaml:"key,omitempty" json:"key,omitempty"`
}

// StageStep is one command to run within a stage.
type StageStep struct {
	Name             string `yaml:"name,omitempty" json:"name,omitempty"`
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty"`
	Run              string `yaml:"run" json:"run"`
}
