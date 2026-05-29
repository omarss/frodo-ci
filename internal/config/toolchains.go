package config

// Toolchains models .github/frodo-ci/toolchains.yml -- the central catalog of
// formatters and linters. Declaring tools centrally (with pinned versions)
// keeps quality enforcement consistent and stops stage files from introducing
// ad hoc formatter/linter commands.
type Toolchains struct {
	Version    int                  `yaml:"version" json:"version"`
	Formatters map[string]Formatter `yaml:"formatters,omitempty" json:"formatters,omitempty"`
	Linters    map[string]Linter    `yaml:"linters,omitempty" json:"linters,omitempty"`
}

// Formatter describes a language formatter. Blocking formatters fail the build
// on formatting differences.
type Formatter struct {
	Tool     string `yaml:"tool" json:"tool"`
	Version  string `yaml:"version,omitempty" json:"version,omitempty"`
	Blocking bool   `yaml:"blocking,omitempty" json:"blocking,omitempty"`
}

// Linter classifies a language's linters into deterministic (blocking),
// report-only (advisory), and optional (off unless enabled) buckets.
type Linter struct {
	Deterministic []string `yaml:"deterministic,omitempty" json:"deterministic,omitempty"`
	ReportOnly    []string `yaml:"report_only,omitempty" json:"report_only,omitempty"`
	Optional      []string `yaml:"optional,omitempty" json:"optional,omitempty"`
}

// FormatterTools returns the set of formatter tool names declared in the catalog.
func (t *Toolchains) FormatterTools() map[string]bool {
	out := map[string]bool{}
	for _, f := range t.Formatters {
		if f.Tool != "" {
			out[f.Tool] = true
		}
	}
	return out
}

// LinterTools returns the set of all linter tool names declared in the catalog.
func (t *Toolchains) LinterTools() map[string]bool {
	out := map[string]bool{}
	for _, l := range t.Linters {
		for _, group := range [][]string{l.Deterministic, l.ReportOnly, l.Optional} {
			for _, name := range group {
				out[name] = true
			}
		}
	}
	return out
}
