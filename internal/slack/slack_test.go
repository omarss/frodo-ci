package slack

import (
	"strings"
	"testing"

	api "github.com/slack-go/slack"
)

func sampleEvent() Event {
	return Event{
		Trigger:    CIFailureOnMain,
		Repository: "vrtx-mono",
		PR:         "#1234",
		Module:     "cards",
		Stage:      "test",
		FailedStep: "Run integration tests",
		Owner:      "cards-team",
		Expert:     "ahmed",
		Commit:     "abc123",
		WhyItRan:   "cards.test matched services/cards/src/main/**",
		NextAction: "Fix cards test or ask cards-team to review the failure.",
	}
}

func TestFormat(t *testing.T) {
	msg := sampleEvent().Format()
	for _, want := range []string{
		"Frodo CI failed", "Repository: vrtx-mono", "Module: cards", "Stage: test",
		"Failed step: Run integration tests", "Expert reviewer: ahmed", "Why it ran:", "Next action:",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestShouldNotify(t *testing.T) {
	on := []string{"ci_failure_on_main", "security_blocker"}
	if !ShouldNotify(CIFailureOnMain, on) {
		t.Error("ci_failure_on_main should be enabled")
	}
	if ShouldNotify(CDFailure, on) {
		t.Error("cd_failure should be disabled")
	}
}

func TestNotifierDedup(t *testing.T) {
	calls := 0
	n := New("https://hook", "#ci-alerts", []string{"ci_failure_on_main"})
	n.post = func(string, *api.WebhookMessage) error { calls++; return nil }

	e := sampleEvent()
	_ = n.Notify(e)
	_ = n.Notify(e) // duplicate
	if calls != 1 {
		t.Errorf("expected 1 send after dedup, got %d", calls)
	}

	// A disabled trigger is not sent.
	_ = n.Notify(Event{Trigger: CDFailure, Module: "x", Commit: "z"})
	if calls != 1 {
		t.Errorf("disabled trigger should not send, calls=%d", calls)
	}
}

func TestNoWebhookIsNoop(t *testing.T) {
	n := New("", "", []string{"ci_failure_on_main"})
	if err := n.Notify(sampleEvent()); err != nil {
		t.Errorf("no-webhook notify should be a no-op, got %v", err)
	}
}
