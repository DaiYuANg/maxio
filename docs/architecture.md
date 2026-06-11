# MaxIO architecture and component design

## Positioning

MaxIO focuses on a practical storage control plane + metadata plane with a native
HTTP object API. The current architecture is:

- Native object APIs (`/`, `/<bucket>`, `/<bucket>/<key>`) are the primary public
  interface.
- Native object APIs can be disabled for cluster-management-only deployments via
  `enable_native_object_api` when a separate object front door is used.
- Index and dedupe remain first-class value-add features on top of object lifecycle.
- The application is library-first and can be embedded as a Go component.

## Runtime composition

`app.go` builds a single `maxio` runtime from `dix` modules:

1. Configuration and logger
2. Raft and discovery
3. Metadata, storage, cache, and index
4. Object core, scheduler, repair, dedupe
5. HTTP control/management and object handlers

All modules are replaceable where package boundaries permit. `engine` and object
core can be imported as composable packages for library-first integrations, and
index/repair runtime pieces can be embedded as needed.

## API planes

### Native HTTP object plane (optional)

- Controlled by `enable_native_object_api` (default: `true`).
- When `false`, request dispatch only serves control paths and returns `404` for
  native bucket/object routes.

### Control and management plane

- Health/readiness: `/healthz`, `/readyz`
- Metrics: `/metrics`
- Cluster lifecycle and operations: `/_cluster/*`
- Repair/dedupe/index/recovery: `/_repair/*`, `/_dedupe/*`, `/_index/*`,
  `/_recovery/*`
- Internal shard transport: `/_internal/*` (cluster-only authenticated)

## Non-functional requirements

- Data safety and recoverability are implemented through raft-backed metadata,
  background repair, and storage integrity checks.
- Multi-tenant/security boundaries should be enforced at the gateway and reverse
  proxy layer for production, with tokens required by this runtime as configured.
- Observability is provided by request logging/metrics, audit, and health/readiness
  endpoints.

## Deployment intent

- Prefer native object APIs for object client traffic.
- Disable native object API in environments where another service owns all object
  read/write semantics.
- Use `docs/deployment.md` for operational templates and `docs/ROADMAP.md` for the
  production backlog.
