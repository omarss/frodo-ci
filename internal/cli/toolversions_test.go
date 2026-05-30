package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectToolchains(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"),
		`{"engines":{"node":">=24"},"packageManager":"pnpm@10.34.1"}`)
	mustWrite(t, filepath.Join(root, "go.mod"), "module x\n\ngo 1.25\n")
	mustWrite(t, filepath.Join(root, "pom.xml"),
		"<project><properties><maven.compiler.release>21</maven.compiler.release></properties></project>")

	d := detectToolchains(root)
	if d.tools["node"] != "24" {
		t.Errorf("node = %q, want 24 (from engines.node >=24)", d.tools["node"])
	}
	if d.pnpm != "10.34.1" {
		t.Errorf("pnpm = %q, want 10.34.1 (from packageManager)", d.pnpm)
	}
	if d.tools["go"] != "1.25" {
		t.Errorf("go = %q, want 1.25 (from go.mod)", d.tools["go"])
	}
	if d.tools["java"] != "21" {
		t.Errorf("java = %q, want 21 (from maven.compiler.release)", d.tools["java"])
	}
}

func TestDetectNodeVersionFileWins(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "package.json"), `{"engines":{"node":">=18"}}`)
	mustWrite(t, filepath.Join(root, ".nvmrc"), "v20.11.1\n")
	if d := detectToolchains(root); d.tools["node"] != "20.11.1" {
		t.Errorf("node = %q, want 20.11.1 (.nvmrc wins over engines)", d.tools["node"])
	}
}

// TestSyncWorkflowDetectsVersions: repo metadata (engines.node 24, pnpm 10.5.0)
// overrides the module's declared node 22 in the regenerated setup block.
func TestSyncWorkflowDetectsVersions(t *testing.T) {
	root := t.TempDir()
	wf := strings.Join([]string{
		"jobs:", "  final:", "    steps:",
		setupMarkerStart,
		"      - name: Set up Node", "        with:", "          node-version: \"22\"",
		setupMarkerEnd,
	}, "\n")
	mustWrite(t, filepath.Join(root, ".github/workflows/frodo-ci.yml"), wf)
	mustWrite(t, filepath.Join(root, ".github/frodo-ci.yml"), "version: 1\n")
	mustWrite(t, filepath.Join(root, "package.json"),
		`{"engines":{"node":">=24"},"packageManager":"pnpm@10.5.0"}`)
	mustWrite(t, filepath.Join(root, "apps/web/.ci/module.yml"),
		"name: web\nowners:\n  teams: [t]\nci:\n  build:\n    when: [src/**]\n")
	mustWrite(t, filepath.Join(root, "apps/web/.ci/build.yml"),
		"name: build\nsetup:\n  node: {version: 22}\nsteps:\n  - {name: b, run: pnpm -s build}\n")
	mustWrite(t, filepath.Join(root, "apps/web/src/x.txt"), "x\n")

	app := &App{RepoRoot: root, Out: io.Discard}
	if err := app.runSyncWorkflow(false); err != nil {
		t.Fatalf("sync-workflow: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/frodo-ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{`node-version: "24"`, "corepack prepare pnpm@10.5.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `node-version: "22"`) {
		t.Errorf("module's node 22 should be overridden by engines.node 24:\n%s", got)
	}
}
