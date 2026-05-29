# Frodo CI — Requested fixes & enhancements

Logged while adopting Frodo CI in a real monorepo (vrtx-mono fork, ~51 modules).

## Bugs

### 1. `init` emits a non-existent action reference
`frodo-ci init` writes `.github/workflows/frodo-ci.yml` with:

```yaml
uses: frodo-ci/action@v1
```

But the published, usable action lives at the repo root of `omarss/frodo-ci`
(there is a root `action.yml`, released as `v1` / `v1.0.0`). The org/repo
`frodo-ci/action` does not exist, so the generated workflow fails to resolve the
action on the first run.

**Ask:** default the generated `uses:` to the real release coordinates
(`omarss/frodo-ci@v1`), and/or make it configurable, e.g.
`frodo-ci init --action-ref <owner/repo@ref>`. Pin to a tag the README documents.

## Enhancements

### 2. `init-module` cannot declare dependencies
`init-module` only accepts `--name/--type/--path/--owner`; `depends_on` edges must
be hand-appended to each `module.yml` afterward. For a large repo this means
post-processing every file.

**Ask:** add a repeatable `--depends-on <module>[:affects=test,build,scan]` flag
(or `--depends-on-file`) so edges can be scaffolded directly.

### 3. `init-module` is not idempotent / has no `--force`
Re-running `init-module` for an existing module errors instead of overwriting or
no-op'ing, which makes regeneration scripts brittle.

**Ask:** add `--force` (overwrite) and/or make an unchanged re-run a no-op,
mirroring `init`'s idempotent behavior.

### 4. Bulk scaffolding / workspace auto-detection
In a monorepo with pnpm workspaces + a Maven reactor, declaring ~50 modules is
many manual `init-module` calls. Module type, path, and most dependency edges are
derivable from `pnpm-workspace.yaml` + `package.json` workspace deps and from the
Maven reactor + `<dependency>` graph.

**Ask:** a `frodo-ci scaffold --detect` (or `init-modules`) that proposes modules
and coarse `depends_on` edges from existing build metadata, for review before write.

### 5. Generated workflow does not provision the toolchains the templates require
First live Actions run (PR replicating an upstream change touching two node-library
packages) failed immediately with `bash: line 1: pnpm: command not found` on the
`validate` stage. The stock templates declare `setup: { node: {version: 22} }` /
`setup: { java: {version: 25} }` and their steps call `pnpm ...` / `./mvnw ...`,
but `frodo-ci init`'s `frodo-ci.yml` only does `actions/checkout` + `Run Frodo CI`
— nothing installs Node/pnpm/JDK, and the runner does **not** act on the templates'
`setup:` blocks. So the gate fails out-of-the-box on any repo until the user
hand-edits the workflow to add `setup-java` / `setup-node` + corepack.

**Ask (pick one):**
- Make the runner honor each stage's `setup:` block and actually provision the
  declared toolchain (most "it just works"), **or**
- Have `init` scaffold the matching `setup-*` steps into `frodo-ci.yml` based on the
  module types present (e.g. emit `setup-java` + `setup-node`/corepack when
  spring/node modules exist), **or**
- At minimum, document the required pre-`run` setup steps in the README's
  "Use it in your repo" section so adopters aren't surprised by a red first run.

Workaround applied in the fork: added `actions/setup-java@v4` (liberica 25),
`actions/setup-node@v4` (22), and `corepack prepare pnpm@<pinned>` before the
`Run Frodo CI` step.

## Bugs (cont.)

### 6. `$FRODO_IMAGE` is referenced by templates but never injected by the runner
The default `node-app`, `spring-service`, and `docker-image` templates run:

```yaml
- run: docker build -t "$FRODO_IMAGE" .     # package
- run: docker push  "$FRODO_IMAGE"          # publish
```

But the runner only injects `FRODO_MODULE`, `FRODO_STAGE`, `FRODO_ENVIRONMENT`
(`internal/runner/runner.go:344-346`) — there is no `FRODO_IMAGE`. So every
`package` stage fails with:

```
ERROR: failed to build: invalid tag "": repository name must have at least one component
```

i.e. `docker build -t "" .`. Confirmed on the first end-to-end run: tests passed
for all three node modules, only the three `package` stages failed for this reason.

**Ask:** have the runner inject `FRODO_IMAGE` (derived from a configurable
registry/image-name in root or module config, defaulting to `${module}:${env}` or
a local tag) so the stock templates work, or stop shipping `$FRODO_IMAGE` in the
default templates and use `$FRODO_MODULE` instead.

Workaround applied in the fork templates: `-t "${FRODO_IMAGE:-${FRODO_MODULE}:ci}"`.

### 7. Steps do not run in the module directory (no module-dir default, no `FRODO_MODULE_DIR`)
After fixing #6, `package` still failed:

```
ERROR: failed to build: failed to read dockerfile: open Dockerfile: no such file or directory
module=webhooks/business-api/internal-api stage=package   workdir=
```

The modules *do* have Dockerfiles (`services/webhooks/Dockerfile`, `apps/*/Dockerfile`),
but the runner executes every step at `RepoRoot` unless the step sets
`working_directory` (`runner.go:289` passes `step.WorkingDirectory` verbatim;
`exec.go:33-37` falls back to `RepoRoot` when empty). The planner knows each
module's directory as `m.Dir`, but it is never used as the default step cwd.

The default templates are written assuming the step runs **inside the module dir**
(`docker build .`, `./mvnw -pl . -am`). `pnpm`/`mvnw` happen to survive from the
repo root (workspace/reactor aware), but `docker build .` cannot find the module's
Dockerfile, and `mvnw -pl .` silently targets the root reactor instead of the module.

**Ask (either):**
- Default each step's working directory to the module's `Dir` when the step does
  not specify `working_directory` (matches what the stock templates assume), **and/or**
- Inject a `FRODO_MODULE_DIR` env var so templates can do
  `docker build -f "$FRODO_MODULE_DIR/Dockerfile" "$FRODO_MODULE_DIR"` explicitly.

This is the single highest-impact fix: without it, `package`/`publish` (and any
module-relative command) is broken for every module in a real monorepo.
