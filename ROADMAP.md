# MaxIO roadmap

MaxIO is moving from a full S3/storage-engine service to a stateless S3 proxy
with DB metadata and a rebuildable Bleve file index.

## Product target

- S3 upstream stores object bytes.
- MaxIO stores metadata, dedupe relationships, index state, and operational
  events.
- Metadata DB is the source of truth.
- Bleve is a derived index and can be rebuilt.
- Gateway instances are stateless and horizontally scalable.
- Object-level dedupe supports two evolution paths: observe first, alias later.

## Architecture decisions

- Remove Raft as the default control-plane foundation.
- Do not store authoritative object bytes in MaxIO local shard files.
- Persist object visibility, routing, fingerprints, and index status in DB.
- Use DB transactions and leases for write state, worker queues, rebuilds, and
  recovery.
- Keep S3 API as the first-class public data plane.
- Keep management APIs for health, readiness, metrics, indexing, repair, and
  dedupe operations.

## Current implementation status

Status as of 2026-06-13:

- P0 metadata foundation is partially implemented: repository interfaces,
  canonical object/version/digest/index/outbox models, SQLite schema, in-memory
  implementation, and SQLite lifecycle tests exist.
- PostgreSQL is present as a backend direction, but production validation,
  migrations, and schema compatibility checks still need hardening.
- S3 upstream metadata registration and management APIs exist.
- S3 data-path metadata capture is not complete: proxy PUT/GET/HEAD/DELETE are
  not yet fully committed through DB object-version transitions.
- File indexing is partially implemented: Bleve search exists, and the first
  index job state machine plus worker abstraction exists.
- Index queue execution is not complete: jobs are not yet leased from metadata
  DB and wired into the runtime worker loop.
- Dedupe is still metadata-first: digest reference structures exist, but
  observe-mode reporting is not yet connected to the proxy write path.
- Raft/local object-shard storage remains legacy code and is not part of the
  target default product path.

## P0: Metadata foundation

Goal: make DB-backed metadata the durable runtime center.

- Define metadata repository boundaries for tenants, buckets, upstreams,
  routes, object keys, object versions, fingerprints, dedupe links, index jobs,
  and audit/outbox events.
- Implement SQLite for local development and PostgreSQL for production.
- Add migrations and startup compatibility checks.
- Define DB transaction boundaries for PUT, DELETE, COPY, and index enqueue.
- Make object visibility depend on DB state, not local process state.

Acceptance criteria:

- A gateway can restart without losing committed object metadata.
- A second gateway can read the same metadata DB and serve the same logical
  bucket/key namespace.
- Startup readiness fails when DB is unavailable or schema is incompatible.

## P1: S3 proxy write/read path

Goal: route S3 traffic through stateless gateways while preserving metadata.

- Register upstream S3 endpoints and bucket route mappings in metadata.
- Proxy PUT/GET/HEAD/DELETE/COPY through upstream S3.
- Record upstream location, etag, size, checksum, version ID, and commit state.
- Add idempotency handling for write retries.
- Reconcile pending writes and orphan upstream objects.

Acceptance criteria:

- PUT writes bytes to upstream S3 and commits visible metadata.
- GET/HEAD resolve through DB metadata and stream from upstream S3.
- DELETE updates DB visibility and creates upstream delete or repair work
  according to policy.
- Crash recovery can identify pending, orphaned, and repair-fault states.

## P2: File indexing and rebuild

Goal: make content search an asynchronous, rebuildable capability.

- Store index jobs, leases, attempts, schema version, and document state in DB.
- Build Bleve documents from object metadata and upstream bytes.
- Make workers idempotent and lease-based.
- Add index status and rebuild APIs.
- Implement schema-version-aware rebuild.
- Ensure search results resolve back through DB visibility checks.

Acceptance criteria:

- Deleting the Bleve directory and triggering rebuild restores searchable
  documents from DB and upstream bytes.
- Failed jobs retry with backoff and expose last error.
- Search never returns objects that DB marks deleted or invisible.

## P3: Object-level dedupe

Goal: support safe dedupe rollout with observe mode before alias mode.

- Compute object fingerprints on write or via background jobs.
- Group object versions by fingerprint and size.
- Expose duplicate groups and duplicate byte estimates.
- Implement observe mode without changing upstream object locations.
- Define alias mode policy, canonical object selection, reference tracking, and
  safe delete semantics.
- Add alias-mode repair checks before enabling it as a production option.

Acceptance criteria:

- Observe mode reports duplicates without changing read behavior.
- Dedupe grouping can be rebuilt from DB fingerprints.
- Alias mode is gated by explicit bucket or tenant policy.

## P4: Consistency, repair, and operations

Goal: make recovery and day-2 operations explicit.

- Compare DB object versions with upstream HEAD/GET results.
- Mark unreadable upstream bytes as repair faults.
- Detect stale/missing Bleve documents and enqueue re-index.
- Detect orphan upstream objects without DB rows.
- Add metrics for DB transactions, index queue depth, rebuild progress, dedupe
  savings, upstream errors, and repair faults.
- Update backup, restore, and upgrade procedures for DB-first operation.

Acceptance criteria:

- Operators can distinguish DB faults, upstream byte faults, and index drift.
- Repair actions are queued and auditable.
- Backup/restore docs clearly identify authoritative and derived state.

## P5: Hardening and compatibility

Goal: prepare the proxy runtime for production use.

- Complete S3 compatibility contracts for pagination, error mapping, range
  reads, metadata, multipart upload, and versioning behavior.
- Harden auth boundaries for S3 traffic and management APIs.
- Add rate limits and worker concurrency controls.
- Add deployment templates for multi-gateway plus external DB.
- Document supported upstream S3 providers and required capabilities.

## Explicit non-goals for v1

- Chunk-level dedupe.
- Cross-region active-active replication.
- Local object shard storage as a default product path.
- Gateway-local Raft as the control plane.
- Treating Bleve as authoritative object state.

## Immediate next steps

1. Finalize metadata schema and migration plan.
2. Wire S3 proxy PUT/GET/HEAD/DELETE to DB object-version transitions.
3. Implement DB-backed index queue and Bleve rebuild.
4. Ship observe-mode dedupe reporting.
5. Add consistency scanner for DB, upstream S3, and Bleve drift.
