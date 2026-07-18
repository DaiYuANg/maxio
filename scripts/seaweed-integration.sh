#!/usr/bin/env sh
set -eu

ACTION="${1:-up}"
VUS="${VUS:-4}"
DURATION="${DURATION:-30s}"
BUILD="${BUILD:-0}"
PROJECT="${COMPOSE_PROJECT_NAME:-maxio-seaweed}"
ADMIN_TOKEN="${MAXIO_ADMIN_TOKEN:-dev-admin-token}"
CLEANUP_ON_FAILURE="${CLEANUP_ON_FAILURE:-0}"
UPSTREAM_REGISTER_RETRIES="${UPSTREAM_REGISTER_RETRIES:-30}"
UPSTREAM_REGISTER_RETRY_SLEEP="${UPSTREAM_REGISTER_RETRY_SLEEP:-2}"

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$SCRIPT_DIR/.." && pwd)
COMPOSE_FILE="$REPO_ROOT/deploy/compose.seaweed.yaml"

compose() {
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" "$@"
}

is_enabled() {
  value=$(printf '%s' "${1:-}" | tr '[:upper:]' '[:lower:]')
  [ "$value" = "1" ] || [ "$value" = "true" ] || [ "$value" = "yes" ] || [ "$value" = "on" ]
}

set_default_env() {
  name="$1"
  value="$2"
  eval "current=\${$name:-}"
  if [ -z "$current" ]; then
    export "$name=$value"
  fi
}

append_csv() {
  current="$1"
  value="$2"
  if [ -z "$current" ]; then
    printf '%s' "$value"
  else
    printf '%s,%s' "$current" "$value"
  fi
}

snapshot_state() {
  label="$1"
  echo "===== ${label} state snapshot ====="
  compose ps
  echo "===== end ${label} snapshot ====="
}

cleanup_if_enabled() {
  if is_enabled "$CLEANUP_ON_FAILURE"; then
    echo "CLEANUP_ON_FAILURE is enabled, cleaning stack..."
    compose down --remove-orphans --volumes
  fi
}

wait_compose_service_ready() {
  service="$1"
  timeout_seconds="${2:-180}"
  elapsed=0
  while [ "$elapsed" -lt "$timeout_seconds" ]; do
    container_id=$(compose ps -q "$service" 2>/dev/null || true)
    if [ -n "$container_id" ]; then
      status=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id" 2>/dev/null || true)
      if [ "$status" = "healthy" ] || [ "$status" = "running" ]; then
        return 0
      fi
      if [ "$status" = "unhealthy" ] || [ "$status" = "exited" ] || [ "$status" = "dead" ]; then
        echo "Service $service is $status" >&2
        return 1
      fi
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  echo "Timed out waiting for service $service" >&2
  return 1
}

wait_compose_services_ready() {
  for service in seaweed-a seaweed-b seaweed-c; do
    wait_compose_service_ready "$service" 180
  done
}

start_optional_processors() {
  if is_enabled "${MAXIO_PROCESSING_CLAMAV_ENABLED:-false}"; then
    compose --profile av up -d clamav
    wait_compose_service_ready clamav 180
  fi
  if is_enabled "${MAXIO_PROCESSING_TIKA_ENABLED:-false}"; then
    compose --profile tika up -d tika
    wait_http_ok 'http://127.0.0.1:9998/version' 120
  fi
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
  attempts=0
  while [ "$attempts" -lt "$UPSTREAM_REGISTER_RETRIES" ]; do
    attempts=$((attempts + 1))
    if curl -fsS -X POST 'http://127.0.0.1:8080/_s3/upstreams' \
      -H "Authorization: Bearer $ADMIN_TOKEN" \
      -H 'Content-Type: application/json' \
      --data "$payload" >/dev/null; then
      return 0
    fi
    if [ "$attempts" -ge "$UPSTREAM_REGISTER_RETRIES" ]; then
      echo "Failed to register upstream $id after $attempts attempts" >&2
      return 1
    fi
    sleep "$UPSTREAM_REGISTER_RETRY_SLEEP"
  done
}

register_upstreams() {
  register_upstream seaweed-a http://seaweed-a:8333 maxio-a 10
  register_upstream seaweed-b http://seaweed-b:8333 maxio-b 20
  register_upstream seaweed-c http://seaweed-c:8333 maxio-c 30
}

run_with_cleanup() {
  label="$1"
  shift
  set +e
  snapshot_state "$label before"
  "$@"
  status=$?
  snapshot_state "$label after"
  if [ "$status" -ne 0 ]; then
    cleanup_if_enabled
  fi
  set -e
  return "$status"
}

