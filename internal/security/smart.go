// Package security implements smart security scanning: it maps change types to
// the scans that must run, applies time-bounded suppressions, and classifies
// findings as blocking or report-only.
package security

import (
	"path"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ScanType identifies a kind of security scan.
type ScanType string

const (
	DependencyCVE  ScanType = "dependency-cve"
	License        ScanType = "license"
	SAST           ScanType = "sast"
	Secrets        ScanType = "secrets"
	DockerfileLint ScanType = "dockerfile-lint"
	Container      ScanType = "container"
	IaC            ScanType = "iac"
	Actions        ScanType = "github-actions"
	Full           ScanType = "full-vulnerability"
)

// Scan is a scan to run, with the reason it was selected.
type Scan struct {
	Type   ScanType `json:"type"`
	Reason string   `json:"reason"`
}

// Plan maps changed files and run context to the security scans that must run.
// A push to main or a scheduled run always triggers a full vulnerability scan,
// because new CVEs can appear after code was merged.
func Plan(changed []string, event string, onDefaultBranch bool) []Scan {
	if event == "schedule" || onDefaultBranch {
		return []Scan{{Full, "push to main or scheduled run: full vulnerability scan"}}
	}
	var scans []Scan
	seen := map[ScanType]bool{}
	add := func(t ScanType, reason string) {
		if !seen[t] {
			seen[t] = true
			scans = append(scans, Scan{t, reason})
		}
	}
	if anyMatch(changed, isDependencyManifest) {
		add(DependencyCVE, "dependency manifest or lockfile changed")
		add(License, "dependency manifest or lockfile changed")
	}
	if anyMatch(changed, isSource) {
		add(SAST, "source code changed")
		add(Secrets, "source code changed")
	}
	if anyMatch(changed, isDockerfile) {
		add(DockerfileLint, "Dockerfile or image config changed")
		add(Container, "Dockerfile or image config changed")
	}
	if anyMatch(changed, isIaC) {
		add(IaC, "Kubernetes/Helm/Terraform changed")
	}
	if anyMatch(changed, isWorkflow) {
		add(Actions, "GitHub workflow changed")
	}
	sort.Slice(scans, func(i, j int) bool { return scans[i].Type < scans[j].Type })
	return scans
}

func anyMatch(files []string, pred func(string) bool) bool {
	for _, f := range files {
		if pred(f) {
			return true
		}
	}
	return false
}

func isDependencyManifest(f string) bool {
	switch path.Base(f) {
	case "pom.xml", "build.gradle", "build.gradle.kts", "package.json", "pnpm-lock.yaml",
		"package-lock.json", "yarn.lock", "go.mod", "go.sum", "requirements.txt",
		"pyproject.toml", "poetry.lock", "Cargo.toml", "Cargo.lock", "Gemfile", "Gemfile.lock":
		return true
	}
	return false
}

func isSource(f string) bool {
	switch strings.ToLower(path.Ext(f)) {
	case ".java", ".kt", ".ts", ".tsx", ".js", ".jsx", ".go", ".py", ".rb", ".rs", ".cs", ".scala":
		return true
	}
	return false
}

func isDockerfile(f string) bool {
	base := path.Base(f)
	return base == "Dockerfile" || strings.HasSuffix(base, ".dockerfile") ||
		strings.Contains(f, "/src/main/docker/")
}

func isIaC(f string) bool {
	if strings.HasSuffix(f, ".tf") || strings.HasSuffix(f, ".tfvars") {
		return true
	}
	base := path.Base(f)
	if base == "Chart.yaml" || base == "kustomization.yaml" || base == "kustomization.yml" {
		return true
	}
	if ok, _ := doublestar.Match("infra/**", f); ok {
		return strings.HasSuffix(f, ".yaml") || strings.HasSuffix(f, ".yml")
	}
	return false
}

func isWorkflow(f string) bool {
	ok, _ := doublestar.Match(".github/workflows/*.{yml,yaml}", f)
	return ok
}
