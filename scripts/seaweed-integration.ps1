param(
    [ValidateSet('up', 'down', 'restart', 'k6', 'processing-k6', 'logs', 'clean', 'status')]
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
$UpstreamRegisterRetries = if ($env:UPSTREAM_REGISTER_RETRIES) { [int]$env:UPSTREAM_REGISTER_RETRIES } else { 30 }
$UpstreamRegisterRetrySleep = if ($env:UPSTREAM_REGISTER_RETRY_SLEEP) { [double]$env:UPSTREAM_REGISTER_RETRY_SLEEP } else { 2 }

function Invoke-Compose {
    param([string[]]$ComposeArgs)
    & docker compose -p $Project -f $ComposeFile @ComposeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose command failed with exit code ${LASTEXITCODE}: docker compose $($ComposeArgs -join ' ')"
    }
}

function Test-EnvEnabled {
    param([string]$Name)
    $value = [string](Get-Item -Path "Env:$Name" -ErrorAction SilentlyContinue).Value
    return @('1', 'true', 'yes', 'on') -contains $value.Trim().ToLowerInvariant()
}

function Write-IntegrationSnapshot {
    param([string]$Label)
    Write-Host "===== $Label state snapshot ====="
    try {
        Invoke-Compose -ComposeArgs @('ps')
    } catch {
        Write-Host "Snapshot command failed: $($_.Exception.Message)"
    }
    Write-Host "===== end $Label snapshot ====="
}

function Set-DefaultEnv {
    param([string]$Name, [string]$Value)
    $current = [string](Get-Item -Path "Env:$Name" -ErrorAction SilentlyContinue).Value
    if (-not $current) { Set-Item -Path "Env:$Name" -Value $Value }
}

function Wait-ComposeServiceReady {
    param([string]$Service, [int]$TimeoutSeconds = 180)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $containerId = (Invoke-Compose -ComposeArgs @('ps', '-q', $Service) 2>$null) -join ''
        if ($containerId) {
            $status = (docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' $containerId 2>$null) -join ''
            if (@('healthy', 'running') -contains $status) { return }
            if (@('unhealthy', 'exited', 'dead') -contains $status) { throw "Service $Service is $status" }
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "Timed out waiting for service $Service"
}

function Wait-SeaweedServicesReady {
    foreach ($service in @('seaweed-a', 'seaweed-b', 'seaweed-c')) {
        Wait-ComposeServiceReady -Service $service -TimeoutSeconds 180
    }
}

function Start-OptionalProcessors {
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_CLAMAV_ENABLED') {
        Invoke-Compose -ComposeArgs @('--profile', 'av', 'up', '-d', 'clamav')
        Wait-ComposeServiceReady -Service 'clamav' -TimeoutSeconds 180
    }
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_TIKA_ENABLED') {
        Invoke-Compose -ComposeArgs @('--profile', 'tika', 'up', '-d', 'tika')
        Wait-HttpOk -Url 'http://127.0.0.1:9998/version' -TimeoutSeconds 120
    }
}

function Wait-HttpOk {
    param([string]$Url, [int]$TimeoutSeconds = 120)
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 5
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) { return }
        } catch {}
        Start-Sleep -Seconds 2
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

    for ($attempt = 1; $attempt -le $UpstreamRegisterRetries; $attempt++) {
        try {
            Invoke-WebRequest -Method Post -Uri 'http://127.0.0.1:8080/_s3/upstreams' -Headers @{Authorization="Bearer $AdminToken"} -ContentType 'application/json' -Body $payload -UseBasicParsing -TimeoutSec 10 | Out-Null
            return
        } catch {
            if ($attempt -ge $UpstreamRegisterRetries) {
                throw "Failed to register upstream $Id after $attempt attempts"
            }
            Start-Sleep -Seconds $UpstreamRegisterRetrySleep
        }
    }
}

function Register-Upstreams {
    Register-Upstream -Id 'seaweed-a' -Endpoint 'http://seaweed-a:8333' -Bucket 'maxio-a' -Priority 10
    Register-Upstream -Id 'seaweed-b' -Endpoint 'http://seaweed-b:8333' -Bucket 'maxio-b' -Priority 20
    Register-Upstream -Id 'seaweed-c' -Endpoint 'http://seaweed-c:8333' -Bucket 'maxio-c' -Priority 30
}

function Invoke-K6WithCleanup {
    param([string]$Label, [ScriptBlock]$Action)
    Write-IntegrationSnapshot "$Label before"
    try {
        & $Action
    } catch {
        if (Test-EnvEnabled -Name 'CLEANUP_ON_FAILURE') {
            Write-Host 'CLEANUP_ON_FAILURE is enabled, cleaning stack...'
            Invoke-Compose -ComposeArgs @('down', '--remove-orphans', '--volumes')
        }
        throw
    } finally {
        Write-IntegrationSnapshot "$Label after"
    }
}

function Start-Environment {
    Invoke-Compose -ComposeArgs @('up', '-d', 'seaweed-a', 'seaweed-b', 'seaweed-c')
    Wait-SeaweedServicesReady
    Start-OptionalProcessors
    Initialize-SeaweedBuckets
    $composeArgs = @('up', '-d')
    if ($Build) { $composeArgs += '--build' }
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_ENABLED') { $composeArgs += '--force-recreate' }
    $composeArgs += 'maxio'
    Invoke-Compose -ComposeArgs $composeArgs
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
    Invoke-Compose -ComposeArgs @('--profile', 'perf', 'run', '--rm', 'k6', 'run', '/scripts/seaweed-smoke.js')
}

