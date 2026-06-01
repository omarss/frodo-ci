package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/omarss/frodo-ci/internal/config"
	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/spf13/cobra"
)

const (
	setupMarkerStart = "# >>> frodo-ci:setup"
	setupMarkerEnd   = "# <<< frodo-ci:setup"
	envMarkerStart   = "# >>> frodo-ci:env"
	envMarkerEnd     = "# <<< frodo-ci:env"
	regMarkerStart   = "# >>> frodo-ci:registries"
	regMarkerEnd     = "# <<< frodo-ci:registries"
	workflowRelPath  = ".github/workflows/frodo-ci.yml"

	// Pinned auth actions for the generated registry-login steps.
	gcpAuthAction     = "google-github-actions/auth@7c6bc770dae815cd3e89ee6cdf493a5fab2cc093 # v3"
	dockerLoginAction = "docker/login-action@650006c6eb7dba73a995cc03b0b2d7f5ca915bee # v4.2.0"
)

// setupAction describes how to provision a toolchain via a maintained, SHA-pinned
// setup-* action. Node additionally gets a corepack/pnpm step appended, rendered
// with the repo's resolved pnpm version.
type setupAction struct {
	order          int
	stepName       string
	uses           string // action@sha # version
	versionKey     string // the `with:` key carrying the version
	defaultVersion string
	distKey        string // optional distribution key (java)
	defaultDist    string
}

// setupActions maps a declared toolchain to its setup action. Tools without an
// entry are reported so the user can add a step manually rather than getting a
// wrong/unpinned action.
var setupActions = map[string]setupAction{
	"java": {order: 0, stepName: "Set up Java", versionKey: "java-version", defaultVersion: "25",
		distKey: "distribution", defaultDist: "liberica",
		uses: "actions/setup-java@be666c2fcd27ec809703dec50e508c2fdc7f6654 # v5.2.0"},
	"node": {order: 1, stepName: "Set up Node", versionKey: "node-version", defaultVersion: "22",
		uses: "actions/setup-node@48b55a011bda9f5d6aeb4c2d9c7362e8dae4041e # v6.4.0"},
	"go": {order: 2, stepName: "Set up Go", versionKey: "go-version", defaultVersion: "1.25",
		uses: "actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0"},
}

type toolchain struct {
	tool, version, distribution string
}

func newSyncWorkflowCommand(app *App) *cobra.Command {
	var check bool
	var actionRef string
	cmd := &cobra.Command{
		Use:   "sync-workflow",
		Short: "Regenerate the workflow's toolchain setup (and optionally the action ref)",
		Long: "Regenerates the managed setup section of " + workflowRelPath + " from repo metadata and modules' " +
			"`setup:` blocks using SHA-pinned setup-* actions. With --action-ref it also pins the Frodo CI " +
			"action `uses:` ref, so the version can be bumped via CLI and drift-checked with --check.",
		RunE: func(_ *cobra.Command, _ []string) error { return app.runSyncWorkflow(check, actionRef) },
	}
	cmd.Flags().BoolVar(&check, "check", false, "fail if the workflow is out of sync instead of writing it")
	cmd.Flags().StringVar(&actionRef, "action-ref", "", "pin the Frodo CI action `uses:` ref (e.g. omarss/frodo-ci@v1.10.0)")
	return cmd
}

func (a *App) runSyncWorkflow(check bool, actionRef string) error {
	path, original, updated, unknown, err := a.renderManagedWorkflow()
	if err != nil {
		return err
	}
	for _, u := range unknown {
		fmt.Fprintf(a.Out, "warning: no setup action for toolchain %q — add its setup step manually\n", u)
	}
	updated = replaceActionRef(updated, actionRef)
	if updated == original {
		fmt.Fprintln(a.Out, "workflow already in sync")
		return nil
	}
	if check {
		return fmt.Errorf("%s is out of sync — run `frodo-ci sync-workflow`", workflowRelPath)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "updated %s\n", workflowRelPath)
	return nil
}

// reFrodoAction matches the Frodo CI action reference line
// (uses: <owner>/frodo-ci@<ref>), capturing the "uses:" prefix.
var reFrodoAction = regexp.MustCompile(`(?m)^(\s*uses:\s*)\S*/frodo-ci@\S+`)

// replaceActionRef pins the Frodo CI action `uses:` ref to the given value,
// leaving the rest of the workflow (and other actions) untouched. An empty ref
// is a no-op.
func replaceActionRef(workflow, ref string) string {
	if ref == "" {
		return workflow
	}
	return reFrodoAction.ReplaceAllString(workflow, "${1}"+ref)
}

