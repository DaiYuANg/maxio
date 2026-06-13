# Backup, restore, and upgrade

MaxIO's target runtime is a stateless S3 proxy. Authoritative state lives in the
metadata database and upstream S3. The local Bleve index and temporary runtime
files are derived state.

## Backup scope

Always back up:

```text
metadata database
upstream S3 buckets or provider snapshots
configuration and secret references
deployment manifests
```

Usually exclude and rebuild:

```text
data_dir/index/bleve
process-local temporary files
logs
```

Do not rely on local shard directories or process-local state for a consistent
application backup.

## Cold backup

1. Stop external writes or put the upstream route in maintenance mode.
2. Back up the metadata DB with the database-native backup mechanism.
3. Back up or snapshot the upstream S3 bucket according to provider guidance.
4. Back up config, environment, secrets references, and deployment assets.
5. Start MaxIO and verify readiness.

```sh
curl --fail "$MAXIO_URL/readyz"
```

## Restore

1. Stop MaxIO gateways that point at the target metadata DB.
2. Restore the metadata DB.
3. Restore upstream S3 data or point routes at the restored upstream snapshot.
4. Restore config and deployment assets.
5. Start MaxIO gateways.
6. Verify `/healthz`, `/readyz`, and representative object reads.
7. Rebuild derived indexes when needed.

```sh
curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_index/rebuild"
```

## Upgrade

1. Read release notes for metadata schema compatibility.
2. Back up the metadata DB and upstream S3 state.
3. Roll gateways one at a time when the release supports mixed gateway versions.
4. Wait for `/readyz` after each gateway update.
5. Run smoke tests after all gateways are upgraded.

If the upgraded version commits incompatible metadata changes, rollback requires
restoring the pre-upgrade metadata DB snapshot and the previous binary or
container image.

## Operational limits

The current MVP does not provide built-in point-in-time backup orchestration,
single-object restore, or automatic schema rollback. Use database and upstream
provider tools for authoritative backups. MaxIO should add DB/upstream
consistency and index rebuild workflows before documenting post-restore
verification helpers.
