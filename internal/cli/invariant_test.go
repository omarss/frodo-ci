package cli

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestCLIOwnershipInvariant is HANDOFF.md's definition of done: after the CLI
// and declarative config produce everything (init, scaffold, config-driven
// sync-workflow), a clean `sync-workflow --check` passes — no supported scenario
// requires hand-editing the workflow, templates, or any generated file.
func TestCLIOwnershipInvariant(t *testing.T) {
	root := t.TempDir()
	// A mixed, real-shaped repo: pnpm workspace (engines/packageManager, a package
	// missing typecheck/test) + a Maven module with a Dockerfile + CODEOWNERS.
	mustWrite(t, filepath.Join(root, "package.json"),
		`{"name":"root","private":true,"engines":{"node":">=24"},"packageManager":"pnpm@10.5.0","workspaces":["apps/*","packages/*"]}`)
	mustWrite(t, filepath.Join(root, "pnpm-workspace.yaml"), "packages:\n  - apps/*\n  - packages/*\n")
	mustWrite(t, filepath.Join(root, "apps/web/package.json"),
		`{"name":"@acme/web","scripts":{"build":"tsc"},"dependencies":{"@acme/ui":"workspace:*"}}`)
	mustWrite(t, filepath.Join(root, "packages/ui/package.json"), `{"name":"@acme/ui","version":"1.0.0"}`)
	mustWrite(t, filepath.Join(root, "services/cards/pom.xml"),
		`<project><artifactId>cards</artifactId><properties><maven.compiler.release>21</maven.compiler.release></properties></project>`)
	mustWrite(t, filepath.Join(root, "services/cards/Dockerfile"), "FROM eclipse-temurin:21\n")
	mustWrite(t, filepath.Join(root, ".github/CODEOWNERS"), "* @acme/platform\n")

	app := &App{RepoRoot: root, Out: io.Discard, Err: io.Discard}
	if err := app.runInit(false, ""); err != nil {
		t.Fatalf("init: %v", err)
	}
	// Declare job env + a private registry in config — the declarative interface,
	// not a workflow hand-edit.
	appendTo(t, filepath.Join(root, ".github/frodo-ci.yml"),
		"\nworkflow:\n  env:\n    NODE_OPTIONS: \"--max-old-space-size=4096\"\n"+
			"registries:\n  - host: me-central2-docker.pkg.dev\n    auth: gcp-wif\n"+
			"    workload_identity_provider_var: GCP_WIF_PROVIDER\n    service_account_var: GCP_WIF_SERVICE_ACCOUNT\n")
	// Scaffold modules (types, edges, owner-derived reviews) from build metadata.
	if err := app.runScaffold(true, false, "platform"); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	// Regenerate the workflow from config + repo metadata + modules.
	if err := app.runSyncWorkflow(false, ""); err != nil {
		t.Fatalf("sync-workflow: %v", err)
	}

	// The invariant: a second --check is clean. Everything is CLI/config-owned, so
	// there is nothing left to hand-edit.
	if err := app.runSyncWorkflow(true, ""); err != nil {
		t.Errorf("sync-workflow --check must pass on a clean tree (zero manual edits): %v", err)
	}
}

func appendTo(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}
