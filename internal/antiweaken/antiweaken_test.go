package antiweaken

import (
	"strings"
	"testing"

	"github.com/omarss/frodo-ci/internal/config"
)

func has(ws []Weakening, substr string) bool {
	for _, w := range ws {
		if strings.Contains(w.Detail, substr) {
			return true
		}
	}
	return false
}

func TestRootWeakenings(t *testing.T) {
	base := &config.RootConfig{}
	base.Security.FailOnNewCritical = true
	base.Experts.Enabled = true
	base.Execution.FullRunTimeoutMinutes = 90
	base.ProtectedFiles = []config.ProtectedFile{{Name: "framework", Require: config.ProtectedRequire{Teams: map[string]int{"platform-team": 1}}}}

	head := &config.RootConfig{}
	head.Security.FailOnNewCritical = false // disabled
	head.Experts.Enabled = true
	head.Execution.FullRunTimeoutMinutes = 180 // increased
	// protected rule removed

	ws := Root(".github/frodo-ci.yml", base, head)
	if !has(ws, "fail_on_new_critical") {
		t.Error("expected security flag weakening")
	}
	if !has(ws, "increased execution.full_run_timeout_minutes") {
		t.Error("expected timeout-increase weakening")
	}
	if !has(ws, "removed protected-file rule framework") {
		t.Error("expected removed-protected-rule weakening")
	}
}

func TestModuleWeakenings(t *testing.T) {
	one := 1
	base := &config.ModuleConfig{
		CI: map[string]config.ModuleStage{
			"scan": {},
			"test": {When: []config.Matcher{{Glob: "src/**"}, {Glob: "pom.xml"}}},
		},
		Reviews: map[string]config.ReviewRule{
			"default": {Require: config.ReviewRequire{Owners: &one}},
		},
	}
	head := &config.ModuleConfig{
		CI: map[string]config.ModuleStage{
			// scan removed
			"test": {When: []config.Matcher{{Glob: "src/**"}}}, // narrowed
		},
		Reviews: map[string]config.ReviewRule{
			"default": {Require: config.ReviewRequire{}}, // owners reduced to 0/unset
		},
	}
	ws := Module("services/cards/.ci/module.yml", base, head)
	if !has(ws, "removed the scan stage") {
		t.Error("expected scan-removed weakening")
	}
	if !has(ws, "narrowed matched files for ci.test") {
		t.Error("expected narrowed-when weakening")
	}
	if !has(ws, "reduced required owners") {
		t.Error("expected reduced-owners weakening")
	}
}
