# MaxIO smoke tests

These checks verify that a freshly started MaxIO node or local Compose cluster
can accept traffic, persist objects, expose operational state, and run the main
maintenance entry points.

Set the target URL and admin token first:

```sh
export MAXIO_URL="${MAXIO_URL:-http://127.0.0.1:8080}"
export MAXIO_ADMIN_TOKEN="${MAXIO_ADMIN_TOKEN:-dev-admin-token}"
export MAXIO_BUCKET="${MAXIO_BUCKET:-smoke}"
export MAXIO_OBJECT="${MAXIO_OBJECT:-hello.txt}"
```

For the three-node Compose example, point `MAXIO_URL` at node 1:

```sh
export MAXIO_URL="http://127.0.0.1:8081"
```

## Process and readiness

```sh
curl --fail "$MAXIO_URL/healthz"
curl --fail "$MAXIO_URL/readyz"
```

Readiness should include checks such as:

```text
object_service
engine
storage_writable
repair_backlog
```

## Metrics

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/metrics" \
  | grep -E "maxio_ready|maxio_http_requests_total|maxio_buckets"
```

Expected result: the command prints at least readiness, HTTP request, and object metric lines.

## Bucket and object API

Create a bucket:

```sh
curl --fail -X PUT \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/$MAXIO_BUCKET"
```

Upload an object:

```sh
printf "hello maxio smoke test\n" > /tmp/maxio-smoke-object.txt
curl --fail -X PUT \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  -H "Content-Type: text/plain" \
  --data-binary @/tmp/maxio-smoke-object.txt \
  "$MAXIO_URL/$MAXIO_BUCKET/$MAXIO_OBJECT"
```

Read the object back:

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/$MAXIO_BUCKET/$MAXIO_OBJECT"
```

Check object headers:

```sh
curl --fail -I \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/$MAXIO_BUCKET/$MAXIO_OBJECT"
```

Check range reads:

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  -H "Range: bytes=0-4" \
  "$MAXIO_URL/$MAXIO_BUCKET/$MAXIO_OBJECT"
```

Expected result: the range response body is `hello`.

## Search index

Rebuild the derived content index:

```sh
curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_index/rebuild"
```

Search indexed content:

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_search?q=smoke&bucket=$MAXIO_BUCKET"
```

Inspect index status:

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_index/status"
```

## Maintenance entry points

Inspect dedupe status:

```sh
curl --fail \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_dedupe/status"
```

Run object-level dedupe reconciliation:

```sh
curl --fail -X POST \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/_dedupe/run"
```

For a three-node Compose cluster, also verify every exposed node is ready:

```sh
for port in 8081 8082 8083; do
  curl --fail "http://127.0.0.1:$port/readyz"
done
```

## Cleanup

Delete the smoke object:

```sh
curl --fail -X DELETE \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/$MAXIO_BUCKET/$MAXIO_OBJECT"
```

Delete the smoke bucket:

```sh
curl --fail -X DELETE \
  -H "Authorization: Bearer $MAXIO_ADMIN_TOKEN" \
  "$MAXIO_URL/$MAXIO_BUCKET"
```
