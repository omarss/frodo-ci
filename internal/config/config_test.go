package config

import (
	"path/filepath"
	"testing"
	"time"
)

func td(name string) string { return filepath.Join("testdata", name) }

func TestLoadRoot(t *testing.T) {
	c, src, err := LoadRoot(td("root.yml"))
	if err != nil {
		t.Fatalf("LoadRoot: %v", err)
	}
	if src == nil || len(src.Bytes) == 0 {
		t.Fatal("expected source bytes to be retained")
	}
	if c.Version != 1 {
		t.Errorf("version = %d, want 1", c.Version)
	}
	if c.ModuleFile != ".ci/module.yml" {
		t.Errorf("module_file = %q", c.ModuleFile)
	}
	if got := len(c.Stages.CI); got != 5 {
		t.Errorf("ci stages = %d, want 5", got)
	}
	if c.Execution.FinalCheckName != "Frodo CI / final" {
		t.Errorf("final_check_name = %q", c.Execution.FinalCheckName)
	}
	if !c.Execution.FailFast {
		t.Error("fail_fast should be true")
	}
	if c.Execution.FullRunTimeoutMinutes != 90 {
		t.Errorf("full_run_timeout_minutes = %d, want 90", c.Execution.FullRunTimeoutMinutes)
	}
	if c.Experts.Window.Duration() != 30*24*time.Hour {
		t.Errorf("experts.window = %s, want 720h", c.Experts.Window)
	}
	if c.Security.Mode != "smart" || c.Security.BlockingProfile != "strict" {
		t.Errorf("security mode/profile = %q/%q", c.Security.Mode, c.Security.BlockingProfile)
	}
	if c.Slack.Channel != "#ci-alerts" || len(c.Slack.NotifyOn) != 5 {
		t.Errorf("slack channel/notify = %q / %d", c.Slack.Channel, len(c.Slack.NotifyOn))
	}
	if len(c.ProtectedFiles) != 2 {
		t.Fatalf("protected_files = %d, want 2", len(c.ProtectedFiles))
	}
	if c.ProtectedFiles[0].Require.Teams["platform-team"] != 1 {
		t.Errorf("platform-team requirement = %d", c.ProtectedFiles[0].Require.Teams["platform-team"])
	}
	if c.ProtectedFiles[1].Require.Teams["security-team"] != 1 {
		t.Errorf("security-team requirement = %d", c.ProtectedFiles[1].Require.Teams["security-team"])
	}
}

func TestLoadModule(t *testing.T) {
	c, _, err := LoadModule(td("module.yml"))
	if err != nil {
		t.Fatalf("LoadModule: %v", err)
	}
	if c.Name != "cards" || c.Type != "spring-service" {
		t.Errorf("name/type = %q/%q", c.Name, c.Type)
	}
	if c.Use.Profile != "spring-service" {
		t.Errorf("use.profile = %q", c.Use.Profile)
	}
	if len(c.Owners.Teams) != 1 || c.Owners.Teams[0] != "cards-team" {
		t.Errorf("owners.teams = %v", c.Owners.Teams)
	}
	if len(c.DependsOn) != 1 || c.DependsOn[0].Module != "money" {
		t.Fatalf("depends_on = %+v", c.DependsOn)
	}
	if len(c.DependsOn[0].Affects) != 4 {
		t.Errorf("affects = %v", c.DependsOn[0].Affects)
	}
	test, ok := c.CI["test"]
	if !ok {
		t.Fatal("missing ci.test")
	}
	if len(test.Inputs) != 3 || test.Inputs[0] != "../../pom.xml" {
		t.Errorf("test.inputs = %v", test.Inputs)
	}
	scan := c.CI["scan"]
	if scan.Security == nil || scan.Security.Profile != "fintech-service" {
		t.Errorf("scan.security = %+v", scan.Security)
	}
	deploy := c.CD["deploy"]
	if len(deploy.Environments) != 2 {
		t.Errorf("deploy.environments = %v", deploy.Environments)
	}

	def, ok := c.Reviews["default"]
	if !ok || def.Require.Owners == nil || *def.Require.Owners != 1 || def.Require.Expert == nil || *def.Require.Expert != 1 {
		t.Errorf("reviews.default.require = %+v", def.Require)
	}
	sensitive, ok := c.Reviews["sensitive_card_data"]
	if !ok {
		t.Fatal("missing reviews.sensitive_card_data")
	}
	if len(sensitive.When) != 4 {
		t.Fatalf("sensitive.when len = %d, want 4", len(sensitive.When))
	}
	if !sensitive.When[3].IsRegex() {
		t.Errorf("sensitive.when[3] should be a regex matcher, got %q", sensitive.When[3])
	}
	if sensitive.When[0].Glob != "src/main/**/CardToken*.java" {
		t.Errorf("sensitive.when[0] = %q", sensitive.When[0].Glob)
	}
	if sensitive.Require.Teams["security-team"] != 1 {
		t.Errorf("sensitive.require.teams = %v", sensitive.Require.Teams)
	}
	if c.Performance.Budgets["test"].Duration() != 20*time.Minute {
		t.Errorf("performance.budgets.test = %s", c.Performance.Budgets["test"])
	}
}

