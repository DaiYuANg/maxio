# Object processing pipeline

MaxIO object processing is an optional pipeline boundary for work that should not
be hard-coded into the core S3 proxy path.

Processing records are stored in the metadata DB so strict read gates and status
inspection survive process restarts and work across stateless gateway instances.

For DELETE races, each gateway also keeps a bounded in-process version tombstone
set. This prevents an async processor from recreating a just-deleted processing
record after the object version has been discarded. Tombstones are version-only,
FIFO bounded, and intentionally not used for digest-only preflight cleanup so
upload retries with the same digest are not blocked.

The pipeline supports two placement styles:

- Post-commit processing: the object is committed first, then processors run from
  the captured upload stream.
- Inline strict processing: processors run before upstream write on proxy misses and before metadata commit on all writes; failures reject the write.

## Modes

- `disabled`: processing is off. This is the default effective mode when
  `processing_enabled` is false.
- `async_permissive`: run processors after commit; reads are allowed while
  processing is pending or failed.
- `async_strict`: run processors after commit; reads are blocked until processing
  succeeds.
- `inline_strict`: run processors before commit; writes are rejected when a
  processor blocks or fails.

`processing_fail_open=true` makes strict failures permissive, including processor errors and processing record store lookup failures. It is rejected when any enabled processor runs in a strict mode; do not use it for security controls.

`processing_mode` is the default placement policy for processors that do not
set their own mode. Built-in processors have explicit modes by default:
`processing_clamav_mode=inline_strict` and
`processing_tika_mode=async_permissive`. This supports the common production
shape where ClamAV blocks writes before commit while Tika enriches objects
after commit. Set `processing_tika_fail_open=true` only for local smoke or non-security enrichment deployments where Tika runs in a permissive mode and errors should not block the pipeline.

## Built-in optional processors

- `clamav`: talks to `clamd` over TCP using the INSTREAM protocol. It blocks
  infected objects when the service reports `FOUND` and records `verdict`,
  `signature`, and raw `response` metadata for control-plane inspection. Failed service interactions record `reason` and `address` metadata.
- `tika`: talks to Apache Tika Server over HTTP using `/rmeta/text`, records
  extracted text byte counts and selected metadata, records `reason`, `endpoint`, and `fail_open` metadata for failed service interactions, and intentionally does not
  expose extracted text payloads through the control plane.

Both processors require `processing_enabled=true`; enabling a processor while the pipeline is disabled is rejected as invalid configuration.

Both processors are disabled by default. When ClamAV is enabled, `processing_clamav_address` must be a TCP `host:port` address, for example `clamav:3310`. When Tika is enabled, `processing_tika_url` must be an absolute `http` or `https` URL, for example `http://tika:9998`. Use `processing_clamav_mode` and `processing_tika_mode` to override processor placement independently of the default `processing_mode`. Set `processing_tika_fail_open=true` only when Tika runs as best-effort enrichment in a permissive mode. Strict Tika modes reject processor-level fail-open configuration.

## Status endpoint

Use `GET /_processing/status` with admin credentials to inspect the resolved
pipeline mode, timeout, global fail-open policy, active processor names,
processor modes, processor-local fail-open policy, and active capabilities. This
is a control-plane endpoint; it does not expose object payloads or extracted
content.

Example status response:

```json
{
  "enabled": true,
  "mode": "async_permissive",
  "fail_open": false,
  "timeout": "30s",
  "processors": ["clamav", "tika"],
  "processor_modes": {
    "clamav": "inline_strict",
    "tika": "async_permissive"
  },
  "processor_fail_open": {
    "tika": true
  },
  "capabilities": ["antivirus", "metadata_extract", "text_extraction"]
}
```

Use `GET /_processing/records?bucket=...&key=...&version_id=...` to inspect a
single object processing record. Use `GET /_processing/records?status=failed&limit=50`
to list recent records by status for operations and debugging. Valid status filters are `skipped`, `queued`, `running`, `succeeded`, `failed`, and `blocked`. The single-record
identity parameters and status-list parameters are mutually exclusive. The
response includes status, read gate decision fields (`read_allowed` and
`read_block_reason`), and processor result metadata only; it does not expose
original object payloads or extracted text.

Example processing config:

```json
{
  "processing_enabled": true,
  "processing_mode": "inline_strict",
  "processing_fail_open": false,
  "processing_clamav_enabled": true,
  "processing_clamav_mode": "inline_strict",
  "processing_clamav_address": "clamav:3310",
  "processing_tika_enabled": false,
  "processing_tika_mode": "async_permissive",
  "processing_tika_fail_open": false,
  "processing_tika_url": "http://tika:9998",
  "processing_tika_max_bytes": 104857600
}
```

For local Docker integration, enable services with environment variables:

```sh
MAXIO_PROCESSING_ENABLED=true \
MAXIO_PROCESSING_MODE=inline_strict \
MAXIO_PROCESSING_CLAMAV_ENABLED=true \
MAXIO_PROCESSING_CLAMAV_MODE=inline_strict \
./scripts/seaweed-integration.sh up
```

