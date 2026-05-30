package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"21", "25", true}, {"25", "21", false}, {"25", "25", false},
		{"1.22", "1.25", true}, {"1.25", "1.22", false}, {"", "22", true},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestReplaceSetupBlock(t *testing.T) {
	wf := strings.Join([]string{
		"    steps:",
		"      - name: Checkout",
		setupMarkerStart,
		"      - name: OLD",
		setupMarkerEnd,
		"      - name: Run",
	}, "\n")
	out, err := replaceSetupBlock(wf, "      - name: NEW")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "OLD") || !strings.Contains(out, "NEW") {
		t.Errorf("block not replaced:\n%s", out)
	}
	for _, keep := range []string{"name: Checkout", "name: Run", setupMarkerStart, setupMarkerEnd} {
		if !strings.Contains(out, keep) {
			t.Errorf("replace dropped %q:\n%s", keep, out)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSyncWorkflowFromSetup is the end-to-end: two modules declare setup:
// toolchains (java 25, go 1.25); sync-workflow must bump java past the workflow's
// stale 21, add a setup-go step, keep the markers and surrounding steps.
func TestSyncWorkflowFromSetup(t *testing.T) {
	root := t.TempDir()
	wf := strings.Join([]string{
		"name: ci", "jobs:", "  final:", "    steps:",
		"      - name: Checkout", "        uses: actions/checkout@x",
		setupMarkerStart,
		"      - name: Set up Java", "        uses: actions/setup-java@OLD",
		"        with:", "          java-version: \"21\"",
		setupMarkerEnd,
		"      - name: Run Frodo CI", "        run: frodo-ci run", "",
	}, "\n")
	mustWrite(t, filepath.Join(root, ".github/workflows/frodo-ci.yml"), wf)
	mustWrite(t, filepath.Join(root, ".github/frodo-ci.yml"), "version: 1\n")
	mustWrite(t, filepath.Join(root, "svc/.ci/module.yml"), "name: svc\nowners:\n  teams: [t]\nci:\n  build:\n    when: [src/**]\n")
	mustWrite(t, filepath.Join(root, "svc/.ci/build.yml"), "name: build\nsetup:\n  java: {version: 25}\nsteps:\n  - {name: b, run: echo}\n")
	mustWrite(t, filepath.Join(root, "tool/.ci/module.yml"), "name: tool\nowners:\n  teams: [t]\nci:\n  build:\n    when: [src/**]\n")
	mustWrite(t, filepath.Join(root, "tool/.ci/build.yml"), "name: build\nsetup:\n  go: {version: \"1.25\"}\nsteps:\n  - {name: b, run: echo}\n")
	mustWrite(t, filepath.Join(root, "svc/src/x.txt"), "x\n")
	mustWrite(t, filepath.Join(root, "tool/src/x.txt"), "x\n")

	app := &App{RepoRoot: root, Out: io.Discard}
	if err := app.runSyncWorkflow(false, ""); err != nil {
		t.Fatalf("sync-workflow: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/frodo-ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`java-version: "25"`, "Set up Go", `go-version: "1.25"`,
		setupMarkerStart, setupMarkerEnd, "name: Checkout", "name: Run Frodo CI",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("workflow missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `java-version: "21"`) {
		t.Errorf("stale java 21 should have been replaced:\n%s", got)
	}
}

func TestReplaceActionRef(t *testing.T) {
	wf := "      - name: Run Frodo CI\n        uses: omarss/frodo-ci@v1\n        with:\n          command: run\n"
	out := replaceActionRef(wf, "omarss/frodo-ci@v1.10.0")
	if !strings.Contains(out, "uses: omarss/frodo-ci@v1.10.0") {
		t.Errorf("ref not bumped:\n%s", out)
	}
	if strings.Contains(out, "frodo-ci@v1\n") {
		t.Error("old ref should be gone")
	}
	if replaceActionRef(wf, "") != wf {
		t.Error("empty ref must be a no-op")
	}
	// must not touch setup-* actions
	other := "        uses: actions/setup-node@abc # v6\n"
	if replaceActionRef(other, "omarss/frodo-ci@v2") != other {
		t.Error("non-frodo actions must be untouched")
	}
}