up() {
  compose up -d seaweed-a seaweed-b seaweed-c
  wait_compose_services_ready
  start_optional_processors
  init_buckets
  if is_enabled "${MAXIO_PROCESSING_ENABLED:-false}"; then
    if [ "$BUILD" = "1" ]; then
      compose up -d --build --force-recreate maxio
    else
      compose up -d --force-recreate maxio
    fi
  elif [ "$BUILD" = "1" ]; then
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

run_processing_k6() {
  set_default_env MAXIO_PROCESSING_ENABLED true
  set_default_env MAXIO_PROCESSING_MODE async_permissive
  set_default_env MAXIO_PROCESSING_CLAMAV_ENABLED true
  set_default_env MAXIO_PROCESSING_CLAMAV_MODE inline_strict
  set_default_env MAXIO_PROCESSING_TIKA_ENABLED true
  set_default_env MAXIO_PROCESSING_TIKA_MODE async_permissive

  clamav_mode=$(printf '%s' "${MAXIO_PROCESSING_CLAMAV_MODE:-inline_strict}" | tr '[:upper:]' '[:lower:]')
  tika_mode=$(printf '%s' "${MAXIO_PROCESSING_TIKA_MODE:-async_permissive}" | tr '[:upper:]' '[:lower:]')
  if is_enabled "${MAXIO_PROCESSING_TIKA_ENABLED:-false}"; then
    if [ "$tika_mode" = "async_permissive" ]; then
      set_default_env MAXIO_PROCESSING_TIKA_FAIL_OPEN true
    else
      set_default_env MAXIO_PROCESSING_TIKA_FAIL_OPEN false
    fi
  fi
  if is_enabled "${MAXIO_PROCESSING_TIKA_FAIL_OPEN:-false}"; then
    tika_fail_open=true
  else
    tika_fail_open=false
  fi

  processors=""
  processor_modes=""
  processor_fail_open=""
  capabilities=""
  result_metadata=""
  if is_enabled "${MAXIO_PROCESSING_CLAMAV_ENABLED:-false}"; then
    processors=$(append_csv "$processors" clamav)
    processor_modes=$(append_csv "$processor_modes" "clamav:$clamav_mode")
    capabilities=$(append_csv "$capabilities" antivirus)
    result_metadata=$(append_csv "$result_metadata" clamav:verdict=clean)
    result_metadata=$(append_csv "$result_metadata" clamav:response)
  fi
  if is_enabled "${MAXIO_PROCESSING_TIKA_ENABLED:-false}"; then
    processors=$(append_csv "$processors" tika)
    processor_modes=$(append_csv "$processor_modes" "tika:$tika_mode")
    processor_fail_open=$(append_csv "$processor_fail_open" "tika:$tika_fail_open")
    capabilities=$(append_csv "$capabilities" text_extraction)
    capabilities=$(append_csv "$capabilities" metadata_extract)
    result_metadata=$(append_csv "$result_metadata" tika:endpoint)
    result_metadata=$(append_csv "$result_metadata" tika:text_bytes)
    result_metadata=$(append_csv "$result_metadata" tika:document_count)
  fi

  if [ -z "$processors" ]; then
    processing_mode=$(printf '%s' "${MAXIO_PROCESSING_MODE:-async_permissive}" | tr '[:upper:]' '[:lower:]')
    processors=noop
    processor_modes="noop:$processing_mode"
  fi

  set_default_env PROCESSING_RECORD_CHECK true
  set_default_env PROCESSING_EXPECT_PROCESSORS "$processors"
  set_default_env PROCESSING_EXPECT_PROCESSOR_MODES "$processor_modes"
  set_default_env PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN "$processor_fail_open"
  set_default_env PROCESSING_EXPECT_CAPABILITIES "$capabilities"
  set_default_env PROCESSING_EXPECT_RESULT_METADATA "$result_metadata"
  set_default_env PROCESSING_RECORD_RETRIES 30
  set_default_env PROCESSING_RECORD_RETRY_SLEEP 0.5
  if is_enabled "${MAXIO_PROCESSING_CLAMAV_ENABLED:-false}" && [ "$clamav_mode" = "inline_strict" ]; then
    set_default_env CLAMAV_BLOCK_CHECK true
  else
    set_default_env CLAMAV_BLOCK_CHECK false
  fi
  run_k6
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
    run_with_cleanup 'k6' run_k6
    ;;
  processing-k6)
    run_with_cleanup 'processing-k6' run_processing_k6
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
    echo "Usage: $0 {up|restart|k6|processing-k6|logs|status|down|clean}" >&2
    echo "Environment: BUILD=1 VUS=8 DURATION=1m COMPOSE_PROJECT_NAME=maxio-seaweed CLEANUP_ON_FAILURE=1" >&2
    echo "Optional processors: MAXIO_PROCESSING_CLAMAV_ENABLED=true MAXIO_PROCESSING_TIKA_ENABLED=true" >&2
    echo "Processing smoke: $0 processing-k6" >&2
    exit 2
    ;;
esac
