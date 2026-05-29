# Example monorepo

A small, vrtx-style monorepo that exercises Frodo CI across languages and a
dependency edge:

| Module | Path | Type | Notes |
|---|---|---|---|
| cards | `services/cards` | spring-service | depends on `vrtx-common`; has a sensitive-file review rule |
| vrtx-common | `packages/vrtx-common` | java-library | shared Java library |
| portal | `apps/portal` | node-app | depends on `shared-ui` |
| shared-ui | `packages/shared-ui` | node-library | shared TS library |
| k8s | `infra/k8s` | k8s-infra | Kubernetes manifests |

Try it:

```bash
frodo-ci -C examples/monorepo validate-config
frodo-ci -C examples/monorepo lint-config
frodo-ci -C examples/monorepo plan
frodo-ci -C examples/monorepo explain packages/vrtx-common/src/main/java/Common.java
frodo-ci -C examples/monorepo fingerprint cards.test
```

Changing `vrtx-common` re-runs `cards`' `test`, `build`, `package`, and `scan`
(its declared `affects`), but not `cards.validate`.
