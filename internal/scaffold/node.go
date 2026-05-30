package scaffold

import (
	"encoding/json"
	"path"
	"strings"
)

type packageJSON struct {
	Name            string            `json:"name"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	// Workspaces marks a monorepo root (npm/yarn/bun). Such a package declares
	// members but is not itself a module, so it is skipped.
	Workspaces json.RawMessage `json:"workspaces"`
}

// nodeFrameworks are runtime deps that mark a package as an application.
var nodeFrameworks = map[string]bool{
	"next": true, "fastify": true, "express": true, "@nestjs/core": true,
	"react-scripts": true, "vite": true, "@remix-run/server-runtime": true,
	"@sveltejs/kit": true, "@angular/core": true,
}

// detectNode finds every Node package in the repo (walking package.json files,
// the same way the Maven detector walks pom.xml) and resolves intra-repo
// dependencies into module edges. Walking all manifests -- rather than only the
// dirs a pnpm-workspace.yaml expands to -- means nested or generated packages
// (e.g. clients/<svc>/node) are modeled too, so a JS monorepo's graph is
// complete and the self-only-build contract holds.
func detectNode(root string, excludes []string) []detected {
	type info struct {
		dir, name   string
		deps        map[string]string
		appLike, ts bool
	}
	var infos []info
	names := map[string]bool{} // every internal package name in the repo

	walkFiles(root, excludes, func(rel string) {
		if path.Base(rel) != "package.json" {
			return
		}
		data, err := readFile(root, rel)
		if err != nil {
			return
		}
		var pj packageJSON
		if json.Unmarshal(data, &pj) != nil || pj.Name == "" {
			return
		}
		dir := path.Dir(rel)
		if dir == "." || len(pj.Workspaces) > 0 {
			return // the repo/workspace root is not itself a module
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
	})

	out := make([]detected, 0, len(infos))
	for _, in := range infos {
		var depKeys []string
		seen := map[string]bool{}
		for depName, ver := range in.deps {
			// Internal when the dependency is another package in this repo, or it
			// uses the workspace: protocol (pnpm/yarn/bun), which is internal by
			// definition -- so the edge is found even before the target is matched.
			internal := names[depName] || strings.HasPrefix(ver, "workspace:")
			if !internal || depName == in.name || seen[depName] {
				continue
			}
			seen[depName] = true
			depKeys = append(depKeys, nodeKey(depName))
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
