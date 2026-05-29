# Frodo CI

**Opinionated modular CI/CD framework for monorepos.**

Frodo CI lets every module own its local automation under `.ci`, while the
platform keeps the whole repository fast, secure, reviewable, and predictable
through **one** standard workflow and **one** required merge check.

> Core principle: **run the minimum necessary work, but never skip risk.**

## What Frodo CI gives you

- One GitHub workflow: `.github/workflows/frodo-ci.yml`
- One required PR check: `Frodo CI / final`
- Dynamic module/stage planning at startup (deterministic)
- Decentralized module automation under `.ci`, with central quality profiles
- Exact fingerprint-based stage skipping (cache never skips review/security/policy)
- Built-in templates, smart security scanning, performance budgets
- Owner and expert reviewer enforcement, anti-weakening checks
- Slack notifications for actionable failures
- JSON Schema autocomplete and human-friendly config linting

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/omarss/frodo-ci/main/install.sh | bash
frodo-ci doctor
```

Or build from source (Go 1.25+):

```bash
make build      # -> ./bin/frodo-ci
make install    # -> $GOBIN/frodo-ci
```

## Bootstrap a repository

```bash
frodo-ci init
```

Creates (idempotently; `--force` to overwrite):

```text
.github/workflows/frodo-ci.yml      # the only workflow you need
.github/frodo-ci.yml                # root config
.github/frodo-ci/schemas/           # 7 JSON Schemas (editor autocomplete)
.github/frodo-ci/templates/         # 7 module templates
.github/frodo-ci/toolchains.yml     # central formatters/linters
.github/frodo-ci/security/          # baseline, suppressions, rulesets
.github/frodo-ci/lint/rules.yml
.github/frodo-ci/performance/budgets.yml
.vscode/settings.json
```

## Add a module

```bash
frodo-ci init-module --name cards --type spring-service --path services/cards --owner cards-team
```

A module only declares what differs from its template:

```yaml
# services/cards/.ci/module.yml
name: cards
type: spring-service
use:
  profile: spring-service
owners:
  teams: [cards-team]
depends_on:
  - module: money
    affects: [test, build, package, scan]
```

Optional `.ci/<stage>.yml` files override a stage's steps when the template is
not enough.

## Commands

```bash
frodo-ci validate-config      # structural validation against the JSON Schemas
frodo-ci lint-config          # semantic linting (cycles, broad inputs, weakening, ...)
frodo-ci plan                 # what will run, and why (add --json)
frodo-ci explain <file>       # which modules/stages a file affects, and why
frodo-ci fingerprint cards.test   # the deterministic fingerprint for a stage
frodo-ci run                  # calculate the plan and execute it (the final check)
frodo-ci ci | cd              # run only CI or only CD stages
frodo-ci review               # evaluate review/owner/expert requirements
frodo-ci doctor               # environment + config health
frodo-ci schemas export       # (re)write the JSON Schemas
frodo-ci templates list|explain <name>
```

## How planning works

At the start of every run Frodo CI calculates the full plan before any expensive
setup, deterministically:

1. load and **validate** the root config (invalid config fails fast)
2. discover modules and **lint** semantics
3. load templates and toolchains; build the dependency graph
4. detect changed files and map them to module stages
5. propagate dependency-affected stages via `affects`
6. compute per-stage fingerprints and consult the cache
7. execute only the required, uncached stages

The plan is deterministic: the same repository state, config, templates,
toolchains, and base/head refs always produce the same plan.

## The final check

Frodo CI runs as a single GitHub job (`final`) that orchestrates everything
**inside the runner** with bounded parallelism and strict timeouts:

- per-stage, full-run, and no-progress timeouts
- `stop_module_on_stage_failure`, `stop_dependents_on_dependency_failure`,
  `stop_expensive_stages_after_validation_failure`
- the cache may skip `validate`/`test`/`build`/`package` on an exact fingerprint
  match, but **never** `scan` (security), CD stages, reviews, or policy checks

**Never use `always()`** in the workflow or steps — cleanup, summaries, check
finalization, and reporting all happen in normal control flow.

## Branch protection

Require only this status check:

```text
Frodo CI / final
```

Do not require the dynamic module/stage checks individually; Frodo CI tracks them
internally and makes the final check pass or fail.

## Development

```bash
make build      # compile
make test       # race tests
make lint       # golangci-lint (or go vet)
make fmt        # gofmt
make schemas    # regenerate JSON Schemas
```

## Implementation status

This repository implements the deterministic core end to end:

- ✅ Config domain + position-aware YAML loading
- ✅ JSON Schema generation + validation with did-you-mean diagnostics
- ✅ Module discovery, git change detection, glob/regex matching
- ✅ Dependency graph + integrity checks (cycles, fan-out, broad/escaping paths)
- ✅ Deterministic fingerprinting + cache (local; CI-persisted via actions/cache)
- ✅ Startup planner; `plan`/`explain`/`fingerprint`/`validate-config`/`lint-config`
- ✅ Templates, quality profiles, ad-hoc command detection
- ✅ Stage execution engine (bounded parallelism, timeouts, fail-fast)
- ✅ `init`/`init-module`/`doctor` and the embedded scaffolding
- ✅ GitHub dynamic check-runs + review/expert enforcement (`go-github`)
- ✅ Smart security scanning + anti-weakening + protected-file governance
- ✅ Performance budgets/regression + deduplicated Slack notifications

The deterministic decision logic for every layer is implemented and unit-tested.
The thin I/O edges that require external systems are wired against their real
clients but can only be exercised with live credentials/tooling:

- GitHub API calls (check-runs, reviews, permissions) need `GITHUB_TOKEN`
- Slack delivery needs `SLACK_WEBHOOK_URL`
- security scanners (trivy, semgrep, gitleaks, hadolint, ...) run when installed
- the cache directory is persisted across CI runs via `actions/cache`

## License

MIT — see [LICENSE](LICENSE).
