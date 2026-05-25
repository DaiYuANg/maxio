# MaxIO Roadmap

MaxIO is currently a runnable prototype with a library-first runtime, HTTP API,
single-shard Dragonboat metadata, erasure-coded local storage, object indexing,
event publishing, Web UI, and a Raft-backed background repair scheduler.

This document tracks the remaining work required before the project can be
treated as a production-grade object storage service.

## Current status

- The application is library-first and can be embedded through the root Go package.
- Core components are moving toward composable library packages: `engine`,
  `model`, `object`, and `s3` can now be imported directly, while the root
  runtime still assembles them with dix for the full server.
- Runtime composition is dix-first, with config, logging, event bus, HTTP, Raft,
  metadata, storage, index, S3 endpoint registration, scheduler, and repair
  assembled as modules.
- Metadata is backed by the Raft state machine rather than an external KV store.
- Object data is written through the current storage engine with local erasure
  coding and repair primitives.
- Storage placement now has local and remote `StorageNode` implementations. The
  remote shard path uses the internal HTTP shard endpoint, and tests cover a
  two-node write/read loop through that transport.
- Background repair is scheduled through gocron and guarded by Raft leadership,
  so only the current leader runs cluster-wide repair jobs.
- Background repair now scrubs healthy objects by verifying shard checksums and
  decoded object checksums, and exposes scrub counters in repair status.
- Object-level dedupe has a leader-only background scanner that reconciles
  committed object hashes, blob reference counts, orphan blob refs, and object
  layouts against canonical blob refs.
- Cluster backend now exposes a normalized node registry that merges Raft
  membership, gossip discovery state, and storage node registration/liveness.
- Cluster node registry, metrics, and rebalance/decommission APIs now expose
  current object, shard, and used byte ownership derived from committed shard
  layouts. Rebalance plan/action now validates Raft membership before scanning
  or moving object layouts, replacement reports the old node's logical bytes, and
  replacement validation maps local/missing replicas to explicit client errors.
  Node registry now exposes explicit storage lifecycle state and flags drained
  storage nodes that still own object shards. Decommission conflict responses now
  include remaining object, shard, and logical byte counts.
- Cluster lifecycle now returns structured blocked responses for Raft address
  changes and removed replica reappearance, reports the same conditions in
  reconcile plans and node registry issues, and treats empty member rebalance as
  an idempotent no-op.
- Object read errors for decoded object corruption and unrecoverable shard
  recovery failures are surfaced as explicit `503 Service Unavailable` responses.
  Read-path tests now cover unrecoverable shard corruption.
- Orphan shard cleanup treats fresh staged pending writes as live shard sets so
  recovery cannot delete in-flight `BlobPrepared` data before the pending TTL.
- Expired overwrite recovery restores the committed object layout from the
  committed blob ref before removing replacement orphan shards.
- Expired retained overwrite recovery releases the replacement blob ref and keeps
  the committed object readable.
- Recovery results now expose pending cleanup action counts for staged deletes,
  layout rollbacks, blob releases, and committed stale cleanup.
- Repair tests now cover rebuilding a missing local shard and verifying the
  object remains readable after repair.
- Repair tests now cover rebuilding a corrupted local shard through explicit
  repair, not only through read-time reconstruction.
- Repair now returns current health in `HealthAfter` for unrecoverable shard
  loss, and tests cover too many missing shards returning `ErrShardRecoveryFailed`.
- Overwrite tests now verify replaced blob refs and shard sets are released only
  after the new committed object remains readable.
- Delete tests now verify shared dedupe blob refs and shard sets survive until
  the last object reference is removed.
- Index rebuild now prunes stale search records for deleted objects, so missed
  delete events can be reconciled from committed metadata.
- Metrics now expose background dedupe scanner running state, scan counts,
  fix counts, drift/orphan layout findings, reclaimed bytes, and limit status.
- Metrics now expose content indexing queue, retry, drop, success, failure, and
  rebuild counters for operational visibility into search freshness.
- Metrics now expose storage recovery completion, failure state, pending action
  counts, and orphan shard cleanup counts.
- Metrics now expose aggregate HTTP request counters, error counters, status
  class counters, and request duration totals/max values.
- Metrics now expose current in-flight HTTP requests for load and stuck-request
  visibility.
- Metrics now expose Raft local replica ID, leader availability, local leader
  state, config change ID, and voting/non-voting/witness/removed member counts.
- Readiness checks now distinguish storage writability, Raft membership, Raft
  leader availability, and repair backlog state.
