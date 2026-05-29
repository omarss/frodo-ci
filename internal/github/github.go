// Package github wraps the GitHub API for the parts Frodo CI needs: run
// context from the Actions environment, dynamic check-runs, and the pull-request
// reviews and permissions used by review/expert governance.
package github

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"

	gh "github.com/google/go-github/v75/github"
	"golang.org/x/oauth2"
)

// Context is the GitHub run context derived from the Actions environment.
type Context struct {
	Token    string
	Owner    string
	Repo     string
	Event    string
	SHA      string
	PRNumber int
	PRAuthor string
	BaseRef  string
	HeadRef  string
}

// FromEnv builds a Context from the standard GitHub Actions environment.
func FromEnv() Context {
	c := Context{
		Token:   firstEnv("GITHUB_TOKEN", "GH_TOKEN"),
		Event:   os.Getenv("GITHUB_EVENT_NAME"),
		SHA:     os.Getenv("GITHUB_SHA"),
		BaseRef: os.Getenv("GITHUB_BASE_REF"),
		HeadRef: os.Getenv("GITHUB_HEAD_REF"),
	}
	if owner, repo, ok := strings.Cut(os.Getenv("GITHUB_REPOSITORY"), "/"); ok {
		c.Owner, c.Repo = owner, repo
	}
	c.PRNumber, c.PRAuthor = parseEvent()
	return c
}

// Enabled reports whether enough context exists to call the GitHub API.
func (c Context) Enabled() bool { return c.Token != "" && c.Owner != "" && c.Repo != "" }

// HasPR reports whether a pull request is in context.
func (c Context) HasPR() bool { return c.PRNumber > 0 }

// Client is a thin GitHub API client.
type Client struct {
	api *gh.Client
	ctx Context
}

// NewClient builds an authenticated client for the given context.
func NewClient(ctx context.Context, c Context) *Client {
	httpClient := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: c.Token}))
	return &Client{api: gh.NewClient(httpClient), ctx: c}
}

// CheckRun is the minimal state of a check-run we create/update.
type CheckRun struct {
	ID         int64
	Name       string
	Status     string // queued | in_progress | completed
	Conclusion string // success | failure | cancelled | timed_out | skipped | neutral
	Title      string
	Summary    string
}

// CreateCheckRun creates a check-run on the head SHA and returns its ID.
func (c *Client) CreateCheckRun(ctx context.Context, cr CheckRun) (int64, error) {
	opts := gh.CreateCheckRunOptions{
		Name:    cr.Name,
		HeadSHA: c.ctx.SHA,
		Status:  ptr(cr.Status),
		Output:  &gh.CheckRunOutput{Title: ptr(cr.Title), Summary: ptr(cr.Summary)},
	}
	if cr.Conclusion != "" {
		opts.Conclusion = ptr(cr.Conclusion)
	}
	out, _, err := c.api.Checks.CreateCheckRun(ctx, c.ctx.Owner, c.ctx.Repo, opts)
	if err != nil {
		return 0, err
	}
	return out.GetID(), nil
}

// UpdateCheckRun updates an existing check-run.
func (c *Client) UpdateCheckRun(ctx context.Context, id int64, cr CheckRun) error {
	opts := gh.UpdateCheckRunOptions{
		Name:   cr.Name,
		Status: ptr(cr.Status),
		Output: &gh.CheckRunOutput{Title: ptr(cr.Title), Summary: ptr(cr.Summary)},
	}
	if cr.Conclusion != "" {
		opts.Conclusion = ptr(cr.Conclusion)
	}
	_, _, err := c.api.Checks.UpdateCheckRun(ctx, c.ctx.Owner, c.ctx.Repo, id, opts)
	return err
}

// Review is the latest-known state of one reviewer.
type Review struct {
	User        string
	State       string // APPROVED | CHANGES_REQUESTED | COMMENTED | DISMISSED
	IsBot       bool
	CommitID    string
	SubmittedAt int64
}

