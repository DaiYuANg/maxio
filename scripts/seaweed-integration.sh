#!/usr/bin/env sh
set -eu

ACTION="${1:-up}"
VUS="${VUS:-4}"
DURATION="${DURATION:-30s}"
BUILD="${BUILD:-0}"
PROJECT="${COMPOSE_PROJECT_NAME:-maxio-seaweed}"
ADMIN_TOKEN="${MAXIO_ADMIN_TOKEN:-dev-admin-token}"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deploy/compose.seaweed.yaml"

compose() {
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"
}

wait_http_ok() {
  url="$1"
  timeout_seconds="${2:-120}"
  elapsed=0
  while [ "$elapsed" -lt "$timeout_seconds" ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "Timed out waiting for $url" >&2
  return 1
}

put_bucket() {
  endpoint="$1"
  bucket="$2"
  curl -fsS -X PUT "$endpoint/$bucket" >/dev/null || true
  curl -fsS -I "$endpoint/$bucket" >/dev/null
}

init_buckets() {
  wait_http_ok 'http://127.0.0.1:8331/'
  wait_http_ok 'http://127.0.0.1:8332/'
  wait_http_ok 'http://127.0.0.1:8333/'
  put_bucket 'http://127.0.0.1:8331' 'maxio-a'
  put_bucket 'http://127.0.0.1:8332' 'maxio-b'
  put_bucket 'http://127.0.0.1:8333' 'maxio-c'
}

register_upstream() {
  id="$1"
  endpoint="$2"
  bucket="$3"
  priority="$4"
  payload=$(cat <<JSON
{"id":"$id","name":"$id","endpoint":"$endpoint","region":"us-east-1","weight":1,"priority":$priority,"buckets":["$bucket"],"enabled":true}
JSON
)
  curl -fsS -X POST 'http://127.0.0.1:8080/_s3/upstreams' \
    -H "Authorization: Bearer $ADMIN_TOKEN" \
    -H 'Content-Type: application/json' \
    --data "$payload" >/dev/null
}

register_upstreams() {
  register_upstream seaweed-a http://seaweed-a:8333 maxio-a 10
  register_upstream seaweed-b http://seaweed-b:8333 maxio-b 20
  register_upstream seaweed-c http://seaweed-c:8333 maxio-c 30
}

up() {
  compose up -d seaweed-a seaweed-b seaweed-c
  init_buckets
  if [ "$BUILD" = "1" ]; then
    compose up -d --build maxio
  else
    compose up -d maxio
  fi
  wait_http_ok 'http://127.0.0.1:8080/healthz'
  wait_http_ok 'http://127.0.0.1:8080/readyz'
  register_upstreams
  echo 'MaxIO control plane: http://127.0.0.1:8080'
  echo 'MaxIO S3 proxy:     http://127.0.0.1:8081'
  echo 'SeaweedFS S3:       http://127.0.0.1:8331, http://127.0.0.1:8332, http://127.0.0.1:8333'
}

run_k6() {
  up
  K6_VUS="$VUS" K6_DURATION="$DURATION" compose --profile perf run --rm k6 run /scripts/seaweed-smoke.js
}

case "$ACTION" in
  up)
    up
    ;;
  restart)
    compose down --remove-orphans
    up
    ;;
  k6)
    run_k6
    ;;
  logs)
    compose logs -f --tail 200
    ;;
  status)
    compose ps
    ;;
  down)
    compose down --remove-orphans
    ;;
  clean)
    compose down --remove-orphans --volumes
    ;;
  *)
    echo "Usage: $0 {up|restart|k6|logs|status|down|clean}" >&2
    echo "Environment: BUILD=1 VUS=8 DURATION=1m COMPOSE_PROJECT_NAME=maxio-seaweed" >&2
    exit 2
    ;;
esac
