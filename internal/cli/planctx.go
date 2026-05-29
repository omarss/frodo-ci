package cli

import (
	"os"
	"strings"

	"github.com/frodo-ci/frodo-ci/internal/cache"
	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/plan"
)

// planContext derives the plan context from CLI flags and the GitHub Actions
// environment, falling back to local working-tree mode.
func (a *App) planContext() plan.Context {
	ctx := plan.Context{
		Base:        a.Base,
		Head:        a.Head,
		Environment: a.Environment,
		Event:       envOr("GITHUB_EVENT_NAME", "local"),
	}

	ref := os.Getenv("GITHUB_REF")
	if ctx.Event == "push" && (ref == "refs/heads/main" || ref == "refs/heads/master") {
		ctx.OnDefaultBranch = true
	}

	// For pull requests, diff against the merge-base of the base branch.
	if ctx.Base == "" {
		if base := os.Getenv("GITHUB_BASE_REF"); base != "" {
			ctx.Base = "origin/" + base
			ctx.ThreeDot = true
		}
	}
	return ctx
}

// openCache opens the fingerprint cache, enabled only when the root config opts
// into unchanged-skipping with exact matching.
func openCache(root *config.RootConfig) (cache.Cache, error) {
	enabled := root.Minutes.SkipUnchanged && root.Minutes.ExactFingerprintMatchOnly
	return cache.Open(enabled, "")
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
