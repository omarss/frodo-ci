package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/omarss/frodo-ci/internal/config"
)

// reviewSpec is one review rule built from CLI flags.
type reviewSpec struct {
	name    string
	when    []string
	require config.ReviewRequire
}

// parseReviewSpecs builds review rules from --review (the default rule) and
// repeatable --review-path "<glob>:<requirements>" flags.
func parseReviewSpecs(review string, paths []string) ([]reviewSpec, error) {
	var specs []reviewSpec
	if strings.TrimSpace(review) != "" {
		req, err := parseReviewRequire(review)
		if err != nil {
			return nil, fmt.Errorf("--review: %w", err)
		}
		specs = append(specs, reviewSpec{name: "default", require: req})
	}
	used := map[string]bool{}
	for _, p := range paths {
		glob, reqspec, ok := strings.Cut(p, ":")
		glob = strings.TrimSpace(glob)
		if !ok || glob == "" {
			return nil, fmt.Errorf("--review-path %q must be glob:requirements", p)
		}
		req, err := parseReviewRequire(reqspec)
		if err != nil {
			return nil, fmt.Errorf("--review-path %q: %w", p, err)
		}
		specs = append(specs, reviewSpec{name: reviewRuleName(glob, used), when: []string{glob}, require: req})
	}
	return specs, nil
}

// parseReviewRequire parses "owners=1,expert=1,teams=security:1" into a
// ReviewRequire.
func parseReviewRequire(spec string) (config.ReviewRequire, error) {
	var req config.ReviewRequire
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := strings.Cut(part, "=")
		if !ok {
			return req, fmt.Errorf("requirement %q must be key=value", part)
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "owners":
			n, err := strconv.Atoi(val)
			if err != nil {
				return req, fmt.Errorf("owners=%q is not a number", val)
			}
			req.Owners = &n
		case "expert":
			n, err := strconv.Atoi(val)
			if err != nil {
				return req, fmt.Errorf("expert=%q is not a number", val)
			}
			req.Expert = &n
		case "teams", "users":
			who, n := splitCount(val)
			if who == "" {
				return req, fmt.Errorf("%s=%q is missing a name", key, val)
			}
			if key == "teams" {
				if req.Teams == nil {
					req.Teams = map[string]int{}
				}
				req.Teams[who] = n
			} else {
				if req.Users == nil {
					req.Users = map[string]int{}
				}
				req.Users[who] = n
			}
		default:
			return req, fmt.Errorf("unknown requirement %q (want owners|expert|teams|users)", key)
		}
	}
	return req, nil
}

// splitCount parses "name:N", defaulting N to 1.
func splitCount(s string) (string, int) {
	name, cnt, ok := strings.Cut(s, ":")
	n := 1
	if ok {
		if v, err := strconv.Atoi(strings.TrimSpace(cnt)); err == nil {
			n = v
		}
	}
	return strings.TrimSpace(name), n
}

// reviewRuleName derives a stable rule key from a glob (its last literal path
// segment), disambiguating collisions.
func reviewRuleName(glob string, used map[string]bool) string {
	name := ""
	for _, seg := range strings.Split(glob, "/") {
		if seg != "" && !strings.ContainsAny(seg, "*?[]{}") {
			name = seg
		}
	}
	if name == "" {
		name = "path"
	}
	base, i := name, 2
	for used[name] {
		name = fmt.Sprintf("%s-%d", base, i)
		i++
	}
	used[name] = true
	return name
}

// renderReviews renders the reviews: section of a module.yml from specs.
func renderReviews(specs []reviewSpec) string {
	if len(specs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\nreviews:\n")
	for _, s := range specs {
		fmt.Fprintf(&b, "  %s:\n", s.name)
		if len(s.when) > 0 {
			b.WriteString("    when:\n")
			for _, w := range s.when {
				fmt.Fprintf(&b, "      - %s\n", w)
			}
		}
		b.WriteString("    require:\n")
		r := s.require
		if r.Owners != nil {
			fmt.Fprintf(&b, "      owners: %d\n", *r.Owners)
		}
		if r.Expert != nil {
			fmt.Fprintf(&b, "      expert: %d\n", *r.Expert)
		}
		writeCountMap(&b, "teams", r.Teams)
		writeCountMap(&b, "users", r.Users)
	}
	return b.String()
}

func writeCountMap(b *strings.Builder, key string, m map[string]int) {
	if len(m) == 0 {
		return
	}
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	fmt.Fprintf(b, "      %s:\n", key)
	for _, n := range names {
		fmt.Fprintf(b, "        %s: %d\n", n, m[n])
	}
}
