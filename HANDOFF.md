# Frodo CI — Work Order: "CLI owns all changes, zero manual editing"

## Context
In a real monorepo adoption (~100 modules: vrtx-mono fork, PR
`vrtx-fintech/vrtx-mono-fork#27`), every config/workflow change should be made by
a `frodo-ci` command or driven by declared config, so the setup stays
reproducible and `sync-workflow --check` can detect drift. Today a handful of
changes still require hand-editing the generated `.github/workflows/frodo-ci.yml`,
the templates, or `module.yml` / `suppressions.yml`. Each item below is the spec
to close one gap. Ship as usual — atomic PR + release each, `make test` green,
README / Known-limitations updated.

**Already done (do not redo):** `init`, `init-module --depends-on/--force`,
`scaffold` (modules/types/owners/edges), `sync-workflow` (toolchain block),
generated cache restore/save split (`!cancelled()`) + SHA-pinned actions +
base-branch warm-start + security-tool install, `init --action-ref`.

---

## 1. Detect toolchain versions from repo metadata (highest priority)
**Problem:** template `setup:` hardcodes versions (e.g. Node 22). The repo's real
requirement (`package.json` `engines.node >=24`, `packageManager: pnpm@10.34.1`)
only takes effect after hand-editing the templates; the mismatch produced
`Unsupported engine` warnings and contributed to red runs.
**Deliverable:** `scaffold` and `sync-workflow` resolve toolchain versions from
repo metadata, not template defaults:
- Node: `package.json#engines.node` / `.nvmrc` / `.node-version`; pnpm: `packageManager`.
- Java: `.java-version` / `.mvn/` / `pom.xml` (`maven.compiler.release`); Go: `go.mod`.
**Acceptance:** on a repo declaring Node 24 + pnpm 10.34.1, `frodo-ci init` (or
`sync-workflow`) emits the managed setup block with Node 24 and that pnpm version —
no template edit. `sync-workflow --check` passes against the detected versions.

## 2. Declarative registry/auth -> generated workflow steps
**Problem:** the `package`/`publish` docker build needs to pull/push a private
registry (GCP Artifact Registry). There is no way to declare this, so the
auth/login steps were hand-added to the workflow.
**Deliverable:** a config block (root and/or module), e.g.
```yaml
registries:
  - host: me-central2-docker.pkg.dev
    auth: gcp-wif            # or: gcp-key | ecr | acr | ghcr | docker
    workload_identity_provider_var: GCP_WIF_PROVIDER   # repo var/secret names
    service_account_var: GCP_WIF_SERVICE_ACCOUNT
```
`init`/`sync-workflow` generate the corresponding (SHA-pinned) auth + `docker login`
steps in a managed, drift-checked section; nothing hardcoded — credentials come
from repo vars/secrets.
**Acceptance:** declaring a registry regenerates the workflow with working auth
steps; removing it removes them; `--check` detects drift. No hand-edit needed to
push to a private registry.

## 3. Stock node templates default to `--if-present`
**Problem:** stock `node-app`/`node-library`/`typescript-app` run
`pnpm -s typecheck|test|build` with **no `--if-present`**. In a real repo **32/39
packages lacked a `typecheck` script** (many lack `test`), so `validate`/`test`
failed out-of-the-box; we had to hand-edit templates.
**Deliverable:** change the three node templates' typecheck/test/build steps to
`pnpm -s --if-present <script>` (a missing script is a skip, not a failure).
Pure template-default fix — no new CLI.
**Acceptance:** a module with only a `build` script passes `validate`/`test`
(skipped) without any template edit.

## 4. Review requirements via CLI / scaffold
**Problem:** `reviews:` (owner/expert/team rules) can only be added by hand-editing
`module.yml`.
**Deliverable:**
- `init-module --review "owners=1,expert=1"` and a repeatable path-rule flag,
  e.g. `--review-path "src/**/settlements/**:teams=security:1"`.
- `scaffold` derives default `reviews.require` from CODEOWNERS coverage where present.
**Acceptance:** review requirements can be declared/updated entirely via CLI;
`frodo-ci review` reflects them with no hand-edit.

## 5. Security suppressions via CLI
**Problem:** accepting a finding means hand-writing `suppressions.yml`
(id/path/reason/owner/expiry/approver).
**Deliverable:** `frodo-ci suppress add --id <rule> --path <glob> --reason ...
--owner ... --approver ... --expiry <date>` (rejects missing/past expiry), plus
`suppress list` and `suppress prune` (drop expired). Bonus: `--from-finding <id>`
to seed path/id from the last run's SARIF.
**Acceptance:** a false-positive can be suppressed (and a real one re-surfaces on
expiry) without touching YAML by hand; expiry is enforced at write time.

