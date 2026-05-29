package security

import (
	"testing"
	"time"

	"github.com/frodo-ci/frodo-ci/internal/config"
)

func types(scans []Scan) map[ScanType]bool {
	m := map[ScanType]bool{}
	for _, s := range scans {
		m[s.Type] = true
	}
	return m
}

func TestPlanByChangeType(t *testing.T) {
	if !types(Plan([]string{"services/cards/pom.xml"}, "pull_request", false))[DependencyCVE] {
		t.Error("dependency manifest should trigger a CVE scan")
	}
	src := types(Plan([]string{"services/cards/src/main/java/X.java"}, "pull_request", false))
	if !src[SAST] || !src[Secrets] {
		t.Error("source change should trigger SAST + secrets")
	}
	d := types(Plan([]string{"services/cards/Dockerfile"}, "pull_request", false))
	if !d[DockerfileLint] || !d[Container] {
		t.Error("Dockerfile change should trigger dockerfile + container scans")
	}
	if !types(Plan([]string{"infra/k8s/deploy.yaml"}, "pull_request", false))[IaC] {
		t.Error("infra yaml should trigger an IaC scan")
	}
	if !types(Plan([]string{".github/workflows/ci.yml"}, "pull_request", false))[Actions] {
		t.Error("workflow change should trigger an actions scan")
	}
}

func TestPlanFullOnMainAndSchedule(t *testing.T) {
	for _, p := range []struct {
		event string
		main  bool
	}{{"push", true}, {"schedule", false}} {
		scans := Plan([]string{"README.md"}, p.event, p.main)
		if len(scans) != 1 || scans[0].Type != Full {
			t.Errorf("event=%s main=%v should be a single full scan, got %v", p.event, p.main, scans)
		}
	}
}

func TestIsSuppressed(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := config.Date{Time: now.AddDate(0, 1, 0)}
	past := config.Date{Time: now.AddDate(0, -1, 0)}

	active := config.Suppression{ID: "CVE-1", Path: "services/cards/**", Expiry: future}
	expired := config.Suppression{ID: "CVE-2", Path: "services/cards/**", Expiry: past}
	noExpiry := config.Suppression{ID: "CVE-3", Path: "services/cards/**"}

	f := Finding{RuleID: "CVE-1", Path: "services/cards/pom.xml"}
	if !IsSuppressed(f, []config.Suppression{active}, now) {
		t.Error("active matching suppression should apply")
	}
	if IsSuppressed(Finding{RuleID: "CVE-2", Path: "services/cards/pom.xml"}, []config.Suppression{expired}, now) {
		t.Error("expired suppression must not apply")
	}
	if IsSuppressed(Finding{RuleID: "CVE-3", Path: "services/cards/pom.xml"}, []config.Suppression{noExpiry}, now) {
		t.Error("suppression without expiry must not apply")
	}
}

func TestIsBlocking(t *testing.T) {
	rs := &config.Rulesets{Blocking: []string{"confirmed-secret"}}
	if !IsBlocking("confirmed-secret", rs) || IsBlocking("noisy-sast", rs) {
		t.Error("blocking classification wrong")
	}
}
