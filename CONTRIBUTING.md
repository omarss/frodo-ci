# Contributing to Frodo CI

Thanks for working on Frodo CI. This guide covers the local workflow, project
layout, and the conventions that keep the codebase consistent.

## Prerequisites

- **Go 1.25+** (or any Go with `GOTOOLCHAIN=auto`, which fetches 1.25 on first build)
- **git**
- Optional: `golangci-lint` (the `make lint` target falls back to `go vet` if absent)

## Local workflow

Everything goes through the Makefile:

```bash
make build      # compile to ./bin/frodo-ci
make test       # race tests across all packages
make lint       # golangci-lint (or go vet)
make fmt        # gofmt -w
make vet        # go vet
make tidy       # go mod tidy && go mod verify
make schemas    # regenerate the JSON Schemas from the Go types
make help       # list targets
```

Before sending a change, make sure this is clean:

```bash
make fmt && make vet && make test
```

> Note on coverage: `make test` runs `-race` without a coverage profile. A
> profile (`make cover`) needs a full Go SDK for the `covdata` tool; the
> auto-downloaded module toolchain does not ship it. CI uses a full SDK.

## Project layout

```text
cmd/frodo-ci/            # CLI entrypoint (main)
internal/
  config/                # typed config model + position-aware YAML loaders
  schema/                # JSON Schema generation (invopop) + validation (santhosh-tekuri)
  discover/              # module discovery (.ci/module.yml)
  match/                 # doublestar glob + regex matching, path resolution
  vcs/                   # git CLI wrapper (changed files, ls-files, show)
  graph/                 # dependency graph + integrity checks
  fingerprint/           # deterministic stage fingerprints
  cache/                 # fingerprint cache backends (local fs, noop)
  configlint/            # semantic config linting
  plan/                  # the deterministic startup planner
  templates/             # module templates + effective-stage merge (embedded defaults)
  scaffold/              # module auto-detection (Maven/pnpm/go/Docker/IaC + CODEOWNERS)
  runner/                # stage execution engine (parallelism, timeouts, fail-fast)
  github/                # go-github client + Actions run context
  reviews/               # review enforcement + expert scoring (pure logic)
  security/              # smart scan selection, suppressions, classification
  antiweaken/            # base-vs-head governance-weakening detection
  protected/             # protected-file rule matching
  perf/                  # performance budgets + regression
  slack/                 # notifications (slack-go)
  assets/                # embedded files written by `frodo-ci init`
  cli/                   # cobra commands
  logging/ version/      # logging + build metadata
examples/monorepo/       # a runnable, vrtx-style example
```

## Design conventions

- **Keep decision logic pure and tested; isolate I/O.** Packages like `reviews`,
  `security`, `antiweaken`, `perf`, and `plan` keep their decision logic free of
  network/disk so it can be unit-tested; the GitHub/Slack/git edges live behind
  small clients.
- **Determinism.** The planner must produce the same plan for the same inputs.
  Sort before emitting; never depend on map iteration order or wall-clock time
  (inject `now`).
- **Errors are for humans.** Config problems should read like the `validate-config`
  / `lint-config` output (file + message + did-you-mean), not raw library errors.
- Small files and functions; comment *why*, not *what*.

## Common tasks

**Add a config field:** add it (with `yaml` + `json` tags) to the struct in
`internal/config`, run `make schemas`, and extend the relevant loader/validator
test. Optional fields use `,omitempty` so the schema treats them as optional.

**Add a module template:** drop a `<name>.yml` in
`internal/templates/defaults/`. It is embedded automatically; `templates list`
and `init` pick it up. Add a case to the templates test if it needs custom merge
behavior.

**Add a CLI command:** add a `newXxxCommand(app *App)` constructor in
`internal/cli` and register it in `internal/cli/root.go`. Keep the body thin and
push logic into an `internal/` package.

## Tests

- Every package with logic has `_test.go` coverage; add tests with your change.
- Tests that need git create a temp repo and skip when git is unavailable.
- Prefer table-driven tests and assert on behavior (e.g. "money change re-runs
  cards.test but not cards.validate"), not on internal representation.

## Commits and PRs

- Commit titles: lowercase, imperative, ≤ 50 characters
  (e.g. `feat: add expert scoring`).
- Keep commits atomic — one logical change each.
- Open a PR against `main`; ensure `make fmt && make vet && make test` pass and
  CI is green before requesting review.
