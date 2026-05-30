package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/omarss/frodo-ci/internal/cache"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/rs/zerolog"
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

// repoWithSteps builds a module "alpha" whose validate/test steps are the given
// shell commands, returns the loaded state.
func repoWithSteps(t *testing.T, validateRun, testRun string) *plan.Loaded {
	t.Helper()
	root := t.TempDir()
	write(t, filepath.Join(root, ".github/frodo-ci.yml"),
		"version: 1\nexecution:\n  stop_module_on_stage_failure: true\n")
	write(t, filepath.Join(root, "alpha/.ci/module.yml"),
		"name: alpha\nowners:\n  teams: [t]\nci:\n  validate:\n    when: [src/**]\n  test:\n    when: [src/**]\n")
	write(t, filepath.Join(root, "alpha/.ci/validate.yml"),
		"name: validate\nsteps:\n  - {name: v, run: "+validateRun+"}\n")
	write(t, filepath.Join(root, "alpha/.ci/test.yml"),
		"name: test\nsteps:\n  - {name: t, run: "+testRun+"}\n")
	write(t, filepath.Join(root, "alpha/src/x.txt"), "hi\n")

	loaded, err := plan.Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HasErrors() {
		t.Fatalf("config errors: %v", loaded.Problems)
	}
	return loaded
}

// manualPlan builds a plan that runs the given stages of alpha, bypassing change
// detection so the runner is exercised in isolation.
func manualPlan(loaded *plan.Loaded, stages ...string) *plan.Plan {
	mp := &plan.ModulePlan{Name: "alpha", Dir: "alpha"}
	for _, s := range stages {
		mp.Stages = append(mp.Stages, &plan.StagePlan{Module: "alpha", Stage: s, Group: "ci", Cacheable: true})
	}
	return &plan.Plan{RepoRoot: loaded.RepoRoot, Modules: []*plan.ModulePlan{mp}}
}

func opts() Options {
	return Options{StopModuleOnFailure: true, Log: zerolog.Nop(), Cache: cache.Noop{}}
}

// TestStepRunsInModuleDirWithEnv guards REQUESTED.md #6/#7: steps run in the
// module's directory and FRODO_IMAGE / FRODO_MODULE_DIR are injected.
func TestStepRunsInModuleDirWithEnv(t *testing.T) {
	check := `test "$(basename "$PWD")" = alpha && test -n "$FRODO_IMAGE" && test -n "$FRODO_MODULE_DIR"`
	loaded := repoWithSteps(t, check, "echo ok")
	res := New(loaded, opts()).Run(context.Background(), manualPlan(loaded, "validate"), "all")
	if !res.Success {
		t.Fatalf("step should run in module dir with FRODO_IMAGE/FRODO_MODULE_DIR set; got %s: %+v",
			res.Summary(), res.Stages)
	}
}

func TestRunSuccess(t *testing.T) {
	loaded := repoWithSteps(t, "echo validate-ok", "echo test-ok")
	res := New(loaded, opts()).Run(context.Background(), manualPlan(loaded, "validate", "test"), "all")
	if !res.Success {
		t.Fatalf("expected success, got %s", res.Summary())
	}
	if len(res.Stages) != 2 {
		t.Fatalf("expected 2 stage results, got %d", len(res.Stages))
	}
	for _, s := range res.Stages {
		if s.Status != StatusSuccess {
			t.Errorf("%s = %s", s.Stage, s.Status)
		}
	}
}

func TestRunFailFast(t *testing.T) {
	loaded := repoWithSteps(t, "exit 1", "echo test-ok")
	res := New(loaded, opts()).Run(context.Background(), manualPlan(loaded, "validate", "test"), "all")
	if res.Success {
		t.Fatal("expected failure")
	}
	byStage := map[string]Status{}
	for _, s := range res.Stages {
		byStage[s.Stage] = s.Status
	}
	if byStage["validate"] != StatusFailure {
		t.Errorf("validate = %s, want failure", byStage["validate"])
	}
	if byStage["test"] != StatusSkipped {
		t.Errorf("test = %s, want skipped (stop_module_on_stage_failure)", byStage["test"])
	}
}

