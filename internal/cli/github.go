package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/github"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/omarss/frodo-ci/internal/reviews"
	"github.com/omarss/frodo-ci/internal/runner"
)

// githubReporter creates one internal check-run per stage as it finishes. The
// "Frodo CI / final" check is the job's own conclusion, not a check-run.
type githubReporter struct {
	ctx    context.Context
	client *github.Client
}

func (g *githubReporter) StageFinished(sr runner.StageResult) {
	_, _ = g.client.CreateCheckRun(g.ctx, github.CheckRun{
		Name:       fmt.Sprintf("Frodo CI / %s / %s", sr.Module, sr.Stage),
		Status:     "completed",
		Conclusion: mapConclusion(sr.Status),
		Title:      fmt.Sprintf("%s %s", sr.Stage, sr.Status),
		Summary:    sr.Note,
	})
}

func mapConclusion(s runner.Status) string {
	switch s {
	case runner.StatusSuccess:
		return "success"
	case runner.StatusSkipped:
		return "skipped"
	case runner.StatusCancelled:
		return "cancelled"
	case runner.StatusTimedOut:
		return "timed_out"
	default:
		return "failure"
	}
}

// maybeReporter returns a GitHub check-run reporter when the environment has a
// token and repository; otherwise a nil reporter (the runner uses a no-op).
func maybeReporter(ctx context.Context) runner.Reporter {
	gh := github.FromEnv()
	if !gh.Enabled() {
		return nil
	}
	return &githubReporter{ctx: ctx, client: github.NewClient(ctx, gh)}
}

// newReviewCommand evaluates review, owner, and expert requirements.
func newReviewCommand(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "review",
		Short: "Evaluate review, owner, and expert requirements for the current PR",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return app.runReview(cmd.Context())
		},
	}
}

func (a *App) runReview(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	gh := github.FromEnv()

	for _, m := range loaded.Modules {
		rules := m.Config.Reviews
		if len(rules) == 0 {
			continue
		}
		fmt.Fprintf(a.Out, "%s (%s)\n", m.Name, m.Dir)
		for _, name := range sortedReviewRules(rules) {
			req := requirementOf(rules[name].Require)
			fmt.Fprintf(a.Out, "  %-20s owners=%d expert=%d teams=%v\n", name, req.Owners, req.Expert, req.Teams)
		}
	}

	if !gh.Enabled() || !gh.HasPR() {
		fmt.Fprintln(a.Out, "\n(no GitHub PR context; showing configured requirements only)")
		fmt.Fprintln(a.Out, "Set GITHUB_TOKEN and run inside a pull_request to evaluate live approvals.")
		return nil
	}

	outcome, err := a.evaluateReviews(ctx, loaded, gh)
	if err != nil {
		return err
	}
	printReviewOutcome(a.Out, outcome)
	if !outcome.Satisfied {
		return ErrExitQuiet
	}
	return nil
}

// ReviewOutcome is the result of evaluating a PR's review requirements across
// modules. Evaluated is false when there is no pull-request context, so reviews
// do not apply (e.g. a push to the default branch).
type ReviewOutcome struct {
	Evaluated bool
	Satisfied bool
	Modules   []ModuleReview
}

// ModuleReview is one module's review-gate status, including the reviewers that
// can satisfy it (owner teams, the resolved expert, required teams).
type ModuleReview struct {
	Module       string
	OK           bool
	Expert       string
	Missing      []string
	RequestTeams []string // team slugs to request as reviewers
	RequestUsers []string // user logins to request (the expert)
}

// gateReviews evaluates review requirements when running inside a pull request,
// so the single Frodo CI gate enforces them. Outside a PR it returns
// Evaluated=false and never blocks.
func (a *App) gateReviews(ctx context.Context, loaded *plan.Loaded) (ReviewOutcome, error) {
	gh := github.FromEnv()
	if !gh.Enabled() || !gh.HasPR() {
		return ReviewOutcome{}, nil
	}
	return a.evaluateReviews(ctx, loaded, gh)
}

