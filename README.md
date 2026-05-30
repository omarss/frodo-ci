# Frodo CI

**Opinionated modular CI/CD framework for monorepos.**

Frodo CI lets every module own its local automation under `.ci`, while the
platform keeps the whole repository fast, secure, reviewable, and predictable
through **one** standard workflow and **one** required merge check.

> Core principle: **run the minimum necessary work, but never skip risk.**

> [!WARNING]
> **Status: v1.0.0.** The deterministic core — planning, config validation,
> semantic linting, fingerprinting, and the execution engine — is implemented
> and unit-tested (22 packages, `make test` green). The GitHub check-run,
> review/expert, security-tool, and Slack integrations are implemented against
> their real clients but have **not yet been exercised in a live GitHub Actions
> run**. **Treat the integration layers as beta — don't rely on it as your only
> merge gate yet.** See [Known limitations](#known-limitations).

## Contents

- [What you get](#what-you-get)
- [Requirements](#requirements)
- [Install](#install)
- [Quickstart: the example monorepo](#quickstart-the-example-monorepo)
- [Use it in your repo](#use-it-in-your-repo)
- [Commands](#commands)
- [How planning works](#how-planning-works)
- [The final check](#the-final-check)
- [Branch protection](#branch-protection)
- [Known limitations](#known-limitations)
- [Contributing](#contributing)
- [License](#license)

## What you get

- One GitHub workflow: `.github/workflows/frodo-ci.yml`
- One required PR check: `Frodo CI / final`
- Deterministic module/stage planning at startup
- Decentralized module automation under `.ci`, with central quality profiles
- Exact fingerprint-based stage skipping, with build-output restore for dependents
  (cache never skips review/security/policy)
- Built-in templates, smart security-scan selection, performance budgets
- Owner and expert reviewer enforcement, anti-weakening checks
- Deduplicated Slack notifications for actionable failures
- JSON Schema autocomplete and human-friendly config linting

## Requirements

- **Go 1.25+** to build from source (`GOTOOLCHAIN=auto` will fetch it if your
  local Go is older).
- **git** — used for change detection.
- To actually *execute* stages with `frodo-ci run`, the build tools the steps
  call must be present. Frodo CI runs your steps; it does not install language
  toolchains itself. The workflow `frodo-ci init` generates already includes
  `setup-java` + `setup-node` + corepack (matching the stock templates) — trim
  what your repo doesn't use and add others (e.g. `setup-go`).

## Install

### From source

```bash
git clone https://github.com/omarss/frodo-ci.git
cd frodo-ci
make build           # -> ./bin/frodo-ci
make install         # -> $(go env GOBIN)/frodo-ci  (add it to your PATH)
frodo-ci doctor
```

### Script / GitHub Action install

```bash
curl -fsSL https://raw.githubusercontent.com/omarss/frodo-ci/main/install.sh | bash
```

> Installs the latest released binary (linux/macOS, amd64/arm64), falling back
> to `go install` when no prebuilt binary matches. You can also
> `go install github.com/omarss/frodo-ci/cmd/frodo-ci@v1.0.0`. In a workflow,
> reference the Action as `omarss/frodo-ci@v1`.

## Quickstart: the example monorepo

The repo ships a small, multi-language example you can drive immediately. No
tokens or external tools are needed for these commands.

```bash
# A Spring service (cards) depends on a Java library (vrtx-common);
# a Node app (portal) depends on a Node library (shared-ui); plus k8s infra.
frodo-ci -C examples/monorepo validate-config
frodo-ci -C examples/monorepo lint-config
frodo-ci -C examples/monorepo explain packages/vrtx-common/src/main/java/Common.java
frodo-ci -C examples/monorepo fingerprint cards.test
```

`explain` shows the key behavior: a change to `vrtx-common` re-runs `cards`'
`test`, `build`, `package`, and `scan` (its declared `affects`) — but **not**
`cards.validate`. See [`examples/monorepo/README.md`](examples/monorepo/README.md).

## Use it in your repo

```bash
cd /path/to/your/monorepo
frodo-ci init                                   # scaffold config, schemas, templates, workflow
frodo-ci init-module --name cards --type spring-service \
  --path services/cards --owner cards-team
frodo-ci validate-config && frodo-ci lint-config
frodo-ci plan                                   # preview what would run, and why
```

`init` writes (idempotently; `--force` to overwrite):

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

### Dependencies build once — don't rebuild the closure

Frodo runs a module's `depends_on` modules **before** the module itself, in the
same job on the same filesystem. So by the time `cards` builds, `money` has
already been built and its output (`dist/`, installed `~/.m2` artifacts, …) is on
disk. **Each module should build only itself** and reuse what its dependencies
already produced:

```yaml
# do this — self-only; deps are already built
run: pnpm -s build          # or: ./mvnw -pl "$FRODO_MODULE_PATH" -am

# not this — rebuilds the whole dependency closure on every module, every stage
run: pnpm --filter "{.}..." build   # trailing "..." pulls in every dependency
```

Filtering by the dependency closure re-compiles the same shared packages once per
dependent and once per stage — in a monorepo where every app shares the same
libraries, that is most of your CI time. The fix is to **model each shared
library/client as its own Frodo module** (`scaffold` detects them) and let the
dependency order do the work; the per-module build then stays self-only. The
generated workflow caches the pnpm/Maven stores so even the install starts warm.

### Build outputs are cached, not rebuilt

A stage can declare the artifacts it produces with `outputs:`. Frodo archives
them keyed by the stage's fingerprint, and on a hit **restores them instead of
re-running** — for this module and, in dependency order, for dependents in the
same run. So an unchanged library doesn't just skip its own build; its `dist/`
comes back on disk for every dependent that *did* change. The stock templates
already declare it (`dist/` for Node/TS, `target/` for Maven):

```yaml
# <module>/.ci/build.yml
outputs: [dist/]        # archived by fingerprint; restored on a hit
```

This is what turns "fast when nothing changed" into "fast when one thing
changed" — the real-world case. Restoring is always keyed by the exact
fingerprint, so it can only ever skip work, never change a result. (Distinct from
`cache.paths`, which persists incidental package stores like `~/.m2` at the
workflow level.)

### Stage environment

Each step runs **in its module's directory** by default (override per step with
`working_directory`), with these variables exported so templates need no
hard-coded paths:

| Variable | Value |
|---|---|
| `FRODO_MODULE` | module name (e.g. `cards`) |
| `FRODO_MODULE_PATH` | repo-relative module dir (e.g. `services/cards`) |
| `FRODO_MODULE_DIR` | absolute module dir |
| `FRODO_REPO_ROOT` | absolute repo root (e.g. for `cd "$FRODO_REPO_ROOT" && ./mvnw -pl "$FRODO_MODULE_PATH"`) |
| `FRODO_STAGE` / `FRODO_ENVIRONMENT` | current stage / target environment |
| `FRODO_IMAGE` | `<prefix><module>:<tag>` for build/push; set `images.prefix` in the root config to add a registry, and override per stage via `env:` |

## Commands

Run any command with `--help` for details. Global flags apply to all of them.

| Command | What it does |
|---|---|
| `init` | Scaffold Frodo CI into the repository (`--action-ref` for forks; `--force`) |
| `init-module --name --type --path --owner` | Scaffold a module's `.ci/module.yml` (`--depends-on m[:affects=a,b]` repeatable; `--force`) |
| `scaffold` | Detect modules from build metadata (Maven/pnpm/go/Docker/IaC), resolve intra-repo dependencies into `depends_on` edges, and propose `.ci/module.yml` files (`--write` to apply, `--owner` fallback) |
| `sync-workflow` | Regenerate the workflow's toolchain setup from modules' `setup:` blocks, using SHA-pinned `setup-*` actions (`--check` to verify in CI) |
| `validate-config` | Validate config against the JSON Schemas |
| `lint-config` | Semantic linting (cycles, broad inputs, weakening, ...) |
| `plan` | Calculate and print the execution plan |
| `explain <file>` | Which modules/stages a file affects, and why |
| `fingerprint <module.stage>` | Deterministic fingerprint for a stage (`--inputs` to list inputs) |
| `run` / `ci` / `cd` | Execute the plan (all / CI-only / CD-only) — the final check |
| `review` | Evaluate review/owner/expert requirements for the PR |
| `doctor` | Environment + configuration health |
| `schemas export` | (Re)write the JSON Schemas |
| `templates list` / `templates explain <name>` | Inspect module templates |

**Global flags:**

| Flag | Meaning |
|---|---|
| `-C, --repo <path>` | Repository root (default: current directory) |
| `--base <ref>` / `--head <ref>` | Change-detection range (default: auto / working tree) |
| `--environment <env>` | Target environment for CD stages (default: `staging`) |
| `--json` | Machine-readable JSON output where supported |
| `--log-level <level>` | `trace`\|`debug`\|`info`\|`warn`\|`error` |

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

## Known limitations

Be aware of these before adopting it to gate merges:

- **Not yet run end-to-end in GitHub Actions.** Dynamic check-run creation, the
  `Frodo CI / final` gating, review/expert enforcement, and Slack delivery are
  implemented (`go-github`, `slack-go`) but unproven against a live run.
- **Security scanning enforces.** It selects the scans per change type, runs the
  tool (semgrep/gitleaks/trivy/hadolint), parses SARIF, and **blocks** on findings
  per the module's profile (`fail_on_new_critical`, secrets, blocking rulesets),
  honoring time-bounded suppressions. The generated workflow installs the tools;
  a triggered scan whose tool is missing fails (non-bypassable).
- **Toolchains are provisioned by the workflow, generated from `setup:`.** `run`
  executes your steps assuming the tools exist; the workflow provisions them via
  maintained, SHA-pinned `setup-*` actions. Run `frodo-ci sync-workflow` to
  regenerate that section from the union of your modules' `setup:` blocks (it
  picks the highest version per tool) — so adding a module with `setup: {go: 1.25}`
  needs no hand-editing. `--check` fails CI if the workflow drifts from `setup:`.
- **GitHub/Slack features need credentials** (`GITHUB_TOKEN`, `SLACK_WEBHOOK_URL`)
  and a PR context to do anything; locally they degrade gracefully.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short:

```bash
make build      # compile to ./bin
make test       # race tests (19 packages)
make lint       # golangci-lint (falls back to go vet)
make fmt        # gofmt
make schemas    # regenerate JSON Schemas
```

Repository layout:

```text
cmd/frodo-ci/            # CLI entrypoint
internal/
  config/                # typed config model + YAML loaders
  schema/                # JSON Schema generation + validation
  discover/ match/ vcs/  # module discovery, glob matching, git
  graph/                 # dependency graph + integrity checks
  fingerprint/ cache/    # deterministic fingerprints + cache backends
  configlint/ plan/      # semantic linting + the startup planner
  templates/             # module templates + effective-stage merge
  runner/                # stage execution engine
  github/ reviews/       # GitHub API + review/expert governance
  security/ antiweaken/ protected/   # smart scanning + governance
  perf/ slack/           # performance budgets + notifications
  cli/                   # cobra commands
examples/monorepo/       # a runnable, vrtx-style example
```

## License

MIT — see [LICENSE](LICENSE).
