// Package reviews implements review governance: expert-reviewer resolution and
// evaluation of owner/expert/team approval requirements. The scoring and
// evaluation logic is pure and unit-tested; data gathering lives in the resolver.
package reviews

import "math"

// Contributor is a candidate's recent activity on the affected module files.
type Contributor struct {
	Login          string
	Commits        int
	ChangedLines   int
	LastActiveUnix int64
}

// Candidate is a contributor plus eligibility facts.
type Candidate struct {
	Contributor
	IsBot    bool
	HasWrite bool
}

// lineCap bounds the influence of raw changed lines so it never dominates.
const lineCap = 500.0

// Score balances recent commits, bounded changed lines, and a recency bonus.
// Raw line count alone must not decide the expert, so it is capped and weighted
// below commit count and recency.
func Score(c Contributor, nowUnix int64, windowDays float64) float64 {
	commits := float64(c.Commits)
	bounded := math.Min(float64(c.ChangedLines), lineCap) / lineCap // 0..1
	recency := 0.0
	if windowDays > 0 {
		ageDays := float64(nowUnix-c.LastActiveUnix) / 86400.0
		recency = 1 - ageDays/windowDays
		if recency < 0 {
			recency = 0
		}
	}
	return commits*1.0 + bounded*0.5 + recency*2.0
}

// PickExpert returns the highest-scoring eligible contributor: not the PR
// author, not a bot, with write access, and mapped to a login. Ties break on
// login for determinism.
func PickExpert(cands []Candidate, author string, nowUnix int64, windowDays float64) (string, bool) {
	best := ""
	bestScore := math.Inf(-1)
	for _, c := range cands {
		if c.Login == "" || c.Login == author || c.IsBot || !c.HasWrite {
			continue
		}
		s := Score(c.Contributor, nowUnix, windowDays)
		if s > bestScore || (s == bestScore && (best == "" || c.Login < best)) {
			best, bestScore = c.Login, s
		}
	}
	return best, best != ""
}
