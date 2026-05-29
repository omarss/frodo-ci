package plan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/frodo-ci/frodo-ci/internal/cache"
	"github.com/frodo-ci/frodo-ci/internal/config"
	"github.com/frodo-ci/frodo-ci/internal/configlint"
	"github.com/frodo-ci/frodo-ci/internal/discover"
	"github.com/frodo-ci/frodo-ci/internal/fingerprint"
	"github.com/frodo-ci/frodo-ci/internal/graph"
	"github.com/frodo-ci/frodo-ci/internal/match"
	"github.com/frodo-ci/frodo-ci/internal/schema"
	"github.com/frodo-ci/frodo-ci/internal/templates"
	"github.com/frodo-ci/frodo-ci/internal/vcs"
)

// RootConfigPath is the repo-relative path to the root config.
const RootConfigPath = ".github/frodo-ci.yml"

// kindedSource pairs a config source with its schema kind for validation.
type kindedSource struct {
	kind schema.Kind
	src  *config.Source
}

// Catalog holds the loaded framework catalog used for fingerprints and linting.
type Catalog struct {
	ToolchainsBytes         []byte
	Toolchains              *config.Toolchains
	SecurityBaseline        *config.SecurityBaseline
	SecurityBaselineVersion int
	Suppressions            *config.Suppressions
	SuppressionsPath        string
	LintRules               *config.LintRules
	LintRulesVersion        int
	TemplatesDir            string
	sources                 []kindedSource
}

// TemplateBytes returns the raw template file for a profile, or nil.
func (c *Catalog) TemplateBytes(profile string) []byte {
	if profile == "" || c.TemplatesDir == "" {
		return nil
	}
	b, _ := os.ReadFile(filepath.Join(c.TemplatesDir, profile+".yml"))
	return b
}

// Loaded is the validated, linted repository state from which plans are derived.
type Loaded struct {
	RepoRoot         string
	Root             *config.RootConfig
	RootSource       *config.Source
	Modules          []*discover.Module
	ByName           map[string]*discover.Module
	Graph            *graph.Graph
	Catalog          *Catalog
	Problems         []configlint.Problem // schema + semantic, combined
	SchemaProblems   []configlint.Problem
	SemanticProblems []configlint.Problem

	tmpl *templates.Loader
	eff  map[string]map[string]templates.EffectiveStage

	files    []string
	filesSet bool
	modFiles map[string][]string
	modFP    map[string]string
}

// EffectiveStages returns the module's stages with template defaults merged in,
// keyed by stage name. The result is cached per module.
func (l *Loaded) EffectiveStages(m *discover.Module) map[string]templates.EffectiveStage {
	if es, ok := l.eff[m.Name]; ok {
		return es
	}
	t, _ := l.tmpl.Get(profileOf(m))
	es := templates.Resolve(t, m)
	l.eff[m.Name] = es
	return es
}

// effStage returns the merged effective stage for a module, if it has one.
func (l *Loaded) effStage(m *discover.Module, stage string) (templates.EffectiveStage, bool) {
	es, ok := l.EffectiveStages(m)[stage]
	return es, ok
}

// stagePatterns resolves a stage's effective when/inputs to repo-relative
// globs/regexes. ok is false when the module has no such stage.
func (l *Loaded) stagePatterns(m *discover.Module, stage string) (globs []string, regexes []*regexp.Regexp, inputGlobs []string, ok bool) {
	es, ok := l.effStage(m, stage)
	if !ok {
		return nil, nil, nil, false
	}
	globs, regexes = resolveMatchers(m.Dir, es.When)
	inputGlobs, _ = match.ResolveAll(m.Dir, es.Inputs)
	return globs, regexes, inputGlobs, true
}

