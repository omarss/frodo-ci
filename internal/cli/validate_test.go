package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunValidateConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github/frodo-ci.yml"),
		"version: 1\nstages:\n  ci:\n    - validate\n")
	writeFile(t, filepath.Join(root, "services/cards/.ci/module.yml"),
		"name: cards\ntype: spring-service\n")

	var out bytes.Buffer
	app := &App{RepoRoot: root, Out: &out, Err: &out}

	if err := app.runValidateConfig(); err != nil {
		t.Fatalf("expected valid config, got: %v\n%s", err, out.String())
	}

	// Introduce an unknown stage name and expect a friendly, quiet failure.
	writeFile(t, filepath.Join(root, "services/cards/.ci/module.yml"),
		"name: cards\nci:\n  tests: {}\n")
	out.Reset()
	err := app.runValidateConfig()
	if err == nil {
		t.Fatal("expected a failure for an unknown stage name")
	}
	if !errors.Is(err, ErrExitQuiet) {
		t.Errorf("want ErrExitQuiet, got %v", err)
	}
	if !strings.Contains(out.String(), "did you mean") {
		t.Errorf("expected a did-you-mean suggestion, got:\n%s", out.String())
	}
}

func TestDiscoverConfigFilesSkipsHeavyDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".github/frodo-ci.yml"), "version: 1\n")
	// A module-looking file buried in node_modules must be ignored.
	writeFile(t, filepath.Join(root, "node_modules/pkg/.ci/module.yml"), "name: nope\n")
	writeFile(t, filepath.Join(root, "services/cards/.ci/module.yml"), "name: cards\n")

	files, err := discoverConfigFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(f.Rel, "node_modules") {
			t.Errorf("node_modules config should be skipped, found %s", f.Rel)
		}
	}
}
