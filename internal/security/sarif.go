package security

import (
	"encoding/json"
	"strconv"
	"strings"
)

// SARIF (2.1.0) is the common output format we ask every tool that supports it
// to emit, so a single parser covers semgrep, trivy, gitleaks, hadolint, and any
// other SARIF-producing scanner. We extract just what the gate needs.
type sarifLog struct {
	Runs []sarifRun `json:"runs"`
}

type sarifRun struct {
	Tool struct {
		Driver struct {
			Name  string      `json:"name"`
			Rules []sarifRule `json:"rules"`
		} `json:"driver"`
	} `json:"tool"`
	Results []sarifResult `json:"results"`
}

type sarifRule struct {
	ID                   string `json:"id"`
	DefaultConfiguration struct {
		Level string `json:"level"`
	} `json:"defaultConfiguration"`
	Properties struct {
		SecuritySeverity string   `json:"security-severity"`
		Tags             []string `json:"tags"`
	} `json:"properties"`
}

type sarifResult struct {
	RuleID    string `json:"ruleId"`
	RuleIndex int    `json:"ruleIndex"`
	Level     string `json:"level"`
	Message   struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		PhysicalLocation struct {
			ArtifactLocation struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				StartLine int `json:"startLine"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// parseSARIF turns a SARIF document into findings, attributing the scan kind and
// tool. Severity comes from the rule's numeric `security-severity` (the GitHub
// convention) when present, else the result/rule `level`.
func parseSARIF(data []byte, kind ScanType, tool string) ([]Finding, error) {
	var log sarifLog
	if err := json.Unmarshal(data, &log); err != nil {
		return nil, err
	}
	var out []Finding
	for _, run := range log.Runs {
		byID := map[string]sarifRule{}
		for _, r := range run.Tool.Driver.Rules {
			byID[r.ID] = r
		}
		for _, res := range run.Results {
			rule, byIdx := sarifRule{}, run.Tool.Driver.Rules
			if r, ok := byID[res.RuleID]; ok {
				rule = r
			} else if res.RuleIndex >= 0 && res.RuleIndex < len(byIdx) {
				rule = byIdx[res.RuleIndex]
			}
			f := Finding{
				RuleID:   res.RuleID,
				Message:  strings.TrimSpace(res.Message.Text),
				Tool:     tool,
				Kind:     kind,
				Severity: severityFromSARIF(firstNonEmpty(res.Level, rule.DefaultConfiguration.Level), rule.Properties.SecuritySeverity),
			}
			if f.RuleID == "" {
				f.RuleID = rule.ID
			}
			if len(res.Locations) > 0 {
				loc := res.Locations[0].PhysicalLocation
				f.Path = loc.ArtifactLocation.URI
				f.Line = loc.Region.StartLine
			}
			// Secrets are categorically blocking-worthy; ensure a severity floor.
			if kind == Secrets && severityRank(f.Severity) < severityRank("high") {
				f.Severity = "high"
			}
			f.FixAvailable = strings.Contains(strings.ToLower(f.Message), "fixed version")
			out = append(out, f)
		}
	}
	return out, nil
}

// severityFromSARIF maps a SARIF result to critical/high/medium/low. The numeric
// `security-severity` (0.0-10.0) wins when present, following GitHub's banding;
// otherwise the SARIF `level` (error/warning/note) is used.
func severityFromSARIF(level, securitySeverity string) string {
	if securitySeverity != "" {
		if v, err := strconv.ParseFloat(securitySeverity, 64); err == nil {
			switch {
			case v >= 9.0:
				return "critical"
			case v >= 7.0:
				return "high"
			case v >= 4.0:
				return "medium"
			case v > 0:
				return "low"
			}
		}
	}
	switch strings.ToLower(level) {
	case "error":
		return "high"
	case "warning":
		return "medium"
	case "note", "info", "none":
		return "low"
	}
	return ""
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
