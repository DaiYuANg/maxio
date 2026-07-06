param(
    [ValidateSet('up', 'down', 'restart', 'k6', 'logs', 'clean', 'status')]
    [string]$Action = 'up',
    [int]$Vus = 4,
    [string]$Duration = '30s',
    [switch]$Build
)

$ErrorActionPreference = 'Stop'
$RepoRoot = Split-Path -Parent $PSScriptRoot
$ComposeFile = Join-Path $RepoRoot 'deploy\compose.seaweed.yaml'
$Project = if ($env:COMPOSE_PROJECT_NAME) { $env:COMPOSE_PROJECT_NAME } else { 'maxio-seaweed' }
$AdminToken = if ($env:MAXIO_ADMIN_TOKEN) { $env:MAXIO_ADMIN_TOKEN } else { 'dev-admin-token' }

function Invoke-Compose {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Args)
    docker compose -p $Project -f $ComposeFile @Args
}

function Wait-HttpOk {
    param([string]$Url, [int]$TimeoutSeconds = 120)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) { return }
        } catch { Start-Sleep -Seconds 2 }
    } while ((Get-Date) -lt $deadline)
    throw "Timed out waiting for $Url"
}

function Initialize-SeaweedBuckets {
    Wait-HttpOk -Url 'http://127.0.0.1:8331/'
    Wait-HttpOk -Url 'http://127.0.0.1:8332/'
    Wait-HttpOk -Url 'http://127.0.0.1:8333/'
    foreach ($pair in @(@('http://127.0.0.1:8331', 'maxio-a'), @('http://127.0.0.1:8332', 'maxio-b'), @('http://127.0.0.1:8333', 'maxio-c'))) {
        try { Invoke-WebRequest -Method Put -Uri "$($pair[0])/$($pair[1])" -UseBasicParsing -TimeoutSec 10 | Out-Null } catch {}
        Invoke-WebRequest -Method Head -Uri "$($pair[0])/$($pair[1])" -UseBasicParsing -TimeoutSec 10 | Out-Null
    }
}

function Register-Upstream {
    param([string]$Id, [string]$Endpoint, [string]$Bucket, [int]$Priority)
    $payload = @{
        id = $Id
        name = $Id
        endpoint = $Endpoint
        region = 'us-east-1'
        weight = 1
        priority = $Priority
        buckets = @($Bucket)
        enabled = $true
    } | ConvertTo-Json -Compress
    Invoke-WebRequest -Method Post -Uri 'http://127.0.0.1:8080/_s3/upstreams' -Headers @{Authorization="Bearer $AdminToken"} -ContentType 'application/json' -Body $payload -UseBasicParsing -TimeoutSec 10 | Out-Null
}

function Register-Upstreams {
    Register-Upstream -Id 'seaweed-a' -Endpoint 'http://seaweed-a:8333' -Bucket 'maxio-a' -Priority 10
    Register-Upstream -Id 'seaweed-b' -Endpoint 'http://seaweed-b:8333' -Bucket 'maxio-b' -Priority 20
    Register-Upstream -Id 'seaweed-c' -Endpoint 'http://seaweed-c:8333' -Bucket 'maxio-c' -Priority 30
}

function Start-Environment {
    Invoke-Compose up -d seaweed-a seaweed-b seaweed-c
    Initialize-SeaweedBuckets
    $args = @('up', '-d')
    if ($Build) { $args += '--build' }
    $args += 'maxio'
    Invoke-Compose @args
    Wait-HttpOk -Url 'http://127.0.0.1:8080/healthz'
    Wait-HttpOk -Url 'http://127.0.0.1:8080/readyz'
    Register-Upstreams
    Write-Host 'MaxIO control plane: http://127.0.0.1:8080'
    Write-Host 'MaxIO S3 proxy:     http://127.0.0.1:8081'
    Write-Host 'SeaweedFS S3:       http://127.0.0.1:8331, http://127.0.0.1:8332, http://127.0.0.1:8333'
}

function Run-K6 {
    $env:K6_VUS = [string]$Vus
    $env:K6_DURATION = $Duration
    Invoke-Compose --profile perf run --rm k6 run /scripts/seaweed-smoke.js
}

switch ($Action) {
    'up' { Start-Environment }
    'restart' { Invoke-Compose down --remove-orphans; Start-Environment }
    'k6' { Start-Environment; Run-K6 }
    'logs' { Invoke-Compose logs -f --tail 200 }
    'status' { Invoke-Compose ps }
    'down' { Invoke-Compose down --remove-orphans }
    'clean' { Invoke-Compose down --remove-orphans --volumes }
}
