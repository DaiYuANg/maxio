# MaxIO deployment notes

MaxIO's target runtime is a stateless S3 proxy backed by an external metadata
database and upstream S3-compatible storage. Local disk is not authoritative for
object bytes.

## Required services

Production:

```text
MaxIO gateway instances
PostgreSQL metadata database
Upstream S3-compatible storage
Persistent volume for derived Bleve index, optional but recommended
Metrics/logging stack
```

Development:

```text
Single MaxIO process
SQLite metadata database
Local or remote S3-compatible upstream
Local ./data directory for Bleve
```

## Configuration baseline

`config.example.json` is the minimal local baseline. `config.proxy.example.json`
shows a proxy-oriented shape with one upstream.

Important settings:

```json
{
  "metadata_backend": "sqlite",
  "metadata_dsn": "./data/maxio.db",
  "metadata_auto_migrate": true,
  "enable_native_object_api": false,
  "enable_s3_proxy": true,
  "s3_proxy_entrypoint": ":8080",
  "s3_proxy_upstreams": []
}
```

For production, prefer:

```json
{
  "metadata_backend": "postgres",
  "metadata_dsn": "postgres://maxio:***@postgres:5432/maxio?sslmode=require",
  "metadata_auto_migrate": false
}
```

Run migrations through the release procedure before starting new gateway
versions in production.

## Stateless gateway scaling

Multiple MaxIO instances can serve the same logical service when they share:

- the same metadata DB;
- compatible configuration and migration version;
- access to the same upstream S3 routes;
- the same admin/API credential policy;
- independent or shared Bleve rebuild strategy.

The gateway must not depend on local consensus state or local object shard files
in the target architecture. New deployments should treat gateway instances as
replaceable stateless processes.

## Local startup

Start the server with the default config path:

```sh
./maxio
```

Useful local checks:

```sh
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/metrics
```

If the S3 proxy implementation is not wired in the current binary, object routes
may still return `501`. That is a runtime implementation gap, not a change to
the product architecture.

## Container image

Build the local image:

```sh
docker build -t maxio:dev .
```

Run a development container:

```sh
docker run --rm \
  --name maxio \
  -p 8080:8080 \
  -v maxio-data:/app/data \
  -e MAXIO_ADMIN_TOKEN="$MAXIO_ADMIN_TOKEN" \
  -e MAXIO_API_TOKEN="$MAXIO_API_TOKEN" \
  maxio:dev
```

Mount a production config or inject environment variables according to the
runtime config loader.

## Metadata DB operations

Back up the metadata DB before upgrades and before destructive upstream
maintenance.

Operational expectations:

- migration state is explicit and versioned;
- gateways fail readiness when DB connectivity or migration compatibility fails;
- write paths use DB transactions for visible object state;
- background workers use DB leases for index and dedupe jobs;
- recovery scans DB state, not local disk, after crashes.

## Upstream S3 operations

Configure upstream S3 services with durability, lifecycle, and versioning
appropriate for the selected dedupe mode.

Observe mode:

- safest default;
- logical objects keep their own upstream byte locations;
- delete behavior can mirror normal S3 object deletion.

Alias mode:

- requires canonical object retention while references exist;
- should use upstream versioning or protected prefixes;
- needs explicit repair and lifecycle checks before deleting canonical bytes.

## Bleve index operations

Inspect index worker status:

```sh
curl -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" http://127.0.0.1:8080/_index/status
```

Rebuild the derived Bleve index:

```sh
curl -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  http://127.0.0.1:8080/_index/rebuild
```

Bleve can be stored on a persistent volume to avoid rebuild time, but it remains
derived. If index state is suspected to be corrupt, rebuild from DB rather than
treating the local index as authoritative.

## Admin protection

Set these credentials for non-local deployments:

```text
MAXIO_ADMIN_TOKEN
MAXIO_API_TOKEN
MAXIO_CLUSTER_TOKEN, if internal routes remain enabled
```

Admin requests can use:

```text
Authorization: Bearer <token>
X-Maxio-Control: <token>
```

S3/object API requests should use the configured S3 auth path or the API token
compatibility path while the auth model is being completed.

`/healthz` and `/readyz` should remain unauthenticated for load balancers.

## TLS termination

Terminate TLS at a reverse proxy, ingress, or load balancer in front of MaxIO:

```text
client HTTPS -> reverse proxy / ingress / load balancer -> private MaxIO HTTP
```

Forward credential headers unchanged:

```text
Authorization
X-Maxio-Control
X-Maxio-Cluster
X-Maxio-API
```

Keep internal management endpoints private.

## Readiness model

Gateway readiness should require:

- metadata DB reachable;
- migration version compatible;
- required upstream routes loaded;
- background workers able to lease work or intentionally disabled;
- local Bleve path writable when indexing is enabled.

Health may remain process-local. Readiness should reflect whether the instance
can serve proxy traffic safely.

## Upgrade model

Recommended sequence:

```text
1. Back up metadata DB.
2. Confirm upstream S3 lifecycle/versioning assumptions.
3. Run DB migrations.
4. Roll gateways gradually.
5. Watch DB transaction errors, index queue depth, and repair_fault count.
6. Trigger index rebuild only when schema version changes require it.
```

Because gateways are stateless, rollback normally means running the previous
binary against a compatible DB schema. Incompatible schema changes need an
explicit migration rollback plan.
