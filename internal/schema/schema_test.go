package schema

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestGenerateAllExportedKinds(t *testing.T) {
	for _, k := range ExportedKinds() {
		b, err := JSON(k)
		if err != nil {
			t.Fatalf("JSON(%s): %v", k, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: invalid JSON: %v", k, err)
		}
		if _, ok := doc["properties"]; !ok {
			t.Errorf("%s schema has no top-level properties", k)
		}
	}
}

func TestValidateGoodFixtures(t *testing.T) {
	v := NewValidator()
	cases := map[Kind]string{
		KindRoot:       "../config/testdata/root.yml",
		KindModule:     "../config/testdata/module.yml",
		KindStage:      "../config/testdata/test.yml",
		KindToolchains: "../config/testdata/toolchains.yml",
	}
	for k, path := range cases {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if err := v.ValidateBytes(k, path, data); err != nil {
			t.Errorf("expected %s valid against %s schema, got:\n%v", path, k, err)
		}
	}
}

func TestValidateUnknownStageSuggests(t *testing.T) {
	v := NewValidator()
	data, err := os.ReadFile("testdata/bad_stage_name.yml")
	if err != nil {
		t.Fatal(err)
	}
	err = v.ValidateBytes(KindModule, "bad_stage_name.yml", data)
	if err == nil {
		t.Fatal("expected a validation error for an unknown stage name")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"test"`) {
		t.Errorf("expected did-you-mean suggestion of \"test\", got:\n%s", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "allowed") {
		t.Errorf("expected an allowed-list, got:\n%s", msg)
	}
}

func TestValidateUnknownFieldSuggests(t *testing.T) {
	v := NewValidator()
	data, err := os.ReadFile("testdata/bad_unknown_field.yml")
	if err != nil {
		t.Fatal(err)
	}
	err = v.ValidateBytes(KindModule, "bad_unknown_field.yml", data)
	if err == nil {
		t.Fatal("expected a validation error for an unknown field")
	}
	if !strings.Contains(err.Error(), "owners") {
		t.Errorf("expected suggestion of \"owners\", got:\n%s", err.Error())
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"test", "test", 0},
		{"tests", "test", 1},
		{"owner", "owners", 1},
		{"abc", "xyz", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
	if suggest("validate", []string{"validate", "test"}) != "validate" {
		t.Error("exact match should suggest itself")
	}
	if suggest("zzzzzzzz", []string{"validate", "test"}) != "" {
		t.Error("far-off word should not suggest")
	}
}
