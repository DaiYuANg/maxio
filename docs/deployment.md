# MaxIO deployment notes

## Operational runbooks

Before running MaxIO with persistent data, read:

```text
docs/data-layout.md
docs/backup-restore-upgrade.md
docs/smoke-tests.md
```

`docs/data-layout.md` explains what lives under `data_dir`, which data is
authoritative, and which derived state can be rebuilt. `docs/backup-restore-upgrade.md`
covers MVP-safe backup, restore, upgrade, and rollback procedures.

## Single node

Use `config.example.json` as the baseline config. For local single-node runs, keep:

```json
{
  "raft_node_id": 1,
  "raft_bootstrap": true,
  "raft_join": false,
  "raft_initial_members": ""
}
```

Start the server with the default config path:

```sh
./maxio
```

## Container image

Build the local image:

```sh
docker build -t maxio:dev .
```

Run a single-node development container with a persistent data volume:

```sh
docker run --rm \
  --name maxio \
  -p 8080:8080 \
  -p 63000:63000 \
  -p 7946:7946 \
  -v maxio-data:/app/data \
  -e MAXIO_ADMIN_TOKEN="$MAXIO_ADMIN_TOKEN" \
  maxio:dev
```

The image copies `config.example.json` to `/app/config.json`. Override config
values with environment variables such as `MAXIO_ADMIN_TOKEN`,
`MAXIO_API_TOKEN`, `MAXIO_RAFT_ADDRESS`, and `MAXIO_STORAGE_ADDRESS`.

## Docker Compose

Start a single-node local stack:

```sh
docker compose -f deploy/compose.single.yaml up --build
```

Check readiness and metrics:

```sh
curl http://127.0.0.1:8080/readyz
curl -H "Authorization: Bearer ${MAXIO_ADMIN_TOKEN:-dev-admin-token}" http://127.0.0.1:8080/metrics
```

Stop the single-node stack and keep its volume:

```sh
docker compose -f deploy/compose.single.yaml down
```

Start a three-node local stack:

```sh
docker compose -f deploy/compose.three-node.yaml up --build
```

The three nodes expose HTTP on host ports `8081`, `8082`, and `8083`. Raft and
gossip traffic stays on the Compose network through service names such as
`maxio-1`, `maxio-2`, and `maxio-3`.

Check cluster members from node 1:

```sh
curl -H "Authorization: Bearer ${MAXIO_ADMIN_TOKEN:-dev-admin-token}" http://127.0.0.1:8081/_cluster/members
```

Stop the three-node stack and keep volumes:

```sh
docker compose -f deploy/compose.three-node.yaml down
```

Remove local Compose data volumes only when you intentionally want a clean
cluster:

```sh
docker compose -f deploy/compose.three-node.yaml down -v
```

## Systemd

The systemd assets are intended for a single host or for manually managed
cluster nodes outside containers.

Install the binary and config:

```sh
sudo install -D -m 0755 ./maxio /usr/local/bin/maxio
sudo install -D -m 0644 deploy/systemd/maxio.env.example /etc/maxio/maxio.env
sudo install -D -m 0644 deploy/systemd/maxio.service /etc/systemd/system/maxio.service
```

Create the service user and data directory:

```sh
sudo useradd --system --home-dir /var/lib/maxio --shell /usr/sbin/nologin maxio
sudo mkdir -p /var/lib/maxio /var/log/maxio
sudo chown -R maxio:maxio /var/lib/maxio /var/log/maxio
```

Edit `/etc/maxio/maxio.env` before starting. At minimum, set:

```text
MAXIO_ADMIN_TOKEN
MAXIO_STORAGE_ADDRESS
MAXIO_RAFT_ADDRESS
```

For multi-node systemd deployments, also set stable `MAXIO_RAFT_NODE_ID`,
`MAXIO_RAFT_INITIAL_MEMBERS`, `MAXIO_GOSSIP_ADVERTISE_ADDRESS`, and
`MAXIO_GOSSIP_SEEDS` on every node.

Start the service:

```sh
sudo systemctl daemon-reload
sudo systemctl enable --now maxio
sudo systemctl status maxio
```

Check logs and readiness:

```sh
journalctl -u maxio -f
curl http://127.0.0.1:8080/readyz
```

## Admin protection

Set `admin_token` or `MAXIO_ADMIN_TOKEN` to protect management APIs.
Set `cluster_token` or `MAXIO_CLUSTER_TOKEN` to protect internal shard APIs used between MaxIO nodes.
Set `api_token` or `MAXIO_API_TOKEN` to protect bucket and object APIs.
Set `s3_access_key`, `s3_secret_key`, and `s3_region` to require SigV4 header or presigned URL authentication for S3-compatible APIs.
Set `http_body_limit` to control the maximum request body accepted by the Fiber HTTP adapter. The default is `1073741824` bytes so standard S3 multipart upload parts work out of the box.

Native HTTP authorization is split by route class. Control-plane routes require the admin token when `admin_token` is set. Internal shard routes require the cluster token when `cluster_token` is set. Native bucket and object routes require object authorization when `api_token` is set; the API token grants object read/write access, and the admin token is accepted as an object superuser. The API token does not authorize control-plane routes.

