// Package diag turns a failed step's raw output into an intelligent, tech-stack
// aware diagnosis: it detects the stack (from the command, module type, and the
// output itself), extracts the salient error out of verbose logs, and produces
// a one-line summary plus actionable "how to fix" guidance. Cross-cutting root
// causes (auth, network, OOM, ...) that apply to any stack take precedence.
package diag

import (
	"regexp"
	"strings"
)

// Input is a failed step's context.
type Input struct {
	Command    string // the step's command (strongest stack signal)
	Output     string // raw combined output (a generous tail)
	Stage      string // validate | test | build | package | scan | ...
	ModuleType string // template profile, e.g. spring-service, node-app
}

// Result is the diagnosis.
type Result struct {
	Stack   string `json:"stack"`             // detected stack, e.g. "maven", "go"
	Summary string `json:"summary,omitempty"` // one-line "what failed"
	Hint    string `json:"hint,omitempty"`    // how to fix
	Snippet string `json:"snippet,omitempty"` // the extracted, high-signal error lines
}

// Analyze produces a diagnosis. Order: cross-cutting root causes first (they
// explain failures regardless of stack), then the stack-specific extractor,
// then a generic distillation.
func Analyze(in Input) Result {
	stack := DetectStack(in)

	if r, ok := crossCutting(in.Output); ok {
		r.Stack = stack
		if r.Snippet == "" {
			r.Snippet = concise(in.Output)
		}
		return r
	}

	if ex := extractors[stack]; ex != nil {
		if r, ok := ex(in.Output); ok {
			r.Stack = stack
			if r.Snippet == "" {
				r.Snippet = concise(in.Output)
			}
			if r.Hint == "" {
				r.Hint = StageHint(in.Stage)
			}
			return r
		}
	}

	return Result{Stack: stack, Hint: StageHint(in.Stage), Snippet: concise(in.Output)}
}

// DetectStack resolves the tech stack from the command, then the module type,
// then markers in the output.
func DetectStack(in Input) string {
	if s := stackFromCommand(in.Command); s != "" {
		return s
	}
	if s := stackFromModuleType(in.ModuleType); s != "" {
		return s
	}
	return stackFromOutput(in.Output)
}

func stackFromCommand(cmd string) string {
	for _, m := range commandStacks {
		if m.re.MatchString(cmd) {
			return m.stack
		}
	}
	return ""
}

// commandStacks maps a command token to a stack, most-specific first.
var commandStacks = []struct {
	re    *regexp.Regexp
	stack string
}{
	{regexp.MustCompile(`\b(mvn|mvnw|\./mvnw)\b`), "maven"},
	{regexp.MustCompile(`\b(gradle|gradlew|\./gradlew)\b`), "gradle"},
	{regexp.MustCompile(`\btsc\b`), "typescript"},
	{regexp.MustCompile(`\b(pnpm|npm|yarn|npx)\b`), "node"},
	{regexp.MustCompile(`\b(pytest|py\.test|python -m pytest|tox|pip|poetry)\b`), "python"},
	{regexp.MustCompile(`\b(docker|buildah|podman)\s+build|docker\s+buildx`), "docker"},
	{regexp.MustCompile(`\b(terraform|tofu)\b`), "terraform"},
	{regexp.MustCompile(`\b(kubeconform|kubeval|kubectl|helm|kustomize)\b`), "kubernetes"},
	{regexp.MustCompile(`\b(go)\s+(test|build|vet|run)|golangci-lint|gofmt\b`), "go"},
	{regexp.MustCompile(`\bdotnet\b`), "dotnet"},
	{regexp.MustCompile(`\bcargo\b`), "rust"},
	{regexp.MustCompile(`\b(bundle|rake|rspec)\b`), "ruby"},
}

func stackFromModuleType(t string) string {
	switch t {
	case "spring-service", "java-library":
		return "maven"
	case "node-app", "node-library":
		return "node"
	case "typescript-app":
		return "typescript"
	case "go-module":
		return "go"
	case "k8s-infra":
		return "kubernetes"
	case "docker-image":
		return "docker"
	default:
		return ""
	}
}

func stackFromOutput(out string) string {
	for _, m := range outputStacks {
		if m.re.MatchString(out) {
			return m.stack
		}
	}
	return "generic"
}