function Run-ProcessingK6 {
    Set-DefaultEnv -Name 'MAXIO_PROCESSING_ENABLED' -Value 'true'
    Set-DefaultEnv -Name 'MAXIO_PROCESSING_MODE' -Value 'async_permissive'
    Set-DefaultEnv -Name 'MAXIO_PROCESSING_CLAMAV_ENABLED' -Value 'true'
    Set-DefaultEnv -Name 'MAXIO_PROCESSING_CLAMAV_MODE' -Value 'inline_strict'
    Set-DefaultEnv -Name 'MAXIO_PROCESSING_TIKA_ENABLED' -Value 'true'
    Set-DefaultEnv -Name 'MAXIO_PROCESSING_TIKA_MODE' -Value 'async_permissive'

    $clamavMode = [string](Get-Item -Path 'Env:MAXIO_PROCESSING_CLAMAV_MODE' -ErrorAction SilentlyContinue).Value
    $tikaMode = [string](Get-Item -Path 'Env:MAXIO_PROCESSING_TIKA_MODE' -ErrorAction SilentlyContinue).Value
    $clamavMode = $clamavMode.Trim().ToLowerInvariant()
    $tikaMode = $tikaMode.Trim().ToLowerInvariant()
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_TIKA_ENABLED') {
        if ($tikaMode -eq 'async_permissive') {
            Set-DefaultEnv -Name 'MAXIO_PROCESSING_TIKA_FAIL_OPEN' -Value 'true'
        } else {
            Set-DefaultEnv -Name 'MAXIO_PROCESSING_TIKA_FAIL_OPEN' -Value 'false'
        }
    }
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_TIKA_FAIL_OPEN') {
        $tikaFailOpen = 'true'
    } else {
        $tikaFailOpen = 'false'
    }

    $processors = @()
    $processorModes = @()
    $processorFailOpen = @()
    $capabilities = @()
    $resultMetadata = @()
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_CLAMAV_ENABLED') {
        $processors += 'clamav'
        $processorModes += "clamav:$clamavMode"
        $capabilities += 'antivirus'
        $resultMetadata += 'clamav:verdict=clean'
        $resultMetadata += 'clamav:response'
    }
    if (Test-EnvEnabled -Name 'MAXIO_PROCESSING_TIKA_ENABLED') {
        $processors += 'tika'
        $processorModes += "tika:$tikaMode"
        $processorFailOpen += "tika:$tikaFailOpen"
        $capabilities += 'text_extraction'
        $capabilities += 'metadata_extract'
        $resultMetadata += 'tika:endpoint'
        $resultMetadata += 'tika:text_bytes'
        $resultMetadata += 'tika:document_count'
    }

    if ($processors.Count -eq 0) {
        $processingMode = [string](Get-Item -Path 'Env:MAXIO_PROCESSING_MODE' -ErrorAction SilentlyContinue).Value
        $processingMode = $processingMode.Trim().ToLowerInvariant()
        $processors += 'noop'
        $processorModes += "noop:$processingMode"
    }

    Set-DefaultEnv -Name 'PROCESSING_RECORD_CHECK' -Value 'true'
    Set-DefaultEnv -Name 'PROCESSING_EXPECT_PROCESSORS' -Value ($processors -join ',')
    Set-DefaultEnv -Name 'PROCESSING_EXPECT_PROCESSOR_MODES' -Value ($processorModes -join ',')
    Set-DefaultEnv -Name 'PROCESSING_EXPECT_PROCESSOR_FAIL_OPEN' -Value ($processorFailOpen -join ',')
    Set-DefaultEnv -Name 'PROCESSING_EXPECT_CAPABILITIES' -Value ($capabilities -join ',')
    Set-DefaultEnv -Name 'PROCESSING_EXPECT_RESULT_METADATA' -Value ($resultMetadata -join ',')
    Set-DefaultEnv -Name 'PROCESSING_RECORD_RETRIES' -Value '30'
    Set-DefaultEnv -Name 'PROCESSING_RECORD_RETRY_SLEEP' -Value '0.5'
    if ((Test-EnvEnabled -Name 'MAXIO_PROCESSING_CLAMAV_ENABLED') -and $clamavMode -eq 'inline_strict') {
        Set-DefaultEnv -Name 'CLAMAV_BLOCK_CHECK' -Value 'true'
    } else {
        Set-DefaultEnv -Name 'CLAMAV_BLOCK_CHECK' -Value 'false'
    }

    Start-Environment
    Run-K6
}

switch ($Action) {
    'up' { Start-Environment }
    'restart' { Invoke-Compose -ComposeArgs @('down', '--remove-orphans'); Start-Environment }
    'k6' { Invoke-K6WithCleanup -Label 'k6' -Action { Start-Environment; Run-K6 } }
    'processing-k6' { Invoke-K6WithCleanup -Label 'processing-k6' -Action { Run-ProcessingK6 } }
    'logs' { Invoke-Compose -ComposeArgs @('logs', '-f', '--tail', '200') }
    'status' { Invoke-Compose -ComposeArgs @('ps') }
    'down' { Invoke-Compose -ComposeArgs @('down', '--remove-orphans') }
    'clean' { Invoke-Compose -ComposeArgs @('down', '--remove-orphans', '--volumes') }
}
