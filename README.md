# Frodo CI

**Opinionated modular CI/CD framework for monorepos.**

Frodo CI lets every module own its local automation under `.ci`, while the
platform keeps the whole repository fast, secure, reviewable, and predictable
through **one** standard workflow and **one** required merge check.

> Core principle: **run the minimum necessary work, but never skip risk.**

> [!WARNING]
> **Status: experimental (v0.1).** The deterministic core — planning,
> config validation, semantic linting, fingerprinting, and the execution engine
> — is implemented and unit-tested (19 packages, `make test` green). The GitHub
> check-run, review/expert, security-tool, and Slack integrations are
> implemented against their real clients but have **not yet been exercised in a
> live GitHub Actions run**, and security scanning currently *decides which
> scans to run* without fully parsing/enforcing each tool's findings. **Do not
> rely on it as your only merge gate yet.** See [Known limitations](#known-limitations).

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
- Exact fingerprint-based stage skipping (cache never skips review/security/policy)
- Built-in templates, smart security-scan selection, performance budgets
- Owner and expert reviewer enforcement, anti-weakening checks
- Deduplicated Slack notifications for actionable failures
- JSON Schema autocomplete and human-friendly config linting

## Requirements

- **Go 1.25+** to build from source (`GOTOOLCHAIN=auto` will fetch it if your
  local Go is older).
- **git** — used for change detection.
- To actually *execute* stages with `frodo-ci run`, the target repo's build
  tools must be present (e.g. `mvnw`, `pnpm`, `docker`). Frodo CI runs your
  steps; it does not install language toolchains itself (the GitHub workflow's
  `setup-*` steps, or your machine, provide those).

## Install

### From source (works today)

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

> This one-liner and the `omarss/frodo-ci@v1` GitHub Action become usable once
> the repository is **public** and a **tagged release** exists. While the repo is
> private with no releases, use the from-source path above.

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

## Commands

Run any command with `--help` for details. Global flags apply to all of them.

| Command | What it does |
|---|---|
| `init` | Scaffold Frodo CI into the repository |
| `init-module --name --type --path --owner` | Scaffold a module's `.ci/module.yml` |
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
- **Security scanning decides, it does not fully enforce.** It correctly selects
  which scans to run per change type and invokes a tool if installed, but does
  not yet parse each tool's findings (SARIF) and block on them.
- **No toolchain provisioning.** `run` executes your steps assuming the tools
  exist; installing JDK/Node/etc. is the workflow's or host's job.
- **Distribution.** The curl installer and `@v1` Action need the repo to be
  public with a tagged release; until then, build from source.
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