// Load performs the deterministic startup pipeline up to and including config
// linting: load root, discover modules, validate schemas, lint semantics, load
// the catalog, and build the dependency graph.
func Load(repoRoot string, now time.Time) (*Loaded, error) {
	rootPath := filepath.Join(repoRoot, filepath.FromSlash(RootConfigPath))
	root, rootSrc, err := config.LoadRoot(rootPath)
	if err != nil {
		return nil, err
	}
	root.ApplyDefaults()

	cat := loadCatalog(repoRoot, root)

	modules, err := discover.Discover(repoRoot, root)
	if err != nil {
		return nil, err
	}
	g, gproblems := graph.Build(modules, graph.DefaultLimits())

	l := &Loaded{
		RepoRoot:   repoRoot,
		Root:       root,
		RootSource: rootSrc,
		Modules:    modules,
		ByName:     discover.ByName(modules),
		Graph:      g,
		Catalog:    cat,
		tmpl:       templates.NewLoader(cat.TemplatesDir),
		eff:        map[string]map[string]templates.EffectiveStage{},
		modFiles:   map[string][]string{},
		modFP:      map[string]string{},
	}

	l.SchemaProblems = l.schemaProblems()
	l.SemanticProblems = configlint.Check(configlint.Input{
		Root:             root,
		Modules:          modules,
		Graph:            g,
		GraphProblems:    gproblems,
		Toolchains:       cat.Toolchains,
		Suppressions:     cat.Suppressions,
		SuppressionsPath: relTo(repoRoot, cat.SuppressionsPath),
		SecurityBaseline: cat.SecurityBaseline,
		LintRules:        cat.LintRules,
		Now:              now,
	})
	l.Problems = append(append([]configlint.Problem{}, l.SchemaProblems...), l.SemanticProblems...)
	return l, nil
}

// loadCatalog loads the optional framework catalog files. Missing files are not
// errors here; schema validation and linting flag malformed ones.
func loadCatalog(repoRoot string, root *config.RootConfig) *Catalog {
	c := &Catalog{TemplatesDir: filepath.Join(repoRoot, filepath.FromSlash(root.Templates.Path))}
	base := filepath.Join(repoRoot, ".github", "frodo-ci")

	if tc, src, err := config.LoadToolchains(filepath.Join(base, "toolchains.yml")); err == nil {
		c.Toolchains, c.ToolchainsBytes = tc, src.Bytes
		c.sources = append(c.sources, kindedSource{schema.KindToolchains, src})
	}
	if sb, src, err := config.LoadSecurityBaseline(filepath.Join(base, "security", "baseline.yml")); err == nil {
		c.SecurityBaseline, c.SecurityBaselineVersion = sb, sb.Version
		c.sources = append(c.sources, kindedSource{schema.KindSecurity, src})
	}
	if sup, src, err := config.LoadSuppressions(filepath.Join(base, "security", "suppressions.yml")); err == nil {
		c.Suppressions, c.SuppressionsPath = sup, src.Path
		c.sources = append(c.sources, kindedSource{schema.KindSuppressions, src})
	}
	if lr, src, err := config.LoadLintRules(filepath.Join(base, "lint", "rules.yml")); err == nil {
		c.LintRules, c.LintRulesVersion = lr, lr.Version
		c.sources = append(c.sources, kindedSource{schema.KindLint, src})
	}
	if pb, src, err := config.LoadPerformanceBudgets(filepath.Join(base, "performance", "budgets.yml")); err == nil {
		_ = pb
		c.sources = append(c.sources, kindedSource{schema.KindPerformance, src})
	}
	return c
}

// schemaProblems validates every loaded config source against its schema.
func (l *Loaded) schemaProblems() []configlint.Problem {
	v := schema.NewValidator()
	var ps []configlint.Problem
	validate := func(kind schema.Kind, src *config.Source) {
		if src == nil {
			return
		}
		if err := v.ValidateBytes(kind, relTo(l.RepoRoot, src.Path), src.Bytes); err != nil {
			var fe *schema.FriendlyError
			if errors.As(err, &fe) {
				for _, m := range fe.Messages {
					ps = append(ps, configlint.Problem{Severity: configlint.Error, Path: fe.Path, Message: m})
				}
			} else {
				ps = append(ps, configlint.Problem{Severity: configlint.Error, Path: relTo(l.RepoRoot, src.Path), Message: err.Error()})
			}
		}
	}
	validate(schema.KindRoot, l.RootSource)
	for _, m := range l.Modules {
		validate(schema.KindModule, m.Source)
		for _, sr := range m.Stages {
			validate(schema.KindStage, sr.Source)
		}
	}
	for _, ks := range l.Catalog.sources {
		validate(ks.kind, ks.src)
	}
	return ps
}

