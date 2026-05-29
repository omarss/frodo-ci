package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/omarss/frodo-ci/internal/configlint"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/omarss/frodo-ci/internal/templates"
	"github.com/omarss/frodo-ci/internal/vcs"
	"github.com/omarss/frodo-ci/internal/version"
)

type healthCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
}

func (a *App) runDoctor() error {
	var checks []healthCheck
	add := func(name, status, detail string) {
		checks = append(checks, healthCheck{name, status, detail})
	}

	add("version", "ok", version.Info())

	if vcs.New(a.RepoRoot).Available() {
		add("git", "ok", "git repository detected")
	} else {
		add("git", "warn", "not a git repository; change detection falls back to working tree")
	}

	if loaded, err := plan.Load(a.RepoRoot, time.Now()); err != nil {
		add("config", "fail", err.Error())
	} else {
		errs := 0
		for _, p := range loaded.Problems {
			if p.Severity == configlint.Error {
				errs++
			}
		}
		if errs > 0 {
			add("config", "fail", fmt.Sprintf("%d error(s); run `frodo-ci validate-config` and `frodo-ci lint-config`", errs))
		} else {
			add("config", "ok", fmt.Sprintf("%d module(s) discovered, no errors", len(loaded.Modules)))
		}
	}

	add("templates", "ok", fmt.Sprintf("%d built-in templates available", len(templates.DefaultNames())))

	if os.Getenv("GITHUB_TOKEN") != "" || os.Getenv("GH_TOKEN") != "" {
		add("github token", "ok", "present")
	} else {
		add("github token", "warn", "GITHUB_TOKEN not set; check-runs, reviews, and expert resolution are disabled")
	}

	if os.Getenv("SLACK_WEBHOOK_URL") != "" {
		add("slack", "ok", "webhook configured")
	} else {
		add("slack", "warn", "SLACK_WEBHOOK_URL not set; notifications are disabled")
	}

	if a.JSON {
		if err := writeJSON(a.Out, checks); err != nil {
			return err
		}
	} else {
		ok, warn, fail := 0, 0, 0
		for _, c := range checks {
			fmt.Fprintf(a.Out, "  %-14s [%s] %s\n", c.Name, c.Status, c.Detail)
			switch c.Status {
			case "ok":
				ok++
			case "warn":
				warn++
			default:
				fail++
			}
		}
		fmt.Fprintf(a.Out, "\n%d ok, %d warning(s), %d failure(s)\n", ok, warn, fail)
	}
	for _, c := range checks {
		if c.Status == "fail" {
			return ErrExitQuiet
		}
	}
	return nil
}
