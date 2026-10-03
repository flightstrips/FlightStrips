param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('init', 'build', 'brokers', 'bootstrap', 'start', 'stop', 'restart', 'down', 'status')]
    [string]$Action,
    [string]$Project = 'flightstrips-local'
)
$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    function Compose {
        docker compose -f docker-compose.yaml -p $Project @args
        if ($LASTEXITCODE -ne 0) { throw 'Docker Compose failed' }
    }
    function Wait-Ready {
        $deadline = (Get-Date).AddSeconds(90)
        do {
            $ready = $true
            foreach ($port in @(8090, 8091)) {
                try {
                    $response = Invoke-WebRequest "http://127.0.0.1:$port/readyz" -UseBasicParsing -TimeoutSec 2
                    if ($response.StatusCode -ne 200) { $ready = $false }
                } catch { $ready = $false }
            }
            if ($ready) { Write-Output 'Both backends are ready'; return }
            Start-Sleep -Seconds 1
        } while ((Get-Date) -lt $deadline)
        throw 'Backends did not become ready; inspect docker compose logs'
    }
    switch ($Action) {
        'init' {
            $keyPath = Join-Path $PSScriptRoot '.local-secrets/effects-v1.key'
            if (!(Test-Path -LiteralPath $keyPath)) {
                New-Item -ItemType Directory -Force -Path (Split-Path $keyPath) | Out-Null
                $bytes = New-Object byte[] 32
                $random = [System.Security.Cryptography.RandomNumberGenerator]::Create()
                try { $random.GetBytes($bytes) } finally { $random.Dispose() }
                [System.IO.File]::WriteAllBytes($keyPath, $bytes)
            }
            if ((Get-Item -LiteralPath $keyPath).Length -ne 32) { throw 'Effect key must contain exactly 32 raw bytes' }
            Write-Output 'Shared local effect key exists; existing key preserved'
        }
        'build' { Compose build backend-a }
        'brokers' { Compose up -d nats-1 nats-2 nats-3 }
        'bootstrap' { Compose --profile bootstrap run --rm nats-bootstrap }
        'start' { Compose up -d backend-a backend-b backend-proxy; Wait-Ready }
        'stop' { Compose stop backend-proxy backend-a backend-b }
        'restart' { Compose restart backend-a backend-b; Wait-Ready }
        'down' { Compose down }
        'status' {
            Compose ps
            foreach ($port in @(8090, 8091)) {
                Invoke-WebRequest "http://127.0.0.1:$port/readyz" -UseBasicParsing -TimeoutSec 2 | Select-Object StatusCode
            }
        }
    }
} finally { Pop-Location }