// Plan computes the execution plan for the given context against a cache.
func (l *Loaded) Plan(ctx Context, c cache.Cache) (*Plan, error) {
	changes, err := l.changedFiles(ctx)
	if err != nil {
		return nil, err
	}
	p := &Plan{RepoRoot: l.RepoRoot, Context: ctx, Changes: changes, Problems: l.Problems}

	for _, m := range l.Modules {
		mp := &ModulePlan{Name: m.Name, Dir: m.Dir, Owners: ownersOf(m)}
		if fp, err := l.moduleFingerprint(m); err == nil {
			mp.Fingerprint = fp
		}
		for _, stage := range l.Root.AllStages() {
			if !l.stageApplicable(m, stage, ctx) {
				continue
			}
			reasons, matched := l.stageTriggers(m, stage, ctx, changes)
			if len(reasons) == 0 {
				continue
			}
			sfp, _, err := l.StageFingerprint(m, stage)
			if err != nil {
				return nil, err
			}
			sp := &StagePlan{
				Module:       m.Name,
				Stage:        stage,
				Group:        l.group(stage),
				Reasons:      reasons,
				Fingerprint:  sfp,
				Cacheable:    IsCacheable(stage),
				Expensive:    IsExpensive(stage),
				MatchedFiles: matched,
				TimeoutMin:   l.stageTimeout(m, stage),
				Budget:       m.Config.Performance.Budgets[stage],
			}
			if sp.Cacheable && l.Root.Minutes.SkipUnchanged {
				if ok, _ := c.Has(sfp); ok {
					sp.Cached = true
					sp.Skipped = l.Root.Minutes.ExactFingerprintMatchOnly
				}
			}
			mp.Stages = append(mp.Stages, sp)
		}
		if len(mp.Stages) > 0 {
			p.Modules = append(p.Modules, mp)
		}
	}
	return p, nil
}

// StageFingerprint computes a stage's deterministic fingerprint and the file
// surface that fed it.
func (l *Loaded) StageFingerprint(m *discover.Module, stage string) (string, []string, error) {
	globs, regexes, inputGlobs, _ := l.stagePatterns(m, stage)
	if len(globs) == 0 && len(regexes) == 0 && len(inputGlobs) == 0 {
		globs = []string{m.Dir + "/**"} // default surface: the whole module
	}

	files := matchFiles(globs, regexes, l.moduleFilesOf(m))
	if len(inputGlobs) > 0 {
		files = append(files, match.MatchingPaths(inputGlobs, l.repoFiles())...)
	}
	files = append(files, l.lockfilesFor(m)...)

	var stageBytes []byte
	if sr, ok := m.Stages[stage]; ok {
		stageBytes = sr.Source.Bytes
	}

	var depFPs []string
	for _, e := range l.Graph.Dependencies(m.Name) {
		if !containsStr(e.Affects, stage) {
			continue
		}
		if dep := l.ByName[e.To]; dep != nil {
			if fp, err := l.moduleFingerprint(dep); err == nil {
				depFPs = append(depFPs, fp)
			}
		}
	}

	fp, err := fingerprint.Compute(l.RepoRoot, fingerprint.StageInputs{
		RootConfig:              l.RootSource.Bytes,
		Toolchains:              l.Catalog.ToolchainsBytes,
		SecurityBaselineVersion: l.Catalog.SecurityBaselineVersion,
		LintRulesVersion:        l.Catalog.LintRulesVersion,
		ModuleConfig:            m.Source.Bytes,
		StageFile:               stageBytes,
		Template:                l.Catalog.TemplateBytes(profileOf(m)),
		Files:                   files,
		DependencyFingerprints:  depFPs,
	})
	return fp, files, err
}

