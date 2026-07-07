# SeaweedFS integration and k6 pressure test

This profile runs MaxIO against three independent SeaweedFS S3-compatible
upstreams. It is intended for local integration and pressure testing of the
stateless proxy direction.

## Topology

| Service | Container endpoint | Host endpoint | Bucket |
| --- | --- | --- | --- |
| MaxIO control plane | `http://maxio:8080` | `http://127.0.0.1:8080` | n/a |
| MaxIO S3 proxy | `http://maxio:8081` | `http://127.0.0.1:8081` | routes all buckets |
| SeaweedFS A S3 | `http://seaweed-a:8333` | `http://127.0.0.1:8331` | `maxio-a` |
| SeaweedFS B S3 | `http://seaweed-b:8333` | `http://127.0.0.1:8332` | `maxio-b` |
| SeaweedFS C S3 | `http://seaweed-c:8333` | `http://127.0.0.1:8333` | `maxio-c` |

Each SeaweedFS service runs its own master, volume, filer, and S3 endpoint with
an independent Docker volume. the integration helper creates the three buckets before MaxIO starts.

## Start locally

Cross-platform helper:

```sh
BUILD=1 ./scripts/seaweed-integration.sh up
```

Windows PowerShell convenience wrapper:

```powershell
.\scripts\seaweed-integration.ps1 up -Build
```

Equivalent Docker Compose command:

```powershell
docker compose -p maxio-seaweed -f deploy\compose.seaweed.yaml up -d --build
```

Health checks:

```powershell
Invoke-WebRequest http://127.0.0.1:8080/healthz -UseBasicParsing
Invoke-WebRequest http://127.0.0.1:8080/readyz -UseBasicParsing
Invoke-WebRequest http://127.0.0.1:8080/_s3/upstreams -Headers @{Authorization='Bearer dev-admin-token'} -UseBasicParsing
```

## Run k6

Run through Docker so local k6 installation is not required:

```sh
VUS=8 DURATION=1m ./scripts/seaweed-integration.sh k6
```

Windows PowerShell convenience wrapper:

```powershell
.\scripts\seaweed-integration.ps1 k6 -Vus 8 -Duration 1m
```

Equivalent Docker Compose command:

```powershell
$env:K6_VUS='8'
$env:K6_DURATION='1m'
docker compose -p maxio-seaweed -f deploy\compose.seaweed.yaml --profile perf run --rm k6 run /scripts/seaweed-smoke.js
```

The k6 script emits `maxio_processing_expectation_config_ok`; this threshold fails when processing expectation variables contain malformed entries.

The k6 script exercises:

- `/healthz` and `/readyz`;
- authenticated `/metrics`;
- authenticated `/_s3/upstreams`;
- authenticated `/_index/status`;
- S3 proxy `PUT`, `GET`, and `DELETE` across `maxio-a`, `maxio-b`, and
  `maxio-c`.

Useful environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BASE_URL` | `http://maxio:8080` in Compose | MaxIO control plane URL. |
| `S3_URL` | `http://maxio:8081` in Compose | MaxIO S3 proxy URL. |
| `ADMIN_TOKEN` | `dev-admin-token` | Admin token for control routes. |
| `BUCKETS` | `maxio-a,maxio-b,maxio-c` | Comma-separated S3 proxy buckets. |
| `K6_VUS` | `4` | Virtual users. |
| `K6_DURATION` | `30s` | Test duration. |
| `S3_OBJECT_BYTES` | `1024` | PUT body size per object. |

## Run processing smoke

Use the processing preset when you want the local Docker topology to exercise
both optional processors:

```sh
BUILD=1 ./scripts/seaweed-integration.sh processing-k6
```

Windows PowerShell convenience wrapper:

```powershell
.\scripts\seaweed-integration.ps1 processing-k6 -Build
```

