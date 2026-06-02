package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// detectedToolchains holds toolchain versions resolved from a repo's own
// metadata, so the generated workflow provisions what the repo actually needs
// rather than the template's hardcoded defaults. A tool key is present when the
// repo uses that toolchain; the value is its version ("" if present but no
// version could be resolved). pnpm is the packageManager pin for corepack.
type detectedToolchains struct {
	tools map[string]string // node | java | go -> version
	pnpm  string
}

var reFirstVersion = regexp.MustCompile(`\d+(\.\d+)*`)

// detectToolchains resolves toolchain versions from repo metadata:
//   - node: .node-version / .nvmrc / package.json engines.node
//   - pnpm: package.json packageManager
//   - java: .java-version / pom.xml (maven.compiler.release|source, java.version)
//   - go:   go.mod
func detectToolchains(root string) detectedToolchains {
	d := detectedToolchains{tools: map[string]string{}}

	pkg := readRootPackageJSON(root)
	nodeVer := detectNodeVersion(root)
	if pkg != nil || nodeVer != "" || fileExists(root, ".nvmrc") || fileExists(root, ".node-version") {
		d.tools["node"] = nodeVer
	}
	if pkg != nil {
		d.pnpm = pnpmFromPackageManager(pkg.PackageManager)
	}
	if fileExists(root, ".java-version") || fileExists(root, "pom.xml") {
		d.tools["java"] = detectJavaVersion(root)
	}
	if fileExists(root, "go.mod") {
		d.tools["go"] = detectGoVersion(root)
	}
	return d
}

type rootPackage struct {
	Engines        map[string]string `json:"engines"`
	PackageManager string            `json:"packageManager"`
}

func readRootPackageJSON(root string) *rootPackage {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil
	}
	var p rootPackage
	if json.Unmarshal(data, &p) != nil {
		return nil
	}
	return &p
}

func fileExists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func detectNodeVersion(root string) string {
	for _, f := range []string{".node-version", ".nvmrc"} {
		if v := firstLineVersion(root, f); v != "" {
			return v // an explicit version file is authoritative
		}
	}
	// Otherwise aggregate engines.node across every package.json (root and
	// workspace sub-packages), taking the highest required major -- a workspace
	// often pins Node in sub-packages, not at the root.
	return maxEnginesNode(root)
}

// maxEnginesNode walks every package.json (skipping heavy/irrelevant dirs) and
// returns the highest engines.node version any of them requires.
func maxEnginesNode(root string) string {
	best := ""
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "dist", "target", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		var p rootPackage
		if json.Unmarshal(data, &p) != nil {
			return nil
		}
		// engines.node is a range like ">=24" or "^24.1"; take the major.
		if v := reFirstVersion.FindString(p.Engines["node"]); v != "" && (best == "" || versionLess(best, v)) {
			best = v
		}
		return nil
	})
	return best
}

// firstLineVersion reads the first line of a version file (.nvmrc / .node-version
// / .java-version), trimming whitespace and a leading "v". It preserves forms
// like "lts/*" that setup-node accepts.
func firstLineVersion(root, rel string) string {
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(data))
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimPrefix(strings.TrimSpace(s), "v")
}

func pnpmFromPackageManager(pm string) string {
	if !strings.HasPrefix(pm, "pnpm@") {
		return ""
	}
	v := strings.TrimPrefix(pm, "pnpm@")
	if i := strings.IndexByte(v, '+'); i >= 0 { // strip an integrity hash suffix
		v = v[:i]
	}
	return v
}

func detectJavaVersion(root string) string {
	if v := firstLineVersion(root, ".java-version"); v != "" {
		return v
	}
	data, err := os.ReadFile(filepath.Join(root, "pom.xml"))
	if err != nil {
		return ""
	}
	for _, tag := range []string{"maven.compiler.release", "maven.compiler.source", "java.version"} {
		re := regexp.MustCompile(`<` + regexp.QuoteMeta(tag) + `>\s*(\d+)`)
		if m := re.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func detectGoVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?m)^go\s+(\d+\.\d+(\.\d+)?)`)
	if m := re.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return ""
}
