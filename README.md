# MaxIO

MaxIO is being rebuilt as a stateless S3 proxy with DB-backed metadata and a
rebuildable Bleve file index.

The upstream S3-compatible service stores object bytes. MaxIO stores metadata,
dedupe relationships, index state, and operational events.

## Current product direction

- S3 proxy is the primary data plane.
- Gateway instances should be stateless and horizontally scalable.
- Metadata DB is the source of truth.
- Bleve is derived state and can be rebuilt.
- Object-level dedupe starts in observe mode and can evolve to alias mode.
- Gateway coordination, indexing, rebuild, and recovery are DB-backed; no
  gateway-local membership or local byte-store is authoritative.
- Object processing is an optional pipeline boundary. The default mode is
  disabled; deployments can later enable post-commit or inline-strict processors
  such as Tika, ClamAV, OCR, DLP, or policy engines without making them hard
  dependencies of the S3 proxy. Built-in optional processors include ClamAV
  over clamd TCP and Apache Tika over the Tika Server HTTP API.

## Core documents

- `docs/architecture.md` - proxy-only architecture and component boundaries.
- `docs/metadata-indexing.md` - metadata tables/entities, write path, async
  indexing, dedupe modes, rebuild, and consistency strategy.
- `docs/processing.md` - optional object processing pipeline, modes, and
  ClamAV/Tika processor configuration.
- `docs/data-layout.md` - authoritative vs derived state and local runtime
  directories.
- `docs/deployment.md` - deployment and configuration guidance.
- `docs/seaweed-k6.md` - local multi-upstream SeaweedFS S3 topology,
  k6 load-test entrypoint, and `processing-k6` smoke preset for ClamAV/Tika.
- `ROADMAP.md` - staged implementation roadmap.
## Code layout

- `cmd/maxio` is the executable entrypoint.
- `internal/app` owns process assembly and startup wiring.
- `internal/cache` and `internal/object` are internal implementation packages;
  they are no longer public root-level libraries.
- Any legacy local object-store implementation is internal compatibility code
  only. Proxy mode treats upstream S3 bytes and DB metadata as the durable
  boundary.

## Minimal local configuration

Start from `config.example.json` for local development. For a more explicit
proxy-oriented shape, see `config.proxy.example.json`.

Key settings:

```json
{
  "metadata_backend": "sqlite",
  "metadata_dsn": "./data/maxio.db",
  "metadata_auto_migrate": true,
  "enable_s3_proxy": true,
  "s3_proxy_entrypoint": ":8080",
  "processing_enabled": false,
  "processing_mode": "async_permissive",
  "processing_clamav_enabled": false,
  "processing_clamav_mode": "inline_strict",
  "processing_clamav_address": "clamav:3310",
  "processing_tika_enabled": false,
  "processing_tika_mode": "async_permissive",
  "processing_tika_fail_open": false,
  "processing_tika_url": "http://tika:9998",
  "processing_tika_max_bytes": 104857600
}
```

Production should use an external DB, usually PostgreSQL, and one or more
configured upstream S3 routes.

## Operational model

Authoritative state:

- metadata DB;
- upstream S3 object bytes;
- configuration and secret references.

Derived or rebuildable state:

- Bleve index under `data_dir/index/bleve`;
- index queues and rebuild jobs recreated from committed DB object versions;
- dedupe grouping recreated from fingerprints;
- process-local temporary files.

## Dedupe modes

- `observe`: record duplicate fingerprints and groups without changing where
  object versions read bytes from. This is the recommended first production
  mode.
- `alias`: allow multiple logical object versions to resolve to canonical bytes.
  This requires stricter delete, repair, and lifecycle controls.

## Implementation status

Status as of 2026-06-24:

Implemented:

- Proxy-only product direction is documented.
- Metadata backend selection supports SQLite for local development and a
  PostgreSQL-oriented backend path.
- Metadata repository now has first-pass canonical models for upstreams, object
  records, object versions, digest references, index documents, index jobs, and
  index outbox events.
- SQLite schema and repository methods cover the first metadata catalog
  lifecycle.
- S3 upstream registration is stored in metadata and exposed through management
  APIs.
- Scheduler leasing is SQL-backed for DB-owned runtime work.
- Bleve search exists, `/_search` routes through `index.SearchEngine`, and a
  first index job state machine plus worker abstraction exists.

Not yet implemented:

- S3 proxy PUT/GET/HEAD/DELETE are not yet fully wired to canonical DB object
  record/version transitions.
- Runtime index worker loop hardening, `/_index/status`, `/_index/rebuild`,
  and stale-document repair still need code-side confirmation.
- Dedupe observe reports are not yet connected to the proxy write path.
- DB migrations, schema compatibility checks, and production PostgreSQL
  validation still need hardening.
- Full S3 compatibility, auth hardening, metrics, tracing, and operational
  repair workflows remain roadmap items.

## Development status

The previous full storage-engine direction is closed. Remaining product work
should stay within the stateless proxy, DB metadata, upstream S3 bytes, and
derived Bleve index model.

Remaining gaps are target-architecture work: completing the proxy data path,
hardening DB-leased index workers, confirming `/_index/status` and
`/_index/rebuild`, connecting dedupe observe reporting to proxy writes, and
hardening migrations, PostgreSQL, S3 compatibility, auth, metrics, tracing, and
repair workflows.