For Tika enrichment:

```sh
MAXIO_PROCESSING_ENABLED=true \
MAXIO_PROCESSING_MODE=async_permissive \
MAXIO_PROCESSING_TIKA_ENABLED=true \
MAXIO_PROCESSING_TIKA_MODE=async_permissive \
MAXIO_PROCESSING_TIKA_FAIL_OPEN=true \
./scripts/seaweed-integration.sh up
```

The PowerShell helper accepts the same environment variables. The compose file starts `clamav` only under the `av` profile and `tika` only under the `tika` profile. The helper scripts start enabled optional processors before MaxIO, wait for ClamAV's compose healthcheck, and wait for Tika's `/version` endpoint.

The compose topology defaults `MAXIO_PROCESSING_TIKA_FAIL_OPEN=false` to match the application config and strict-mode validation. The `processing-k6` preset sets it to true only when Tika stays in permissive enrichment mode.

Tika processor results include fields such as `text_bytes`, `text_truncated`, `document_count`, `detected_content_type`, `parsed_by`, and `endpoint`. Extracted text stays inside the processor boundary and is not returned by `/_processing/records`.

For a one-command local processing smoke run with ClamAV, Tika, result metadata checks, and the EICAR block check enabled, use `./scripts/seaweed-integration.sh processing-k6` or `./scripts/seaweed-integration.ps1 -Action processing-k6`. The action only sets defaults when the matching environment variable is unset, so individual settings remain overridable.

The k6 smoke script supports optional processing assertions:

Manual equivalent for a local ClamAV + Tika smoke run when not using the preset:

```sh
MAXIO_PROCESSING_ENABLED=true \
MAXIO_PROCESSING_MODE=async_permissive \
MAXIO_PROCESSING_CLAMAV_ENABLED=true \
MAXIO_PROCESSING_CLAMAV_MODE=inline_strict \
MAXIO_PROCESSING_TIKA_ENABLED=true \
MAXIO_PROCESSING_TIKA_MODE=async_permissive \
MAXIO_PROCESSING_TIKA_FAIL_OPEN=true \
PROCESSING_EXPECT_PROCESSORS=clamav,tika \
PROCESSING_EXPECT_PROCESSOR_MODES=clamav:inline_strict,tika:async_permissive \
PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN=tika:true \
PROCESSING_EXPECT_RESULT_METADATA=clamav:verdict=clean,clamav:response,tika:endpoint,tika:text_bytes,tika:document_count \
PROCESSING_EXPECT_CAPABILITIES=antivirus,text_extraction,metadata_extract \
PROCESSING_RECORD_CHECK=true \
PROCESSING_RECORD_LIST_STATUS=succeeded \
CLAMAV_BLOCK_CHECK=true \
./scripts/seaweed-integration.sh k6
```

- `PROCESSING_RECORD_CHECK=true` checks `/_processing/records` after each PUT, including the `read_allowed` decision field.
- `PROCESSING_RECORD_LIST_STATUS=succeeded` checks the status-list endpoint;
  `PROCESSING_RECORD_LIST_LIMIT` controls the request limit.
- `PROCESSING_EXPECT_CAPABILITIES=antivirus,text_extraction` checks that
  `/_processing/status` exposes the expected active capabilities.
- `PROCESSING_EXPECT_PROCESSORS=clamav,tika` checks that `/_processing/status`
  exposes the expected active processor names.
- `PROCESSING_EXPECT_PROCESSOR_MODES=clamav:inline_strict` checks that
  `/_processing/status` exposes the expected processor placement modes;
  malformed entries fail the k6 check.
- `PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN=tika:true` checks that
  `/_processing/status` exposes expected processor-local fail-open behavior,
  currently used by Tika. Boolean values accept `true`, `false`, `1`, `0`,
  `yes`, `no`, `on`, and `off`; malformed entries fail the k6 check instead
  of being ignored.
- `PROCESSING_EXPECT_RESULT_METADATA=clamav:verdict=clean,clamav:response,tika:endpoint,tika:text_bytes` checks
  `/_processing/records` processor result metadata. Use `processor:key=value` for exact matches
  and `processor:key` to require a non-empty field. Malformed entries fail the
  k6 check.
- `PROCESSING_RECORD_RETRIES` and `PROCESSING_RECORD_RETRY_SLEEP` control the
  short polling window for processing record visibility in k6.
- `maxio_processing_expectation_config_ok` fails when processing expectation
  variables contain malformed entries.
- `CLAMAV_BLOCK_CHECK=true` uploads the EICAR test string, expects ClamAV to
  reject it, and checks the digest-only processing record for `status=blocked`,
  `read_allowed=false`, and `clamav` metadata with `verdict=infected`,
  `signature`, and `response`. Use this
  only with `processing_clamav_mode=inline_strict` and ClamAV enabled.


















For detailed `processing-k6` preset override rules, including how processor enable
flags affect expected processors, capabilities, result metadata, and EICAR block
checks, see `docs/seaweed-k6.md`.