func TestLoadStage(t *testing.T) {
	c, _, err := LoadStage(td("test.yml"))
	if err != nil {
		t.Fatalf("LoadStage: %v", err)
	}
	if c.Name != "Test cards" || c.TimeoutMinutes != 20 {
		t.Errorf("name/timeout = %q/%d", c.Name, c.TimeoutMinutes)
	}
	java, ok := c.Setup["java"]
	if !ok || java.Distribution != "liberica" || java.Version != "25" {
		t.Errorf("setup.java = %+v", java)
	}
	if c.Cache == nil || len(c.Cache.Paths) != 1 || c.Cache.Paths[0] != "~/.m2/repository" {
		t.Errorf("cache = %+v", c.Cache)
	}
	if len(c.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(c.Steps))
	}
	if c.Steps[0].WorkingDirectory != "services/cards" {
		t.Errorf("steps[0].working_directory = %q", c.Steps[0].WorkingDirectory)
	}
}

func TestLoadToolchains(t *testing.T) {
	c, _, err := LoadToolchains(td("toolchains.yml"))
	if err != nil {
		t.Fatalf("LoadToolchains: %v", err)
	}
	if c.Version != 1 {
		t.Errorf("version = %d", c.Version)
	}
	java := c.Formatters["java"]
	if java.Tool != "spotless" || !java.Blocking {
		t.Errorf("formatters.java = %+v", java)
	}
	if len(c.Linters["java"].Deterministic) != 2 {
		t.Errorf("linters.java.deterministic = %v", c.Linters["java"].Deterministic)
	}
	if got := c.Linters["typescript"].Optional; len(got) != 1 || got[0] != "eslint" {
		t.Errorf("linters.typescript.optional = %v", got)
	}
	if !c.FormatterTools()["spotless"] || !c.LinterTools()["checkstyle"] {
		t.Error("tool-name index missing expected entries")
	}
}

func TestApplyDefaults(t *testing.T) {
	var c RootConfig
	c.ApplyDefaults()
	if c.ModuleFile != ".ci/module.yml" {
		t.Errorf("module_file default = %q", c.ModuleFile)
	}
	if len(c.Stages.CI) != 5 || len(c.Stages.CD) != 3 {
		t.Errorf("stage defaults = ci:%v cd:%v", c.Stages.CI, c.Stages.CD)
	}
	if c.Execution.FinalCheckName != "Frodo CI / final" {
		t.Errorf("final_check_name default = %q", c.Execution.FinalCheckName)
	}
	if c.Execution.MaxParallelModules != 4 || c.Execution.MaxParallelExpensiveStages != 1 {
		t.Errorf("parallel defaults = %d/%d", c.Execution.MaxParallelModules, c.Execution.MaxParallelExpensiveStages)
	}
	if !c.IsCDStage("deploy") || c.IsCDStage("test") {
		t.Error("IsCDStage classification wrong")
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{
		"":      0,
		"30d":   30 * 24 * time.Hour,
		"20m":   20 * time.Minute,
		"1h30m": 90 * time.Minute,
		"2w":    14 * 24 * time.Hour,
		"45s":   45 * time.Second,
	}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil {
			t.Errorf("ParseDuration(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseDuration(%q) = %s, want %s", in, got, want)
		}
	}
	if _, err := ParseDuration("10x"); err == nil {
		t.Error("expected error for unknown unit")
	}
}