- Operational delivery now includes a checked-in `config.example.json` that
  matches the runtime config schema and Docker image expectations, plus Docker
  build/run deployment notes.
- Operational delivery now includes Docker Compose examples for single-node and
  three-node local clusters, including readiness, metrics, member inspection,
  and cleanup commands.
- Operational delivery now includes a smoke-test guide covering process
  readiness, metrics, object write/read/delete, range reads, search indexing,
  repair, dedupe, recovery, and cluster status checks.
- Operational delivery now includes systemd service and environment examples
  for non-container deployments, with install, startup, log, and readiness
  commands documented.
- P0 test coverage now includes three-node remote shard write/read placement,
  restart recovery for fresh and expired pending writes, and cluster lifecycle
  idempotency/leader diagnostics for bootstrap, join, decommission, rebalance,
  readiness, and metrics.
- P0 test coverage now verifies object updated/deleted events are published
  only after successful committed object mutations, failed put/delete operations
  do not publish events, and overwrite events match the final committed object.
- P0 security now separates node-to-node shard transport from admin access with
  `cluster_token` / `MAXIO_CLUSTER_TOKEN`, and remote shard calls send
  `X-Maxio-Cluster` by default.
- S3 compatibility coverage now includes presigned reads, range reads, invalid
  range XML errors, stable ETags, list ETags, and a documented supported subset.
- S3 compatibility coverage now exercises the supported object, range,
  presign, and multipart paths through the official AWS SDK for Go v2 client.
- Operational delivery now documents data layout, backup, restore, and upgrade
  procedures alongside the deployment guide.
- CI now runs GitHub Actions checks for Go tests, golangci-lint, package
  builds, release binary builds, and Docker image build validation on pushes and
  pull requests targeting `main`.
- Release automation now uses GoReleaser on SemVer tags to publish multi-OS
  archives, Linux `deb`/`rpm`/`apk` packages, checksums, and GHCR Docker images
  built from UPX-compressed Linux binaries.
- HTTP responses now include a generated or client-supplied request ID, and
  audit logs include the same request_id for request-to-log correlation.
- Basic S3-compatible HTTP endpoints exist, but S3 compatibility is not yet a
  production target.

## Production gaps

## Minimum production MVP gates

MaxIO should not be called production-ready until all P0 gates below are
complete. These gates define the smallest acceptable production target: a small
trusted cluster that can safely store non-temporary data, recover from common
failures, expose enough operational state, and be deployed repeatably.

### P0.1 Data safety gate

Goal: committed object data must remain readable or explicitly report
unrecoverable corruption after common single-node, process, and shard failures.

Acceptance criteria:

- Multi-node write/read tests cover remote shard placement for at least three
  nodes.
- Partial write and process restart tests prove pending objects are either
  completed, rolled back, or garbage-collected.
- Missing shard and corrupted shard tests prove repair can restore data from
  healthy shards when enough shards remain.
- Delete and overwrite tests prove old blob refs, object layouts, index records,
  and events are updated only after committed metadata changes.
- Object and shard checksum verification is enforced on read, scrub, and repair.

### P0.2 Cluster lifecycle gate

Goal: operators can safely bootstrap, join, drain, replace, rebalance, and
decommission nodes without guessing cluster state.

Acceptance criteria:

- Bootstrap and join are idempotent across repeated startup and leader changes.
- Drain, replace, rebalance, and decommission APIs are idempotent where possible
  and return structured blocked states when data still exists on a node.
- Node registry exposes membership, discovery, storage registration, storage
  lifecycle state, shard ownership, logical bytes, and actionable issues.
- Address changes and removed-node reappearance are handled explicitly.
- Multi-node tests cover leader change during membership operations.

### P0.3 Security gate

Goal: no data or admin API can be exposed outside a fully trusted development
network without authentication and internal API protection.

Acceptance criteria:

- Admin APIs require authentication.
- Object APIs require access key authentication and bucket/object authorization.
- Internal shard and cluster APIs require a cluster token or equivalent
  machine-to-machine authentication.
- TLS can be configured for external HTTP traffic.
- Audit logs exist for admin operations and object mutations.

### P0.4 S3 core compatibility gate

Goal: the S3 layer supports the minimum behavior required by common SDK clients
without pretending to be a complete S3 implementation.

Acceptance criteria:

- AWS Signature V4 is implemented for supported S3 routes.
- Access key and secret key management is available.
- `PUT`, `GET`, `HEAD`, `DELETE`, `LIST`, bucket create/delete, range reads,
  and presigned URLs are covered by compatibility tests.
