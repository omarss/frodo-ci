package cli

import (
	"io"
	"testing"
	"time"
)

func TestSuppressLifecycle(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	app := &App{RepoRoot: root, Out: io.Discard}

	// A past (or present) expiry is rejected at write time.
	if err := app.runSuppressAdd("CVE-1", "", "r", "o", "a", "2025-06-01", now); err == nil {
		t.Error("a past expiry must be rejected")
	}
	// A malformed date is rejected.
	if err := app.runSuppressAdd("CVE-1", "", "r", "o", "a", "not-a-date", now); err == nil {
		t.Error("a malformed expiry must be rejected")
	}

	// Two valid suppressions with different expiries.
	if err := app.runSuppressAdd("CVE-1", "svc/**", "false positive", "team", "approver", "2026-12-31", now); err != nil {
		t.Fatal(err)
	}
	if err := app.runSuppressAdd("CVE-2", "", "tracked", "team", "approver", "2026-02-01", now); err != nil {
		t.Fatal(err)
	}
	sup, err := app.loadSuppressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sup.Suppressions) != 2 {
		t.Fatalf("want 2 suppressions, got %d", len(sup.Suppressions))
	}

	// Prune as of mid-2026: CVE-2 (Feb) is expired, CVE-1 (Dec) is kept.
	if err := app.runSuppressPrune(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	after, err := app.loadSuppressions()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Suppressions) != 1 || after.Suppressions[0].ID != "CVE-1" {
		t.Errorf("prune should keep only the unexpired CVE-1, got %+v", after.Suppressions)
	}
	if after.Suppressions[0].Path != "svc/**" || after.Suppressions[0].Reason != "false positive" {
		t.Errorf("kept suppression lost fields on round-trip: %+v", after.Suppressions[0])
	}
}
