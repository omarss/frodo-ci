package reviews

import "testing"

const day = int64(86400)

func TestScoreCommitsBeatRawLines(t *testing.T) {
	now := int64(1_000_000_000)
	// Many lines, few commits, long ago.
	bulk := Contributor{Login: "bulk", Commits: 1, ChangedLines: 100000, LastActiveUnix: now - 25*day}
	// Fewer lines but more commits and recent.
	steady := Contributor{Login: "steady", Commits: 8, ChangedLines: 200, LastActiveUnix: now - 1*day}
	if Score(steady, now, 30) <= Score(bulk, now, 30) {
		t.Error("raw line count must not dominate commits + recency")
	}
}

func TestPickExpertEligibility(t *testing.T) {
	now := int64(1_000_000_000)
	cands := []Candidate{
		{Contributor{Login: "author", Commits: 99, LastActiveUnix: now}, false, true},    // excluded: author
		{Contributor{Login: "bot", Commits: 50, LastActiveUnix: now}, true, true},        // excluded: bot
		{Contributor{Login: "readonly", Commits: 40, LastActiveUnix: now}, false, false}, // excluded: no write
		{Contributor{Login: "ahmed", Commits: 10, LastActiveUnix: now - 2*day}, false, true},
		{Contributor{Login: "sara", Commits: 5, LastActiveUnix: now - 1*day}, false, true},
	}
	expert, ok := PickExpert(cands, "author", now, 30)
	if !ok || expert != "ahmed" {
		t.Errorf("expert = %q (ok=%v), want ahmed", expert, ok)
	}
}

func TestPickExpertNoneEligible(t *testing.T) {
	now := int64(1_000_000_000)
	cands := []Candidate{{Contributor{Login: "author", Commits: 9, LastActiveUnix: now}, false, true}}
	if _, ok := PickExpert(cands, "author", now, 30); ok {
		t.Error("expected no eligible expert when only the author qualifies")
	}
}

func TestEvaluateDefaultRule(t *testing.T) {
	req := Requirement{Owners: 1, Expert: 1}
	in := Inputs{
		Author:       "alice",
		Expert:       "ahmed",
		Owners:       map[string]bool{"owner1": true},
		IgnoreAuthor: true,
		IgnoreBots:   true,
		Reviews: []ReviewState{
			{User: "alice", Approved: true},  // author, ignored
			{User: "owner1", Approved: true}, // satisfies owners
			{User: "ahmed", Approved: true},  // satisfies expert
		},
	}
	ok, missing := Evaluate(req, in)
	if !ok {
		t.Errorf("expected satisfied, missing: %v", missing)
	}
}

func TestEvaluateMissingExpertAndTeam(t *testing.T) {
	req := Requirement{Owners: 1, Expert: 1, Teams: map[string]int{"security-team": 1}}
	in := Inputs{
		Author:      "alice",
		Expert:      "ahmed",
		Owners:      map[string]bool{"owner1": true},
		TeamMembers: map[string][]string{"security-team": {"sec1", "sec2"}},
		IgnoreBots:  true,
		Reviews: []ReviewState{
			{User: "owner1", Approved: true},
			{User: "ahmed", Approved: false}, // expert did not approve
		},
	}
	ok, missing := Evaluate(req, in)
	if ok {
		t.Fatal("expected unsatisfied")
	}
	if len(missing) != 2 {
		t.Errorf("expected 2 missing (expert + team), got %v", missing)
	}
}

func TestSuggestUnmetParts(t *testing.T) {
	req := Requirement{Owners: 1, Expert: 1, Teams: map[string]int{"security-team": 1}}
	in := Inputs{
		Author:      "alice",
		Expert:      "ahmed",
		Owners:      map[string]bool{"owner1": true},
		TeamMembers: map[string][]string{"security-team": {"sec1", "sec2"}},
		Reviews: []ReviewState{
			{User: "owner1", Approved: true}, // owner requirement satisfied
			{User: "ahmed", Approved: false}, // expert did not approve
		},
	}
	s := Suggest(req, in)
	if s.OwnersShort {
		t.Error("owner requirement is satisfied; OwnersShort should be false")
	}
	if !s.ExpertNeeded || s.ExpertLogin != "ahmed" {
		t.Errorf("expert still needed; got %+v", s)
	}
	if len(s.TeamsShort) != 1 || s.TeamsShort[0] != "security-team" {
		t.Errorf("security-team should be short; got %v", s.TeamsShort)
	}
}

func TestEvaluateStaleApprovalIgnored(t *testing.T) {
	req := Requirement{Owners: 1}
	in := Inputs{
		Owners:                   map[string]bool{"owner1": true},
		RequireAfterLatestCommit: true,
		Reviews:                  []ReviewState{{User: "owner1", Approved: true, Stale: true}},
	}
	if ok, _ := Evaluate(req, in); ok {
		t.Error("a stale approval should not count when require-after-latest-commit is set")
	}
}

func TestLatestPerUser(t *testing.T) {
	history := []ReviewWithTime{
		{User: "a", State: "COMMENTED", SubmittedAt: 1, CommitID: "c1"},
		{User: "a", State: "APPROVED", SubmittedAt: 2, CommitID: "c2"},
		{User: "b", State: "APPROVED", SubmittedAt: 1, CommitID: "c1"},
	}
	order := map[string]int{"c1": 1, "c2": 2}
	states := LatestPerUser(history, "c2", order)
	byUser := map[string]ReviewState{}
	for _, s := range states {
		byUser[s.User] = s
	}
	if !byUser["a"].Approved || byUser["a"].Stale {
		t.Errorf("a should be a fresh approval: %+v", byUser["a"])
	}
	if !byUser["b"].Stale {
		t.Errorf("b approved on an older commit and should be stale: %+v", byUser["b"])
	}
}
