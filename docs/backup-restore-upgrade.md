# MaxIO backup, restore, and upgrade runbook

This runbook covers the current MVP operational model. MaxIO has deployment
assets, health checks, smoke tests, repair, dedupe, recovery, and index rebuild
entry points, but it does not yet provide a built-in online backup or migration
controller.

Read `docs/data-layout.md` before using this runbook.

## Scope and assumptions

The supported MVP-safe backup method is either:

```text
Cold file backup while the MaxIO process is stopped.
External crash-consistent or application-consistent volume snapshots.
```

Plain recursive file copy while MaxIO is accepting writes is not a consistent
backup method. Active writes can involve Raft WAL updates, local shard writes,
layout files, upload staging files, and temporary OS staging files.

## Pre-backup checklist

Record:

```text
MaxIO binary or image tag
config.json or environment file
MAXIO_DATA_DIR
MAXIO_RAFT_DATA_DIR
raft_node_id for every node
raft_address and storage_address for every node
admin/API token configuration, stored securely
```

Check health:

```sh
curl --fail "$MAXIO_URL/healthz"
curl --fail "$MAXIO_URL/readyz"
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_cluster/members"
```

For a complete smoke-test sequence, use `docs/smoke-tests.md`.

## Cold backup: single node

1. Stop clients or route writes away from MaxIO.
2. Stop MaxIO.

Systemd:

```sh
sudo systemctl stop maxio
```

Docker Compose:

```sh
docker compose -f deploy/compose.single.yaml down
```

3. Back up the effective `data_dir`.

Examples:

```sh
sudo tar -C /var/lib/maxio -czf maxio-data-$(date +%Y%m%d%H%M%S).tgz data
```

For Docker named volumes, use your platform's volume backup procedure. The
Compose examples persist `/app/data` in named volumes.

4. If `raft_data_dir` is an absolute path outside `data_dir`, back it up too.
5. Back up config and deployment assets separately from data.
6. Start MaxIO and verify readiness.

```sh
sudo systemctl start maxio
curl --fail "$MAXIO_URL/readyz"
```

## Cold backup: multi-node cluster

For a full-cluster disaster-recovery backup, capture all nodes from one
consistent point in time.

Recommended MVP procedure:

1. Stop external writes.
2. Stop all MaxIO nodes, or take coordinated storage snapshots for all nodes.
3. Back up each node's `data_dir`.
4. Back up each node's `raft_data_dir` if it is outside `data_dir`.
5. Back up each node's config and identity mapping.
6. Restart enough nodes to regain Raft quorum, then restart the rest.
7. Check readiness and cluster membership.

Rolling per-node backups are useful for node-local disaster recovery, but they
are not a substitute for a cluster-consistent backup because Raft metadata and
object shards can advance between nodes.

## Backup inclusion rules

Always include:

```text
data_dir shard/layout directories
raft_data_dir
config and environment
```

Usually exclude, then rebuild:

```text
data_dir/index/bleve
```

Do not back up only a subdirectory such as `objects/` or `index/`. The current
engine stores shard and layout data directly under `data_dir/<shard-dir>/...`.

## Restore: single node

1. Stop MaxIO.

```sh
sudo systemctl stop maxio
```

2. Move the broken target data directory aside instead of merging files.
3. Restore `data_dir` from the backup.
4. Restore `raft_data_dir` if it was backed up separately.
5. Restore config and environment.
6. Ensure filesystem ownership and permissions match the service user.

```sh
sudo chown -R maxio:maxio /var/lib/maxio /var/log/maxio
```

7. Start MaxIO.

```sh
sudo systemctl start maxio
```

8. Verify process and readiness.

```sh
curl --fail "$MAXIO_URL/healthz"
curl --fail "$MAXIO_URL/readyz"
```

9. Run maintenance checks.

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_recovery/status"

curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_recovery/run"

curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_repair/run"

curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_index/rebuild"
```

10. Run the smoke tests in `docs/smoke-tests.md`.

## Restore: multi-node cluster

Use matching backups for all restored replicas.

1. Stop all MaxIO nodes.
2. Restore each node's own `data_dir`.
3. Restore each node's own `raft_data_dir`.
4. Preserve each node's original `raft_node_id`.
5. Restore config with compatible `raft_address`, `storage_address`, and
   `raft_initial_members`.
6. Start enough nodes to form quorum, then start the remaining nodes.
7. Verify membership from a healthy node.

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_cluster/members"
```

8. Run recovery, repair, dedupe, index rebuild, and smoke tests.

Important restrictions:

```text
Do not restore one node's raft_data_dir onto a different raft_node_id.
Do not mix backups from unrelated clusters.
Do not start duplicate replicas from the same restored Raft directory.
Do not use full-cluster restore as a node replacement workflow.
```

If only one node failed and the remaining cluster is healthy, prefer the node
replacement flow in `docs/deployment.md`.

## Restore limits

Current MVP limitations:

```text
No built-in online snapshot API.
No cluster-wide write freeze API.
No single-object restore/import tool.
No supported metadata reconstruction from shard files alone.
No automated restore of in-progress upload state.
No automated validation that a restored backup is complete.
```

After every restore, run smoke tests and at least one representative read of
important buckets.

## Upgrade: single node

1. Read the release notes for the target version.
2. Take a cold backup or external snapshot.
3. Stop MaxIO.
4. Replace the binary or container image.
5. Keep `data_dir`, `raft_data_dir`, and `raft_node_id` unchanged.
6. Start MaxIO.
7. Check readiness, metrics, and smoke tests.

Systemd example:

```sh
sudo systemctl stop maxio
sudo install -D -m 0755 ./maxio /usr/local/bin/maxio
sudo systemctl start maxio
curl --fail "$MAXIO_URL/readyz"
```

Docker Compose example:

```sh
docker compose -f deploy/compose.single.yaml pull
docker compose -f deploy/compose.single.yaml up -d --build
curl --fail "$MAXIO_URL/readyz"
```

If search fails after an upgrade, rebuild the derived index:

```sh
curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_index/rebuild"
```

## Upgrade: multi-node cluster

Use a rolling upgrade only when the target release is documented as compatible
with the currently running version.

Recommended sequence:

1. Take a cluster-consistent backup or coordinated snapshots.
2. Confirm the cluster is healthy.
3. Upgrade one node at a time.
4. Wait for the upgraded node to pass `/readyz`.
5. Confirm `_cluster/members` before upgrading the next node.
6. Keep Raft quorum available at all times.
7. Upgrade the leader last when you know which node is leader; otherwise expect
   a brief leader election.
8. Run smoke tests after all nodes are upgraded.

For long maintenance on a storage node, consider draining and rebalancing shards
with the operational endpoints in `docs/deployment.md` before stopping it.

Upgrade cautions:

```text
Do not change raft_node_id during an upgrade.
Do not delete or reinitialize raft_data_dir.
Do not skip required intermediate versions unless the release notes allow it.
Quiesce writes or expect retries for object uploads targeting the node being upgraded.
```

## Rollback

Fast rollback is safe only when the upgraded version did not commit incompatible
metadata or layout changes.

Conservative rollback:

1. Stop MaxIO.
2. Restore the pre-upgrade `data_dir` and `raft_data_dir` snapshot.
3. Restore the old binary or image.
4. Restore the old config.
5. Start MaxIO.
6. Run readiness checks and smoke tests.

If the process fails immediately after binary replacement and no writes were
accepted, replacing the binary with the previous version may be enough. If any
writes, membership changes, repair, rebalance, dedupe, or recovery actions ran
after the upgrade, prefer restoring the pre-upgrade snapshot.

Multi-node rollback should keep all nodes on the same version unless release
notes explicitly allow mixed versions.

## Online operations and MVP limits

The following are not yet guaranteed by the MVP:

```text
Online file-copy backup consistency.
Coordinated point-in-time snapshot orchestration.
Automatic schema migration rollback.
Cross-node migration of in-progress uploads.
Single-bucket or single-object point-in-time restore.
Changing restored Raft replica IDs or cluster addresses during restore.
```

Use external orchestration for snapshots and treat MaxIO's maintenance endpoints
as post-restore/post-upgrade verification and repair tools, not as a replacement
for backups.
