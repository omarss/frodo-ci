package scaffold

import (
	"path"
	"strings"
)

// detectGeneric finds modules that aren't Maven/Node from clear content signals:
// a Go module, an IaC directory (Helm/Kustomize/Terraform), or a standalone
// Dockerfile. It is deliberately conservative to avoid proposing a module for
// every directory that merely contains a YAML file.
func detectGeneric(root string, excludes []string, covered map[string]bool) []detected {
	type sig struct {
		gomod, chart, kustomize, tf, dockerfile, manifest bool
	}
	dirs := map[string]*sig{}
	get := func(dir string) *sig {
		s := dirs[dir]
		if s == nil {
			s = &sig{}
			dirs[dir] = s
		}
		return s
	}

	walkFiles(root, excludes, func(rel string) {
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		switch base := path.Base(rel); {
		case base == "go.mod":
			get(dir).gomod = true
		case base == "Chart.yaml":
			get(dir).chart = true
		case base == "kustomization.yaml" || base == "kustomization.yml":
			get(dir).kustomize = true
		case strings.HasSuffix(base, ".tf"):
			get(dir).tf = true
		case base == "Dockerfile":
			get(dir).dockerfile = true
		case base == "pom.xml" || base == "package.json":
			get(dir).manifest = true
		}
	})

	var out []detected
	for dir, s := range dirs {
		if dir == "" || covered[dir] {
			continue
		}
		switch {
		case s.gomod:
			out = append(out, detected{Path: dir, Type: "go-module"})
		case s.chart || s.kustomize || s.tf:
			out = append(out, detected{Path: dir, Type: "k8s-infra"})
		case s.dockerfile && !s.manifest:
			out = append(out, detected{Path: dir, Type: "docker-image"})
		}
	}
	return out
}
