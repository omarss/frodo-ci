package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/omarss/frodo-ci/internal/config"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sampleRepo builds a generic monorepo: a Maven reactor, a pnpm workspace, a Go
// module, CODEOWNERS, and one already-configured module.
func sampleRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// Maven reactor: root aggregator + a service (with Dockerfile) and a library.
	write(t, root, "pom.xml", `<project><groupId>com.acme</groupId><artifactId>root</artifactId><packaging>pom</packaging><modules><module>services/cards</module><module>libs/common</module></modules></project>`)
	write(t, root, "services/cards/pom.xml", `<project><groupId>com.acme</groupId><artifactId>cards</artifactId><dependencies><dependency><groupId>com.acme</groupId><artifactId>common</artifactId></dependency></dependencies></project>`)
	write(t, root, "services/cards/Dockerfile", "FROM eclipse-temurin:25\n")
	write(t, root, "libs/common/pom.xml", `<project><groupId>com.acme</groupId><artifactId>common</artifactId></project>`)

	// pnpm workspace: an app depending on a library.
	write(t, root, "pnpm-workspace.yaml", "packages:\n  - apps/*\n  - packages/*\n")
	write(t, root, "apps/web/package.json", `{"name":"@acme/web","scripts":{"dev":"vite"},"dependencies":{"@acme/ui":"workspace:*"}}`)
	write(t, root, "packages/ui/package.json", `{"name":"@acme/ui","version":"1.0.0"}`)

	// A standalone Go module.
	write(t, root, "tools/gen/go.mod", "module example.com/gen\n\ngo 1.25\n")

	// An already-configured module must be skipped.
	write(t, root, "libs/legacy/pom.xml", `<project><groupId>com.acme</groupId><artifactId>legacy</artifactId></project>`)
	write(t, root, "libs/legacy/.ci/module.yml", "name: legacy\ntype: java-library\n")

	// Ownership.
	write(t, root, ".github/CODEOWNERS", "* @acme/platform\n/services/ @acme/cards-team\n")
	return root
}

func find(res *Result, path string) *Module {
	for i := range res.Modules {
		if res.Modules[i].Path == path {
			return &res.Modules[i]
		}
	}
	return nil
}

func TestDetect(t *testing.T) {
	root := sampleRepo(t)
	cfg := &config.RootConfig{}
	cfg.ApplyDefaults()

	res, err := Detect(root, cfg)
	if err != nil {
		t.Fatal(err)
	}

	cards := find(res, "services/cards")
	if cards == nil || cards.Type != "spring-service" {
		t.Fatalf("cards = %+v, want spring-service", cards)
	}
	if len(cards.DependsOn) != 1 || cards.DependsOn[0].Module != "common" {
		t.Errorf("cards.depends_on = %+v, want [common]", cards.DependsOn)
	}
	if len(cards.Owners.Teams) != 1 || cards.Owners.Teams[0] != "cards-team" {
		t.Errorf("cards owner = %+v, want cards-team (last CODEOWNERS match)", cards.Owners)
	}

	if c := find(res, "libs/common"); c == nil || c.Type != "java-library" || len(c.Owners.Teams) == 0 || c.Owners.Teams[0] != "platform" {
		t.Errorf("common = %+v, want java-library owned by platform", c)
	}

	web := find(res, "apps/web")
	if web == nil || web.Type != "node-app" {
		t.Fatalf("web = %+v, want node-app", web)
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0].Module != "ui" {
		t.Errorf("web.depends_on = %+v, want [ui]", web.DependsOn)
	}
	if ui := find(res, "packages/ui"); ui == nil || ui.Type != "node-library" {
		t.Errorf("ui = %+v, want node-library", ui)
	}
	if gen := find(res, "tools/gen"); gen == nil || gen.Type != "go-module" {
		t.Errorf("gen = %+v, want go-module", gen)
	}

	// The already-configured module is skipped, not proposed.
	if find(res, "libs/legacy") != nil {
		t.Error("libs/legacy already has a module.yml and must not be proposed")
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "libs/legacy" {
		t.Errorf("skipped = %v, want [libs/legacy]", res.Skipped)
	}
}

// TestDetectNodeNestedPackages covers the JS-graph parity fix: every package.json
// is walked (like the Maven pom walk), so a nested client that no workspace glob
// lists is still detected and an app's dependency on it resolves to an edge --
// even when referenced by a published-style version rather than workspace:. The
// npm/yarn `workspaces` root is not itself a module.
func TestDetectNodeNestedPackages(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", `{"name":"monorepo","private":true,"workspaces":["apps/*","clients/*"]}`)
	write(t, root, "apps/api/package.json", `{"name":"@acme/api","scripts":{"start":"node ."},"dependencies":{"@acme/api-client":"1.4.0"}}`)
	write(t, root, "clients/api-client/package.json", `{"name":"@acme/api-client","version":"1.4.0"}`)

	cfg := &config.RootConfig{}
	cfg.ApplyDefaults()
	res, err := Detect(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if find(res, ".") != nil || find(res, "") != nil {
		t.Error("the workspaces root must not be proposed as a module")
	}
	api := find(res, "apps/api")
	if api == nil {
		t.Fatalf("apps/api not detected: %+v", res.Modules)
	}
	if len(api.DependsOn) != 1 || api.DependsOn[0].Module != "api-client" {
		t.Errorf("api.depends_on = %+v, want [api-client]", api.DependsOn)
	}
	if c := find(res, "clients/api-client"); c == nil || c.Type != "node-library" {
		t.Errorf("client = %+v, want node-library (nested, no workspace glob)", c)
	}
}

func TestRenderIsValid(t *testing.T) {
	data, err := Render(Module{
		Name: "cards", Type: "spring-service",
		Owners:    Owners{Teams: []string{"cards-team"}},
		DependsOn: []Dependency{{Module: "common", Affects: []string{"test", "build"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	m, err := config.ParseModule(data)
	if err != nil {
		t.Fatalf("rendered module.yml does not parse: %v\n%s", err, data)
	}
	if m.Name != "cards" || m.Type != "spring-service" || m.Use.Profile != "spring-service" {
		t.Errorf("parsed = %+v", m)
	}
	if len(m.DependsOn) != 1 || m.DependsOn[0].Module != "common" {
		t.Errorf("depends_on = %+v", m.DependsOn)
	}
}
