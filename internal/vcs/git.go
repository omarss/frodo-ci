// Package vcs wraps the git CLI to detect changed files for the planner. It
// shells out to git so it works against any checkout without extra setup.
package vcs

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// Git runs git commands within a repository root.
type Git struct {
	root string
}

// New returns a Git bound to the given repository root.
func New(root string) *Git { return &Git{root: root} }

// Available reports whether root is inside a git work tree.
func (g *Git) Available() bool {
	_, err := g.run("rev-parse", "--is-inside-work-tree")
	return err == nil
}

func (g *Git) run(args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", g.root}, args...)...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// DiffNames returns repo-relative paths changed for a range expression such as
// "base...head", "before..after", or "HEAD".
func (g *Git) DiffNames(rangeExpr string) ([]string, error) {
	out, err := g.run("diff", "--name-only", "--no-renames", rangeExpr)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// Untracked returns untracked, non-ignored files.
func (g *Git) Untracked() ([]string, error) {
	out, err := g.run("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// ListFiles returns every tracked and untracked (non-ignored) file in the
// working tree, repo-relative and sorted. Used to enumerate a stage's input
// surface for fingerprinting.
func (g *Git) ListFiles() ([]string, error) {
	tracked, err := g.run("ls-files")
	if err != nil {
		return nil, err
	}
	untracked, err := g.Untracked()
	if err != nil {
		return nil, err
	}
	return dedupeSort(append(splitLines(tracked), untracked...)), nil
}

// MergeBase returns the best common ancestor of two refs.
func (g *Git) MergeBase(a, b string) (string, error) {
	out, err := g.run("merge-base", a, b)
	return strings.TrimSpace(out), err
}

// ShowFile returns the contents of a repo-relative file at a given ref, e.g.
// the base-branch revision of the root config for anti-weakening comparison.
func (g *Git) ShowFile(ref, path string) ([]byte, error) {
	out, err := g.run("show", ref+":"+path)
	if err != nil {
		return nil, err
	}
	return []byte(out), nil
}

// RevParse resolves a ref to a commit SHA.
func (g *Git) RevParse(ref string) (string, error) {
	out, err := g.run("rev-parse", ref)
	return strings.TrimSpace(out), err
}

// WorkingChanges returns files changed in the working tree versus HEAD,
// including staged, unstaged, and untracked files (local `frodo-ci plan`).
func (g *Git) WorkingChanges() ([]string, error) {
	tracked, err := g.DiffNames("HEAD")
	if err != nil {
		return nil, err
	}
	untracked, err := g.Untracked()
	if err != nil {
		return nil, err
	}
	return dedupeSort(append(tracked, untracked...)), nil
}

// Changes describes how to compute the changed-file set.
type Changes struct {
	Base     string // base ref/sha; empty means working-tree mode
	Head     string // head ref/sha; empty defaults to HEAD
	ThreeDot bool   // use base...head (changes since merge-base) vs base..head
}

// Resolve computes the changed files for the given specification.
func (g *Git) Resolve(c Changes) ([]string, error) {
	if c.Base == "" {
		return g.WorkingChanges()
	}
	head := c.Head
	if head == "" {
		head = "HEAD"
	}
	sep := ".."
	if c.ThreeDot {
		sep = "..."
	}
	names, err := g.DiffNames(c.Base + sep + head)
	if err != nil {
		return nil, err
	}
	return dedupeSort(names), nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func dedupeSort(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
