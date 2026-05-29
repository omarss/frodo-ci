package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"golang.org/x/sync/semaphore"

	"github.com/omarss/frodo-ci/internal/cache"
	"github.com/omarss/frodo-ci/internal/discover"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/omarss/frodo-ci/internal/templates"
)

// Status is a stage or module outcome.
type Status string

const (
	StatusSuccess   Status = "success"
	StatusFailure   Status = "failure"
	StatusSkipped   Status = "skipped"
	StatusCancelled Status = "cancelled"
	StatusTimedOut  Status = "timed_out"
)

// StageResult is the outcome of executing one stage.
type StageResult struct {
	Module   string        `json:"module"`
	Stage    string        `json:"stage"`
	Group    string        `json:"group"`
	Status   Status        `json:"status"`
	Duration time.Duration `json:"duration"`
	Steps    []StepResult  `json:"steps,omitempty"`
	Note     string        `json:"note,omitempty"`
	// Env is the resolved FRODO_* (and stage) environment the steps ran with, so
	// failure reporting can produce a literally-runnable reproduce command.
	Env map[string]string `json:"env,omitempty"`
}

// Result is the overall execution outcome.
type Result struct {
	Stages  []StageResult `json:"stages"`
	Success bool          `json:"success"`
}

// SecurityScanner runs the scan stage for a module (wired by the security
// subsystem). When nil, scan stages with no steps simply succeed.
type SecurityScanner interface {
	Scan(ctx context.Context, module string) (Status, string)
}

// Reporter receives stage outcomes as they complete, e.g. to create or update
// GitHub check-runs. The final job's own exit code is the "Frodo CI / final"
// check, so reporters only track the internal per-stage checks.
type Reporter interface {
	StageFinished(StageResult)
}

type noopReporter struct{}

func (noopReporter) StageFinished(StageResult) {}

// Options configures a Runner. Most values come from the root config's
// execution block.
type Options struct {
	Env                  map[string]string
	MaxParallelModules   int
	MaxParallelExpensive int
	FullRunTimeout       time.Duration
	NoProgressTimeout    time.Duration
	StageDefaultTimeout  time.Duration
	StopModuleOnFailure  bool
	StopDependents       bool
	Cache                cache.Cache
	Scanner              SecurityScanner
	Reporter             Reporter
	ImagePrefix          string // prepended to "<module>:<tag>" for $FRODO_IMAGE
	Log                  zerolog.Logger
}

// Runner executes plans for a loaded repository.
type Runner struct {
	loaded *plan.Loaded
	opts   Options
}

// New returns a Runner. Zero-valued options are filled with safe defaults.
func New(loaded *plan.Loaded, opts Options) *Runner {
	if opts.MaxParallelModules <= 0 {
		opts.MaxParallelModules = 4
	}
	if opts.MaxParallelExpensive <= 0 {
		opts.MaxParallelExpensive = 1
	}
	if opts.FullRunTimeout <= 0 {
		opts.FullRunTimeout = 90 * time.Minute
	}
	if opts.StageDefaultTimeout <= 0 {
		opts.StageDefaultTimeout = 30 * time.Minute
	}
	if opts.Cache == nil {
		opts.Cache = cache.Noop{}
	}
	if opts.Reporter == nil {
		opts.Reporter = noopReporter{}
	}
	return &Runner{loaded: loaded, opts: opts}
}

