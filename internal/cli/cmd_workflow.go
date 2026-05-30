package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/omarss/frodo-ci/internal/plan"
	"github.com/spf13/cobra"
)

const (
	setupMarkerStart = "# >>> frodo-ci:setup"
	setupMarkerEnd   = "# <<< frodo-ci:setup"
	workflowRelPath  = ".github/workflows/frodo-ci.yml"
)

// setupAction describes how to provision a toolchain via a maintained, SHA-pinned
// setup-* action. extra holds steps appended after the setup step (e.g. corepack
// for pnpm), already indented to the step level.
type setupAction struct {
	order          int
	stepName       string
	uses           string // action@sha # version
	versionKey     string // the `with:` key carrying the version
	defaultVersion string
	distKey        string // optional distribution key (java)
	defaultDist    string
	extra          string
}

// setupActions maps a declared toolchain to its setup action. Tools without an
// entry are reported so the user can add a step manually rather than getting a
// wrong/unpinned action.
var setupActions = map[string]setupAction{
	"java": {order: 0, stepName: "Set up Java", versionKey: "java-version", defaultVersion: "25",
		distKey: "distribution", defaultDist: "liberica",
		uses: "actions/setup-java@be666c2fcd27ec809703dec50e508c2fdc7f6654 # v5.2.0"},
	"node": {order: 1, stepName: "Set up Node", versionKey: "node-version", defaultVersion: "22",
		uses:  "actions/setup-node@48b55a011bda9f5d6aeb4c2d9c7362e8dae4041e # v6.4.0",
		extra: "      - name: Enable pnpm\n        shell: bash\n        run: corepack enable && corepack prepare pnpm@latest --activate"},
	"go": {order: 2, stepName: "Set up Go", versionKey: "go-version", defaultVersion: "1.25",
		uses: "actions/setup-go@4a3601121dd01d1626a1e23e37211e3254c1c06c # v6.4.0"},
}

type toolchain struct {
	tool, version, distribution string
}

func newSyncWorkflowCommand(app *App) *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "sync-workflow",
		Short: "Regenerate the workflow's toolchain setup from modules' setup: blocks",
		Long: "Unions every module's declared `setup:` toolchains, picks the highest version per tool, and " +
			"rewrites the managed setup section of " + workflowRelPath + " using maintained, SHA-pinned setup-* actions.",
		RunE: func(_ *cobra.Command, _ []string) error { return app.runSyncWorkflow(check) },
	}
	cmd.Flags().BoolVar(&check, "check", false, "fail if the workflow is out of sync instead of writing it")
	return cmd
}

func (a *App) runSyncWorkflow(check bool) error {
	loaded, err := plan.Load(a.RepoRoot, time.Now())
	if err != nil {
		return err
	}
	tools, unknown := collectToolchains(loaded)
	for _, u := range unknown {
		fmt.Fprintf(a.Out, "warning: no setup action for toolchain %q — add its setup step manually\n", u)
	}

	path := filepath.Join(a.RepoRoot, filepath.FromSlash(workflowRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s (run `frodo-ci init` first?): %w", workflowRelPath, err)
	}
	updated, err := replaceSetupBlock(string(data), renderSetupBlock(tools))
	if err != nil {
		return err
	}
	if updated == string(data) {
		fmt.Fprintln(a.Out, "workflow already in sync with modules' setup: blocks")
		return nil
	}
	if check {
		return fmt.Errorf("%s is out of sync with modules' setup: blocks — run `frodo-ci sync-workflow`", workflowRelPath)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "updated %s from %d toolchain(s)\n", workflowRelPath, len(tools))
	return nil
}

// collectToolchains unions every module's effective-stage setup: blocks, keeping
// the highest version per tool. It returns the resolved toolchains (in render
// order) and any declared tools with no known setup action.
func collectToolchains(loaded *plan.Loaded) ([]toolchain, []string) {
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
// the job's `steps:`). The block excludes the marker lines themselves.
func renderSetupBlock(tools []toolchain) string {
	header := "      # Toolchains. Regenerate from your modules' `setup:` blocks with\n" +
		"      # `frodo-ci sync-workflow`; everything up to the closing marker is managed."
	if len(tools) == 0 {
		return header + "\n      # (no module declares a setup: toolchain)"
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
		if act.extra != "" {
			b.WriteString("\n" + act.extra)
		}
	}
	return b.String()
}

// replaceSetupBlock swaps the lines between the setup markers, keeping the marker
// lines in place.
func replaceSetupBlock(workflow, block string) (string, error) {
	lines := strings.Split(workflow, "\n")
	start, end := -1, -1
	for i, l := range lines {
		switch strings.TrimSpace(l) {
		case setupMarkerStart:
			if start == -1 {
				start = i
			}
		case setupMarkerEnd:
			if start != -1 && end == -1 {
				end = i
			}
		}
	}
	if start == -1 || end == -1 || end < start {
		return "", fmt.Errorf("setup markers not found in %s — re-run `frodo-ci init` to refresh it", workflowRelPath)
	}
	out := make([]string, 0, len(lines))
	out = append(out, lines[:start+1]...)
	out = append(out, strings.Split(block, "\n")...)
	out = append(out, lines[end:]...)
	return strings.Join(out, "\n"), nil
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