- Multipart upload is implemented or explicitly rejected with S3-compatible
  errors until implemented.
- XML errors, status codes, and ETag behavior are documented and tested for the
  supported subset.

### P0.5 Observability gate

Goal: operators can determine whether the service is healthy, degraded, or
unsafe before data loss becomes invisible.

Acceptance criteria:

- Health and readiness endpoints distinguish process up, metadata available,
  leader available, storage writable, and repair backlog states.
- Metrics cover request latency, throughput, errors, Raft leader/state changes,
  storage ownership, repair/scrub/dedupe progress, and S3 status classes.
- Structured logs include request IDs or trace IDs for object and admin
  operations.
- Repair, scrub, dedupe, rebalance, and decommission expose last run, current
  status, failures, and retry counters.

### P0.6 Operational delivery gate

Goal: the same build can be deployed and restored repeatably outside a developer
workstation.

Acceptance criteria:

- Docker image and example config files are available.
- Single-node and three-node deployment docs exist.
- Data directory layout, backup, restore, and upgrade procedures are documented.
- Configuration defaults are safe for local development, while production docs
  call out required secrets, data paths, and network ports.
- Smoke-test commands are documented for startup, object write/read, S3 access,
  repair status, and cluster node status.

### MVP non-goals

The first production MVP does not need to be a complete S3 clone, a globally
distributed object store, or a chunk-level dedupe system.

Explicit non-goals:

- Full S3 API surface beyond the documented supported subset.
- Cross-region replication.
- Multi-tenant billing or quota management.
- Chunk-level dedupe.
- Custom binary TCP protocol.
- Advanced placement across racks, zones, or disks beyond node-level separation.

### 1. Distributed write path

The biggest missing piece is the true distributed object write path. Object data
still needs to flow through node placement, shard persistence, quorum or commit
rules, metadata commit, index update, and event publication as one recoverable
pipeline.

Required work:

- Add a `StorageNode` abstraction for local and remote shard targets.
- Add a `PlacementPlanner` that chooses shard targets based on membership,
  capacity, node health, and failure domains.
- Implement a distributed write pipeline that writes data shards and parity
  shards to selected nodes before metadata is committed.
- Make object reads resolve shard locations from metadata and reconstruct from
  remote shards when needed.
- Define failure semantics for partial writes, pending metadata, and retries.

### 2. Write consistency and recovery

The system needs a clear object lifecycle so crashes and partial failures can be
recovered deterministically.

Required work:

- Define states such as pending, committed, deleted, and tombstoned.
- Persist enough write-intent data to recover after process or node crashes.
- Add orphan shard garbage collection.
- Add pending object expiration and retry policy.
- Ensure index and event publication follow committed metadata, not partial data.

### 3. Data verification and self-healing

Repair exists, but production repair needs stronger verification and operational
controls.

Required work:

- Store and verify per-shard checksums.
- Add object-level checksum validation.
- Add background scrub jobs for bitrot detection.
- Add repair rate limiting and retry backoff.
- Expose repair progress, failures, and last-run status.
- Support repair from remote healthy replicas or shards.

### 4. Cluster membership lifecycle

Membership exists at the Raft and discovery layer, but production clusters need
safe operational flows.

Required work:

- Implement join and bootstrap flows that are safe for repeated startup.
- Add node drain, decommission, and rebalance flows.
- Track node liveness, disk capacity, and shard ownership.
- Handle node loss, node replacement, and address changes.
- Add admin APIs and Web UI screens for membership operations.

### 5. Replication and erasure placement

Erasure coding exists locally, but the placement model must be cluster-aware.

Required work:

- Define storage classes such as replicated and erasure-coded.
- Place chunks across distinct nodes and, later, distinct failure domains.
- Prevent unsafe layouts where too many chunks land on one node or disk.
- Add rebalancing when nodes are added or removed.
- Add read repair when stale or missing shards are detected during reads.

### 6. S3 compatibility

The S3 layer should remain a compatibility layer over the MaxIO core API. It is
not ready for production yet.

Required work:

- Add AWS Signature V4 authentication.
- Add access keys, secret keys, and request authorization.
- Implement multipart upload.
- Implement presigned URLs.
- Implement range reads and correct ETag semantics.
- Align XML error responses and status codes with S3 behavior.
- Add S3 compatibility tests.

### 7. Security and access control

The service should not be exposed outside a trusted development environment
until security primitives are implemented.

Required work:

