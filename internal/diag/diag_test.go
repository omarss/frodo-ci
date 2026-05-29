package diag

import (
	"strings"
	"testing"
)

func TestDetectStack(t *testing.T) {
	cases := []struct {
		in   Input
		want string
	}{
		{Input{Command: "./mvnw -pl . -am verify"}, "maven"},
		{Input{Command: "pnpm -s build"}, "node"},
		{Input{Command: "tsc --noEmit"}, "typescript"},
		{Input{Command: "go test ./..."}, "go"},
		{Input{Command: "docker build -t x ."}, "docker"},
		{Input{Command: "terraform plan"}, "terraform"},
		{Input{Command: "kubeconform -strict ."}, "kubernetes"},
		{Input{Command: "pytest -q"}, "python"},
		{Input{Command: "echo hi", ModuleType: "spring-service"}, "maven"},                  // via module type
		{Input{Command: "echo hi", Output: "error TS2304: Cannot find name"}, "typescript"}, // via output
		{Input{Command: "echo hi"}, "generic"},
	}
	for _, c := range cases {
		if got := DetectStack(c.in); got != c.want {
			t.Errorf("DetectStack(%q/%q) = %q, want %q", c.in.Command, c.in.ModuleType, got, c.want)
		}
	}
}

func TestCrossCuttingWins(t *testing.T) {
	// A registry 403 during a docker build is diagnosed as auth, not a build bug.
	r := Analyze(Input{Command: "docker build -t x .", Stage: "package",
		Output: "#10 transferring dockerfile: done\n#10 ERROR: failed to authorize: 403 Forbidden"})
	if !strings.Contains(r.Summary, "auth") && !strings.Contains(strings.ToLower(r.Summary), "auth") {
		t.Errorf("summary = %q, want an auth diagnosis", r.Summary)
	}
	if !strings.Contains(r.Hint, "registry") {
		t.Errorf("hint = %q, want registry guidance", r.Hint)
	}
	if r.Stack != "docker" {
		t.Errorf("stack = %q, want docker", r.Stack)
	}
}

func TestExtractMavenTests(t *testing.T) {
	out := `[INFO] Running com.cards.CardServiceTest
[ERROR] Tests run: 4, Failures: 1, Errors: 0, Skipped: 0 -- in com.cards.CardServiceTest
[ERROR] com.cards.CardServiceTest.transfer:42 expected:<200> but was:<500>
[INFO] BUILD FAILURE`
	r := Analyze(Input{Command: "./mvnw -pl services/cards -am verify", Stage: "test", Output: out})
	if r.Stack != "maven" || !strings.Contains(r.Summary, "test") {
		t.Errorf("maven test summary = %q (stack %q)", r.Summary, r.Stack)
	}
	if !strings.Contains(r.Snippet, "expected:<200> but was:<500>") {
		t.Errorf("snippet should carry the assertion, got:\n%s", r.Snippet)
	}
	if strings.Contains(r.Snippet, "[ERROR]") {
		t.Errorf("snippet should strip the [ERROR] prefix, got:\n%s", r.Snippet)
	}
}

func TestExtractGo(t *testing.T) {
	out := `=== RUN   TestTransfer
    card_test.go:42: expected 200, got 500
--- FAIL: TestTransfer (0.00s)
FAIL
FAIL	github.com/x/cards	0.012s`
	r := Analyze(Input{Command: "go test ./...", Stage: "test", Output: out})
	if r.Stack != "go" || !strings.Contains(r.Summary, "TestTransfer") {
		t.Errorf("go summary = %q (stack %q)", r.Summary, r.Stack)
	}
	if !strings.Contains(r.Snippet, "--- FAIL: TestTransfer") {
		t.Errorf("snippet missing the FAIL line:\n%s", r.Snippet)
	}
}

func TestExtractTypeScript(t *testing.T) {
	out := "src/index.ts(10,5): error TS2322: Type 'string' is not assignable to type 'number'."
	r := Analyze(Input{Command: "pnpm -s typecheck", Stage: "validate", ModuleType: "node-app", Output: out})
	if !strings.Contains(r.Summary, "TypeScript error") || !strings.Contains(r.Snippet, "TS2322") {
		t.Errorf("ts diagnosis = %+v", r)
	}
}

func TestExtractDockerLine(t *testing.T) {
	out := "#8 [2/5] RUN npm ci\n#8 ERROR: process did not complete\nDockerfile:14\n------\nfailed to solve: process exited"
	r := Analyze(Input{Command: "docker build -t x .", Stage: "package", Output: out})
	if r.Stack != "docker" || !strings.Contains(r.Summary, "Dockerfile:14") {
		t.Errorf("docker summary = %q", r.Summary)
	}
}

func TestExtractPnpmCode(t *testing.T) {
	out := "ERR_PNPM_NO_PKG_MANIFEST  No package.json found"
	r := Analyze(Input{Command: "pnpm install", Stage: "validate", Output: out})
	if !strings.Contains(r.Summary, "ERR_PNPM_NO_PKG_MANIFEST") {
		t.Errorf("pnpm summary = %q", r.Summary)
	}
}

func TestGenericFallback(t *testing.T) {
	out := "doing thing\nsomething went wrong: widget exploded\nbye"
	r := Analyze(Input{Command: "./run.sh", Stage: "build", Output: out})
	if r.Snippet == "" {
		t.Error("generic fallback should still produce a snippet")
	}
	if r.Hint == "" {
		t.Error("generic fallback should still produce a stage hint")
	}
}
