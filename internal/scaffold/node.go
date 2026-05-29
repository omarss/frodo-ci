package scaffold

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/goccy/go-yaml"
)

type packageJSON struct {
	Name            string            `json:"name"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// nodeFrameworks are runtime deps that mark a package as an application.
var nodeFrameworks = map[string]bool{
	"next": true, "fastify": true, "express": true, "@nestjs/core": true,
	"react-scripts": true, "vite": true, "@remix-run/server-runtime": true,
	"@sveltejs/kit": true, "@angular/core": true,
}

// detectNode finds Node packages declared by pnpm workspaces and their
// intra-workspace dependencies.
func detectNode(root string, excludes []string) []detected {
	pkgDirs := map[string]bool{}
	walkFiles(root, excludes, func(rel string) {
		if path.Base(rel) == "pnpm-workspace.yaml" {
			wsDir := path.Dir(rel)
			if wsDir == "." {
				wsDir = ""
			}
			for _, d := range expandWorkspace(root, wsDir, rel) {
				pkgDirs[d] = true
			}
		}
	})
	if len(pkgDirs) == 0 {
		return nil
	}

	type info struct {
		dir, name string
		deps      map[string]string
		appLike   bool
		ts        bool
	}
	var infos []info
	names := map[string]bool{}
	for dir := range pkgDirs {
		data, err := readFile(root, path.Join(dir, "package.json"))
		if err != nil {
			continue
		}
		var pj packageJSON
		if json.Unmarshal(data, &pj) != nil || pj.Name == "" {
			continue
		}
		deps := map[string]string{}
		for n, v := range pj.Dependencies {
			deps[n] = v
		}
		for n, v := range pj.DevDependencies {
			deps[n] = v
		}
		infos = append(infos, info{
			dir: dir, name: pj.Name, deps: deps,
			appLike: nodeAppLike(pj),
			ts:      isFile(absUnder(root, dir, "tsconfig.json")),
		})
		names[pj.Name] = true
	}

	var out []detected
	for _, in := range infos {
		var depKeys []string
		for depName, ver := range in.deps {
			if names[depName] || strings.HasPrefix(ver, "workspace:") {
				if names[depName] {
					depKeys = append(depKeys, nodeKey(depName))
				}
			}
		}
		out = append(out, detected{
			Path:    in.dir,
			Type:    nodeType(in.appLike, in.ts),
			Key:     nodeKey(in.name),
			DepKeys: depKeys,
		})
	}
	return out
}

func nodeKey(name string) string { return "npm:" + name }

func nodeAppLike(pj packageJSON) bool {
	if pj.Scripts["start"] != "" || pj.Scripts["dev"] != "" {
		return true
	}
	for dep := range pj.Dependencies {
		if nodeFrameworks[dep] {
			return true
		}
	}
	return false
}

func nodeType(appLike, ts bool) string {
	if !appLike {
		return "node-library"
	}
	if ts {
		return "typescript-app"
	}
	return "node-app"
}

// expandWorkspace expands a pnpm-workspace.yaml's package globs to the
// repo-relative directories that contain a package.json.
func expandWorkspace(root, wsDir, rel string) []string {
	data, err := readFile(root, rel)
	if err != nil {
		return nil
	}
	var ws struct {
		Packages []string `yaml:"packages"`
	}
	if yaml.Unmarshal(data, &ws) != nil {
		return nil
	}
	absWs := filepath.Join(root, filepath.FromSlash(wsDir))
	fsys := os.DirFS(absWs)
	var dirs []string
	for _, patt := range ws.Packages {
		patt = strings.TrimSpace(patt)
		if patt == "" || strings.HasPrefix(patt, "!") {
			continue
		}
		matches, err := doublestar.Glob(fsys, patt)
		if err != nil {
			continue
		}
		for _, m := range matches {
			abs := filepath.Join(absWs, filepath.FromSlash(m))
			if isDir(abs) && isFile(filepath.Join(abs, "package.json")) {
				dirs = append(dirs, path.Join(wsDir, m))
			}
		}
	}
	return dirs
}