- Add admin authentication.
- Add access key and secret management.
- Add bucket/object authorization model.
- Support TLS configuration.
- Add audit logs for data and admin operations.
- Protect internal cluster APIs.

### 8. Observability

Production operation requires clear visibility into cluster and storage behavior.

Required work:

- Add metrics for request latency, throughput, error rates, and object sizes.
- Add metrics for Raft state, leader changes, membership, and apply latency.
- Add metrics for storage capacity, shard health, repair, and scrub.
- Add tracing around write/read/delete/search paths.
- Add structured audit logs.
- Expose health and readiness checks suitable for orchestration.

### 9. Testing and failure injection

The current test coverage is not enough to validate object storage correctness.

Required work:

- Add integration tests for single-node and multi-node clusters.
- Add crash recovery tests for partial writes and pending metadata.
- Add network partition and leader-change tests.
- Add corrupted shard and missing shard tests.
- Add concurrent put/get/delete/list tests.
- Add S3 compatibility tests.
- Add long-running soak tests.

### 10. Packaging and operations

The project needs production deployment assets and upgrade guidance.

Required work:

- Add Docker image build.
- Add example config files.
- Add systemd and container deployment examples.
- Add Kubernetes examples after the cluster model stabilizes.
- Document data directory layout.
- Document backup, restore, and upgrade procedures.

## Recommended implementation order

### Phase 1: Distributed storage core

- Add `StorageNode` and `PlacementPlanner`.
- Implement local node through the same abstraction used by future remote nodes.
- Persist shard placement metadata.
- Update object read/write paths to use placement metadata.
- Add integration tests for object write/read over planned shard layouts.

### Phase 2: Recoverable writes

- Add object lifecycle states.
- Add write-intent persistence.
- Add orphan shard garbage collection.
- Add pending object recovery on startup.
- Make index and events strictly follow committed metadata.

### Phase 3: Cluster-aware data durability

- Add remote shard write/read transport.
- Distribute data and parity shards across multiple nodes.
- Add read reconstruction from remote shards.
- Add read repair.
- Extend repair scheduler to repair from remote healthy shards.

### Phase 4: Cluster operations

- Harden bootstrap and join flows.
- Add node drain and decommission.
- Add rebalancing.
- Add admin APIs and Web UI views for cluster operations.

### Phase 5: Compatibility, security, and operations

- Complete S3 SigV4 and multipart upload.
- Add access control and admin authentication.
- Add metrics, tracing, and audit logs.
- Add deployment assets and operational documentation.

## Immediate next step

The next implementation should continue turning the prototype into a production
storage service in small, testable iterations.

1. Repair hardening

- Store and verify per-shard checksums.
- Add object-level checksum validation.
- Add background scrub jobs for bitrot detection.
- Add repair rate limiting and retry backoff.
- Expose repair progress, failures, and last-run status.
- Repair missing or corrupted shards from remote healthy shards.

2. Cluster lifecycle

- Make bootstrap and join flows idempotent across restarts.
- Add drain, decommission, and rebalance flows.
- Track node liveness, disk capacity, and shard ownership.
- Handle node loss, node replacement, and address changes.
- Add admin APIs and Web UI screens for cluster membership operations.

3. S3 compatibility

- Add AWS Signature V4 authentication.
- Add access keys, secret keys, and request authorization.
- Implement multipart upload.
- Implement presigned URLs.
- Implement range reads and correct ETag semantics.
- Align XML error responses and status codes with S3 behavior.
- Add S3 compatibility tests.

4. Security

- Add admin authentication.
- Add access key and secret management.
- Add bucket/object authorization.
- Support TLS configuration.
- Add audit logs for data and admin operations.
- Protect internal cluster and shard APIs.

5. Observability

- Add metrics for request latency, throughput, error rates, and object sizes.
- Add Raft metrics for state, leader changes, membership, and apply latency.
- Add storage metrics for capacity, shard health, repair, and scrub.
- Add tracing around write/read/delete/search paths.
- Add structured audit logs.
- Expose health and readiness checks suitable for orchestration.

6. Test coverage

- Add multi-node integration tests beyond the current remote shard transport
  loop.
- Add leader-change, network-partition, and node-restart tests.
- Add partial write and crash recovery tests.
- Add corrupted shard and missing shard tests.
- Add concurrent put/get/delete/list tests.
- Add S3 compatibility tests.

7. Operations

- Add Docker image build.
- Add example configuration files.
- Add systemd and container deployment examples.
- Add Kubernetes examples after the cluster model stabilizes.
- Document data directory layout.
- Document backup, restore, and upgrade procedures.
