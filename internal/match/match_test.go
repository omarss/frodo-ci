package match

import "testing"

func TestGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"services/cards/src/**", "services/cards/src/main/Foo.java", true},
		{"services/**", "services/cards", true},
		{"**/target/**", "services/cards/target/x.class", true},
		{"pom.xml", "pom.xml", true},
		{"src/**", "services/cards/src/main/Foo.java", false}, // not resolved yet
		{"*.go", "main.go", true},
		{"*.go", "pkg/main.go", false},
	}
	for _, c := range cases {
		if got := Glob(c.pattern, c.path); got != c.want {
			t.Errorf("Glob(%q,%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		dir, pattern, want string
		escapes            bool
	}{
		{"services/cards", "src/**", "services/cards/src/**", false},
		{"services/cards", ".ci/**", "services/cards/.ci/**", false},
		{"services/cards", "../../pom.xml", "pom.xml", false},
		{"services/cards", "../../proto/cards/**", "proto/cards/**", false},
		{"services/cards", "../../../outside", "../outside", true},
	}
	for _, c := range cases {
		got, esc := Resolve(c.dir, c.pattern)
		if got != c.want || esc != c.escapes {
			t.Errorf("Resolve(%q,%q) = (%q,%v), want (%q,%v)", c.dir, c.pattern, got, esc, c.want, c.escapes)
		}
	}
}

func TestResolvedGlobMatches(t *testing.T) {
	resolved, _ := Resolve("services/cards", "src/main/**")
	if !Glob(resolved, "services/cards/src/main/Card.java") {
		t.Errorf("resolved %q should match the nested file", resolved)
	}
}

func TestAnyPathMatchesAndMatchingPaths(t *testing.T) {
	patterns := []string{"services/cards/src/**", "services/cards/pom.xml"}
	paths := []string{"services/cards/pom.xml", "services/cards/src/x.java", "libs/other/y.go"}
	if !AnyPathMatches(patterns, paths) {
		t.Error("expected a match")
	}
	got := MatchingPaths(patterns, paths)
	if len(got) != 2 {
		t.Errorf("MatchingPaths = %v, want 2", got)
	}
}

func TestIsBroadGlob(t *testing.T) {
	for _, p := range []string{"**", "**/*", "*", "./**"} {
		if !IsBroadGlob(p) {
			t.Errorf("IsBroadGlob(%q) = false, want true", p)
		}
	}
	if IsBroadGlob("src/**") {
		t.Error("src/** should not be considered broad")
	}
}
