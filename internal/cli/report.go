package cli

import (
	"os"

	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/github"
	"github.com/frodo-ci/frodo-ci/internal/perf"
	"github.com/frodo-ci/frodo-ci/internal/plan"
	"github.com/frodo-ci/frodo-ci/internal/runner"
	"github.com/frodo-ci/frodo-ci/internal/slack"
)

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
