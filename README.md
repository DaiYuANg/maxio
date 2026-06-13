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
- Local Raft/object-shard storage is legacy and not part of the target default
  architecture.

## Core documents

- `docs/architecture.md` - proxy-only architecture and component boundaries.
- `docs/metadata-indexing.md` - metadata tables/entities, write path, async
  indexing, dedupe modes, rebuild, and consistency strategy.
- `docs/data-layout.md` - authoritative vs derived state and local runtime
  directories.
- `docs/deployment.md` - deployment and configuration guidance.
- `ROADMAP.md` - staged implementation roadmap.

## Minimal local configuration

Start from `config.example.json` for local development. For a more explicit
proxy-oriented shape, see `config.proxy.example.json`.

Key settings:

```json
{
  "metadata_backend": "sqlite",
  "metadata_dsn": "./data/maxio.db",
  "metadata_auto_migrate": true,
  "enable_native_object_api": false,
  "enable_s3_proxy": true,
  "s3_proxy_entrypoint": ":8080"
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
- index queues recreated from committed DB object versions;
- dedupe grouping recreated from fingerprints;
- process-local temporary files.

## Dedupe modes

- `observe`: record duplicate fingerprints and groups without changing where
  object versions read bytes from. This is the recommended first production
  mode.
- `alias`: allow multiple logical object versions to resolve to canonical bytes.
  This requires stricter delete, repair, and lifecycle controls.

## Development status

Some code and older docs may still contain migration-era concepts from the
previous full S3/storage-engine design. New work should align with the
proxy-only direction described in the documents above.