Prefer the helper actions for processing smoke runs; direct `docker compose up` does not sequence optional processor readiness before MaxIO startup. The preset enables ClamAV in `inline_strict` mode and Tika in
`async_permissive` mode by default. The helper waits for ClamAV's compose
healthcheck and Tika's `/version` endpoint before starting MaxIO. Processing
expectations are generated from the effective processor enable flags and mode
environment values. Processing record polling waits for expected metadata and
strict read-gate readiness before the S3 GET check continues. If both optional processors are disabled while the pipeline
remains enabled, the preset expects the service fallback `noop` processor. Tika
fail-open defaults to true only for permissive enrichment; if you override Tika
to a strict mode, the preset defaults Tika fail-open to false so it matches
processing config validation. `CLAMAV_BLOCK_CHECK` defaults to true only when
ClamAV remains enabled and `inline_strict`. Each value is only a default; set
the corresponding environment variable before running the helper to override it.
## Stop and clean

```sh
./scripts/seaweed-integration.sh down
./scripts/seaweed-integration.sh clean
```

Windows PowerShell convenience wrapper:

```powershell
.\scripts\seaweed-integration.ps1 down
.\scripts\seaweed-integration.ps1 clean
```

`clean` removes Docker volumes for MaxIO and all SeaweedFS instances.

## Troubleshooting

- If ports are busy, check `8080`, `8081`, `8331`, `8332`, `8333`, `8881`,
  `8882`, `8883`, `9331`, `9332`, `9333`, and `19090`.
- If bucket initialization fails, inspect the SeaweedFS S3 endpoints on ports `8331`, `8332`, and `8333`, then rerun the helper.
- If k6 S3 operations fail while control-plane checks pass, inspect MaxIO and
  the targeted SeaweedFS service logs; this usually means the proxy route or
  upstream S3 compatibility behavior needs implementation work.



## Processing preset override reference

`processing-k6` is a preset over the normal `k6` action. It sets defaults only
when the matching environment variable is unset, so callers can override any
value before running the helper.

| Variable | Preset default | Derived behavior |
| --- | --- | --- |
| `MAXIO_PROCESSING_ENABLED` | `true` | Enables the processing pipeline for the smoke run. |
| `MAXIO_PROCESSING_CLAMAV_ENABLED` | `true` | When disabled, ClamAV expectations and EICAR block checks are removed. |
| `MAXIO_PROCESSING_CLAMAV_MODE` | `inline_strict` | Used to derive `PROCESSING_EXPECT_PROCESSOR_MODES`; `CLAMAV_BLOCK_CHECK` defaults to true only in this mode. |
| `MAXIO_PROCESSING_TIKA_ENABLED` | `true` | When disabled, Tika expectations, capabilities, and metadata checks are removed. |
| `MAXIO_PROCESSING_TIKA_MODE` | `async_permissive` | Used to derive `PROCESSING_EXPECT_PROCESSOR_MODES`. |
| `MAXIO_PROCESSING_TIKA_FAIL_OPEN` | enable- and mode-dependent | Defaults to true only when Tika is enabled and remains `async_permissive`; strict modes default it to false. |
| `PROCESSING_EXPECT_PROCESSORS` | derived | Generated from enabled processors, or `noop` when both optional processors are disabled. |
| `PROCESSING_EXPECT_PROCESSOR_MODES` | derived | Checks `/_processing/status.processor_modes` using `processor:mode` entries; malformed entries fail the check. |
| `PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN` | derived | Checks `/_processing/status.processor_fail_open` using `processor:true|false` entries; common boolean aliases such as `1`, `0`, `yes`, `no`, `on`, and `off` are accepted, and malformed entries fail the check. Currently generated for Tika. |
| `PROCESSING_EXPECT_CAPABILITIES` | derived | Generated from enabled processor capabilities. |
| `PROCESSING_EXPECT_RESULT_METADATA` | derived | Checks ClamAV clean verdict/response and Tika endpoint/metadata only when the matching processor is enabled; malformed entries fail the check. |
| `CLAMAV_BLOCK_CHECK` | derived | Checks EICAR rejection and blocked digest record only when ClamAV is enabled as `inline_strict`. |

Examples:

```sh
MAXIO_PROCESSING_TIKA_ENABLED=false ./scripts/seaweed-integration.sh processing-k6
```

```sh
MAXIO_PROCESSING_TIKA_MODE=async_strict ./scripts/seaweed-integration.sh processing-k6
```

```sh
MAXIO_PROCESSING_CLAMAV_ENABLED=false MAXIO_PROCESSING_TIKA_ENABLED=false ./scripts/seaweed-integration.sh processing-k6
```
