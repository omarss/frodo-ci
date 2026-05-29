package scaffold

import (
	"encoding/xml"
	"path"
	"strings"
)

// pomXML is the minimal Maven POM shape we need.
type pomXML struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Packaging  string `xml:"packaging"`
	Parent     struct {
		GroupID string `xml:"groupId"`
	} `xml:"parent"`
	Dependencies struct {
		Dependency []struct {
			GroupID    string `xml:"groupId"`
			ArtifactID string `xml:"artifactId"`
		} `xml:"dependency"`
	} `xml:"dependencies"`
}

// detectMaven finds buildable Maven modules and their inter-module dependencies.
// A dependency is treated as internal when its artifactId belongs to a module in
// this repo and its groupId is empty, a property (${...}), or another internal
// group — which covers the common reactor conventions without false positives
// from external libraries that merely share an artifactId.
func detectMaven(root string, excludes []string) []detected {
	type pomInfo struct {
		dir, group, artifact string
		deps                 [][2]string
		aggregator           bool
	}
	var poms []pomInfo

	walkFiles(root, excludes, func(rel string) {
		if path.Base(rel) != "pom.xml" {
			return
		}
		data, err := readFile(root, rel)
		if err != nil {
			return
		}
		var p pomXML
		if xml.Unmarshal(data, &p) != nil || p.ArtifactID == "" {
			return
		}
		group := p.GroupID
		if group == "" {
			group = p.Parent.GroupID
		}
		dir := path.Dir(rel)
		if dir == "." {
			dir = ""
		}
		var deps [][2]string
		for _, d := range p.Dependencies.Dependency {
			deps = append(deps, [2]string{d.GroupID, d.ArtifactID})
		}
		poms = append(poms, pomInfo{dir: dir, group: group, artifact: p.ArtifactID,
			deps: deps, aggregator: strings.EqualFold(p.Packaging, "pom")})
	})

	internalArtifacts := map[string]bool{}
	internalGroups := map[string]bool{}
	for _, p := range poms {
		internalArtifacts[p.artifact] = true
		if p.group != "" {
			internalGroups[p.group] = true
		}
	}

	var out []detected
	for _, p := range poms {
		if p.aggregator || p.dir == "" {
			continue // skip reactor aggregators and the repo-root POM
		}
		var depKeys []string
		for _, d := range p.deps {
			g, a := d[0], d[1]
			if !internalArtifacts[a] {
				continue
			}
			if g == "" || strings.HasPrefix(g, "${") || internalGroups[g] {
				depKeys = append(depKeys, mavenKey(a))
			}
		}
		out = append(out, detected{
			Path:    p.dir,
			Type:    mavenType(root, p.dir),
			Key:     mavenKey(p.artifact),
			DepKeys: depKeys,
		})
	}
	return out
}

// mavenKey identifies a Maven module by artifactId, which is unique within a
// well-formed monorepo and robust to property-based groupIds in dependencies.
func mavenKey(artifactID string) string { return "mvn:" + artifactID }

func mavenType(root, dir string) string {
	if isFile(absUnder(root, dir, "Dockerfile")) || isDir(absUnder(root, dir, "src/main/docker")) {
		return "spring-service"
	}
	return "java-library"
}
