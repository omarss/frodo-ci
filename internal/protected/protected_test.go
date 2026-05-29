package protected

import (
	"testing"

	"github.com/frodo-ci/frodo-ci/internal/config"
)

func TestMatches(t *testing.T) {
	rules := []config.ProtectedFile{
		{Name: "framework", Files: []string{".github/frodo-ci.yml", "**/.ci/module.yml"},
			Require: config.ProtectedRequire{Teams: map[string]int{"platform-team": 1}}},
		{Name: "workflow", Files: []string{".github/workflows/frodo-ci.yml"},
			Require: config.ProtectedRequire{Teams: map[string]int{"devex-team": 1}}},
	}
	changed := []string{"services/cards/.ci/module.yml", "services/cards/src/X.java"}

	matches := Matches(changed, rules)
	if len(matches) != 1 || matches[0].Name != "framework" {
		t.Fatalf("expected only the framework rule to match, got %+v", matches)
	}
	if matches[0].Require.Teams["platform-team"] != 1 {
		t.Errorf("unexpected requirement: %+v", matches[0].Require)
	}
	if len(matches[0].Files) != 1 || matches[0].Files[0] != "services/cards/.ci/module.yml" {
		t.Errorf("unexpected matched files: %v", matches[0].Files)
	}
}