func TestRunCacheSaveOnSuccess(t *testing.T) {
	loaded := repoWithSteps(t, "echo ok", "echo ok")
	c, err := cache.NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := opts()
	o.Cache = c
	p := manualPlan(loaded, "validate")
	p.Modules[0].Stages[0].Fingerprint = "deadbeef12345678"
	New(loaded, o).Run(context.Background(), p, "all")
	if ok, _ := c.Has("deadbeef12345678"); !ok {
		t.Error("successful cacheable stage should be saved to the cache")
	}
}

// TestCacheRestoresOutputsOnHit is the heart of the output cache: a stage that
// produces declared outputs archives them on success, and a later fingerprint
// hit restores them to disk instead of rebuilding -- even after the workspace
// is wiped (a fresh runner / clean checkout).
func TestCacheRestoresOutputsOnHit(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".github/frodo-ci.yml"), "version: 1\n")
	write(t, filepath.Join(root, "alpha/.ci/module.yml"),
		"name: alpha\nowners:\n  teams: [t]\nci:\n  build:\n    when: [src/**]\n")
	write(t, filepath.Join(root, "alpha/.ci/build.yml"),
		"name: build\noutputs: [dist]\nsteps:\n  - {name: b, run: 'mkdir -p dist && echo built > dist/out.txt'}\n")
	write(t, filepath.Join(root, "alpha/src/x.txt"), "hi\n")

	loaded, err := plan.Load(root, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.HasErrors() {
		t.Fatalf("config errors: %v", loaded.Problems)
	}
	c, err := cache.NewLocalCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := opts()
	o.Cache = c
	const fp = "feedface00112233"
	stage := func(cached bool) *plan.Plan {
		return &plan.Plan{RepoRoot: loaded.RepoRoot, Modules: []*plan.ModulePlan{{Name: "alpha", Dir: "alpha",
			Stages: []*plan.StagePlan{{Module: "alpha", Stage: "build", Group: "ci",
				Cacheable: true, Fingerprint: fp, Cached: cached, Skipped: cached}}}}}
	}
	dist := filepath.Join(root, "alpha/dist/out.txt")

	// Run 1: build runs, produces dist/, and archives it keyed by the fingerprint.
	if res := New(loaded, o).Run(context.Background(), stage(false), "all"); !res.Success {
		t.Fatalf("run 1 failed: %+v", res.Stages)
	}
	if _, err := os.Stat(dist); err != nil {
		t.Fatalf("run 1 should have produced dist/out.txt: %v", err)
	}

	// Wipe outputs to simulate a fresh runner, then re-run as a cache hit.
	if err := os.RemoveAll(filepath.Join(root, "alpha/dist")); err != nil {
		t.Fatal(err)
	}
	res := New(loaded, o).Run(context.Background(), stage(true), "all")
	if !res.Success || len(res.Stages) != 1 || res.Stages[0].Status != StatusSkipped {
		t.Fatalf("run 2 should be a single skipped stage: %+v", res.Stages)
	}
	if !res.Stages[0].RestoredOutputs {
		t.Error("run 2 should report restored outputs")
	}
	got, err := os.ReadFile(dist)
	if err != nil {
		t.Fatalf("dist/out.txt should have been restored: %v", err)
	}
	if string(got) != "built\n" {
		t.Errorf("restored content = %q, want %q", got, "built\n")
	}
}

func TestStageTimeout(t *testing.T) {
	loaded := repoWithSteps(t, "sleep 5", "echo ok")
	o := opts()
	o.StageDefaultTimeout = 200 * time.Millisecond
	res := New(loaded, o).Run(context.Background(), manualPlan(loaded, "validate"), "all")
	if res.Success {
		t.Fatal("expected timeout failure")
	}
	if res.Stages[0].Status != StatusTimedOut {
		t.Errorf("validate = %s, want timed_out", res.Stages[0].Status)
	}
}
