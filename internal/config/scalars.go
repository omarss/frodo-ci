// Package config defines the typed domain model for every Frodo CI config file
// (root, module, stage, toolchains, security, lint, performance) and loads them
// from YAML with position-aware diagnostics.
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Duration is a YAML-friendly duration that understands the units used across
// Frodo CI config: s, m, h, d, w (e.g. "30d", "20m", "1h30m"). The standard
// library's time.ParseDuration supports neither days nor weeks, so we parse it
// ourselves.
type Duration time.Duration

// Duration returns the value as a time.Duration.
func (d Duration) Duration() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }

// MarshalJSON renders the duration as its human string (e.g. "20m0s") so plan
// output and reports are readable.
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + d.String() + `"`), nil
}

// UnmarshalYAML accepts a scalar duration string.
func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, err := ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

// ParseDuration parses a duration with s, m, h, d, w units. An empty string is
// treated as zero so optional fields may be omitted.
func ParseDuration(s string) (time.Duration, error) {
	in := strings.TrimSpace(s)
	if in == "" {
		return 0, nil
	}
	var total time.Duration
	for i := 0; i < len(in); {
		start := i
		for i < len(in) && (in[i] == '.' || (in[i] >= '0' && in[i] <= '9')) {
			i++
		}
		if start == i {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		num, err := strconv.ParseFloat(in[start:i], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
		unitStart := i
		for i < len(in) && in[i] != '.' && !(in[i] >= '0' && in[i] <= '9') {
			i++
		}
		mult, ok := durationUnit(strings.TrimSpace(in[unitStart:i]))
		if !ok {
			return 0, fmt.Errorf("invalid duration %q: use units s, m, h, d, or w", s)
		}
		total += time.Duration(num * float64(mult))
	}
	return total, nil
}

func durationUnit(u string) (time.Duration, bool) {
	switch u {
	case "s":
		return time.Second, true
	case "m":
		return time.Minute, true
	case "h":
		return time.Hour, true
	case "d":
		return 24 * time.Hour, true
	case "w":
		return 7 * 24 * time.Hour, true
	default:
		return 0, false
	}
}

// FlexStr is a string that accepts any YAML scalar (string, int, float, bool)
// and keeps its textual form. It is handy for fields like tool versions where
// both "25" and 25 are reasonable inputs.
type FlexStr string

func (f FlexStr) String() string { return string(f) }

// UnmarshalYAML coerces any scalar to its textual representation.
func (f *FlexStr) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	if err := unmarshal(&v); err != nil {
		return err
	}
	if v == nil {
		*f = ""
		return nil
	}
	*f = FlexStr(fmt.Sprintf("%v", v))
	return nil
}

// Matcher is one entry in a `when` list. It is either a glob (a bare string) or
// a regular expression written as `{regex: "..."}`.
type Matcher struct {
	Glob  string `yaml:"-" json:"glob,omitempty"`
	Regex string `yaml:"regex,omitempty" json:"regex,omitempty"`
}

// IsRegex reports whether the matcher is a regular expression.
func (m Matcher) IsRegex() bool { return m.Regex != "" }

func (m Matcher) String() string {
	if m.Regex != "" {
		return "regex:" + m.Regex
	}
	return m.Glob
}

// UnmarshalYAML accepts either a scalar glob string or a {regex: "..."} mapping.
func (m *Matcher) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		m.Glob = s
		return nil
	}
	var obj struct {
		Regex string `yaml:"regex"`
	}
	if err := unmarshal(&obj); err != nil {
		return fmt.Errorf("matcher must be a glob string or a {regex: ...} object")
	}
	if obj.Regex == "" {
		return fmt.Errorf("matcher object must set a non-empty 'regex'")
	}
	m.Regex = obj.Regex
	return nil
}

// Date is a calendar date (YYYY-MM-DD) used for suppression expiry.
type Date struct{ time.Time }

// UnmarshalYAML accepts a YYYY-MM-DD (or RFC3339) scalar.
func (d *Date) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var v interface{}
	if err := unmarshal(&v); err != nil {
		return err
	}
	switch t := v.(type) {
	case nil:
		return nil
	case time.Time:
		d.Time = t
	case string:
		parsed, err := parseDate(t)
		if err != nil {
			return err
		}
		d.Time = parsed
	default:
		return fmt.Errorf("invalid date %v: expected YYYY-MM-DD", v)
	}
	return nil
}

func parseDate(s string) (time.Time, error) {
	in := strings.TrimSpace(s)
	for _, layout := range []string{"2006-01-02", time.RFC3339, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, in); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date %q: expected YYYY-MM-DD", s)
}
