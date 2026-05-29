// Package match provides repo-relative glob and regex matching used to map
// changed files to modules and stages. Globs use doublestar semantics so `**`
// spans path separators.
package match

import (
	"path"
	"regexp"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Glob reports whether a repo-relative path matches a doublestar glob pattern.
func Glob(pattern, p string) bool {
	ok, err := doublestar.Match(normalize(pattern), normalize(p))
	return err == nil && ok
}

// GlobAny reports whether path p matches any of the patterns.
func GlobAny(patterns []string, p string) bool {
	for _, pat := range patterns {
		if Glob(pat, p) {
			return true
		}
	}
	return false
}

// AnyPathMatches reports whether any path matches any pattern.
func AnyPathMatches(patterns, paths []string) bool {
	for _, p := range paths {
		if GlobAny(patterns, p) {
			return true
		}
	}
	return false
}

// MatchingPaths returns the subset of paths matching any pattern, preserving order.
func MatchingPaths(patterns, paths []string) []string {
	var out []string
	for _, p := range paths {
		if GlobAny(patterns, p) {
			out = append(out, p)
		}
	}
	return out
}

// Resolve converts a module-relative pattern into a repo-relative one. The
// second return value reports whether the pattern escapes the repository root
// (a leading ".." after cleaning), which the dependency graph rejects.
func Resolve(moduleDir, pattern string) (string, bool) {
	pattern = normalize(pattern)
	if pattern == "" {
		return "", false
	}
	joined := path.Join(normalize(moduleDir), pattern)
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return joined, true
	}
	return joined, false
}

// ResolveAll resolves a list of module-relative patterns, returning the
// repo-relative patterns and the subset that escape the repository.
func ResolveAll(moduleDir string, patterns []string) (resolved, escaping []string) {
	for _, p := range patterns {
		r, esc := Resolve(moduleDir, p)
		if r == "" {
			continue
		}
		resolved = append(resolved, r)
		if esc {
			escaping = append(escaping, p)
		}
	}
	return resolved, escaping
}

// CompileRegex compiles a regular expression used by a regex matcher.
func CompileRegex(expr string) (*regexp.Regexp, error) { return regexp.Compile(expr) }

// IsBroadGlob reports whether a pattern is so broad it would match (almost) the
// whole repository, e.g. "**", "**/*", or "/**". Such patterns defeat selective
// execution and are flagged by the linter.
func IsBroadGlob(pattern string) bool {
	switch normalize(pattern) {
	case "**", "**/*", "*", "**/**":
		return true
	default:
		return false
	}
}

func normalize(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "./")
	return p
}