// Run executes the plan for the given group ("ci", "cd", or "" / "all" for
// everything), returning per-stage results. Modules run with bounded
// parallelism in dependency order; a module starts only after its in-plan
// dependencies succeed.
func (r *Runner) Run(ctx context.Context, p *plan.Plan, group string) *Result {
	byModule, order := r.scope(p, group)

	ctx, cancel := context.WithTimeout(ctx, r.opts.FullRunTimeout)
	defer cancel()
	progress := make(chan struct{}, 128)
	go watchdog(ctx, cancel, r.opts.NoProgressTimeout, progress)

	done := make(map[string]chan struct{}, len(order))
	status := make(map[string]Status, len(order))
	for _, name := range order {
		done[name] = make(chan struct{})
	}
	modSem := semaphore.NewWeighted(int64(r.opts.MaxParallelModules))
	expSem := semaphore.NewWeighted(int64(r.opts.MaxParallelExpensive))

	var (
		mu  sync.Mutex
		res Result
		wg  sync.WaitGroup
	)
	record := func(sr StageResult) {
		mu.Lock()
		res.Stages = append(res.Stages, sr)
		mu.Unlock()
		r.opts.Reporter.StageFinished(sr)
		select {
		case progress <- struct{}{}:
		default:
		}
	}
	setStatus := func(name string, s Status) {
		mu.Lock()
		status[name] = s
		mu.Unlock()
	}
	depStatus := func(name string) Status {
		mu.Lock()
		defer mu.Unlock()
		return status[name]
	}

	for _, name := range order {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			defer close(done[name])

			if !r.awaitDeps(ctx, name, done, depStatus) {
				setStatus(name, StatusCancelled)
				for _, s := range byModule[name] {
					record(StageResult{Module: name, Stage: s.Stage, Group: s.Group, Status: StatusCancelled,
						Note: "a dependency failed or was cancelled"})
				}
				return
			}
			if err := modSem.Acquire(ctx, 1); err != nil {
				setStatus(name, StatusCancelled)
				return
			}
			defer modSem.Release(1)
			setStatus(name, r.runModule(ctx, p, byModule[name], expSem, record))
		}(name)
	}
	wg.Wait()

	res.Success = true
	for _, s := range res.Stages {
		if s.Status != StatusSuccess && s.Status != StatusSkipped {
			res.Success = false
			break
		}
	}
	sort.SliceStable(res.Stages, func(i, j int) bool {
		if res.Stages[i].Module != res.Stages[j].Module {
			return res.Stages[i].Module < res.Stages[j].Module
		}
		return res.Stages[i].Stage < res.Stages[j].Stage
	})
	return &res
}

