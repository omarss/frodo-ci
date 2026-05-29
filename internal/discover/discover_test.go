package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/frodo-ci/frodo-ci/internal/config"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sampleRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, "services/cards/.ci/module.yml"), "name: cards\ntype: spring-service\n")
	write(t, filepath.Join(root, "services/cards/.ci/test.yml"), "name: Test cards\nsteps:\n  - run: echo hi\n")
	write(t, filepath.Join(root, "packages/money/.ci/module.yml"), "name: money\ntype: node-library\n")
	// A module inside an excluded directory and a heavy directory.
	write(t, filepath.Join(root, "skip/bad/.ci/module.yml"), "name: bad\n")
	write(t, filepath.Join(root, "node_modules/pkg/.ci/module.yml"), "name: vendored\n")
	return root
}

func baseConfig() *config.RootConfig {
	c := &config.RootConfig{}
	c.ApplyDefaults()
	c.Scan.Exclude = []string{"skip/**"}
	return c
}

func TestDiscover(t *testing.T) {
	root := sampleRepo(t)
	mods, err := Discover(root, baseConfig())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(mods) != 2 {
		t.Fatalf("found %d modules, want 2 (%v)", len(mods), names(mods))
	}
	// Sorted by directory: packages/money before services/cards.
	if mods[0].Name != "money" || mods[1].Name != "cards" {
		t.Errorf("order = %v, want [money cards]", names(mods))
	}
	cards := mods[1]
	if cards.Dir != "services/cards" {
		t.Errorf("cards.Dir = %q", cards.Dir)
	}
	if _, ok := cards.Stages["test"]; !ok {
		t.Errorf("cards should have a test stage file, got %v", cards.Stages)
	}
	if cards.ConfigPath != "services/cards/.ci/module.yml" {
		t.Errorf("cards.ConfigPath = %q", cards.ConfigPath)
	}
}

func TestDiscoverIncludeFilter(t *testing.T) {
	root := sampleRepo(t)
	c := baseConfig()
	c.Scan.Include = []string{"services/**"}
	mods, err := Discover(root, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(mods) != 1 || mods[0].Name != "cards" {
		t.Errorf("include filter yielded %v, want [cards]", names(mods))
	}
}

func TestByName(t *testing.T) {
	root := sampleRepo(t)
	mods, _ := Discover(root, baseConfig())
	idx := ByName(mods)
	if idx["cards"] == nil || idx["money"] == nil {
		t.Errorf("ByName missing entries: %v", idx)
	}
}

func names(mods []*Module) []string {
	out := make([]string, len(mods))
	for i, m := range mods {
		out[i] = m.Name
	}
	return out
}
