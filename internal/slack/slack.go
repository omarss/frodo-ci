// Package slack sends deduplicated, actionable Slack notifications for CI/CD
// failures, security blockers, and framework failures.
package slack

import (
	"fmt"
	"strings"

	api "github.com/slack-go/slack"
)

// Trigger identifies a notifiable event class (matches the root config's
// notify_on entries).
type Trigger string

const (
	CIFailureOnMain        Trigger = "ci_failure_on_main"
	CDFailure              Trigger = "cd_failure"
	ProductionDeployFailed Trigger = "production_deploy_failure"
	SecurityBlocker        Trigger = "security_blocker"
	PerformanceExceeded    Trigger = "performance_budget_exceeded"
	FrameworkFailure       Trigger = "framework_failure"
)

// Event is a notification's content.
type Event struct {
	Trigger    Trigger
	Repository string
	PR         string
	Module     string
	Stage      string
	FailedStep string
	Owner      string
	Expert     string
	Commit     string
	WhyItRan   string
	NextAction string
}

// ShouldNotify reports whether a trigger is enabled in notify_on.
func ShouldNotify(t Trigger, notifyOn []string) bool {
	for _, n := range notifyOn {
		if Trigger(n) == t {
			return true
		}
	}
	return false
}

// DedupKey collapses repeated notifications for the same failure.
func (e Event) DedupKey() string {
	return strings.Join([]string{string(e.Trigger), e.Module, e.Stage, e.Commit}, "|")
}

// Format renders the human-readable message body.
func (e Event) Format() string {
	var b strings.Builder
	b.WriteString("Frodo CI failed\n\n")
	writeField(&b, "Repository", e.Repository)
	writeField(&b, "PR", e.PR)
	writeField(&b, "Module", e.Module)
	writeField(&b, "Stage", e.Stage)
	writeField(&b, "Failed step", e.FailedStep)
	writeField(&b, "Owner", e.Owner)
	writeField(&b, "Expert reviewer", e.Expert)
	writeField(&b, "Commit", e.Commit)
	if e.WhyItRan != "" {
		fmt.Fprintf(&b, "\nWhy it ran:\n%s\n", e.WhyItRan)
	}
	if e.NextAction != "" {
		fmt.Fprintf(&b, "\nNext action:\n%s\n", e.NextAction)
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeField(b *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(b, "%s: %s\n", label, value)
	}
}

// Notifier sends notifications to a webhook, deduplicating within its lifetime.
type Notifier struct {
	Webhook  string
	NotifyOn []string
	Channel  string
	sent     map[string]bool
	post     func(url string, msg *api.WebhookMessage) error // injectable for tests
}

// New returns a Notifier for the given webhook and notify_on triggers.
func New(webhook, channel string, notifyOn []string) *Notifier {
	return &Notifier{
		Webhook:  webhook,
		Channel:  channel,
		NotifyOn: notifyOn,
		sent:     map[string]bool{},
		post:     api.PostWebhook,
	}
}

// Notify sends an event if its trigger is enabled and it has not already been
// sent. It is a no-op when no webhook is configured.
func (n *Notifier) Notify(e Event) error {
	if n.Webhook == "" || !ShouldNotify(e.Trigger, n.NotifyOn) {
		return nil
	}
	key := e.DedupKey()
	if n.sent[key] {
		return nil
	}
	n.sent[key] = true
	return n.post(n.Webhook, &api.WebhookMessage{Channel: n.Channel, Text: e.Format()})
}
