package cli

import (
	"context"
	"os"
	"strconv"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/diag"
	"github.com/omarss/frodo-ci/internal/github"
	"github.com/omarss/frodo-ci/internal/perf"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/omarss/frodo-ci/internal/report"
	"github.com/omarss/frodo-ci/internal/runner"
	"github.com/omarss/frodo-ci/internal/slack"
)

// publishReport renders the run summary (what failed, why it ran, how to fix)
// to the GitHub step summary and, on a pull request, upserts it as a single PR
// comment so failures are obvious without digging through logs.
func (a *App) publishReport(ctx context.Context, loaded *plan.Loaded, p *plan.Plan, res *runner.Result, review ReviewOutcome) {
	md := report.BuildComment(a.buildReportInput(loaded, p, res, review))

	if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.WriteString(md + "\n")
			_ = f.Close()
		}
	}

	gh := github.FromEnv()
	if gh.Enabled() && gh.HasPR() {
		if err := github.NewClient(ctx, gh).UpsertComment(ctx, report.Marker, md); err != nil {
			a.Log.Warn().Err(err).Msg("could not post PR summary comment")
		}
	}
}

func (a *App) buildReportInput(loaded *plan.Loaded, p *plan.Plan, res *runner.Result, review ReviewOutcome) report.Input {
	reasons := map[string][]string{}
	for _, m := range p.Modules {
		for _, s := range m.Stages {
			reasons[m.Name+"/"+s.Stage] = s.Reasons
		}
	}
	var stages []report.StageReport
	for _, s := range res.Stages {
		sr := report.StageReport{
			Module: s.Module, Stage: s.Stage, Status: string(s.Status),
			Note: s.Note, Duration: s.Duration, Reasons: reasons[s.Module+"/"+s.Stage],
			Owners: ownerMentions(loaded, s.Module), Env: s.Env,
			Cached: s.Cached, Saved: s.Saved,
		}
		if step := lastFailedStep(s.Steps); step != nil {
			sr.FailedStep = step.Name
			if sr.FailedStep == "" {
				sr.FailedStep = s.Stage
			}
			sr.FailReason = "exit " + strconv.Itoa(step.ExitCode)
			if step.TimedOut {
				sr.FailReason = "timed out"
			}
			sr.ReproduceCmd, sr.ReproduceDir = reproduce(loaded, s.Module, s.Stage, len(s.Steps)-1)
			// Stack-aware diagnosis: detect the tech stack and extract the
			// salient error, summary, and fix hint from the raw output.
			d := diag.Analyze(diag.Input{
				Command:    sr.ReproduceCmd,
				Output:     step.Output,
				Stage:      s.Stage,
				ModuleType: moduleType(loaded, s.Module),
			})
			sr.Summary, sr.Hint, sr.Stack, sr.Output = d.Summary, d.Hint, d.Stack, d.Snippet
		}
		stages = append(stages, sr)
	}
	var reviewReports []report.ReviewReport
	for _, m := range review.Modules {
		reviewReports = append(reviewReports, report.ReviewReport{
			Module: m.Module, OK: m.OK, Missing: m.Missing,
		})
	}
	return report.Input{SHA: github.FromEnv().SHA, Stages: stages, Reviews: reviewReports}
}

func ownerMentions(loaded *plan.Loaded, module string) []string {
	m := loaded.FindModule(module)
	if m == nil {
		return nil
	}
	var out []string
	for _, t := range m.Config.Owners.Teams {
		out = append(out, "@"+t)
	}
	for _, u := range m.Config.Owners.Users {
		out = append(out, "@"+u)
	}
	return out
}

func moduleType(loaded *plan.Loaded, module string) string {
	m := loaded.FindModule(module)
	if m == nil {
		return ""
	}
	if m.Config.Use.Profile != "" {
		return m.Config.Use.Profile
	}
	return m.Config.Type
}