func (a *App) evaluateReviews(ctx context.Context, loaded *plan.Loaded, gh github.Context) (ReviewOutcome, error) {
	client := github.NewClient(ctx, gh)
	raw, err := client.ListReviews(ctx)
	if err != nil {
		return ReviewOutcome{Evaluated: true}, fmt.Errorf("list reviews: %w", err)
	}
	history := make([]reviews.ReviewWithTime, 0, len(raw))
	for _, r := range raw {
		history = append(history, reviews.ReviewWithTime{
			User: r.User, State: r.State, IsBot: r.IsBot, CommitID: r.CommitID, SubmittedAt: r.SubmittedAt,
		})
	}
	states := reviews.LatestPerUser(history, gh.SHA, nil)
	windowDays := loaded.Root.Experts.Window.Duration().Hours() / 24
	since := time.Now().Add(-loaded.Root.Experts.Window.Duration())

	out := ReviewOutcome{Evaluated: true, Satisfied: true}
	for _, m := range loaded.Modules {
		rule, ok := m.Config.Reviews["default"]
		if !ok {
			continue
		}
		expert := a.resolveExpert(ctx, client, loaded, m.Dir, gh.PRAuthor, since, windowDays)
		owners := a.ownerLogins(ctx, client, m.Config.Owners.Teams)
		in := reviews.Inputs{
			Author:                   gh.PRAuthor,
			Reviews:                  states,
			Owners:                   owners,
			Expert:                   expert,
			TeamMembers:              a.teamMembers(ctx, client, rule.Require.Teams),
			IgnoreAuthor:             loaded.Root.Reviews.IgnoreAuthorApproval,
			IgnoreBots:               loaded.Root.Reviews.IgnoreBotApproval,
			RequireAfterLatestCommit: loaded.Root.Reviews.RequireApprovalAfterLatestCommit,
		}
		req := requirementOf(rule.Require)
		satisfied, missing := reviews.Evaluate(req, in)
		mr := ModuleReview{Module: m.Name, OK: satisfied, Expert: expert, Missing: missing}
		if !satisfied {
			out.Satisfied = false
			// Resolve who can unblock it: owner teams when owner approvals are
			// short, required teams that are short, and the expert when needed.
			sug := reviews.Suggest(req, in)
			if sug.OwnersShort {
				mr.RequestTeams = append(mr.RequestTeams, m.Config.Owners.Teams...)
			}
			mr.RequestTeams = append(mr.RequestTeams, sug.TeamsShort...)
			if sug.ExpertNeeded && sug.ExpertLogin != "" && sug.ExpertLogin != gh.PRAuthor {
				mr.RequestUsers = append(mr.RequestUsers, sug.ExpertLogin)
			}
		}
		out.Modules = append(out.Modules, mr)
	}
	return out, nil
}

// requestReviewers asks GitHub to add the resolved owner teams and expert as
// reviewers on the PR, turning an unmet review into actual review requests. It
// is best-effort: a failure (e.g. a team that does not exist) is logged, not
// fatal -- the gate already blocks the merge. Returns the handles it requested.
func (a *App) requestReviewers(ctx context.Context, outcome ReviewOutcome) []string {
	if !outcome.Evaluated || outcome.Satisfied {
		return nil
	}
	teamSet, userSet := map[string]bool{}, map[string]bool{}
	for _, m := range outcome.Modules {
		for _, t := range m.RequestTeams {
			if t != "" {
				teamSet[t] = true
			}
		}
		for _, u := range m.RequestUsers {
			if u != "" {
				userSet[u] = true
			}
		}
	}
	teams, users := sortedSet(teamSet), sortedSet(userSet)
	if len(teams) == 0 && len(users) == 0 {
		return nil
	}
	gh := github.FromEnv()
	if !gh.Enabled() || !gh.HasPR() {
		return nil
	}
	if err := github.NewClient(ctx, gh).RequestReviewers(ctx, users, teams); err != nil {
		a.Log.Warn().Err(err).Msg("could not request reviewers (best-effort)")
		return nil
	}
	var requested []string
	for _, t := range teams {
		requested = append(requested, "@"+t)
	}
	for _, u := range users {
		requested = append(requested, "@"+u)
	}
	return requested
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// printReviewOutcome renders the standalone `review` command's output.
func printReviewOutcome(w io.Writer, o ReviewOutcome) {
	fmt.Fprintln(w)
	for _, m := range o.Modules {
		status := "ok"
		if !m.OK {
			status = "NEEDS REVIEW"
		}
		fmt.Fprintf(w, "%-24s [%s] expert=%s\n", m.Module, status, orNone(m.Expert))
		for _, mm := range m.Missing {
			fmt.Fprintf(w, "    - %s\n", mm)
		}
	}
}

func (a *App) resolveExpert(ctx context.Context, client *github.Client, loaded *plan.Loaded, dir, author string, since time.Time, windowDays float64) string {
	if !loaded.Root.Experts.Enabled {
		return ""
	}
	contribs, err := client.ListContributors(ctx, []string{dir}, since)
	if err != nil {
		return ""
	}
	var cands []reviews.Candidate
	for login, c := range contribs {
		cands = append(cands, reviews.Candidate{
			Contributor: reviews.Contributor{Login: login, Commits: c.Commits, LastActiveUnix: c.LastUnix},
			IsBot:       c.IsBot,
			HasWrite:    client.HasWriteAccess(ctx, login),
		})
	}
	expert, _ := reviews.PickExpert(cands, author, time.Now().Unix(), windowDays)
	return expert
}

func (a *App) ownerLogins(ctx context.Context, client *github.Client, teams []string) map[string]bool {
	out := map[string]bool{}
	for _, t := range teams {
		members, err := client.ListTeamMembers(ctx, t)
		if err != nil {
			continue
		}
		for _, m := range members {
			out[m] = true
		}
	}
	return out
}

func (a *App) teamMembers(ctx context.Context, client *github.Client, teams map[string]int) map[string][]string {
	out := map[string][]string{}
	for t := range teams {
		if members, err := client.ListTeamMembers(ctx, t); err == nil {
			out[t] = members
		}
	}
	return out
}

func requirementOf(r config.ReviewRequire) reviews.Requirement {
	req := reviews.Requirement{Teams: r.Teams}
	if r.Owners != nil {
		req.Owners = *r.Owners
	}
	if r.Expert != nil {
		req.Expert = *r.Expert
	}
	return req
}

func sortedReviewRules(m map[string]config.ReviewRule) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// "default" first, then alphabetical.
	for i, k := range out {
		if k == "default" {
			out[0], out[i] = out[i], out[0]
			break
		}
	}
	return out
}

func orNone(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
