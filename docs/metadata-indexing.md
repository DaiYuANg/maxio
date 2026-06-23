# Metadata and file indexing design

This document defines the target metadata and indexing model for the stateless
S3 proxy architecture.

## Design principles

- Metadata DB is the source of truth.
- Upstream S3 is the source of truth for bytes.
- Bleve is a derived, rebuildable file index accessed through
  `index.SearchEngine`.
- Gateway instances are stateless and can process the same work through DB
  leases and idempotent writes.
- Object-level dedupe starts in observe mode and can evolve to alias mode behind
  explicit bucket or tenant policy.

## Core entities

### Tenant

Represents an isolation boundary for routes, policy, credentials, and index
visibility.

Suggested fields:

```text
id
name
status
created_at
updated_at
```

### Bucket

Represents a logical S3 bucket exposed by MaxIO.

Suggested fields:

```text
id
tenant_id
name
versioning_mode
dedupe_mode          # off | observe | alias
index_mode           # off | metadata | content
status
created_at
updated_at
```

### Upstream

Represents an S3-compatible upstream service.

Suggested fields:

```text
id
name
endpoint
region
credential_ref
health_status
capabilities_json
created_at
updated_at
```

Credentials should be stored in a secret manager or encrypted configuration
where possible. DB rows should prefer references over plaintext secrets.

### Bucket route

Maps a logical MaxIO bucket to an upstream bucket/prefix.

Suggested fields:

```text
id
tenant_id
bucket_id
upstream_id
upstream_bucket
upstream_prefix
priority
read_policy          # primary | fallback | mirror-read
write_policy         # primary | mirror | disabled
status
created_at
updated_at
```

### Object key

Stable identity for a logical bucket/key pair.

Suggested fields:

```text
id
tenant_id
bucket_id
object_key
created_at
updated_at
```

### Object version

A visible or historical logical object version.

Suggested fields:

```text
id
object_key_id
version_id
status               # write_pending | visible | delete_marker | deleted | repair_fault
upstream_id
upstream_bucket
upstream_key
upstream_version_id
size_bytes
etag
checksum_sha256
content_type
user_metadata_json
blob_fingerprint_id
write_idempotency_key
committed_at
deleted_at
created_at
updated_at
```

The latest visible version is resolved from DB state, not from Bleve.

### Blob fingerprint

Object-level content identity used for dedupe grouping.

Suggested fields:

```text
id
algorithm            # sha256 initially
fingerprint
size_bytes
first_seen_object_version_id
ref_count
created_at
updated_at
```

### Dedupe group

Groups object versions that share a fingerprint.

Suggested fields:

```text
id
blob_fingerprint_id
canonical_object_version_id
mode                 # observe | alias
status
created_at
updated_at
```

### Dedupe link

Records how a logical version relates to the canonical version.

Suggested fields:

```text
id
dedupe_group_id
object_version_id
canonical_object_version_id
link_type            # observed_duplicate | metadata_alias
status
created_at
updated_at
```

### Index job

DB-backed work item for asynchronous indexing.

Suggested fields:

```text
id
object_version_id
job_type             # index | delete | rebuild | verify
status               # pending | leased | indexed | skipped | failed | stale
attempts
lease_owner
lease_expires_at
next_run_at
last_error
schema_version
created_at
updated_at
```

### Index document

DB record for the Bleve document that should exist.

Suggested fields:

```text
id
object_version_id
bleve_document_id
schema_version
content_hash
status               # present | stale | missing | deleted | failed
indexed_at
last_verified_at
last_error
created_at
updated_at
```

### Outbox event

Transactionally recoverable side effect for index, dedupe, audit, and repair
workers.

Suggested fields:

```text
id
aggregate_type
aggregate_id
event_type
payload_json
status
attempts
next_run_at
created_at
updated_at
```

## Write path

### PUT object

```text
1. Resolve tenant, bucket, route, and upstream.
2. Insert or update object_key.
3. Insert object_version with write_pending and idempotency key.
4. Stream bytes to upstream S3.
5. Capture upstream etag, version id, size, checksums, and content type.
6. Compute fingerprint inline for small objects or enqueue fingerprint work.
7. Commit object_version as visible.
8. Create index_job and dedupe/outbox work in the same DB transaction.
```

The gateway must handle retries by looking up the idempotency key and resolving
the existing write state instead of creating duplicate visible versions.

### DELETE object

```text
1. Resolve object_key.
2. Insert a delete_marker or mark the target version deleted, depending on
   versioning policy.
3. Write upstream delete when policy requires immediate upstream deletion.
4. Enqueue Bleve delete or stale-document work.
5. Update dedupe references if alias mode is enabled.
```

Delete visibility is governed by DB state. Upstream deletion failures create
repair work rather than silently rolling back already-visible DB transitions
unless the operation is still inside a strict transaction boundary.

### COPY object

COPY should create a new object_version row. It may share the same fingerprint
or alias target after dedupe processing, but the logical version identity remains
distinct.

## Asynchronous indexing

Index workers lease DB jobs:

```text
pending -> leased -> indexed
pending -> leased -> failed -> pending
pending -> skipped
indexed -> stale -> pending
```

Worker rules:

- Lease by DB transaction with `lease_owner` and `lease_expires_at`.
- Use stable Bleve document IDs derived from object_version ID and schema
  version.
- Re-check object_version status before indexing.
- Fetch bytes from upstream S3 only after confirming the DB version is still
  visible and indexable.
- Replace Bleve documents atomically where supported by Bleve.
- Update `index_documents` after Bleve write succeeds.
- Back off and retry transient upstream, extraction, or Bleve errors.
- Mark permanent unsupported file types as `skipped`, not `failed`.

`/_index/status` should report the DB-owned job, lease, and document state used
by these workers.

## File index schema

Bleve documents should include:

```text
tenant_id
bucket_id
bucket_name
object_key
object_version_id
version_id
size_bytes
etag
checksum_sha256
content_type
user_metadata
fingerprint
dedupe_group_id
indexed_text
detected_language
file_extension
created_at
committed_at
schema_version
```

Fields used for access control or object visibility must be checked again
against DB rows at query time.

## Search path

```text
1. Query `index.SearchEngine` for candidate document IDs.
2. Load matching object_versions from DB.
3. Filter by tenant, bucket policy, visible status, delete markers, and caller
   authorization.
4. Return object metadata from DB, optionally with Bleve highlights.
```

The `/_search` endpoint is a caller of `index.SearchEngine`; it must not bypass
DB visibility checks. Bleve must never return an object that DB no longer
considers visible.

## Dedupe modes

### Observe mode

Observe mode records duplicate candidates without changing upstream object
locations.

Use observe mode for:

- initial rollout;
- reporting duplicate storage usage;
- validating fingerprint quality;
- proving repair and rebuild behavior;
- preparing alias migrations.

Behavior:

```text
object_version.upstream_* remains the client-written upstream object
blob_fingerprints and dedupe_links record duplicates
delete only affects the logical object version
repair is straightforward because every version has its own upstream location
```

### Alias mode

Alias mode lets multiple object versions refer to a canonical byte identity.

Use alias mode only when:

- upstream retention and versioning are understood;
- delete and lifecycle policy preserve canonical bytes while references exist;
- repair can verify canonical object presence;
- query and read paths resolve aliases consistently.

Behavior:

```text
dedupe_group.canonical_object_version_id points at canonical bytes
dedupe_links link duplicate versions to canonical version
read path resolves duplicate logical versions to canonical upstream location
delete decrements references and only removes canonical bytes when safe
```

Alias mode should be configured per bucket or tenant. It should not be a global
default until reconciliation tooling is mature.

## Rebuild strategy

### Rebuild Bleve through `/_index/rebuild`

```text
1. Mark all active index_documents stale for the target schema version.
2. Page through visible object_versions from DB by tenant/bucket/key/version cursor.
3. Insert rebuild index_jobs idempotently.
4. Workers recreate Bleve documents from DB metadata and upstream bytes.
5. Missing or failed upstream objects become repair_fault candidates.
6. Mark old schema-version documents deleted after the new schema is complete.
```

### Rebuild dedupe groups

```text
1. Scan visible object_versions with known fingerprints.
2. Group by algorithm, fingerprint, and size.
3. Upsert blob_fingerprints.
4. Upsert dedupe_groups and dedupe_links.
5. In observe mode, do not mutate upstream locations.
6. In alias mode, require an explicit migration plan before changing canonical
   pointers.
```

### Reconcile upstream

```text
1. Sample or scan DB object_versions.
2. HEAD upstream location.
3. Compare size, etag, checksum, and version ID where available.
4. Mark missing or mismatched objects as repair_fault.
5. Enqueue re-index for metadata drift that does not affect object visibility.
```

## Consistency rules

- DB visible state wins over Bleve.
- Upstream HEAD/GET confirms byte availability but does not create logical
  MaxIO visibility by itself.
- Bleve missing documents are repaired by re-indexing.
- DB metadata for unreadable upstream bytes is a repair fault.
- Orphan upstream bytes without DB rows are retained or deleted by explicit
  lifecycle policy.
- Alias mode must preserve canonical bytes while any visible dedupe link points
  at them.

## Failure handling

| Failure | Expected handling |
| --- | --- |
| Gateway crashes before DB commit | pending row expires or recovery marks it failed |
| Gateway crashes after upstream PUT before DB visible commit | recovery checks idempotency key and upstream result |
| DB commit succeeds but index enqueue fails | outbox or rebuild enumeration recreates index job |
| Bleve write fails | job retries with backoff |
| Upstream HEAD fails during index | mark job failed and create repair signal |
| Alias canonical object missing | mark all dependent versions repair_fault until restored |

## Operational metrics

Expose at least:

```text
metadata transaction latency and failures
index queue depth by status
index lease age and retry count
index rebuild cursor progress
Bleve document count by schema version
dedupe duplicate bytes observed
dedupe alias reference count
upstream HEAD/GET/PUT error rate
repair_fault count by reason
```