// ListReviews returns every review on the PR (callers reduce to latest-per-user).
func (c *Client) ListReviews(ctx context.Context) ([]Review, error) {
	var out []Review
	opt := &gh.ListOptions{PerPage: 100}
	for {
		reviews, resp, err := c.api.PullRequests.ListReviews(ctx, c.ctx.Owner, c.ctx.Repo, c.ctx.PRNumber, opt)
		if err != nil {
			return nil, err
		}
		for _, r := range reviews {
			out = append(out, Review{
				User:        r.GetUser().GetLogin(),
				State:       r.GetState(),
				IsBot:       isBot(r.GetUser().GetType(), r.GetUser().GetLogin()),
				CommitID:    r.GetCommitID(),
				SubmittedAt: r.GetSubmittedAt().Unix(),
			})
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

// Permission returns a user's permission level (admin, write, read, none).
func (c *Client) Permission(ctx context.Context, user string) (string, error) {
	lvl, _, err := c.api.Repositories.GetPermissionLevel(ctx, c.ctx.Owner, c.ctx.Repo, user)
	if err != nil {
		return "", err
	}
	return lvl.GetPermission(), nil
}

// HasWriteAccess reports whether a user can write (write or admin).
func (c *Client) HasWriteAccess(ctx context.Context, user string) bool {
	p, err := c.Permission(ctx, user)
	return err == nil && (p == "write" || p == "admin")
}

// Contrib aggregates a login's recent commit activity.
type Contrib struct {
	Login    string
	Commits  int
	LastUnix int64
	IsBot    bool
}

// ListContributors aggregates commit authors for the given repo-relative paths
// since the cutoff time, for expert-reviewer scoring.
func (c *Client) ListContributors(ctx context.Context, paths []string, since time.Time) (map[string]Contrib, error) {
	agg := map[string]*Contrib{}
	for _, p := range paths {
		opt := &gh.CommitsListOptions{Path: p, Since: since, ListOptions: gh.ListOptions{PerPage: 100}}
		for {
			commits, resp, err := c.api.Repositories.ListCommits(ctx, c.ctx.Owner, c.ctx.Repo, opt)
			if err != nil {
				return nil, err
			}
			for _, cm := range commits {
				login := cm.GetAuthor().GetLogin()
				if login == "" {
					continue
				}
				cur := agg[login]
				if cur == nil {
					cur = &Contrib{Login: login, IsBot: isBot(cm.GetAuthor().GetType(), login)}
					agg[login] = cur
				}
				cur.Commits++
				if when := cm.GetCommit().GetAuthor().GetDate(); !when.IsZero() && when.Unix() > cur.LastUnix {
					cur.LastUnix = when.Unix()
				}
			}
			if resp.NextPage == 0 {
				break
			}
			opt.Page = resp.NextPage
		}
	}
	out := make(map[string]Contrib, len(agg))
	for k, v := range agg {
		out[k] = *v
	}
	return out, nil
}

// ListTeamMembers returns the logins in an org team (by slug).
func (c *Client) ListTeamMembers(ctx context.Context, slug string) ([]string, error) {
	var out []string
	opt := &gh.TeamListTeamMembersOptions{ListOptions: gh.ListOptions{PerPage: 100}}
	for {
		members, resp, err := c.api.Teams.ListTeamMembersBySlug(ctx, c.ctx.Owner, slug, opt)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			out = append(out, m.GetLogin())
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

func isBot(userType, login string) bool {
	return userType == "Bot" || strings.HasSuffix(login, "[bot]")
}

func ptr[T any](v T) *T { return &v }

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

// parseEvent resolves the PR number and author from the event payload, falling
// back to the refs/pull/<n>/merge ref for the number.
func parseEvent() (number int, author string) {
	if path := os.Getenv("GITHUB_EVENT_PATH"); path != "" {
		if data, err := os.ReadFile(path); err == nil {
			var payload struct {
				PullRequest struct {
					Number int `json:"number"`
					User   struct {
						Login string `json:"login"`
					} `json:"user"`
				} `json:"pull_request"`
				Number int `json:"number"`
			}
			if json.Unmarshal(data, &payload) == nil {
				number = payload.PullRequest.Number
				if number == 0 {
					number = payload.Number
				}
				author = payload.PullRequest.User.Login
			}
		}
	}
	if number == 0 {
		ref := os.Getenv("GITHUB_REF") // refs/pull/<n>/merge
		if parts := strings.Split(ref, "/"); len(parts) >= 3 && parts[1] == "pull" {
			if n, err := strconv.Atoi(parts[2]); err == nil {
				number = n
			}
		}
	}
	return number, author
}
