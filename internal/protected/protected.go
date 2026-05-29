// Package protected determines which protected-file rules a change set triggers
// and the approvals they require.
package protected

import (
	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/match"
)

// Match is a triggered protected-file rule.
type Match struct {
	Name    string                  `json:"name"`
	Files   []string                `json:"files"`
	Require config.ProtectedRequire `json:"require"`
}

// Matches returns the protected-file rules whose patterns match any changed file.
func Matches(changed []string, rules []config.ProtectedFile) []Match {
	var out []Match
	for _, r := range rules {
		var hit []string
		for _, f := range changed {
			if match.GlobAny(r.Files, f) {
				hit = append(hit, f)
			}
		}
		if len(hit) > 0 {
			out = append(out, Match{Name: r.Name, Files: hit, Require: r.Require})
		}
	}
	return out
}
