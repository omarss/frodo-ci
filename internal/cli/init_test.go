package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestInitProducesValidConfig is the key scaffolding guarantee: what `init`
// writes must pass `validate-config` with zero failures.
func TestInitProducesValidConfig(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	app := &App{RepoRoot: root, Out: &out, Err: &out}

	if err := app.runInit(false, ""); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}
	for _, f := range []string{".github/frodo-ci.yml", ".github/workflows/frodo-ci.yml", ".vscode/settings.json"} {
		if _, err := os.Stat(filepath.Join(root, f)); err != nil {
			t.Errorf("init did not create %s", f)
		}
	}

	out.Reset()
	if err := app.runValidateConfig(); err != nil {
		t.Fatalf("freshly-initialized config failed validation:\n%s", out.String())
	}
}

func TestInitModule(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	app := &App{RepoRoot: root, Out: &out, Err: &out}
	if err := app.runInit(false, ""); err != nil {
		t.Fatal(err)
	}
	deps := parseDependsOn([]string{"money:affects=test,build"})
	if err := app.runInitModule("cards", "spring-service", "services/cards", "cards-team", deps, false); err != nil {
		t.Fatalf("init-module: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "services/cards/.ci/module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"name: cards", "cards-team", "depends_on", "module: money", "affects: [test, build]"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("module.yml missing %q:\n%s", want, data)
		}
	}
	// Re-running without --force must refuse to overwrite.
	if err := app.runInitModule("cards", "spring-service", "services/cards", "cards-team", nil, false); err == nil {
		t.Error("expected init-module to refuse overwriting an existing module")
	}
	// With --force it overwrites.
	if err := app.runInitModule("cards", "spring-service", "services/cards", "cards-team", nil, true); err != nil {
		t.Errorf("init-module --force should overwrite, got %v", err)
	}
}

func TestParseDependsOn(t *testing.T) {
	deps := parseDependsOn([]string{"money:affects=test,build", "common"})
	if len(deps) != 2 {
		t.Fatalf("got %d deps", len(deps))
	}
	if deps[0].Module != "money" || len(deps[0].Affects) != 2 {
		t.Errorf("money dep = %+v", deps[0])
	}
	// Omitted affects defaults to the post-validate CI stages.
	if deps[1].Module != "common" || len(deps[1].Affects) != 4 {
		t.Errorf("common dep = %+v", deps[1])
	}
}
