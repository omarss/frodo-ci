// Package runner executes a calculated plan: it runs each required, uncached
// stage's steps with bounded parallelism and strict timeouts, entirely inside
// the process. There is no always(): cleanup, finalization, and failure
// reporting happen in normal control flow with context cancellation.
package runner

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// StepResult is the outcome of running one step.
type StepResult struct {
	Name     string        `json:"name"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Err      string        `json:"error,omitempty"`
	TimedOut bool          `json:"timed_out,omitempty"`
}

// runStep runs a single step's command under bash in the given working
// directory (repo-relative) with the provided environment. The command is
// bound to ctx so per-stage and full-run timeouts cancel it.
func runStep(ctx context.Context, repoRoot, workdir, run string, env []string, log zerolog.Logger) StepResult {
	start := time.Now()
	dir := repoRoot
	if workdir != "" {
		dir = filepath.Join(repoRoot, filepath.FromSlash(workdir))
	}
	cmd := exec.CommandContext(ctx, "bash", "-c", run)
	cmd.Dir = dir
	cmd.Env = env

	out, err := cmd.CombinedOutput()
	res := StepResult{Duration: time.Since(start)}
	if trimmed := strings.TrimRight(string(out), "\n"); trimmed != "" {
		log.Info().Str("workdir", workdir).Msg(trimmed)
	}
	if err == nil {
		return res
	}
	res.Err = err.Error()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
	} else {
		res.ExitCode = -1
	}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
	}
	return res
}
