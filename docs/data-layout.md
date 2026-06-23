# MaxIO data layout

MaxIO's proxy-only layout separates authoritative state from derived state.
Object bytes live in upstream S3. MaxIO persists metadata in a database and can
rebuild its local Bleve file index from that metadata plus upstream object
content.

## Authoritative state

Authoritative state is stored in the metadata database:

```text
tenants
buckets
upstreams
bucket_routes
object_keys
object_versions
blob_fingerprints
dedupe_groups
dedupe_links
index_jobs
index_documents
outbox_events
audit_events
schema_migrations
```

The exact table names can evolve with migrations, but the ownership rule should
not change: if MaxIO needs a durable product decision, it belongs in the DB.

The DB stores:

- upstream S3 endpoint and route metadata;
- logical bucket/key/version identity;
- upstream object location, etag, size, checksum, and upstream version ID;
- object write/delete visibility state;
- object-level fingerprints and dedupe relationships;
- index queue, lease, retry, and schema-version state;
- reconciliation and audit events.

## Upstream S3 bytes

Upstream S3 stores all authoritative object bytes:

```text
s3://<upstream-bucket>/<upstream-prefix>/<object>
```

MaxIO metadata maps logical objects to upstream locations. The upstream location
may be one-to-one with the requested bucket/key in observe mode, or may point to
a canonical object in alias mode.

Operational rules:

```text
Do not treat MaxIO local disk as an object backup.
Back up upstream S3 according to the upstream provider's durability model.
Preserve upstream versioning where alias mode or rollback requirements need it.
Keep DB backups transactionally aligned with upstream operational recovery.
```

## Local runtime directories

`data_dir` is still useful, but it is not an object storage root in proxy mode.
It must stay limited to derived index files, temporary scratch, and optional
logs.

Default values:

```text
config.example.json: ./data
Docker/container:    /app/data
systemd example:     /var/lib/maxio/data
```

Expected proxy-mode layout:

```text
<data_dir>/
  index/bleve/        # derived search index, rebuildable
  tmp/                # transient local scratch, safe to clear when MaxIO is stopped
  logs/               # optional local logs if file logging is enabled
```

Do not add proxy-mode directories for authoritative object bytes. Upstream S3
and the metadata DB remain the durable boundary for object state.

## Metadata DB

Recommended production DB: PostgreSQL.

Development DB: SQLite.

The DB is the source of truth for:

- object visibility;
- object version lineage;
- delete markers;
- upstream routing;
- dedupe policy and links;
- index state;
- repair and rebuild cursors.

Backup priority:

```text
1. Metadata DB backup and migration version.
2. MaxIO configuration and secret material references.
3. Upstream S3 bucket/versioning configuration.
4. Local Bleve index only if avoiding rebuild time matters.
```

## Bleve index

Path:

```text
<data_dir>/index/bleve
```

Classification: derived, rebuildable.

Bleve documents should be keyed by stable DB identity, for example:

```text
object_version:<object_version_id>:schema:<index_schema_version>
```

The indexed document should include enough fields for search and filtering, but
must not become authoritative for object existence. Search results must always
resolve back to visible DB object versions before being returned.

Safe rebuild:

```text
1. Pause index workers or put the node in maintenance.
2. Remove or move <data_dir>/index/bleve.
3. Start MaxIO and trigger `/_index/rebuild`.
4. Rebuild enumerates DB object_versions and enqueues index_jobs.
5. Workers fetch bytes from upstream S3 and repopulate Bleve.
```

## Temporary upload and extraction files

Large uploads, checksum calculation, file sniffing, and text extraction may use
temporary files. These files are process-local scratch space.

Rules:

```text
They are not part of backup.
They may be deleted when MaxIO is stopped.
They must not be used to decide committed object visibility.
```

## What can be rebuilt

Rebuildable:

```text
Bleve index directory
Index job queue from committed object_versions
Dedupe grouping from blob_fingerprints
Readiness/cache state
Failed or stale outbox work, if represented in DB
```

Not rebuildable from MaxIO local disk:

```text
Metadata DB
Upstream S3 object bytes
Secrets or credential references not stored in config/secret manager
Object versions that never committed to DB
```

## Consistency checks

The repair loop should reconcile these sources:

```text
DB object_versions
DB blob_fingerprints
upstream S3 HEAD/GET metadata
Bleve document presence and schema version
index_jobs and index_documents status
```

Expected actions:

- DB row exists and upstream HEAD succeeds: object can remain visible.
- DB row exists and upstream HEAD fails: mark a repair fault; do not silently
  delete the object.
- Upstream object exists without committed DB row: classify as orphan and apply
  retention policy.
- Bleve document missing or stale: enqueue re-index.
- Dedupe group missing for known fingerprint: rebuild dedupe grouping from DB.

See `docs/metadata-indexing.md` for the entity model and rebuild strategy.