var outputStacks = []struct {
	re    *regexp.Regexp
	stack string
}{
	{regexp.MustCompile(`(?m)^\[ERROR\]|BUILD FAILURE`), "maven"},
	{regexp.MustCompile(`error TS\d+:`), "typescript"},
	{regexp.MustCompile(`(?m)^--- FAIL:|^panic:|\.go:\d+:\d+:`), "go"},
	{regexp.MustCompile(`npm ERR!|ERR_PNPM|ELIFECYCLE`), "node"},
	{regexp.MustCompile(`Traceback \(most recent call last\)|(?m)^E\s{2,}\w`), "python"},
	{regexp.MustCompile(`failed to solve|(?m)^Dockerfile:\d+`), "docker"},
	{regexp.MustCompile(`(?m)^Error: .*\n.*on .* line \d+`), "terraform"},
	{regexp.MustCompile(`> Task .* FAILED|BUILD FAILED|What went wrong:`), "gradle"},
}

// crossCutting matches root causes that explain a failure regardless of stack.
func crossCutting(out string) (Result, bool) {
	low := strings.ToLower(out)
	for _, sig := range crossSignatures {
		for _, p := range sig.patterns {
			if strings.Contains(low, p) {
				return Result{
					Summary: sig.summary,
					Hint:    sig.hint,
					Snippet: linesMatching(out, sig.patterns, 12),
				}, true
			}
		}
	}
	return Result{}, false
}

type crossSig struct {
	patterns []string // lowercase substrings
	summary  string
	hint     string
}

var crossSignatures = []crossSig{
	{[]string{"403 forbidden", "failed to authorize", "denied: ", "pull access denied",
		"401 unauthorized", "unauthorized:", "authentication required", "insufficient_scope",
		"requested access to the resource is denied"},
		"Registry/authentication denied",
		"The runner can't pull or push (auth). Authenticate to the registry — for GitHub OIDC to a cloud registry, configure the workload-identity provider/service-account — and retry."},
	{[]string{"no space left on device"},
		"Out of disk on the runner",
		"Prune caches/images or use a larger runner."},
	{[]string{"out of memory", "oomkilled", "cannot allocate memory", "java.lang.outofmemoryerror"},
		"Out of memory",
		"Use a larger runner, or lower parallelism / heap (e.g. MAVEN_OPTS, NODE_OPTIONS)."},
	{[]string{"could not resolve host", "temporary failure in name resolution", "connection refused",
		"connection timed out", "i/o timeout", "dial tcp", "network is unreachable", "tls handshake timeout"},
		"Network error reaching a remote",
		"Check connectivity, a proxy, or the dependency mirror; retry if transient."},
	{[]string{"permission denied"},
		"Permission denied",
		"Check file modes or the credentials this step uses."},
}

// StageHint is the fallback when no stack/output pattern matches.
func StageHint(stage string) string {
	switch stage {
	case "validate":
		return "A format/lint/schema check failed. Run your formatter and `frodo-ci validate-config`, then re-push."
	case "test":
		return "A test failed. Reproduce with the command below; fix the test or the code."
	case "build":
		return "The build failed — see the output below."
	case "package":
		return "Packaging failed — see the output below (often an image build or path issue)."
	case "scan":
		return "A blocking security finding. Review it; suppressions require an owner, reason, approver, and a future expiry."
	case "publish", "deploy", "verify":
		return "A delivery step failed — check the environment and credentials, then see the output below."
	default:
		return ""
	}
}

// --- shared line helpers ---

// linesMatching returns up to max lines that contain any of the (lowercase)
// substrings, preserving order.
func linesMatching(out string, subs []string, max int) string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		low := strings.ToLower(line)
		for _, s := range subs {
			if strings.Contains(low, s) {
				keep = append(keep, strings.TrimRight(line, "\r"))
				break
			}
		}
		if len(keep) >= max {
			break
		}
	}
	return strings.Join(keep, "\n")
}

// grep returns up to max lines matching re.
func grep(out string, re *regexp.Regexp, max int) []string {
	var keep []string
	for _, line := range strings.Split(out, "\n") {
		if re.MatchString(line) {
			keep = append(keep, strings.TrimRight(line, "\r"))
			if len(keep) >= max {
				break
			}
		}
	}
	return keep
}

// concise distills arbitrary output: error-ish lines if any, else the last
// non-blank lines. Capped for a readable comment.
func concise(out string) string {
	errish := grep(out, regexp.MustCompile(`(?i)error|fail|exception|panic|fatal|cannot|denied|\bE\d{2,}\b`), 20)
	if len(errish) > 0 {
		return strings.Join(errish, "\n")
	}
	var lines []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, "\r"))
		}
	}
	if len(lines) > 15 {
		lines = lines[len(lines)-15:]
	}
	return strings.Join(lines, "\n")
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
