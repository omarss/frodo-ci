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

	if err := app.runInit(false); err != nil {
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
	if err := app.runInit(false); err != nil {
		t.Fatal(err)
	}
	if err := app.runInitModule("cards", "spring-service", "services/cards", "cards-team"); err != nil {
		t.Fatalf("init-module: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, "services/cards/.ci/module.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("name: cards")) || !bytes.Contains(data, []byte("cards-team")) {
		t.Errorf("unexpected module.yml:\n%s", data)
	}
	// Re-running must refuse to overwrite.
	if err := app.runInitModule("cards", "spring-service", "services/cards", "cards-team"); err == nil {
		t.Error("expected init-module to refuse overwriting an existing module")
	}
}
