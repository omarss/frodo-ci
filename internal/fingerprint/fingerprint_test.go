package fingerprint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComputeDeterministic(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "alpha")
	mustWrite(t, filepath.Join(root, "b.txt"), "beta")

	in := StageInputs{
		RootConfig:              []byte("version: 1"),
		Toolchains:              []byte("version: 1"),
		SecurityBaselineVersion: 1,
		LintRulesVersion:        1,
		ModuleConfig:            []byte("name: cards"),
		Files:                   []string{"a.txt", "b.txt"},
		DependencyFingerprints:  []string{"dep1", "dep2"},
	}
	first, err := Compute(root, in)
	if err != nil {
		t.Fatal(err)
	}

	// Reordering files and dependencies must not change the fingerprint.
	in2 := in
	in2.Files = []string{"b.txt", "a.txt"}
	in2.DependencyFingerprints = []string{"dep2", "dep1", "dep2"}
	second, err := Compute(root, in2)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("fingerprint not order-stable:\n %s\n %s", first, second)
	}
}

func TestComputeSensitiveToContent(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	mustWrite(t, file, "alpha")
	base := StageInputs{ModuleConfig: []byte("name: cards"), Files: []string{"a.txt"}}

	before, _ := Compute(root, base)
	mustWrite(t, file, "alpha-changed")
	after, _ := Compute(root, base)
	if before == after {
		t.Error("fingerprint should change when a matched file's content changes")
	}
}

func TestComputeSensitiveToDeps(t *testing.T) {
	root := t.TempDir()
	a := StageInputs{ModuleConfig: []byte("m"), DependencyFingerprints: []string{"x"}}
	b := StageInputs{ModuleConfig: []byte("m"), DependencyFingerprints: []string{"y"}}
	fa, _ := Compute(root, a)
	fb, _ := Compute(root, b)
	if fa == fb {
		t.Error("fingerprint should change when a dependency fingerprint changes")
	}
}

func TestFileDigestAbsent(t *testing.T) {
	d, err := FileDigest(filepath.Join(t.TempDir(), "nope.txt"))
	if err != nil || d != "absent" {
		t.Errorf("absent file digest = %q, %v", d, err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
