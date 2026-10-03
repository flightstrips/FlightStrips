param([string]$NATSServerBinary = '', [switch]$SkipFaultSuite, [ValidateRange(1,6)][int]$ParallelPatterns = 1)
$ErrorActionPreference = 'Stop'
$task23Backend = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$task23Run = Join-Path $task23Backend ('.task23/runs/' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ') + '-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $task23Run | Out-Null
$task23Names = @('NATS_INTEGRATION','NATS_TASK23','NATS_TASK23_SMOKE','NATS_TASK23_PATTERN','NATS_TASK23_OUTPUT','NATS_TASK23_PROFILE_NODE','NATS_TASK23_PARALLEL','NATS_SERVER_BINARY')
$task23Saved = @{}
foreach ($task23Name in $task23Names) { $task23Saved[$task23Name] = [Environment]::GetEnvironmentVariable($task23Name,'Process') }
Push-Location $task23Backend
$task23SavedNativeErrorPreference = $PSNativeCommandUseErrorActionPreference
try {
    # A failed load gate must still reach recovery and the full fault suite.
    # Inspect native exit codes explicitly, even when the caller opts into
    # terminating PowerShell errors for unsuccessful native commands.
    $PSNativeCommandUseErrorActionPreference = $false
    if (!$NATSServerBinary) { $NATSServerBinary = Join-Path ((& go env GOPATH).Trim()) 'bin/nats-server.exe' }
    $NATSServerBinary = (Resolve-Path -LiteralPath $NATSServerBinary).Path
    $task23Version = (& $NATSServerBinary --version).Trim()
    if ($LASTEXITCODE -ne 0 -or $task23Version -notmatch 'v2\.15\.0$') { throw 'Task23 requires pinned native NATS 2.15.0' }
    $task23Machine = Get-CimInstance Win32_ComputerSystem
    $task23OS = Get-CimInstance Win32_OperatingSystem
    $task23CPU = Get-CimInstance Win32_Processor | Select-Object -First 1
    $task23Diff = (& git diff --binary HEAD) -join "`n"
    $task23Untracked = @{}
    foreach ($task23File in @(& git ls-files --others --exclude-standard)) {
        if (Test-Path -LiteralPath $task23File -PathType Leaf) { $task23Untracked[$task23File] = (Get-FileHash -LiteralPath $task23File).Hash.ToLowerInvariant() }
    }
    $task23Metadata = [ordered]@{
        started_utc = [DateTime]::UtcNow.ToString('o')
        source_revision = (& git rev-parse HEAD).Trim()
        tracked_diff_sha256 = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($task23Diff))).ToLowerInvariant()
        untracked_source_sha256 = $task23Untracked
        status = @(& git status --porcelain)
        go = (& go version) -join ''
        nats = $task23Version
        nats_binary_sha256 = (Get-FileHash -LiteralPath $NATSServerBinary).Hash.ToLowerInvariant()
        os = $task23OS.Caption
        os_version = $task23OS.Version
        cpu = $task23CPU.Name
        logical_processors = $task23Machine.NumberOfLogicalProcessors
        memory_bytes = $task23Machine.TotalPhysicalMemory
        disks = @(Get-PhysicalDisk | Select-Object FriendlyName,MediaType,BusType,Size)
        physical_failure_domains = 1
        concurrent_load_fixture_limit = $ParallelPatterns
        shared_host_parallel_load = ($ParallelPatterns -gt 1)
        fixture = "random loopback ports; verified owned child PIDs; separate temporary stores/cache/keys; native R3 AES-GCM; two compiled backends per fixture; up to $ParallelPatterns concurrent load fixtures share CPU/disk; recovery and faults sequential"
        command = "go test ./cmd/server -run '^TestPositionLoad' -count=1 -timeout=2h30m -parallel $ParallelPatterns -json"
    }
    $task23Metadata | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $task23Run 'machine-source.json') -Encoding utf8
    $task23DriveNames = @([IO.Path]::GetPathRoot($task23Backend).TrimEnd('\'),[IO.Path]::GetPathRoot([IO.Path]::GetTempPath()).TrimEnd('\')) | Select-Object -Unique
    $task23DiskBefore = @(Get-CimInstance Win32_LogicalDisk | Where-Object DeviceID -In $task23DriveNames | ForEach-Object {
        $task23Used = 100 * (1 - $_.FreeSpace / $_.Size)
        [ordered]@{ drive=$_.DeviceID; total_bytes=$_.Size; free_bytes=$_.FreeSpace; used_percent=$task23Used; alert_70=($task23Used -ge 70); release_block_85=($task23Used -ge 85) }
    })
    $task23DiskBefore | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $task23Run 'disk-before.json') -Encoding utf8
    & go test ./internal/testing/positionload -run '^TestDiskAlertAndReleaseBoundaries$' -count=1 2>&1 | Tee-Object -FilePath (Join-Path $task23Run 'disk-boundary-test.txt')
    if ($LASTEXITCODE -ne 0) { throw "Disk boundary test failed: $task23Run" }
    if ($task23DiskBefore | Where-Object release_block_85) { Write-Warning '85% disk release block recorded; local diagnostics continue, release qualification cannot pass' }
    if ($task23DiskBefore | Where-Object alert_70) { Write-Warning '70% disk usage alert; recorded in disk-before.json' }
    $env:NATS_INTEGRATION='1'
    $env:NATS_TASK23='1'
    $env:NATS_TASK23_SMOKE=$null
    $env:NATS_TASK23_PATTERN=$null
    $env:NATS_TASK23_PROFILE_NODE=$null
    $env:NATS_TASK23_PARALLEL=[string]$ParallelPatterns
    $env:NATS_TASK23_OUTPUT=$task23Run
    $env:NATS_SERVER_BINARY=$NATSServerBinary
    & go test ./cmd/server -run '^TestPositionLoad' -count=1 -timeout=2h30m -parallel $ParallelPatterns -json 2>&1 | Tee-Object -FilePath (Join-Path $task23Run 'tests.jsonl')
    $task23LoadCode=$LASTEXITCODE
    $task23DiskAfter = @(Get-CimInstance Win32_LogicalDisk | Where-Object DeviceID -In $task23DriveNames | ForEach-Object {
        $task23Used = 100 * (1 - $_.FreeSpace / $_.Size)
        [ordered]@{ drive=$_.DeviceID; total_bytes=$_.Size; free_bytes=$_.FreeSpace; used_percent=$task23Used; alert_70=($task23Used -ge 70); release_block_85=($task23Used -ge 85) }
    })
    $task23DiskAfter | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $task23Run 'disk-after.json') -Encoding utf8
    $task23FaultCode = $null
    if (!$SkipFaultSuite) {
        try { & (Join-Path $PSScriptRoot 'task22.ps1') -NATSServerBinary $NATSServerBinary; $task23FaultCode=0 } catch { $task23FaultCode=1; $_ | Out-String | Set-Content -LiteralPath (Join-Path $task23Run 'fault-failure.txt') }
    }
    $task23ReportsComplete = $true
    foreach ($task23Arrival in @(100,160,40)) {
        foreach ($task23Pattern in @('even','second-burst')) {
            $task23ReportPath = Join-Path $task23Run "$task23Arrival-$task23Pattern.json"
            if (!(Test-Path -LiteralPath $task23ReportPath)) { $task23ReportsComplete=$false; continue }
            $task23Report = Get-Content -LiteralPath $task23ReportPath -Raw | ConvertFrom-Json
            if (!$task23Report.full_duration -or $task23Report.aborted -or !$task23Report.load_pass) { $task23ReportsComplete=$false }
        }
    }
    $task23RecoveryPath = Join-Path $task23Run 'backend-recovery.json'
    $task23RecoveryPassed = (Test-Path -LiteralPath $task23RecoveryPath) -and (Get-Content -LiteralPath $task23RecoveryPath -Raw | ConvertFrom-Json).pass
    $task23DiskBlocked = [bool](@($task23DiskBefore) + @($task23DiskAfter) | Where-Object release_block_85)
    $task23Qualified = $task23ReportsComplete -and $task23RecoveryPassed -and !$task23DiskBlocked -and $task23LoadCode -eq 0 -and $task23FaultCode -eq 0 -and !$SkipFaultSuite
    [ordered]@{ finished_utc=[DateTime]::UtcNow.ToString('o'); local_qualification_pass=$task23Qualified; all_six_full_patterns_passed=$task23ReportsComplete; recovery_passed=$task23RecoveryPassed; load_exit=$task23LoadCode; fault_exit=$task23FaultCode; fault_skipped=[bool]$SkipFaultSuite; disk_release_block=$task23DiskBlocked; concurrent_load_fixture_limit=$ParallelPatterns; shared_host_parallel_load=($ParallelPatterns -gt 1); run_directory=$task23Run } | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $task23Run 'run-result.json') -Encoding utf8
    if (!$task23ReportsComplete -or !$task23RecoveryPassed -or $task23LoadCode -ne 0 -or $task23FaultCode -eq 1 -or $task23DiskBlocked) { throw "Qualification failed; complete pass/fail artifacts: $task23Run" }
    Write-Host "Task23 load/recovery/disk checks passed; artifacts: $task23Run"
} finally {
    $PSNativeCommandUseErrorActionPreference = $task23SavedNativeErrorPreference
    foreach ($task23Name in $task23Names) { [Environment]::SetEnvironmentVariable($task23Name,$task23Saved[$task23Name],'Process') }
    Pop-Location
}