func lastFailedStep(steps []runner.StepResult) *runner.StepResult {
	var found *runner.StepResult
	for i := range steps {
		if steps[i].Err != "" {
			found = &steps[i]
		}
	}
	return found
}

// reproduce returns the exact command and directory to re-run a failed step.
func reproduce(loaded *plan.Loaded, module, stage string, idx int) (cmd, dir string) {
	m := loaded.FindModule(module)
	if m == nil || idx < 0 {
		return "", ""
	}
	es, ok := loaded.EffectiveStages(m)[stage]
	if !ok || idx >= len(es.Steps) {
		return "", ""
	}
	step := es.Steps[idx]
	dir = step.WorkingDirectory
	if dir == "" {
		dir = m.Dir
	}
	return step.Run, dir
}

// reportRun emits the performance summary and Slack notifications after a run.
// It returns true when a budget was exceeded and the config fails on that.
func (a *App) reportRun(loaded *plan.Loaded, p *plan.Plan, result *runner.Result) bool {
	timings := make([]perf.StageTiming, 0, len(result.Stages))
	for _, s := range result.Stages {
		timings = append(timings, perf.StageTiming{Module: s.Module, Stage: s.Stage, Duration: s.Duration})
	}
	moduleBudgets := map[string]map[string]config.Duration{}
	for _, m := range loaded.Modules {
		if len(m.Config.Performance.Budgets) > 0 {
			moduleBudgets[m.Name] = m.Config.Performance.Budgets
		}
	}
	budgets := perf.BuildBudgets(moduleBudgets, loaded.Catalog.PerformanceBudgets)
	violations := perf.Check(timings, budgets)

	if loaded.Root.Performance.Enabled {
		summary := perf.StepSummary(timings, violations)
		if path := os.Getenv("GITHUB_STEP_SUMMARY"); path != "" {
			if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
				_, _ = f.WriteString(summary + "\n")
				_ = f.Close()
			}
		}
	}

	a.notifySlack(loaded, p, result)

	return len(violations) > 0 && loaded.Root.Performance.FailOnBudgetExceeded
}

func (a *App) notifySlack(loaded *plan.Loaded, p *plan.Plan, result *runner.Result) {
	if !loaded.Root.Slack.Enabled || result.Success {
		return
	}
	webhook := os.Getenv("SLACK_WEBHOOK_URL")
	if webhook == "" {
		return
	}
	gh := github.FromEnv()
	n := slack.New(webhook, loaded.Root.Slack.Channel, loaded.Root.Slack.NotifyOn)
	for _, s := range result.Stages {
		if s.Status == runner.StatusSuccess || s.Status == runner.StatusSkipped {
			continue
		}
		trig := triggerFor(s, p.Context)
		if trig == "" {
			continue
		}
		_ = n.Notify(slack.Event{
			Trigger:    trig,
			Repository: gh.Repo,
			Module:     s.Module,
			Stage:      s.Stage,
			Commit:     gh.SHA,
			Owner:      ownerSummary(loaded, s.Module),
			NextAction: "Fix " + s.Module + " " + s.Stage + " or ask the module owners to review the failure.",
		})
	}
}

// triggerFor maps a failed stage to a Slack trigger, avoiding spam for ordinary
// PR failures (only main-branch CI failures, CD failures, and security blockers
// notify).
func triggerFor(s runner.StageResult, ctx plan.Context) slack.Trigger {
	switch s.Group {
	case "cd":
		if s.Stage == "deploy" && ctx.Environment == "production" {
			return slack.ProductionDeployFailed
		}
		return slack.CDFailure
	default:
		if s.Stage == "scan" {
			return slack.SecurityBlocker
		}
		if ctx.OnDefaultBranch {
			return slack.CIFailureOnMain
		}
	}
	return ""
}

func ownerSummary(loaded *plan.Loaded, module string) string {
	m := loaded.FindModule(module)
	if m == nil || len(m.Config.Owners.Teams) == 0 {
		return ""
	}
	return m.Config.Owners.Teams[0]
}
