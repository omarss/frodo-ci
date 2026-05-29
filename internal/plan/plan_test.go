package plan

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/omarss/frodo-ci/internal/cache"
	"github.com/omarss/frodo-ci/internal/configlint"
)

func gitOrSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const cardsModule = `name: cards
type: spring-service
owners:
  teams: [cards-team]
depends_on:
  - module: money
    affects: [test, build, package, scan]
ci:
  validate:
    when: [src/**]
  test:
    when: [src/**]
`

const moneyModule = `name: money
type: node-library
owners:
  teams: [payments-team]
ci:
  validate:
    when: [src/**]
  test:
    when: [src/**]
`

const rootMinutes = `version: 1
minutes:
  skip_unchanged: true
  exact_fingerprint_match_only: true
`

func setupRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".github/frodo-ci.yml"), rootMinutes)
	write(t, filepath.Join(root, "services/cards/.ci/module.yml"), cardsModule)
	write(t, filepath.Join(root, "packages/money/.ci/module.yml"), moneyModule)
	write(t, filepath.Join(root, "services/cards/src/Card.java"), "class Card{}\n")
	write(t, filepath.Join(root, "packages/money/src/index.ts"), "export const x=1\n")

	git(t, root, "-c", "init.defaultBranch=main", "init", "-q")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "init")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(root, "packages/money/src/index.ts"), "export const x=2\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "change money")
	return root
}

func findModule(p *Plan, name string) *ModulePlan {
	for _, m := range p.Modules {
		if m.Name == name {
			return m
		}
	}
	return nil
}

func stageNames(m *ModulePlan) map[string]*StagePlan {
	out := map[string]*StagePlan{}
	for _, s := range m.Stages {
		out[s.Stage] = s
	}
	return out
}

func TestPlanDependencyPropagation(t *testing.T) {
	gitOrSkip(t)
	root := setupRepo(t)
	loaded, err := Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if configlint.HasErrors(loaded.Problems) {
		t.Fatalf("unexpected config problems: %v", loaded.Problems)
	}
	p, err := loaded.Plan(Context{Base: "main", Head: "HEAD", Event: "pull_request"}, cache.Noop{})
	if err != nil {
		t.Fatal(err)
	}

	cards := findModule(p, "cards")
	if cards == nil {
		t.Fatal("cards should be planned because money changed")
	}
	cs := stageNames(cards)
	for _, want := range []string{"test", "build", "package", "scan"} {
		if cs[want] == nil {
			t.Errorf("cards.%s should be planned (dependency affects)", want)
		}
	}
	if cs["validate"] != nil {
		t.Error("cards.validate should NOT be planned (not in affects, no cards change)")
	}

	money := findModule(p, "money")
	if money == nil || stageNames(money)["validate"] == nil {
		t.Error("money.validate should be planned (its source changed)")
	}
}

func TestPlanCacheSkip(t *testing.T) {
	gitOrSkip(t)
	root := setupRepo(t)
	loaded, err := Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	money := loaded.FindModule("money")
	fp, _, err := loaded.StageFingerprint(money, "validate")
	if err != nil {
		t.Fatal(err)
	}
	c, err := cache.NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(cache.Entry{Fingerprint: fp, Module: "money", Stage: "validate", Conclusion: "success"}); err != nil {
		t.Fatal(err)
	}
	p, err := loaded.Plan(Context{Base: "main", Head: "HEAD", Event: "pull_request"}, c)
	if err != nil {
		t.Fatal(err)
	}
	mv := stageNames(findModule(p, "money"))["validate"]
	if mv == nil || !mv.Skipped {
		t.Errorf("money.validate should be skipped on exact cache match, got %+v", mv)
	}
}

func TestLoadDetectsCycle(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".github/frodo-ci.yml"), "version: 1\n")
	write(t, filepath.Join(root, "a/.ci/module.yml"), "name: a\ntype: node-library\ndepends_on:\n  - module: b\n")
	write(t, filepath.Join(root, "b/.ci/module.yml"), "name: b\ntype: node-library\ndepends_on:\n  - module: a\n")
	loaded, err := Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !containsProblem(loaded.SemanticProblems, "cycle") {
		t.Errorf("expected a cycle problem, got %v", loaded.SemanticProblems)
	}
}

func TestLoadDetectsUnknownStage(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".github/frodo-ci.yml"), "version: 1\n")
	write(t, filepath.Join(root, "a/.ci/module.yml"), "name: a\ntype: node-library\nci:\n  deployx:\n    when: [src/**]\n")
	loaded, err := Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !containsProblem(loaded.SemanticProblems, "unknown CI stage") {
		t.Errorf("expected unknown-stage problem, got %v", loaded.SemanticProblems)
	}
}

func containsProblem(ps []configlint.Problem, substr string) bool {
	for _, p := range ps {
		if strings.Contains(p.Message, substr) {
			return true
		}
	}
	return false
}
