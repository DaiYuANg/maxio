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


