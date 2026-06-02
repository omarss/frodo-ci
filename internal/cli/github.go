package cli

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
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
	// Unsatisfiable is true when a required team is empty/unknown, so the gate can
	// never go green until the config is fixed (distinct from "waiting for review").
	Unsatisfiable bool
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
		owners, badOwnerTeams := a.resolveTeams(ctx, client, m.Config.Owners.Teams)
		teamMembers := a.teamMembers(ctx, client, rule.Require.Teams)
		in := reviews.Inputs{
			Author:                   gh.PRAuthor,
			Reviews:                  states,
			Owners:                   owners,
			Expert:                   expert,
			TeamMembers:              teamMembers,
			IgnoreAuthor:             loaded.Root.Reviews.IgnoreAuthorApproval,
			IgnoreBots:               loaded.Root.Reviews.IgnoreBotApproval,
			RequireAfterLatestCommit: loaded.Root.Reviews.RequireApprovalAfterLatestCommit,
		}
		req := requirementOf(rule.Require)
		satisfied, missing := reviews.Evaluate(req, in)
		mr := ModuleReview{Module: m.Name, OK: satisfied, Expert: expert, Missing: missing}
		if !satisfied {
			out.Satisfied = false
			sug := reviews.Suggest(req, in)
			// Distinguish "waiting for a human" from "can never be satisfied": when
			// a required team is empty/unknown, say so instead of "needs approval".
			emptyReq := emptyTeams(req.Teams, teamMembers)
			if diag := reviewDiagnostics(req, len(owners) > 0, badOwnerTeams, emptyReq); len(diag) > 0 {
				mr.Missing = append(mr.Missing, diag...)
				mr.Unsatisfiable = true
			}
			// Request only teams that actually resolve (an unknown team 422s).
			if sug.OwnersShort {
				mr.RequestTeams = append(mr.RequestTeams, resolvable(m.Config.Owners.Teams, badOwnerTeams)...)
			}
			for _, t := range sug.TeamsShort {
				if !inList(emptyReq, t) {
					mr.RequestTeams = append(mr.RequestTeams, t)
				}
			}
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

// resolveTeams returns the union of the teams' members and the teams GitHub can't
// resolve to any member (the team is unknown, empty, or inaccessible) -- so a
// requirement naming only those can never be satisfied.
func (a *App) resolveTeams(ctx context.Context, client *github.Client, teams []string) (map[string]bool, []string) {
	members := map[string]bool{}
	var unresolvable []string
	for _, t := range teams {
		ms, err := client.ListTeamMembers(ctx, t)
		if err != nil || len(ms) == 0 {
			unresolvable = append(unresolvable, t)
			continue
		}
		for _, m := range ms {
			members[m] = true
		}
	}
	return members, unresolvable
}

// emptyTeams returns the required teams (count > 0) that resolved to no members.
func emptyTeams(req map[string]int, members map[string][]string) []string {
	var out []string
	for t, n := range req {
		if n > 0 && len(members[t]) == 0 {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// reviewDiagnostics produces the distinct "unsatisfiable" messages when a
// required role's teams are empty/unknown, so a misconfiguration reads
// differently from a pending human review.
func reviewDiagnostics(req reviews.Requirement, ownersResolved bool, badOwnerTeams, emptyReqTeams []string) []string {
	var out []string
	if req.Owners > 0 && !ownersResolved && len(badOwnerTeams) > 0 {
		out = append(out, fmt.Sprintf(
			"owner approval required, but owner team(s) %s are empty or unknown — unsatisfiable; fix owners.teams or grant the team repo access",
			joinAt(badOwnerTeams)))
	}
	for _, t := range emptyReqTeams {
		out = append(out, fmt.Sprintf("team @%s is empty or unknown — its required approval is unsatisfiable", t))
	}
	return out
}

func resolvable(all, bad []string) []string {
	var out []string
	for _, t := range all {
		if !inList(bad, t) {
			out = append(out, t)
		}
	}
	return out
}

func inList(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func joinAt(teams []string) string {
	q := make([]string, len(teams))
	for i, t := range teams {
		q[i] = "@" + t
	}
	return strings.Join(q, ", ")
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