## 6. Declarative job/stage env (resource hints)
**Problem:** runner/job env (e.g. `NODE_OPTIONS=--max-old-space-size`) had to be
hand-added to the workflow.
**Deliverable:** allow env to be declared in config (root `workflow.env:` and/or
per-stage `env:`, already supported for steps) and have `init`/`sync-workflow`
emit job-level env into a managed block.
**Acceptance:** declaring env regenerates it into the workflow; `--check` detects drift.

## 7. `sync-workflow` manages the action ref
**Problem:** bumping `uses: omarss/frodo-ci@vX` on an existing workflow requires
`init --force` (regenerates everything) or a hand-edit.
**Deliverable:** `sync-workflow` (or `sync-workflow --action-ref <ref>`) manages
and drift-checks the `uses:` ref like the setup block.
**Acceptance:** the action version can be bumped via CLI with `--check`
enforcement; no hand-edit.

---

## Overarching definition of done (the invariant)
After these, the following must hold and should be asserted by a test:

> **No supported scenario requires hand-editing `frodo-ci.yml`, the templates, or
> any `.github/frodo-ci/**` file.** Everything is produced by
> `init`/`init-module`/`scaffold`/`sync-workflow`/`suppress`/config, and
> `frodo-ci sync-workflow --check` (extended to cover the registry/env/action-ref
> managed sections) passes on a clean tree.

Add a regression test: scaffold a fixture repo (mixed Maven + pnpm workspace,
private registry, partial scripts, a CODEOWNERS), run the CLI end-to-end, and
assert a clean `--check` with **zero manual edits**.

---

## Provenance
Every gap above is a real hand-edit made while adopting frodo-ci on a production-
shaped monorepo (services + apps + ~100 modules) and getting PR #27 green. The
corresponding regression scenarios are in the adopter's `TEST_PLAN.md`
(section "CLI Ownership / No Manual Editing").

---
---

# Part 2 — Validation gaps & open items (post-v1.14.1)

Part 1 (CLI-ownership) is shipped + validated. This part is the honest status of
**what is implemented but NOT yet proven on a live run**, plus a couple of still-
open refinements. None of these are "the feature is broken" — they are "the
promise has never actually executed end-to-end, so we don't know." Each item says:
**what's unproven · why it matters · how it came up · how to validate/fix ·
acceptance.** Adopter context: all live validation happened on ONE PR
(`vrtx-fintech/vrtx-mono-fork#27`) replicating one upstream change that touched
**only Node packages** (internal-api + nestjs-* libs under `settlements/**`). That
narrowness is the root of most gaps below.

## A. CD stages (`publish` / `deploy` / `verify`) — never executed
- **Unproven:** no CD stage has ever run successfully. `package` only ever 403'd on
  a private base image or was skipped; `publish`/`deploy`/`verify` never reached.
- **Why it matters:** half the pipeline (the "CD" in CI/CD) is unvalidated — image
  push, environment targeting, deploy, post-deploy verify, and "CD is never cached".
- **How it came up:** the fork faked the registry for safety; later we declared
  `registries:` auth via config but never had real creds, and the PR never triggered
  a deploy.
- **How to validate:** a sandbox repo with a real (or local `registry:2`) registry +
  working `registries:` creds; a module whose change triggers `package`→`publish`;
  a `workflow_dispatch` with `environment: staging` to drive `deploy`/`verify`.
  Assert: image pushed with the `FRODO_IMAGE` tag; deploy/verify run; **none of the
  CD stages are cache-skipped on a re-run** (CD must never cache).
- **Acceptance:** a green run that pushes an image and runs deploy+verify, and a
  second run that still re-runs all CD stages (no cache skip).

## B. Java / Spring / Maven path — never exercised live
- **Unproven:** every live run was Node-only. `spring-service` template, `mvnw -pl
  <module> -am` reactor builds, `target/` output caching, and GraalVM **native
  images** have not run in CI here.
- **Why it matters:** ~18 of the repo's services are Spring/Java; the Maven half of
  planning, output cache (`outputs: [target/]`), and the JDK toolchain path are
  unvalidated.
- **How to validate:** a PR that changes a Java service (or a shared Java lib like
  `vrtx-common`) and confirm: correct reactor build, `target/` archived+restored for
  dependents, JDK provisioned from detected version, and (if used) a `+native` build.
- **Acceptance:** Java service `validate/test/build/package` green on first run,
  `build` **skipped via output cache** on an unrelated re-run, and a dependent Java
  module reuses the lib's restored `target/`.

## C. Review **positive path** — only the *blocking* path has ever been seen
- **Unproven:** we have only ever observed reviews **block** (red). We have **never**
  seen approvals **satisfy** the gate and flip it green. And the **expert** was never
  actually resolved: the fork's history is single-author, so `PickExpert` always
  returned none and fell back to owners — the "resolve a domain expert from 30d git
  history → require → satisfy" flow is entirely untested.
