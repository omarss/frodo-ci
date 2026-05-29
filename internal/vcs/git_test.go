package vcs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitOrSkip(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveThreeDot(t *testing.T) {
	gitOrSkip(t)
	root := t.TempDir()
	mustGit(t, root, "init", "-q")
	write(t, filepath.Join(root, "a.txt"), "one\n")
	mustGit(t, root, "add", ".")
	mustGit(t, root, "commit", "-qm", "c1")

	g := New(root)
	base, err := g.RevParse("HEAD")
	if err != nil {
		t.Fatal(err)
	}

	mustGit(t, root, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(root, "a.txt"), "two\n")
	write(t, filepath.Join(root, "sub/b.txt"), "new\n")
	mustGit(t, root, "add", ".")
	mustGit(t, root, "commit", "-qm", "c2")

	got, err := g.Resolve(Changes{Base: base, Head: "HEAD", ThreeDot: true})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"a.txt": true, "sub/b.txt": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("Resolve = %v, want a.txt + sub/b.txt", got)
	}
}

func TestWorkingChanges(t *testing.T) {
	gitOrSkip(t)
	root := t.TempDir()
	mustGit(t, root, "init", "-q")
	write(t, filepath.Join(root, "tracked.txt"), "v1\n")
	mustGit(t, root, "add", ".")
	mustGit(t, root, "commit", "-qm", "c1")

	// Modify tracked + add untracked, without committing.
	write(t, filepath.Join(root, "tracked.txt"), "v2\n")
	write(t, filepath.Join(root, "untracked.txt"), "new\n")

	g := New(root)
	got, err := g.WorkingChanges()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"tracked.txt": true, "untracked.txt": true}
	if len(got) != 2 || !want[got[0]] || !want[got[1]] {
		t.Errorf("WorkingChanges = %v, want tracked + untracked", got)
	}
	if !g.Available() {
		t.Error("expected Available() to be true in a git repo")
	}
}