// renderManagedWorkflow regenerates the workflow's setup block from repo metadata
// and module setup: blocks, returning the workflow path, its current content, the
// regenerated content, and any toolchains with no known setup action.
func (a *App) renderManagedWorkflow() (path, original, updated string, unknown []string, err error) {
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return "", "", "", nil, err
	}
	detected := detectToolchains(a.RepoRoot)
	tools, unk := collectToolchains(loaded, detected)
	path = filepath.Join(a.RepoRoot, filepath.FromSlash(workflowRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return path, "", "", unk, fmt.Errorf("read %s (run `frodo-ci init` first?): %w", workflowRelPath, err)
	}
	upd, err := replaceSetupBlock(string(data), renderSetupBlock(tools, detected.pnpm))
	if err != nil {
		return path, string(data), "", unk, err
	}
	// The env block is optional: older workflows may lack its markers, in which
	// case job-level env is simply not managed (no error).
	if strings.Contains(upd, envMarkerStart) {
		upd, err = replaceEnvBlock(upd, renderEnvBlock(loaded.Root.Workflow.Env))
		if err != nil {
			return path, string(data), "", unk, err
		}
	}
	if strings.Contains(upd, regMarkerStart) {
		upd, err = replaceRegistriesBlock(upd, renderRegistriesBlock(loaded.Root.Registries))
		if err != nil {
			return path, string(data), "", unk, err
		}
	}
	return path, string(data), upd, unk, err
}

// syncWorkflowAfterInit best-effort regenerates the setup block so a fresh init
// already reflects the repo's toolchains; failures are non-fatal.
func (a *App) syncWorkflowAfterInit() {
	path, original, updated, _, err := a.renderManagedWorkflow()
	if err != nil || updated == original {
		return
	}
	_ = os.WriteFile(path, []byte(updated), 0o644)
}

// collectToolchains unions every module's effective-stage setup: blocks (keeping
// the highest version per tool), then lets the repo's own metadata win: a
// detected version overrides the template default, and a toolchain the repo
// clearly uses (package.json / pom.xml / go.mod) is included even if no module
// declared it. It returns the resolved toolchains (in render order) and any
// declared tools with no known setup action.
func collectToolchains(loaded *plan.Loaded, detected detectedToolchains) ([]toolchain, []string) {
	best := map[string]toolchain{}
	unknownSet := map[string]bool{}
	for _, m := range loaded.Modules {
		for _, es := range loaded.EffectiveStages(m) {
			for tool, st := range es.Setup {
				if _, ok := setupActions[tool]; !ok {
					unknownSet[tool] = true
					continue
				}
				v := st.Version.String()
				cur, seen := best[tool]
				if !seen || versionLess(cur.version, v) {
					dist := st.Distribution
					if dist == "" && seen {
						dist = cur.distribution
					}
					best[tool] = toolchain{tool: tool, version: v, distribution: dist}
				}
			}
		}
	}
	// Repo metadata wins: add detected toolchains and override versions with the
	// repo's real requirement (e.g. engines.node), not the template default.
	for tool, ver := range detected.tools {
		if _, ok := setupActions[tool]; !ok {
			continue
		}
		tc := best[tool] // zero value if absent
		tc.tool = tool
		if ver != "" {
			tc.version = ver
		}
		best[tool] = tc
	}
	tools := make([]toolchain, 0, len(best))
	for _, t := range best {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool {
		return setupActions[tools[i].tool].order < setupActions[tools[j].tool].order
	})
	unknown := make([]string, 0, len(unknownSet))
	for u := range unknownSet {
		unknown = append(unknown, u)
	}
	sort.Strings(unknown)
	return tools, unknown
}

// renderSetupBlock renders the managed setup steps (6-space indented to sit under
// the job's `steps:`). The block excludes the marker lines themselves. Node also
// gets a corepack/pnpm step pinned to the repo's resolved pnpm version.
func renderSetupBlock(tools []toolchain, pnpmVersion string) string {
	header := "      # Toolchains, resolved from repo metadata (engines/.nvmrc/pom.xml/go.mod)\n" +
		"      # and your modules' `setup:` blocks. Regenerate with `frodo-ci sync-workflow`;\n" +
		"      # everything up to the closing marker is managed."
	if len(tools) == 0 {
		return header + "\n      # (no toolchain detected or declared)"
	}
	if pnpmVersion == "" {
		pnpmVersion = "latest"
	}
	var b strings.Builder
	b.WriteString(header)
	for _, t := range tools {
		act := setupActions[t.tool]
		ver := t.version
		if ver == "" {
			ver = act.defaultVersion
		}
		fmt.Fprintf(&b, "\n      - name: %s\n        uses: %s\n        with:\n", act.stepName, act.uses)
		if act.distKey != "" {
			dist := t.distribution
			if dist == "" {
				dist = act.defaultDist
			}
			fmt.Fprintf(&b, "          %s: %s\n", act.distKey, dist)
		}
		fmt.Fprintf(&b, "          %s: \"%s\"", act.versionKey, ver)
		if t.tool == "node" {
			fmt.Fprintf(&b, "\n      - name: Enable pnpm\n        shell: bash\n"+
				"        run: corepack enable && corepack prepare pnpm@%s --activate", pnpmVersion)
		}
	}
	return b.String()
}

