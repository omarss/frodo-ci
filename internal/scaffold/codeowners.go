package scaffold

import (
	"bufio"
	"bytes"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// codeowners is a parsed CODEOWNERS file. Later rules win, matching GitHub's
// last-match-wins semantics.
type codeowners struct{ rules []coRule }

type coRule struct {
	pattern string
	teams   []string
	users   []string
}

func loadCodeowners(root string) *codeowners {
	co := &codeowners{}
	for _, loc := range []string{".github/CODEOWNERS", "CODEOWNERS", "docs/CODEOWNERS"} {
		if data, err := readFile(root, loc); err == nil {
			co.parse(data)
			break
		}
	}
	return co
}

func (c *codeowners) parse(data []byte) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		r := coRule{pattern: fields[0]}
		for _, owner := range fields[1:] {
			owner = strings.TrimPrefix(owner, "@")
			switch {
			case strings.Contains(owner, "/"): // @org/team
				r.teams = append(r.teams, owner[strings.Index(owner, "/")+1:])
			case strings.Contains(owner, "@"): // email -- not a team or user handle
			default:
				r.users = append(r.users, owner)
			}
		}
		c.rules = append(c.rules, r)
	}
}

// Owner returns the owning team/user for a repo-relative directory. The last
// matching rule wins; ok is false when nothing matches.
func (c *codeowners) Owner(dir string) (team, user string, ok bool) {
	for _, r := range c.rules {
		if !coMatch(r.pattern, dir) {
			continue
		}
		team, user, ok = "", "", true
		if len(r.teams) > 0 {
			team = r.teams[0]
		}
		if len(r.users) > 0 {
			user = r.users[0]
		}
	}
	return team, user, ok
}

// coMatch reports whether a CODEOWNERS pattern matches a directory path,
// supporting the common cases: a catch-all `*`, an anchored directory prefix
// (e.g. `/services/`), and glob patterns (e.g. `apps/*`).
func coMatch(pattern, dir string) bool {
	if pattern == "*" || pattern == "" || pattern == "/" {
		return true
	}
	p := strings.TrimSuffix(strings.TrimPrefix(pattern, "/"), "/")
	if dir == p || strings.HasPrefix(dir, p+"/") {
		return true
	}
	if ok, _ := doublestar.Match(p, dir); ok {
		return true
	}
	ok, _ := doublestar.Match(p+"/**", dir)
	return ok
}
