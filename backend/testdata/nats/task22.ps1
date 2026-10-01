param([string]$NATSServerBinary = '')
$ErrorActionPreference = 'Stop'
$task22Backend = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$task22Run = Join-Path $task22Backend ('.task22/runs/' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $task22Run | Out-Null
$task22Names = @('NATS_INTEGRATION', 'NATS_TASK22', 'NATS_TASK22_LONG', 'NATS_SERVER_BINARY')
$task22Saved = @{}
foreach ($task22Name in $task22Names) { $task22Saved[$task22Name] = [Environment]::GetEnvironmentVariable($task22Name, 'Process') }
Push-Location $task22Backend
try {
    if (!$NATSServerBinary) {
        $task22GoPath = (& go env GOPATH).Trim()
        $NATSServerBinary = Join-Path $task22GoPath 'bin/nats-server.exe'
    }
    $NATSServerBinary = (Resolve-Path -LiteralPath $NATSServerBinary).Path
    $task22BrokerVersion = (& $NATSServerBinary --version).Trim()
    if ($LASTEXITCODE -ne 0 -or $task22BrokerVersion -notmatch 'v2\.15\.0$') { throw 'Task22 requires the pinned native NATS 2.15.0 binary' }
    $task22Machine = Get-CimInstance Win32_ComputerSystem
    $task22CPU = Get-CimInstance Win32_Processor | Select-Object -First 1
    $task22OS = Get-CimInstance Win32_OperatingSystem
    $task22Revision = (& git rev-parse HEAD).Trim()
    $task22Diff = (& git diff --binary HEAD) -join "`n"
    $task22DiffDigest = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($task22Diff))).ToLowerInvariant()
    $task22Metadata = [ordered]@{
        started_utc = [DateTime]::UtcNow.ToString('o')
        source_revision = $task22Revision
        tracked_diff_sha256 = $task22DiffDigest
        status = @(& git status --porcelain)
        go = (& go version) -join ''
        nats = $task22BrokerVersion
        nats_binary_sha256 = (Get-FileHash -LiteralPath $NATSServerBinary).Hash.ToLowerInvariant()
        os = $task22OS.Caption
        os_version = $task22OS.Version
        cpu = $task22CPU.Name
        logical_processors = $task22Machine.NumberOfLogicalProcessors
        memory_bytes = $task22Machine.TotalPhysicalMemory
        physical_failure_domains = 1
        command = "go test ./cmd/server -run '^TestServerNATS' -count=1 -timeout=25m -json"
        fixture = 'test-owned random loopback ports, separate temporary stores and keys, AES-GCM, R3; sequential cases'
    }
    $task22Metadata | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $task22Run 'machine-source.json') -Encoding utf8
    $env:NATS_INTEGRATION = '1'
    $env:NATS_TASK22 = '1'
    $env:NATS_TASK22_LONG = '1'
    $env:NATS_SERVER_BINARY = $NATSServerBinary
    # No Compose service, default dev backend, secret file or existing volume
    # is modified. Each Go fixture kills only the exact child process it created.
    & go test ./cmd/server -run '^TestServerNATS' -count=1 -timeout=25m -json 2>&1 | Tee-Object -FilePath (Join-Path $task22Run 'tests.jsonl')
    $task22Code = $LASTEXITCODE
    $task22Outputs = Get-Content -LiteralPath (Join-Path $task22Run 'tests.jsonl') | ForEach-Object {
        try { $task22Event = $_ | ConvertFrom-Json; if ($task22Event.Output) { $task22Event.Output.TrimEnd() } } catch { $_ }
    }
    $task22Outputs | Where-Object { $_ -match 'BUILD |BROKER_CONFIG |FAULT |OUTCOME |PUBACK_LOSS |INITIAL_PARITY |STRIP_RACE |STAND_RACE |AMAN_RACE |STALE |SNAPSHOT |VERSION_FENCE |LONG_OUTAGE |OVERLAP_|CHECKPOINT |BACKUP |RESTORE|ENCRYPTED_BACKUP |--- (PASS|FAIL|SKIP)' } | Set-Content -LiteralPath (Join-Path $task22Run 'evidence.txt') -Encoding utf8
    if ($task22Code -ne 0) { throw "Task22 failed (exit $task22Code); evidence: $task22Run" }
    if ($task22Outputs -match '--- SKIP:') { throw "Task22 acceptance contains skipped cases; evidence: $task22Run" }
    Write-Host "Task22 PASS; machine/source, full JSON test log and fault/restore evidence: $task22Run"
} finally {
    foreach ($task22Name in $task22Names) { [Environment]::SetEnvironmentVariable($task22Name, $task22Saved[$task22Name], 'Process') }
    Pop-Location
}
