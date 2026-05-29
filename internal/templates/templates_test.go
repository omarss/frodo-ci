package templates

import (
	"testing"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/discover"
)

func TestDefaultNames(t *testing.T) {
	names := DefaultNames()
	if len(names) != 7 {
		t.Fatalf("expected 7 built-in templates, got %d: %v", len(names), names)
	}
	for _, n := range []string{"spring-service", "node-app", "k8s-infra", "docker-image"} {
		if _, err := LoadDefault(n); err != nil {
			t.Errorf("LoadDefault(%q): %v", n, err)
		}
	}
}

func TestResolveMergesOverrides(t *testing.T) {
	tmpl, err := LoadDefault("spring-service")
	if err != nil {
		t.Fatal(err)
	}
	m := &discover.Module{
		Name: "cards",
		Dir:  "services/cards",
		Config: &config.ModuleConfig{
			Name: "cards",
			Type: "spring-service",
			CI: map[string]config.ModuleStage{
				"test": {When: []config.Matcher{{Glob: "src/main/**"}}},
			},
		},
		Stages: map[string]*discover.StageRef{
			"test": {Stage: "test", Path: "services/cards/.ci/test.yml",
				File: &config.StageFile{Steps: []config.StageStep{{Run: "custom test"}}}},
		},
	}
	eff := Resolve(tmpl, m)

	test, ok := eff["test"]
	if !ok {
		t.Fatal("test stage missing from effective stages")
	}
	if len(test.When) != 1 || test.When[0].Glob != "src/main/**" {
		t.Errorf("module should override template when: %+v", test.When)
	}
	if len(test.Steps) != 1 || test.Steps[0].Run != "custom test" {
		t.Errorf("stage file should override template steps: %+v", test.Steps)
	}

	// A stage only the template defines keeps the template's steps.
	build, ok := eff["build"]
	if !ok || len(build.Steps) == 0 {
		t.Errorf("build should inherit template steps, got %+v", build)
	}
}
