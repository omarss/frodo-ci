package cli

import (
	"strings"
	"testing"
)

func TestParseReviewSpecs(t *testing.T) {
	specs, err := parseReviewSpecs("owners=1,expert=1,teams=platform:2",
		[]string{"src/**/settlements/**:teams=security:1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 2 {
		t.Fatalf("want 2 specs, got %d", len(specs))
	}
	d := specs[0]
	if d.name != "default" || d.require.Owners == nil || *d.require.Owners != 1 ||
		d.require.Expert == nil || *d.require.Expert != 1 || d.require.Teams["platform"] != 2 {
		t.Errorf("default rule = %+v", d)
	}
	p := specs[1]
	if p.name != "settlements" || len(p.when) != 1 || p.when[0] != "src/**/settlements/**" ||
		p.require.Teams["security"] != 1 {
		t.Errorf("path rule = %+v (when=%v)", p.require, p.when)
	}
	out := renderReviews(specs)
	for _, want := range []string{
		"reviews:", "default:", "owners: 1", "expert: 1", "platform: 2",
		"settlements:", "when:", "- src/**/settlements/**", "security: 1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered reviews missing %q:\n%s", want, out)
		}
	}
}

func TestParseReviewBadSpec(t *testing.T) {
	if _, err := parseReviewSpecs("owners=notanumber", nil); err == nil {
		t.Error("expected error for non-numeric owners")
	}
	if _, err := parseReviewSpecs("", []string{"noseparator"}); err == nil {
		t.Error("expected error for --review-path without a glob:requirements separator")
	}
	if _, err := parseReviewSpecs("bogus=1", nil); err == nil {
		t.Error("expected error for an unknown requirement key")
	}
}
