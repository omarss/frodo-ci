package configlint

import (
	"strings"
	"testing"
	"time"

	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/discover"
)

func defaultRoot() *config.RootConfig {
	c := &config.RootConfig{}
	c.ApplyDefaults()
	return c
}

func hasMessage(ps []Problem, substr string) bool {
	for _, p := range ps {
		if strings.Contains(p.Message, substr) {
			return true
		}
	}
	return false
}

func TestAdHocCommandDetection(t *testing.T) {
	m := &discover.Module{
		Name: "portal", Dir: "apps/portal", ConfigPath: "apps/portal/.ci/module.yml",
		Config: &config.ModuleConfig{Name: "portal", Type: "node-app", Owners: config.Owners{Teams: []string{"web"}}},
		Stages: map[string]*discover.StageRef{
			"validate": {Stage: "validate", Path: "apps/portal/.ci/validate.yml",
				File: &config.StageFile{Steps: []config.StageStep{{Run: "npx eslint ."}}}},
		},
	}
	ps := Check(Input{Root: defaultRoot(), Modules: []*discover.Module{m}, Now: time.Now()})
	if !hasMessage(ps, "eslint") {
		t.Errorf("expected ad hoc eslint warning, got %v", ps)
	}
}

func TestExpiredSuppression(t *testing.T) {
	past := config.Date{}
	_ = past.UnmarshalYAML(func(v interface{}) error { *(v.(*interface{})) = "2000-01-01"; return nil })
	in := Input{
		Root: defaultRoot(),
		Now:  time.Now(),
		Suppressions: &config.Suppressions{Suppressions: []config.Suppression{
			{ID: "CVE-1", Path: "x", Reason: "r", Owner: "o", Approver: "a", Expiry: past},
			{ID: "CVE-2", Path: "y", Reason: "r", Owner: "o", Approver: "a"}, // no expiry
		}},
		SuppressionsPath: ".github/frodo-ci/security/suppressions.yml",
	}
	ps := Check(in)
	if !hasMessage(ps, "expired") {
		t.Errorf("expected expired-suppression error, got %v", ps)
	}
	if !hasMessage(ps, "no expiry") {
		t.Errorf("expected no-expiry error, got %v", ps)
	}
}

func TestUnknownStageSuggestion(t *testing.T) {
	m := &discover.Module{
		Name: "a", Dir: "a", ConfigPath: "a/.ci/module.yml",
		Config: &config.ModuleConfig{Name: "a", Type: "node-library", Owners: config.Owners{Teams: []string{"t"}},
			CI: map[string]config.ModuleStage{"tests": {When: []config.Matcher{{Glob: "src/**"}}}}},
	}
	ps := Check(Input{Root: defaultRoot(), Modules: []*discover.Module{m}, Now: time.Now()})
	if !hasMessage(ps, "unknown CI stage") || !hasMessage(ps, `"test"`) {
		t.Errorf("expected unknown-stage with suggestion, got %v", ps)
	}
}
