# MaxIO architecture and component design

## Positioning

MaxIO is now positioned as an **S3 gateway platform** built on an external
metadata data-store:

- 多个上游 S3 实例/集群可通过网关接入与统一路由。
- S3 API 作为一等公民，负责请求转换、权限透传、对象索引与去重协调。

Native API（若保留）用于平台内控与治理。

- 网关应保持无状态与水平扩展；所有持久协议状态与对象索引状态委托给元数据数据库。

## Runtime composition

`app.go` builds a single `maxio` runtime from `dix` modules:

1. Configuration and logger
2. Metadata repository + metadata transaction boundary
3. Upstream backends registry and connector pool (S3 upstreams)
4. Cache, index, scheduler, repair, dedupe services
5. HTTP control/management and S3 handlers (gateway)

All modules are replaceable where package boundaries permit. `engine`, `model`,
and object core can be imported as composable packages for library-first
integrations. Index/repair runtime pieces can be embedded by other services as
needed.

## API planes

### Native HTTP object plane

- Controlled by `enable_native_object_api` (default: `true`).
- Currently retained for management and internal workflows.
- Keep route behavior deterministic; share request IDs and error semantics where possible.

### Native S3 compatibility plane (first-class)

- Supported as the public ingress API.
- S3 requests are translated into a unified object metadata model used by indexing and
  dedupe.
- Route behavior is defined by explicit S3 compatibility contracts (object operations,
  metadata semantics, errors, and pagination behavior).

### Control and management plane

- Health/readiness: `/healthz`, `/readyz`
- Metrics: `/metrics`
- Cluster and node operations for deployment topology: `/_cluster/*`
- Repair/dedupe/index/recovery: `/_repair/*`, `/_dedupe/*`, `/_index/*`,
  `/_recovery/*`
- Internal shard transport: `/_internal/*` (cluster-authenticated and internal only)

## Data-plane architecture

- **Metadata service (required):** ACID-backed metadata repository (PostgreSQL/MySQL/
  SQLite for dev, with a migration path to managed DB).
  - Upstream S3 endpoint 信息、桶级映射、对象指纹、对象元信息、去重关系、索引任务与审计记录。
- **Upstream connectors (stateful endpoints external):** 各接入 S3 实例的连接配置与运行状态。
- **Gateway（无状态）：** 按租户/路由策略转发 S3 请求，持久化元数据与索引，触发去重扫描与索引任务。

No raft is required in this model.

## Non-functional requirements

- Data safety and recoverability are implemented through database transactions,
  storage integrity checks, and repair jobs.
- Multi-tenant/security boundaries are enforced at gateway and transport layer;
  gateway tokens and cluster tokens are required by runtime config.
- Observability is provided by request logging/metrics, audit logs, and
  health/readiness endpoints.

## Deployment intent

- Default deployment is stateless gateway scaling behind L7 load balancing.
- S3 API is first-class public route; Native API as optional internal management interface.
- Use `docs/deployment.md` for operational templates and `docs/ROADMAP.md` for the
  production backlog.