Admin requests can use either header:

```sh
Authorization: Bearer <token>
X-Maxio-Control: <token>
```

Internal shard requests use the cluster header:

```sh
X-Maxio-Cluster: <cluster-token>
```

Protected paths include:

```text
/_cluster/*
/_repair/*
/_internal/*
/_search
/metrics
```

When `api_token` is configured, bucket and object routes also require either:

```sh
Authorization: Bearer <api-token>
X-Maxio-API: <api-token>
```

The admin token is also accepted for native bucket and object routes.

If both S3 key fields are empty, S3-compatible APIs run without authentication for local development. If either S3 key field is configured, both must be configured and S3 clients must send `Authorization: AWS4-HMAC-SHA256 ...` plus `X-Amz-Date`, or use standard presigned URL query parameters.

S3-compatible routes use SigV4 authentication instead of `api_token`. If both native API token auth and S3 auth are enabled, native bucket/object paths use `api_token` or `admin_token`, while `/s3` paths use `s3_access_key` and `s3_secret_key`.

S3 multipart upload is supported through the compatibility path. In-progress upload state is staged under `data_dir/s3-multipart` and completed objects are committed through the normal MaxIO object write path.

`/healthz` and `/readyz` remain unauthenticated for load balancers.

## TLS termination

Terminate TLS at a reverse proxy, ingress, or load balancer in front of MaxIO.

The current httpx runtime used by MaxIO exposes plain HTTP serving. Keep MaxIO on a private network and expose only the TLS terminator publicly.

Example topology:

```text
client -> TLS proxy or ingress -> MaxIO HTTP address
```

Production deployments should treat the MaxIO HTTP listener as a private upstream, not as the public TLS endpoint. The supported production pattern is:

```text
client HTTPS -> reverse proxy / ingress / load balancer -> private MaxIO HTTP
```

The TLS terminator is responsible for certificate provisioning, renewal, cipher policy, HTTP to HTTPS redirects, HSTS, and any public mTLS policy. MaxIO currently does not terminate TLS itself and does not reload TLS certificates.

Forward these headers unchanged when admin, API, cluster, or S3 credentials are used:

```text
Authorization
X-Maxio-Control
X-Maxio-Cluster
X-Maxio-API
X-Amz-Date
X-Amz-Content-Sha256
```

For SigV4 requests, preserve the request path, query string, and `Host` semantics used by the client when it signs the request. If the proxy rewrites `Host`, configure clients to sign the externally visible host and make the proxy forward that host consistently.

The internal shard API `/_internal/storage/shards/*` must only be reachable by trusted MaxIO nodes or by a private service network. If `cluster_token` is configured, MaxIO remote shard transport automatically sends `X-Maxio-Cluster`. If `cluster_token` is empty, the transport falls back to `admin_token` for development and compatibility.

## Multi-node bootstrap

Each node needs a stable raft address and an HTTP storage address.

Example initial members:

```text
1=10.0.0.1:63000,2=10.0.0.2:63000,3=10.0.0.3:63000
```

Node 1:

```json
{
  "raft_node_id": 1,
  "raft_address": "10.0.0.1:63000",
  "storage_address": "10.0.0.1:8080",
  "raft_bootstrap": true,
  "raft_initial_members": "1=10.0.0.1:63000,2=10.0.0.2:63000,3=10.0.0.3:63000"
}
```

Node 2 and node 3 use their own `raft_node_id`, `raft_address`, and `storage_address`, with the same `raft_initial_members`.

## Runtime cluster operations

List raft members:

```sh
curl -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_cluster/members
```

Synchronize storage node HTTP addresses from raft and gossip discovery:

```sh
curl -X POST -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_cluster/storage-nodes/sync
```

Drain a replica from new shard placements:

```sh
curl -X POST -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_cluster/members/2/drain
```

Preview remaining shard references:

```sh
curl -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" "http://127.0.0.1:8080/_cluster/rebalance/plan?replica_id=2"
```

Rebalance shards away from a drained replica:

```sh
curl -X POST -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" "http://127.0.0.1:8080/_cluster/rebalance?replica_id=2"
```

Remove the replica after rebalance:

```sh
curl -X DELETE -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_cluster/members/2
```

The remove operation is guarded. It returns conflict if object metadata still references the target replica.

## Node replacement

The replacement endpoint adds the new replica, syncs storage nodes, drains and rebalances the old replica, then removes the old replica if safe:

```sh
curl -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"replica_id":4,"target":"10.0.0.4:63000"}' \
  http://127.0.0.1:8080/_cluster/members/2/replace
```

## Observability

Health:

```sh
curl http://127.0.0.1:8080/healthz
```

Readiness:

```sh
curl http://127.0.0.1:8080/readyz
```

Metrics:

```sh
curl -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/metrics
```

For a full post-start smoke-test sequence, use `docs/smoke-tests.md`.

## Index operations

Inspect index worker status:

```sh
curl -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_index/status
```

Rebuild the derived Bleve index from committed object metadata and object content:

```sh
curl -X POST -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_index/rebuild
```
