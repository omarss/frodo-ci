package reviews

import (
	"fmt"
	"sort"
)

// Requirement is the set of approvals a rule demands.
type Requirement struct {
	Owners int
	Expert int
	Teams  map[string]int
}

// ReviewState is the latest-known review from one user.
type ReviewState struct {
	User     string
	Approved bool
	IsBot    bool
	Stale    bool // submitted before the latest commit
}

// Inputs are the facts needed to evaluate a requirement.
type Inputs struct {
	Author                   string
	Reviews                  []ReviewState
	Owners                   map[string]bool     // login -> is an owner
	Expert                   string              // resolved expert login ("" if none)
	TeamMembers              map[string][]string // team -> member logins
	IgnoreAuthor             bool
	IgnoreBots               bool
	RequireAfterLatestCommit bool
}

// Evaluate reports whether the requirement is satisfied and, if not, the list of
// human-friendly missing approvals. Approvals from the author (when ignored),
// bots (when ignored), and stale reviews (when require-after-latest-commit) do
// not count.
func Evaluate(req Requirement, in Inputs) (bool, []string) {
	approvers := approverSet(in)

	var missing []string

	if req.Owners > 0 {
		count := 0
		for u := range approvers {
			if in.Owners[u] {
				count++
			}
		}
		if count < req.Owners {
			missing = append(missing, fmt.Sprintf("need %d owner approval(s), have %d", req.Owners, count))
		}
	}

	if req.Expert > 0 {
		if in.Expert == "" {
			missing = append(missing, "need expert approval, but no expert could be resolved")
		} else if !approvers[in.Expert] {
			missing = append(missing, fmt.Sprintf("need approval from expert @%s", in.Expert))
		}
	}

	for _, team := range sortedKeys(req.Teams) {
		need := req.Teams[team]
		count := 0
		for _, member := range in.TeamMembers[team] {
			if approvers[member] {
				count++
			}
		}
		if count < need {
			missing = append(missing, fmt.Sprintf("need %d approval(s) from @%s, have %d", need, team, count))
		}
	}

	return len(missing) == 0, missing
}

// approverSet is the set of logins whose approval counts, applying the same
// author/bot/staleness filters as Evaluate.
func approverSet(in Inputs) map[string]bool {
	approvers := map[string]bool{}
	for _, r := range in.Reviews {
		if !r.Approved {
			continue
		}
		if in.IgnoreAuthor && r.User == in.Author {
			continue
		}
		if in.IgnoreBots && r.IsBot {
			continue
		}
		if in.RequireAfterLatestCommit && r.Stale {
			continue
		}
		approvers[r.User] = true
	}
	return approvers
}

// Suggestion identifies which parts of a requirement are still unmet, so callers
// can request exactly the right reviewers and not re-ping satisfied ones.
type Suggestion struct {
	OwnersShort  bool     // owner approvals are still needed
	ExpertNeeded bool     // expert approval is required and not yet given
	ExpertLogin  string   // the resolved expert ("" if none could be resolved)
	TeamsShort   []string // team slugs still short of their required approvals
}

// Suggest reports the unmet parts of a requirement, under the same rules as
// Evaluate, so callers know whom to request as reviewers.
func Suggest(req Requirement, in Inputs) Suggestion {
	approvers := approverSet(in)
	var s Suggestion
	if req.Owners > 0 {
		count := 0
		for u := range approvers {
			if in.Owners[u] {
				count++
			}
		}
		s.OwnersShort = count < req.Owners
	}
	if req.Expert > 0 {
		s.ExpertLogin = in.Expert
		s.ExpertNeeded = in.Expert == "" || !approvers[in.Expert]
	}
	for _, team := range sortedKeys(req.Teams) {
		count := 0
		for _, m := range in.TeamMembers[team] {
			if approvers[m] {
				count++
			}
		}
		if count < req.Teams[team] {
			s.TeamsShort = append(s.TeamsShort, team)
		}
	}
	return s
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// LatestPerUser reduces a review history to the latest review per user, marking
// reviews submitted before headCommit as stale when headCommit is known.
func LatestPerUser(history []ReviewWithTime, headCommit string, commitOrder map[string]int) []ReviewState {
	latest := map[string]ReviewWithTime{}
	for _, r := range history {
		if prev, ok := latest[r.User]; !ok || r.SubmittedAt >= prev.SubmittedAt {
			latest[r.User] = r
		}
	}
	out := make([]ReviewState, 0, len(latest))
	for _, r := range latest {
		stale := false
		if headCommit != "" && r.CommitID != "" && r.CommitID != headCommit {
			// If we know commit ordering, a review on an earlier commit is stale.
			if commitOrder != nil {
				stale = commitOrder[r.CommitID] < commitOrder[headCommit]
			} else {
				stale = true
			}
		}
		out = append(out, ReviewState{
			User:     r.User,
			Approved: r.State == "APPROVED",
			IsBot:    r.IsBot,
			Stale:    stale,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].User < out[j].User })
	return out
}

// ReviewWithTime is a raw review with timing/commit info, before reduction.
type ReviewWithTime struct {
	User        string
	State       string
	IsBot       bool
	CommitID    string
	SubmittedAt int64
}