// awaitDeps blocks until all of name's in-plan dependencies finish, returning
// false if any failed (and StopDependents is set) or the run was cancelled.
func (r *Runner) awaitDeps(ctx context.Context, name string, done map[string]chan struct{}, statusOf func(string) Status) bool {
	for _, e := range r.loaded.Graph.Dependencies(name) {
		ch, ok := done[e.To]
		if !ok {
			continue // dependency has no work in this plan
		}
		select {
		case <-ch:
			if r.opts.StopDependents && statusOf(e.To) != StatusSuccess {
				return false
			}
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// runModule runs a module's stages sequentially in canonical order, stopping
// early on failure when configured.
func (r *Runner) runModule(ctx context.Context, p *plan.Plan, stages []*plan.StagePlan, expSem *semaphore.Weighted, record func(StageResult)) Status {
	modStatus := StatusSuccess
	stopped := false
	for _, s := range stages {
		if stopped {
			record(StageResult{Module: s.Module, Stage: s.Stage, Group: s.Group, Status: StatusSkipped,
				Note: "skipped after an earlier stage failed"})
			continue
		}
		if ctx.Err() != nil {
			record(StageResult{Module: s.Module, Stage: s.Stage, Group: s.Group, Status: StatusCancelled})
			modStatus = StatusCancelled
			continue
		}
		sr := r.runStage(ctx, p, s, expSem)
		record(sr)
		if sr.Status != StatusSuccess && sr.Status != StatusSkipped {
			modStatus = StatusFailure
			if r.opts.StopModuleOnFailure {
				stopped = true
			}
		}
	}
	return modStatus
}

// runStage executes a single stage's steps under a per-stage timeout.
func (r *Runner) runStage(ctx context.Context, p *plan.Plan, s *plan.StagePlan, expSem *semaphore.Weighted) StageResult {
	start := time.Now()
	sr := StageResult{Module: s.Module, Stage: s.Stage, Group: s.Group}

	m := r.loaded.FindModule(s.Module)
	eff, ok := r.loaded.EffectiveStages(m)[s.Stage]
	if !ok {
		sr.Status = StatusSkipped
		sr.Note = "no effective stage definition"
		return sr
	}

	timeout := r.opts.StageDefaultTimeout
	if eff.TimeoutMinutes > 0 {
		timeout = time.Duration(eff.TimeoutMinutes) * time.Minute
	}
	sctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if s.Expensive {
		if err := expSem.Acquire(sctx, 1); err != nil {
			sr.Status = StatusCancelled
			return sr
		}
		defer expSem.Release(1)
	}

	// A scan stage with no steps is delegated to the security scanner.
	if len(eff.Steps) == 0 {
		if s.Stage == "scan" && r.opts.Scanner != nil {
			st, note := r.opts.Scanner.Scan(sctx, s.Module)
			sr.Status, sr.Note, sr.Duration = st, note, time.Since(start)
			return sr
		}
		sr.Status = StatusSuccess
		sr.Note = "no steps"
		sr.Duration = time.Since(start)
		return sr
	}

	frodo := r.frodoEnv(s, m, eff, p.Context)
	sr.Env = frodo
	env := r.stageEnvList(frodo)
	sr.Status = StatusSuccess
	for _, step := range eff.Steps {
		step := step
		// Steps default to running in the module's directory (what the stock
		// templates assume); an explicit working_directory overrides it.
		wd := step.WorkingDirectory
		if wd == "" {
			wd = m.Dir
		}
		stepRes := runStep(sctx, r.loaded.RepoRoot, wd, step.Run, env, r.opts.Log.With().Str("module", s.Module).Str("stage", s.Stage).Logger())
		stepRes.Name = step.Name
		sr.Steps = append(sr.Steps, stepRes)
		if stepRes.Err != "" {
			sr.Status = StatusFailure
			if stepRes.TimedOut {
				sr.Status = StatusTimedOut
			}
			break
		}
	}
	sr.Duration = time.Since(start)

	if sr.Status == StatusSuccess && s.Cacheable && r.opts.Cache != nil {
		_ = r.opts.Cache.Save(cache.Entry{
			Fingerprint: s.Fingerprint, Module: s.Module, Stage: s.Stage,
			Conclusion: string(StatusSuccess), SavedAtUnix: time.Now().Unix(),
		})
	}
	return sr
}

// scope selects the stages to run for a group and orders modules and stages.
func (r *Runner) scope(p *plan.Plan, group string) (map[string][]*plan.StagePlan, []string) {
	rank := stageRank(r.loaded.Root.AllStages())
	byModule := map[string][]*plan.StagePlan{}
	var order []string
	for _, mp := range p.Modules {
		var stages []*plan.StagePlan
		for _, s := range mp.Stages {
			if s.Skipped {
				continue
			}
			if group != "" && group != "all" && s.Group != group {
				continue
			}
			stages = append(stages, s)
		}
		if len(stages) == 0 {
			continue
		}
		sort.SliceStable(stages, func(i, j int) bool { return rank[stages[i].Stage] < rank[stages[j].Stage] })
		byModule[mp.Name] = stages
		order = append(order, mp.Name)
	}
	return byModule, order
}

// frodoEnv returns the FRODO_* (and stage) variables for a stage. They let
// templates locate the module and name images without hard-coded paths; a
// stage's own env overrides them, including FRODO_IMAGE.
func (r *Runner) frodoEnv(s *plan.StagePlan, m *discover.Module, eff templates.EffectiveStage, ctx plan.Context) map[string]string {
	env := map[string]string{
		"FRODO_MODULE":      s.Module,
		"FRODO_MODULE_PATH": m.Dir,
		"FRODO_MODULE_DIR":  filepath.Join(r.loaded.RepoRoot, filepath.FromSlash(m.Dir)),
		"FRODO_REPO_ROOT":   r.loaded.RepoRoot,
		"FRODO_STAGE":       s.Stage,
		"FRODO_ENVIRONMENT": ctx.Environment,
		"FRODO_IMAGE":       r.imageFor(s.Module),
	}
	for k, v := range eff.Env {
		env[k] = v.String()
	}
	return env
}

// stageEnvList layers the FRODO_* env on top of the process and option env for
// actual execution.
func (r *Runner) stageEnvList(frodo map[string]string) []string {
	env := os.Environ()
	for k, v := range r.opts.Env {
		env = append(env, k+"="+v)
	}
	for k, v := range frodo {
		env = append(env, k+"="+v)
	}
	return env
}

// imageFor derives the default container image name "<prefix><module>:<tag>".
// The tag is the short commit SHA in CI, else "ci".
func (r *Runner) imageFor(module string) string {
	tag := "ci"
	if sha := os.Getenv("GITHUB_SHA"); len(sha) >= 7 {
		tag = sha[:7]
	}
	return r.opts.ImagePrefix + module + ":" + tag
}

func watchdog(ctx context.Context, cancel context.CancelFunc, timeout time.Duration, progress <-chan struct{}) {
	if timeout <= 0 {
		return
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-progress:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			t.Reset(timeout)
		case <-t.C:
			cancel()
			return
		}
	}
}

func stageRank(stages []string) map[string]int {
	rank := make(map[string]int, len(stages))
	for i, s := range stages {
		rank[s] = i
	}
	return rank
}

// Summary returns a one-line summary of a result.
func (res *Result) Summary() string {
	var ok, fail, skip int
	for _, s := range res.Stages {
		switch s.Status {
		case StatusSuccess:
			ok++
		case StatusSkipped:
			skip++
		default:
			fail++
		}
	}
	return fmt.Sprintf("%d succeeded, %d failed, %d skipped", ok, fail, skip)
}