- **Why it matters:** this is the headline review promise, and half of it (the half
  that lets a correctly-reviewed PR merge) is unproven. A fail-closed gate that can
  *never* go green is indistinguishable from a bug to an adopter.
- **How to validate:** a repo with **real multi-author history** so an expert
  resolves; a PR by author X touching a module expert Y owns; have Y (and an owner,
  and the path-scoped team) approve; assert the gate flips **green**. Then push a new
  commit and assert `require_approval_after_latest_commit` **dismisses** the approval
  and re-blocks.
- **Acceptance:** one run where owner+expert(+team) approvals turn `final` green;
  one run where a post-approval commit re-blocks it. Relevant code:
  `internal/reviews/expert.go` (`PickExpert`), `internal/cli/github.go`
  (`evaluateReviews`/`gateReviews`), `internal/reviews` `LatestPerUser`.

## D. Misconfigured-owner-team diagnosis — STILL OPEN
- **Open:** when an owner team doesn't exist / has no resolvable members (our
  `@platform`), the gate blocks (correct, fail-closed) and the reviewer-request 422s
  (logged best-effort), but the comment still says "needs owner approval" / "should
  be reviewed by @platform". A dev cannot tell **"waiting for a human"** from **"this
  requirement can never be satisfied because the team is empty/unknown."**
- **Why it matters:** an unsatisfiable requirement renders the PR permanently
  un-mergeable with a misleading message — a silent dead-end.
- **Fix:** in the review evaluation, resolve owner teams up front (GitHub teams API /
  membership) and when a required team resolves to **zero members** (or a reviewer
  request returns 422), surface a distinct diagnosis, e.g.
  `owner team @platform is empty or unknown — this requirement is unsatisfiable; fix
  owners or grant the team`, separate from a normal "needs approval".
- **Acceptance:** a module owned by a nonexistent team produces the "unsatisfiable /
  misconfigured" message (not a plain "needs approval"); a real-but-empty team does
  the same; a valid team still shows "needs approval".

## E. Implemented-but-never-triggered subsystems
None of these fired on any live run; each needs a purpose-built scenario:
- **Performance budgets** (`performance.budgets`): a stage exceeding its budget should
  be reported/failed per config. Never breached.
- **Anti-weakening checks**: a PR that loosens a `reviews:`/security requirement
  should be flagged. Never exercised.
- **Timeout / stop policies**: per-stage, full-run, and no-progress timeouts
  (`runner.go` watchdog), and `stop_module_on_stage_failure` /
  `stop_dependents_on_dependency_failure` /
  `stop_expensive_stages_after_validation_failure`. We saw cascade *cancellation*
  once, but never a real timeout or each stop-policy in isolation.
- **Slack notifications** (`SLACK_WEBHOOK_URL`): never configured, so dedup/delivery
  unproven.
- **Validate-fast-fail ordering**: `stop_expensive_stages_after_validation_failure`
  specifically — confirm expensive stages don't start after a validate failure.
- **Acceptance:** one targeted run per item that makes the condition occur and
  asserts the documented behavior.

## F. Scale / parallelism
- **Unproven:** 100 modules are declared, but no change ever fanned out across many
  modules at once. `max_parallel_modules` / `max_parallel_expensive_stages`
  scheduling, and cache behavior across a wide change, are untested at scale.
- **How to validate:** change a low-level shared lib (e.g. `vrtx-common`) that many
  modules depend on; confirm bounded parallelism, no deadlock, correct ordering, and
  sane wall-clock.

## G. Remote / shared cache backend — deferred (note, not a bug)
- Current cache is local-dir + `actions/cache`; fine for the single `final` job. A
  native GH-cache-API / GCS / S3 backend only matters once the run is split into
  **multiple parallel jobs**. Revisit then.

## H. Test-plan coverage is mostly unexecuted
- The adopter's `TEST_PLAN.md` has **~153 cases across 15 areas**; only ~20–25 were
  actually executed (via PR #27). The rest are written but unrun. Prioritise the
  live-CI cases for CD (CD-01..06), Reviews positive path (REV-07/REV-15-ish),
  Security blocking/suppress (SEC-*), Output cache for Java (OUT-*), and the
  Execution-policy/timeout cases (EXE-*).

## Suggested priority for the team
1. **Review positive path + expert-from-history (C)** — the only promise we've *only*
   ever seen fail; highest credibility risk.
2. **CD path to a real/sandbox registry (A)** — the entire unvalidated half.
3. **Java/Maven run (B)** — covers the other language + `target/` output cache.
4. **Misconfigured-owner diagnosis (D)** — small, high-UX-value, still open.
5. Then E/F (policies, perf, scale) and broaden `TEST_PLAN.md` execution.