// stageApplicable reports whether a stage could run for a module in this context.
func (l *Loaded) stageApplicable(m *discover.Module, stage string, ctx Context) bool {
	es, ok := l.effStage(m, stage)
	if !ok {
		return false // the module's template/config does not define this stage
	}
	if l.group(stage) == "cd" {
		if !(ctx.OnDefaultBranch || ctx.Event == "workflow_dispatch") {
			return false
		}
		if len(es.Environments) > 0 && !containsStr(es.Environments, ctx.Environment) {
			return false
		}
	}
	return true
}

// stageTriggers returns the reasons a stage must run and the changed files that
// matched it. An empty reasons slice means the stage is not triggered.
func (l *Loaded) stageTriggers(m *discover.Module, stage string, ctx Context, changes []string) (reasons, matched []string) {
	globs, regexes, inputGlobs, _ := l.stagePatterns(m, stage)
	usingDefault := len(globs) == 0 && len(regexes) == 0 && len(inputGlobs) == 0
	if usingDefault {
		globs = []string{m.Dir + "/**"}
	}

	for _, f := range changes {
		if match.GlobAny(globs, f) || match.GlobAny(inputGlobs, f) || matchesRegex(regexes, f) {
			matched = append(matched, f)
		}
	}
	if len(matched) > 0 {
		if usingDefault {
			reasons = append(reasons, fmt.Sprintf("files changed under %s", m.Dir))
		} else {
			reasons = append(reasons, fmt.Sprintf("changed files match %s.%s", l.group(stage), stage))
		}
	}
	for _, e := range l.Graph.Dependencies(m.Name) {
		if containsStr(e.Affects, stage) && l.moduleChanged(e.To, changes) {
			reasons = append(reasons, fmt.Sprintf("dependency %s changed (affects %s)", e.To, stage))
		}
	}
	if stage == "scan" && (ctx.Event == "schedule" || ctx.OnDefaultBranch) {
		reasons = append(reasons, "full vulnerability scan (push to main or scheduled run)")
	}
	return reasons, matched
}

