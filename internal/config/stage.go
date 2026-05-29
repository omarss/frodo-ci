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
	Cache          *StageCache          `yaml:"cache,omitempty" json:"cache,omitempty"`
	Steps          []StageStep          `yaml:"steps,omitempty" json:"steps,omitempty"`
}

// SetupTool requests a toolchain (e.g. java, node, go) for a stage. Versions
// are pinned centrally in toolchains.yml; this only selects what to provision.
type SetupTool struct {
	Distribution string  `yaml:"distribution,omitempty" json:"distribution,omitempty"`
	Version      FlexStr `yaml:"version,omitempty" json:"version,omitempty"`
}

// StageCache declares directories to cache for a stage. Caching is keyed by the
// stage fingerprint, so it never weakens correctness -- only speed.
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
