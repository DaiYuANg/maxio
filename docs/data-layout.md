# MaxIO data layout

This document describes the runtime data written by MaxIO and how to treat each
area during operations. The examples use the default local layout. Container and
systemd deployments normally override the root with `MAXIO_DATA_DIR`.

## Root directories

`data_dir` is the primary persistent data root.

Default values:

```text
config.example.json: ./data
Docker Compose:      /app/data
systemd example:     /var/lib/maxio/data
```

`raft_data_dir` controls Dragonboat/Raft persistence. The default value is the
relative path `raft`. Relative values are resolved under `data_dir`, so the
default effective path is:

```text
<data_dir>/raft
```

If `raft_data_dir` is absolute, it is outside `data_dir` and must be backed up
and restored separately.

## Current layout summary

Typical runtime data:

```text
<data_dir>/
  raft/                       # Raft nodehost, WAL, snapshots, replicated metadata
  index/bleve/                # Derived Bleve search index
  s3-multipart/<upload-id>/   # In-progress S3 multipart upload state
  <shard-dir>/<hash>/         # Object content shard set
    chunk-0000
    chunk-0001
    ...
  <shard-dir>/<layout-id>/    # Object layout metadata
    meta.json
```

`<shard-dir>` is derived from the first two characters of the object key,
lowercased. Short keys use the whole key. Do not rely on a fixed top-level
`objects/` directory; back up the whole `data_dir`.

## Raft metadata and membership

Path:

```text
<raft_data_dir>
```

Contains:

```text
Dragonboat nodehost identity
Raft WAL and snapshots
Replicated bucket metadata
Replicated object metadata
Blob reference metadata
Cluster membership state
```

Operational classification: authoritative and not safely rebuildable from shard
files alone.

Rules:

```text
Do not delete raft_data_dir on an existing replica.
Do not copy one replica's raft_data_dir to another replica ID.
Do not start two nodes with the same restored raft_data_dir at the same time.
Restore it with the same raft_node_id and compatible cluster membership.
```

If this directory is lost for one node in a healthy multi-node cluster, replace
the node through the cluster replacement flow instead of reusing the old replica
identity with empty Raft state. If the whole cluster loses Raft metadata, object
shards are not enough for an MVP-supported full metadata reconstruction.

## Object shard data

Path shape:

```text
<data_dir>/<shard-dir>/<object-content-sha256>/chunk-0000
<data_dir>/<shard-dir>/<object-content-sha256>/chunk-0001
...
```

MaxIO currently uses a `9+3` erasure layout by default: 9 data chunks and 3
parity chunks. The default shard size is 1 MiB.

In a single-node deployment, every shard for an object is local. In a multi-node
deployment, shard placement metadata can point each chunk at a local or remote
storage node. Each node stores only the chunks assigned to that node, using the
same path shape in that node's own `data_dir`.

Operational classification: primary object data.

Rules:

```text
Do not delete shard sets referenced by object metadata.
Back up every node's data_dir, not only one node's data_dir.
The erasure layout can tolerate some missing shards for an object, but not an
unbounded loss of shard files.
Run repair after node loss, restore, or suspicious storage events.
```

## Object layout metadata

Path shape:

```text
<data_dir>/<shard-dir>/<layout-id>/meta.json
```

The layout file maps a bucket/key to the content shard set, size, ETag, checksum
list, and shard placements. Reads use this layout to find and validate object
shards.

Operational classification: operationally critical local metadata.

The committed Raft metadata also stores object and blob reference information,
but the MVP does not expose a general-purpose "rebuild every missing layout file"
restore command. Treat layout `meta.json` files as part of the object data
backup. If they are missing or inconsistent, use the recovery, dedupe, repair,
and smoke-test flows to detect damage; do not hand-edit them in production.

## Bleve search index

Path:

```text
<data_dir>/index/bleve
```

Contains the local persistent Bleve index used by search APIs.

Operational classification: derived and rebuildable.

The index is derived from committed object metadata and object content. It can be
excluded from backups when rebuild time is acceptable. Rebuild it with:

```sh
curl -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_index/rebuild"
```

If an index backup is restored across MaxIO or Bleve version changes and search
behaves unexpectedly, stop MaxIO, remove only `index/bleve`, restart, and rebuild
the index.

## S3 multipart staging

Path shape:

```text
<data_dir>/s3-multipart/<upload-id>/metadata.json
<data_dir>/s3-multipart/<upload-id>/parts/00001.part
<data_dir>/s3-multipart/<upload-id>/parts/00002.part
...
```

Contains in-progress S3 multipart upload state. Completed uploads are committed
through the normal object write path and then no longer depend on this staging
area.

Operational classification: transient upload state.

Rules:

```text
Include it only if you need to preserve in-progress multipart uploads.
Exclude it after a clean quiesce if no multipart uploads are active.
Loss of this directory invalidates in-progress multipart upload IDs.
Clients must retry failed or missing multipart uploads.
```

MVP limitation: multipart staging is local filesystem state. There is no
cluster-wide multipart upload migration or lifecycle cleanup API.

## Temporary put staging

Normal object writes use OS temporary files named like:

```text
maxio-put-*
```

These files are created in the operating system temp directory, not under
`data_dir`. They are active-write scratch files and are not part of a backup.
Quiesce writes or stop the process before taking a file-level backup.

## What can be rebuilt

Rebuildable:

```text
<data_dir>/index/bleve
Some missing object shards, if enough shards remain for the erasure layout
Some stale pending write state, through recovery
Some orphan shard sets, through recovery/dedupe cleanup
```

Not safely rebuildable in the MVP:

```text
raft_data_dir, from shard files alone
Object shard sets after unrecoverable erasure loss
Arbitrary missing object layout files, without supported repair coverage
In-progress multipart upload state, if s3-multipart is lost
```

## Backup priority

For a production backup, preserve these in order:

```text
1. Config, environment, tokens, and exact MaxIO version.
2. raft_data_dir for each replica.
3. data_dir for each storage node, including shard and layout directories.
4. s3-multipart only when in-progress uploads must survive.
5. index/bleve only when avoiding rebuild time matters.
```

See `docs/backup-restore-upgrade.md` for operational procedures.
