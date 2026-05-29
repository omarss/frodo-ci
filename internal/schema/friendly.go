package schema

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	jsv "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// FriendlyError is a human-readable validation failure for a single config file.
type FriendlyError struct {
	Path     string
	Messages []string
}

func (e *FriendlyError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Invalid file: %s\n", e.Path)
	for _, m := range e.Messages {
		fmt.Fprintf(&b, "\n  - %s", strings.ReplaceAll(m, "\n", "\n    "))
	}
	return b.String()
}

// friendlyFromValidation turns a santhosh ValidationError tree into a small set
// of human-friendly messages, adding did-you-mean hints where possible.
func friendlyFromValidation(k Kind, path string, ve *jsv.ValidationError) *FriendlyError {
	p := message.NewPrinter(language.English)
	vocab := vocabulary(k)

	var msgs []string
	seen := make(map[string]bool)
	add := func(m string) {
		if m != "" && !seen[m] {
			seen[m] = true
			msgs = append(msgs, m)
		}
	}

	var walk func(e *jsv.ValidationError)
	walk = func(e *jsv.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		add(formatKind(e, vocab, p))
	}
	walk(ve)

	if len(msgs) == 0 {
		msgs = []string{ve.Error()}
	}
	return &FriendlyError{Path: path, Messages: msgs}
}

func formatKind(e *jsv.ValidationError, vocab []string, p *message.Printer) string {
	loc := instanceLocation(e.InstanceLocation)
	switch kd := e.ErrorKind.(type) {
	case *kind.AdditionalProperties:
		var parts []string
		for _, prop := range kd.Properties {
			if s := suggest(prop, vocab); s != "" {
				parts = append(parts, fmt.Sprintf("unknown field %q in %s; did you mean %q?", prop, loc, s))
			} else {
				parts = append(parts, fmt.Sprintf("unknown field %q in %s", prop, loc))
			}
		}
		return strings.Join(parts, "\n")
	case *kind.Enum:
		got := fmt.Sprintf("%v", kd.Got)
		want := anyToStrings(kd.Want)
		msg := fmt.Sprintf("%q is not allowed at %s", got, loc)
		if s := suggest(got, want); s != "" {
			msg += fmt.Sprintf("; did you mean %q?", s)
		}
		return msg + "\nallowed: " + strings.Join(want, ", ")
	case *kind.Required:
		return fmt.Sprintf("missing required field(s) %s at %s", strings.Join(kd.Missing, ", "), loc)
	case *kind.Type:
		return fmt.Sprintf("expected %s but got %s at %s", strings.Join(kd.Want, " or "), kd.Got, loc)
	default:
		return fmt.Sprintf("%s: %s", loc, e.ErrorKind.LocalizedString(p))
	}
}

// instanceLocation renders a JSON-pointer-ish path segment list as a/b/c, with
// the document root shown as "(root)".
func instanceLocation(loc []string) string {
	if len(loc) == 0 {
		return "(root)"
	}
	return strings.Join(loc, "/")
}

func anyToStrings(vals []any) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, fmt.Sprintf("%v", v))
	}
	sort.Strings(out)
	return out
}

// Suggest returns the closest candidate to word within an edit-distance budget,
// or "" when nothing is close enough. Exposed for the semantic linter so its
// "did you mean" hints match the validator's.
func Suggest(word string, candidates []string) string { return suggest(word, candidates) }

// suggest returns the closest candidate to word within an edit-distance budget,
// or "" when nothing is close enough to be a helpful suggestion.
func suggest(word string, candidates []string) string {
	word = strings.ToLower(word)
	best, bestDist := "", 1<<30
	budget := len(word)/3 + 1
	if budget < 2 {
		budget = 2
	}
	for _, c := range candidates {
		d := levenshtein(word, strings.ToLower(c))
		if d < bestDist {
			best, bestDist = c, d
		}
	}
	if bestDist <= budget {
		return best
	}
	return ""
}

func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// vocabulary returns every property name declared anywhere in a kind's schema,
// used to power did-you-mean for unknown-field errors. Results are cached.
func vocabulary(k Kind) []string {
	vocabOnce.Do(func() { vocabCache = make(map[Kind][]string) })
	vocabMu.Lock()
	defer vocabMu.Unlock()
	if v, ok := vocabCache[k]; ok {
		return v
	}
	doc, err := document(k)
	names := map[string]bool{}
	if err == nil {
		collectPropertyNames(doc, names)
	}
	out := make([]string, 0, len(names))
	for n := range names {
		out = append(out, n)
	}
	sort.Strings(out)
	vocabCache[k] = out
	return out
}

var (
	vocabOnce  sync.Once
	vocabMu    sync.Mutex
	vocabCache map[Kind][]string
)

// collectPropertyNames walks a schema document collecting the keys of every
// "properties" object it finds.
func collectPropertyNames(node any, into map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		if props, ok := n["properties"].(map[string]any); ok {
			for key := range props {
				into[key] = true
			}
		}
		for _, v := range n {
			collectPropertyNames(v, into)
		}
	case []any:
		for _, v := range n {
			collectPropertyNames(v, into)
		}
	}
}