func replaceSetupBlock(workflow, block string) (string, error) {
	return replaceMarkedBlock(workflow, setupMarkerStart, setupMarkerEnd, block)
}

func replaceEnvBlock(workflow, block string) (string, error) {
	return replaceMarkedBlock(workflow, envMarkerStart, envMarkerEnd, block)
}

func replaceRegistriesBlock(workflow, block string) (string, error) {
	return replaceMarkedBlock(workflow, regMarkerStart, regMarkerEnd, block)
}

// renderRegistriesBlock renders the managed registry-login steps (6-space
// indented, step level) from root config `registries:`, using SHA-pinned auth
// actions. Credentials are referenced from repo vars/secrets, never inlined.
func renderRegistriesBlock(regs []config.Registry) string {
	header := "      # Registry logins from root config `registries:`; managed by `frodo-ci sync-workflow`."
	if len(regs) == 0 {
		return header + "\n      # (no registry declared)"
	}
	var b strings.Builder
	b.WriteString(header)
	for _, r := range regs {
		switch r.Auth {
		case "gcp-wif":
			fmt.Fprintf(&b, "\n      - name: Authenticate to %s\n        uses: %s\n        with:\n"+
				"          workload_identity_provider: ${{ vars.%s }}\n          service_account: ${{ vars.%s }}\n"+
				"      - name: Configure Docker for %s\n        shell: bash\n        run: gcloud auth configure-docker %s --quiet",
				r.Host, gcpAuthAction, r.WorkloadIdentityProviderVar, r.ServiceAccountVar, r.Host, r.Host)
		case "ghcr":
			fmt.Fprintf(&b, "\n      - name: Log in to %s\n        uses: %s\n        with:\n"+
				"          registry: %s\n          username: ${{ github.actor }}\n          password: ${{ secrets.GITHUB_TOKEN }}",
				r.Host, dockerLoginAction, r.Host)
		case "docker":
			fmt.Fprintf(&b, "\n      - name: Log in to %s\n        uses: %s\n        with:\n"+
				"          registry: %s\n          username: ${{ secrets.%s }}\n          password: ${{ secrets.%s }}",
				r.Host, dockerLoginAction, r.Host, r.UsernameVar, r.PasswordVar)
		default:
			fmt.Fprintf(&b, "\n      # registry %s: unsupported auth %q — add its login step manually", r.Host, r.Auth)
		}
	}
	return b.String()
}

// replaceMarkedBlock swaps the lines between a marker pair, keeping the marker
// lines in place.
func replaceMarkedBlock(workflow, startMarker, endMarker, block string) (string, error) {
	lines := strings.Split(workflow, "\n")
	start, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case startMarker:
			if start == -1 {
				start = i
			}
		case endMarker:
			if start != -1 && end == -1 {
				end = i
			}
		}
	}
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("markers %q/%q not found in %s — re-run `frodo-ci init` to refresh it", startMarker, endMarker, workflowRelPath)
	}
	out := make([]string, 0, len(lines))
	out = append(out, lines[:start+1]...)
	out = append(out, strings.Split(block, "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
}

// renderEnvBlock renders the managed job-level env (4-space indented to sit under
// the job), from root config `workflow.env:`. Empty config leaves an inert
// comment so the block round-trips.
func renderEnvBlock(env map[string]config.FlexStr) string {
	if len(env) == 0 {
		return "    # Job-level env from root config `workflow.env:`; managed by `frodo-ci sync-workflow`."
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("    env:")
	for _, k := range keys {
		fmt.Fprintf(&b, "\n      %s: \"%s\"", k, env[k].String())
	}
	return b.String()
}

// versionLess reports whether version a is lower than b, comparing dotted numeric
// components (so "21" < "25" and "1.22" < "1.25"); non-numeric parts compare as 0.
func versionLess(a, b string) bool {
	pa, pb := parseVer(a), parseVer(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x < y
		}
	}
	return false
}

func parseVer(s string) []int {
	parts := strings.Split(s, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, _ := strconv.Atoi(strings.TrimFunc(p, func(r rune) bool { return r < '0' || r > '9' }))
		out = append(out, n)
	}
	return out
}