func (l *Loaded) moduleChanged(name string, changes []string) bool {
	dep := l.ByName[name]
	if dep == nil {
		return false
	}
	prefix := dep.Dir + "/"
	for _, f := range changes {
		if f == dep.Dir || strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// FindModule returns a discovered module by name.
func (l *Loaded) FindModule(name string) *discover.Module { return l.ByName[name] }

// Explanation is one module/stage that a file affects, and why.
type Explanation struct {
	Module string `json:"module"`
	Stage  string `json:"stage"`
	Group  string `json:"group"`
	Reason string `json:"reason"`
}

// Explain reports which module stages a repo-relative file affects: directly via
// when/inputs, and indirectly via dependents whose affects include a stage.
func (l *Loaded) Explain(file string) []Explanation {
	file = filepath.ToSlash(file)
	var out []Explanation
	for _, m := range l.Modules {
		for _, stage := range l.Root.AllStages() {
			globs, regexes, inputGlobs, ok := l.stagePatterns(m, stage)
			if !ok {
				continue // module's template/config does not define this stage
			}
			noPatterns := len(globs) == 0 && len(regexes) == 0 && len(inputGlobs) == 0
			under := file == m.Dir || strings.HasPrefix(file, m.Dir+"/")
			switch {
			case match.GlobAny(globs, file) || matchesRegex(regexes, file):
				out = append(out, Explanation{m.Name, stage, l.group(stage),
					fmt.Sprintf("matches %s.%s.when", l.group(stage), stage)})
			case match.GlobAny(inputGlobs, file):
				out = append(out, Explanation{m.Name, stage, l.group(stage),
					fmt.Sprintf("matches %s.%s.inputs", l.group(stage), stage)})
			case noPatterns && under:
				out = append(out, Explanation{m.Name, stage, l.group(stage),
					fmt.Sprintf("is under %s (default trigger)", m.Dir)})
			}
		}
	}
	for _, m := range l.Modules {
		if file != m.Dir && !strings.HasPrefix(file, m.Dir+"/") {
			continue
		}
		for _, e := range l.Graph.DependentsOf(m.Name) {
			for _, stage := range e.Affects {
				out = append(out, Explanation{e.From, stage, l.group(stage),
					fmt.Sprintf("%s depends on %s (affects %s)", e.From, m.Name, stage)})
			}
		}
	}
	return out
}

func (l *Loaded) changedFiles(ctx Context) ([]string, error) {
	g := vcs.New(l.RepoRoot)
	if !g.Available() {
		return nil, nil
	}
	return g.Resolve(vcs.Changes{Base: ctx.Base, Head: ctx.Head, ThreeDot: ctx.ThreeDot})
}

func (l *Loaded) repoFiles() []string {
	if l.filesSet {
		return l.files
	}
	g := vcs.New(l.RepoRoot)
	if g.Available() {
		if files, err := g.ListFiles(); err == nil {
			l.files = files
			l.filesSet = true
			return l.files
		}
	}
	l.files = walkFiles(l.RepoRoot)
	l.filesSet = true
	return l.files
}

func (l *Loaded) moduleFilesOf(m *discover.Module) []string {
	if f, ok := l.modFiles[m.Name]; ok {
		return f
	}
	prefix := m.Dir + "/"
	var out []string
	for _, f := range l.repoFiles() {
		if strings.HasPrefix(f, prefix) {
			out = append(out, f)
		}
	}
	l.modFiles[m.Name] = out
	return out
}

func (l *Loaded) moduleFingerprint(m *discover.Module) (string, error) {
	if fp, ok := l.modFP[m.Name]; ok {
		return fp, nil
	}
	filesDigest, err := fingerprint.FilesDigest(l.RepoRoot, l.moduleFilesOf(m))
	if err != nil {
		return "", err
	}
	fp := fingerprint.New().Field("module-config", m.Source.Bytes).Text("files", filesDigest).Sum()
	l.modFP[m.Name] = fp
	return fp, nil
}

var lockfileNames = map[string]bool{
	"pnpm-lock.yaml": true, "package-lock.json": true, "yarn.lock": true,
	"go.sum": true, "Cargo.lock": true, "poetry.lock": true, "Gemfile.lock": true,
}

func (l *Loaded) lockfilesFor(m *discover.Module) []string {
	var out []string
	prefix := m.Dir + "/"
	for _, f := range l.repoFiles() {
		base := pathBase(f)
		if !lockfileNames[base] {
			continue
		}
		if strings.HasPrefix(f, prefix) || !strings.Contains(f, "/") {
			out = append(out, f)
		}
	}
	return out
}

func (l *Loaded) group(stage string) string {
	if l.Root.IsCDStage(stage) {
		return "cd"
	}
	return "ci"
}

func (l *Loaded) stageTimeout(m *discover.Module, stage string) int {
	if sr, ok := m.Stages[stage]; ok {
		return sr.File.TimeoutMinutes
	}
	return 0
}

// --- helpers ---

func resolveMatchers(dir string, ms []config.Matcher) (globs []string, regexes []*regexp.Regexp) {
	for _, w := range ms {
		if w.IsRegex() {
			if re, err := regexp.Compile(w.Regex); err == nil {
				regexes = append(regexes, re)
			}
			continue
		}
		if g, _ := match.Resolve(dir, w.Glob); g != "" {
			globs = append(globs, g)
		}
	}
	return globs, regexes
}

func matchFiles(globs []string, regexes []*regexp.Regexp, files []string) []string {
	var out []string
	for _, f := range files {
		if match.GlobAny(globs, f) || matchesRegex(regexes, f) {
			out = append(out, f)
		}
	}
	return out
}

func matchesRegex(regexes []*regexp.Regexp, p string) bool {
	for _, re := range regexes {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

func profileOf(m *discover.Module) string {
	if m.Config.Use.Profile != "" {
		return m.Config.Use.Profile
	}
	return m.Config.Type
}

func ownersOf(m *discover.Module) []string {
	out := append([]string{}, m.Config.Owners.Teams...)
	out = append(out, m.Config.Owners.Users...)
	sort.Strings(out)
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func relTo(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

func pathBase(p string) string {
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// walkFiles lists repo-relative files when git is unavailable, skipping heavy
// directories.
func walkFiles(root string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "target", "dist", "build", "vendor", ".next", "out":
				return filepath.SkipDir
			}
			return nil
		}
		if rel, err := filepath.Rel(root, p); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}
