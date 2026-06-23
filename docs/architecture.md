# MaxIO architecture

MaxIO is a stateless S3 proxy with DB-backed metadata and a rebuildable Bleve
file index. The upstream S3 service stores object bytes. MaxIO stores routing
metadata, object metadata, dedupe relationships, index state, and operational
events.

## Product boundary

MaxIO is not a full object storage engine in the current product direction.
Cluster/gossip, embedded consensus, and local object-shard storage are outside
the target product boundary.

It does:

- expose an S3-compatible proxy entrypoint;
- route requests to one or more upstream S3-compatible services;
- persist object, version, fingerprint, dedupe, and index state in a database;
- build a file/content index in Bleve from committed metadata and upstream
  object bytes;
- provide management APIs for health, readiness, indexing, rebuild, and
  operational inspection.

It does not:

- store authoritative object bytes locally or maintain local object shards;
- use cluster/gossip membership or embedded consensus as the control plane;
- treat Bleve as authoritative state;
- require gateway-local state for horizontal scaling.

## Runtime shape

```text
client
  |
  v
MaxIO gateway instances  <----> metadata DB
  |                         \--> Bleve index directory
  |
  v
upstream S3-compatible storage
```

Gateway instances are interchangeable. They can be placed behind a load
balancer because every durable decision is recorded in the metadata database.
Local disk is only used for derived or temporary state such as Bleve index files,
logs, and transient upload buffers.

## Code organization

- `cmd/maxio` starts the executable process.
- `internal/app` assembles runtime modules and owns application startup.
- `internal/cache` and `internal/object` are internal runtime implementation
  packages, not public extension points.
- Root-level public packages should not define the product boundary; the S3
  proxy and DB-backed metadata model define the runtime shape.

## Components

- **S3 proxy entrypoint:** accepts S3-compatible requests, validates policy,
  selects an upstream route, forwards bytes, and records metadata transitions.
- **Metadata repository:** ACID database layer for tenants, buckets, upstreams,
  object versions, blob fingerprints, dedupe links, index jobs, cursors, and
  audit events.
- **Upstream registry:** DB-backed list of S3 endpoints, bucket mappings,
  credentials references, health state, and routing policy.
- **Indexer:** asynchronous worker that reads committed object metadata, fetches
  bytes from upstream S3 when needed, extracts text/file metadata, and writes a
  Bleve document.
- **Dedupe coordinator:** records object-level fingerprints and either observes
  duplicate candidates or aliases object metadata to a canonical blob identity,
  depending on bucket policy.
- **Management plane:** exposes health, readiness, metrics, index rebuild,
  dedupe inspection, and repair/reconciliation endpoints.

## API planes

### S3 compatibility plane

This is the primary public data plane. S3 requests are proxied to upstream S3
while MaxIO records metadata and indexing work.

Write-like requests use DB state transitions to make retries and partial
failures explicit. Read-like requests prefer metadata for routing and policy,
then stream object bytes from upstream S3.

### Management plane

The management plane is for operators and automation:

```text
/healthz
/readyz
/metrics
/_index/*
/_dedupe/*
```

Management endpoints must not be used as the primary object data plane.

### Native object API

The native object API is not part of the proxy-only runtime. S3 proxy entrypoints
and management APIs are the supported public interfaces.

## Metadata-first write path

For a normal object PUT:

1. Gateway authenticates the request and resolves tenant, bucket, key, and
   upstream route.
2. Gateway creates or updates a DB object-version row with `write_pending`
   status and an idempotency key derived from request identity.
3. Gateway streams bytes to upstream S3.
4. Gateway records upstream object location, size, etag, checksums, version ID,
   and commit timestamp in the DB.
5. Gateway computes or schedules content fingerprinting.
6. Gateway commits the visible object-version state.
7. Gateway enqueues index and dedupe work in the DB in the same transaction or
   by a transactionally recoverable outbox.

The DB commit is the source of truth for whether MaxIO considers an object
version visible. Upstream bytes without committed metadata are reconciled as
orphans. Metadata without readable upstream bytes is reconciled as a consistency
fault.

## Metadata-first read path

For GET/HEAD:

1. Gateway resolves the latest visible object version from the DB.
2. Gateway validates bucket policy, delete markers, version constraints, and
   dedupe alias state.
3. Gateway maps the version to its canonical upstream location.
4. Gateway streams bytes from upstream S3.
5. Gateway records read audit and optional consistency observations.

Reads do not require Bleve. Search results must resolve back through DB object
versions before returning object references to callers.

## Dedupe modes

Object-level dedupe is intentionally modeled as policy, not an always-on storage
mutation.

- **observe:** MaxIO records fingerprints and duplicate groups, but every object
  version continues to point at the upstream object written by the client. This
  is safe for early rollout, reporting, billing analysis, and later migration.
- **alias:** MaxIO may point duplicate object versions at a canonical blob
  identity in metadata. Alias mode requires stricter commit ordering, delete
  semantics, and repair tooling because metadata can make multiple logical
  objects depend on one upstream byte object.

The initial production recommendation is observe mode. Alias mode should be
enabled per bucket or tenant only after reconciliation and delete semantics are
fully exercised.

## Indexing model

Bleve is a derived local index. It stores searchable documents built from:

- DB object metadata;
- file metadata extracted from object content;
- optional text extraction from upstream object bytes;
- index schema version and source object-version identity.

The DB owns queueing and status:

```text
pending -> leased -> indexed
pending -> leased -> failed -> pending
pending -> skipped
indexed -> stale -> pending
```

Workers must be idempotent. Re-indexing the same object-version with the same
schema version should replace the same Bleve document ID.

See `docs/metadata-indexing.md` for the detailed table and consistency design.

## Rebuild and consistency

Because DB is authoritative and Bleve is derived, MaxIO supports destructive
Bleve rebuilds:

1. stop or pause index workers;
2. delete or replace the local Bleve index directory;
3. scan committed DB object versions;
4. enqueue rebuild jobs;
5. let workers repopulate Bleve;
6. compare DB index status and Bleve document counts.

Consistency checks compare DB object-version rows, upstream HEAD results, blob
fingerprints, and Bleve documents. The repair loop should favor DB state and
produce explicit jobs rather than silently mutating object visibility.

## Deployment intent

Production deployments should run multiple stateless gateways against the same
external DB and upstream S3 fleet. SQLite is acceptable only for local
development and single-process demos.

Operational docs:

- `docs/metadata-indexing.md`
- `docs/data-layout.md`
- `docs/deployment.md`
- `docs/backup-restore-upgrade.md`
