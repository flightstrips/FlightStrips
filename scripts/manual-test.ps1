#requires -Version 7.0
<#
.SYNOPSIS
Build and launch a worktree-local FlightStrips manual testing environment.
.EXAMPLE
pwsh ./scripts/manual-test.ps1 init
.EXAMPLE
pwsh ./scripts/manual-test.ps1 start -Scenario ekch-twr-22
.EXAMPLE
pwsh ./scripts/manual-test.ps1 start -Scenario "./scenarios/ekch-twr-Rwy04.txt"
#>
[CmdletBinding()]
param(
    [Parameter(Position=0)][ValidateSet('init','start','status','stop','reset')][string]$Action = 'start',
    [string]$ConfigPath,
    [Alias('ScenarioFile')][string]$Scenario,
    [switch]$OpenBrowser,
    [ValidateRange(1,600)][int]$ConnectionTimeoutSeconds = 120
)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$runtime = Join-Path $root '.manual-test'
if (!$ConfigPath) { $ConfigPath = Join-Path $PSScriptRoot 'manual-test.local.json' }
$ConfigPath = [IO.Path]::GetFullPath($ConfigPath)
$statePath = Join-Path $runtime 'state.json'
$compose = Join-Path $PSScriptRoot 'manual-test/compose.yaml'
$hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($root.ToLowerInvariant()))).Substring(0,10).ToLowerInvariant()
$project = "flightstrips-manual-$hash"
$fsdRegistry = 'HKCU:/Software/Csernak Gergely/EuroScopeFsdServer/LastSession'

function Invoke-Checked([string]$Exe, [string[]]$Arguments) {
    & $Exe @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Exe failed ($LASTEXITCODE): $($Arguments -join ' ')" }
}
function Resolve-TestScenario([string]$Selection) {
    if ([string]::IsNullOrWhiteSpace($Selection)) { $Selection = $config.DefaultScenario }
    if ([string]::IsNullOrWhiteSpace($Selection)) { throw 'Provide -Scenario with a file path or preset name, or set DefaultScenario in the local config.' }
    $presets = if ($config.Scenarios) { $config.Scenarios } else { @{} }
    $isPreset = $presets.ContainsKey($Selection)
    $definition = if ($isPreset) { $presets[$Selection] } else { @{ File=$Selection } }
    if ($definition -is [string]) { $definition = @{ File=$definition } }
    $file = $definition.File
    if ([string]::IsNullOrWhiteSpace($file)) { throw "Missing scenario file for '$Selection'." }
    # Preset paths belong to the config; direct relative paths belong to the caller.
    if ($isPreset -and ![IO.Path]::IsPathRooted($file)) { $file = Join-Path (Split-Path $ConfigPath -Parent) $file }
    if (!(Test-Path -LiteralPath $file -PathType Leaf)) { throw "Unknown scenario preset or missing scenario file: '$Selection' ($file)." }
    $file = (Get-Item -LiteralPath $file).FullName
    $arrivalRunways = $definition.ArrivalRunways
    $departureRunways = $definition.DepartureRunways
    # The EKCH 04/22 scenarios identify their runway direction in the filename.
    # Explicit preset runway settings take precedence over filename inference.
    if ([IO.Path]::GetFileNameWithoutExtension($file) -match '(?i)Rwy[ _-]*(04|22)(?!\d)') {
        $direction = $Matches[1]
        if (!$definition.ContainsKey('ArrivalRunways')) { $arrivalRunways = @("${direction}L") }
        if (!$definition.ContainsKey('DepartureRunways')) { $departureRunways = @("${direction}R") }
    }
    return @{ Name=$(if ($isPreset) { $Selection } else { $file }); File=$file; ArrivalRunways=$arrivalRunways; DepartureRunways=$departureRunways }
}
function Get-SourceStamp {
    Push-Location $root
    try {
        $source = ((& git rev-parse HEAD) + ((& git diff HEAD -- backend frontend euroscope-plugin scripts) -join "`n"))
        foreach ($file in (& git ls-files --others --exclude-standard -- backend frontend euroscope-plugin scripts)) {
            $source += "$file`n$((Get-FileHash -LiteralPath (Join-Path $root $file)).Hash)`n"
        }
        return [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($source)))
    } finally { Pop-Location }
}
function Save-State { $script:state | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath $statePath -Encoding utf8 }
function New-LaunchState($SelectedScenario) {
    $next = @{ RunId=[Guid]::NewGuid().ToString(); Root=$root; ConfigHash=(Get-FileHash -LiteralPath $ConfigPath).Hash; Scenario=$SelectedScenario.Name; ScenarioPath=$SelectedScenario.File; ArrivalRunways=$SelectedScenario.ArrivalRunways; DepartureRunways=$SelectedScenario.DepartureRunways; ControllerCallsign=$config.Callsign; SourceStamp=Get-SourceStamp; Processes=@{}; PluginHashes=@{} }
    # A failed launch can own cleanup obligations even when no process survived.
    # Carry these until stop/reset successfully restores the affected resources.
    foreach ($key in 'SuspendedContainers','OriginalFsdScenario','OriginalFsdSameAsDefault') {
        if ($state.ContainsKey($key)) { $next[$key] = $state[$key] }
    }
    return $next
}
function Get-OwnedProcess($Entry) {
    if (!$Entry) { return $null }
    $process = Get-Process -Id $Entry.Id -ErrorAction SilentlyContinue
    # PowerShell 7.5 parses ISO JSON dates into DateTime automatically; compare ticks.
    $started = if ($Entry.Started -is [datetime]) { $Entry.Started } else { [datetime]::Parse($Entry.Started) }
    if ($process -and $process.StartTime.ToUniversalTime().Ticks -eq $started.ToUniversalTime().Ticks) { return $process }
    return $null
}
function Start-Owned([string]$Name, [string]$Exe, [string[]]$Arguments, [string]$WorkingDirectory, [switch]$Visible) {
    # Start-Process joins arguments on Windows; reject embedded quotes and quote each argument.
    $quoted = foreach ($arg in $Arguments) {
        if ($arg.Contains('"')) { throw 'An argument contains an unsupported double quote.' }
        '"' + $arg + '"'
    }
    $options = @{ FilePath=$Exe; ArgumentList=$quoted; WorkingDirectory=$WorkingDirectory; PassThru=$true }
    if (!$Visible) {
        $options.WindowStyle = 'Hidden'
        $options.RedirectStandardOutput = Join-Path $runtime "$Name.log"
        $options.RedirectStandardError = Join-Path $runtime "$Name.error.log"
    }
    $process = Start-Process @options
    $script:state.Processes[$Name] = @{ Id=$process.Id; Started=$process.StartTime.ToUniversalTime().ToString('o') }
    Save-State
    return $process
}
function Test-Http([string]$Url) {
    try { return Invoke-RestMethod -Uri $Url -TimeoutSec 2 } catch { return $null }
}
function Wait-Http([string]$Url, [string]$ProcessName) {
    $deadline = [DateTime]::UtcNow.AddSeconds(90)
    do {
        if (!(Get-OwnedProcess $state.Processes[$ProcessName])) { throw "$ProcessName exited. See $runtime/$ProcessName.error.log and $ProcessName.log" }
        $response = Test-Http $Url
        if ($response) { return $response }
        Start-Sleep -Milliseconds 500
    } while ([DateTime]::UtcNow -lt $deadline)
    throw "Timed out waiting for $Url. See $runtime."
}
function Assert-PortFree([int]$Port) {
    if (Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue) {
        throw "Port $Port is already occupied. Stop its owner or choose another port in $ConfigPath."
    }
}
function Invoke-Compose([string[]]$Arguments) {
    $previous = $env:MANUAL_TEST_DATABASE_PORT
    try { $env:MANUAL_TEST_DATABASE_PORT = [string]$config.DatabasePort; Invoke-Checked docker (@('compose','-p',$project,'-f',$compose) + $Arguments) }
    finally { $env:MANUAL_TEST_DATABASE_PORT = $previous }
}
function Invoke-TestDatabaseQuery([string]$Query, [hashtable]$Variables = @{}) {
    $arguments = @('compose','-p',$project,'-f',$compose,'exec','-T','postgres','psql','-X','-U','fs','-d','fsdb','-At','-v','ON_ERROR_STOP=1')
    foreach ($entry in $Variables.GetEnumerator()) { $arguments += @('-v',"$($entry.Key)=$($entry.Value)") }
    # psql expands quoted variables from stdin, not from a -c command string.
    $result = $Query | & docker @arguments 2>$null
    if ($LASTEXITCODE -ne 0) { throw 'Manual-test database query failed.' }
    return $result -join "`n"
}
function Get-SessionSummaryQuery {
    return @'
WITH launch AS (
    SELECT :'started_at'::timestamptz AS started_at, :'callsign'::text AS callsign
), connected_sessions AS (
    SELECT DISTINCT s.id, s.airport, s.name
    FROM sessions s
    JOIN controllers c ON c.session = s.id
    CROSS JOIN launch
    WHERE s.airport = 'EKCH' AND s.name = 'SWEATBOX'
      AND c.callsign = launch.callsign
      AND c.last_seen_euroscope >= (launch.started_at AT TIME ZONE 'UTC')
      AND c.last_seen_euroscope > (statement_timestamp() AT TIME ZONE 'UTC') - interval '30 seconds'
)
SELECT json_build_object(
    'controllers', (SELECT count(*) FROM connected_sessions),
    'strips', (SELECT count(*) FROM strips strip
               JOIN connected_sessions s ON s.id = strip.session
               CROSS JOIN launch
               WHERE strip.euroscope_seen_at >= launch.started_at),
    'sessions', (SELECT coalesce(json_agg(json_build_object('airport', airport, 'name', name)), '[]'::json)
                 FROM connected_sessions));
'@
}
function Get-SessionSummary {
    if (!(Get-OwnedProcess $state.Processes.backend) -or !$state.ObservationStartedAt -or !$state.ControllerCallsign) { return $null }
    try {
        $variables = @{ started_at=([datetimeoffset]$state.ObservationStartedAt).ToString('o'); callsign=$state.ControllerCallsign }
        $result = Invoke-TestDatabaseQuery (Get-SessionSummaryQuery) $variables
        if ($result) { return $result | ConvertFrom-Json }
    } catch { Write-Verbose "Session readiness query failed: $_" }
    return $null
}
function Test-ScenarioAgreement {
    if (!$state.ScenarioPath -or !$state.TestProfile -or !(Test-Path -LiteralPath $state.TestProfile)) { return $false }
    $scenarioLine = Get-Content -LiteralPath $state.TestProfile | Where-Object { $_ -match '^LastSession\tscenario\t' } | Select-Object -First 1
    if (!$scenarioLine) { return $false }
    $esScenario = ($scenarioLine -split "`t",3)[2]
    $fsdSettings = Get-ItemProperty -LiteralPath $fsdRegistry -ErrorAction SilentlyContinue
    $fsdScenario = $fsdSettings.Session0ScenarioFile
    if (!$esScenario -or !$fsdScenario -or !(Test-Path -LiteralPath $state.ScenarioPath)) { return $false }
    $expected = [IO.Path]::GetFullPath($state.ScenarioPath)
    return ([IO.Path]::GetFullPath($esScenario) -eq $expected -and
            [IO.Path]::GetFullPath($fsdScenario) -eq $expected -and
            $fsdSettings.Session0SameAsDefault -eq '0' -and
            (Get-FileHash -LiteralPath $expected).Hash -eq $state.ScenarioHash)
}
function Show-Status {
    Write-Host "Worktree: $root"
    Write-Host "Profile:  $($state.TestProfile)"
    Write-Host "Logs:     $runtime"
    foreach ($name in 'backend','frontend','fsd','euroscope') {
        $process = Get-OwnedProcess $state.Processes[$name]
        Write-Host ('{0,-12} {1}' -f $name, $(if ($process) { "running (PID $($process.Id))" } else { 'stopped / not owned' }))
    }
    $backend = Test-Http "http://127.0.0.1:$($config.BackendPort)/healthz"
    $frontend = Test-Http "http://127.0.0.1:$($config.FrontendPort)/__manual-test"
    $summary = Get-SessionSummary
    $scenarioMatches = Test-ScenarioAgreement
    $pluginMatches = $true
    foreach ($entry in $state.PluginHashes.GetEnumerator()) {
        $file = Join-Path $state.TestPluginDirectory $entry.Key
        if (!(Test-Path -LiteralPath $file) -or (Get-FileHash -LiteralPath $file).Hash -ne $entry.Value) { $pluginMatches = $false }
    }
    Write-Host "Backend health: $($backend.status); frontend belongs to run: $($frontend.runId -eq $state.RunId); deployed plugin hashes match: $pluginMatches"
    Write-Host "FSD and EuroScope configured with the same scenario file: $scenarioMatches"
    Write-Host "Scenario: $($state.ScenarioPath)"
    if ($summary) { Write-Host "Connected test controllers: $($summary.controllers); strips observed this launch: $($summary.strips); sessions: $($summary.sessions | ConvertTo-Json -Compress)" }
    if ($backend -and $frontend.runId -eq $state.RunId -and $pluginMatches -and $scenarioMatches -and $summary.controllers -gt 0 -and $summary.strips -gt 0) {
        Write-Host 'READY FOR MANUAL TESTING' -ForegroundColor Green
    } else { Write-Host 'NOT READY: connect EuroScope to localhost and authenticate the FlightStrips plugin if prompted.' -ForegroundColor Yellow }
}
function Stop-Owned {
    if ($state.TestPluginDirectory) {
        $cachedLogin = Join-Path $state.TestPluginDirectory 'flightstrips_user.ini'
        if (Test-Path -LiteralPath $cachedLogin) { Copy-Item -LiteralPath $cachedLogin -Destination (Join-Path $runtime 'plugin/flightstrips_user.ini') -Force }
    }
    foreach ($name in 'euroscope','fsd','frontend','backend') {
        $process = Get-OwnedProcess $state.Processes[$name]
        if ($process) {
            # Stop only this launcher's recorded process tree, including node/server children.
            # Test GUI processes are terminated too, avoiding save/close confirmation dialogs.
            Invoke-Checked taskkill @('/PID',[string]$process.Id,'/T','/F')
        }
        $state.Processes.Remove($name)
        Save-State
    }
    if ($state.ContainsKey('OriginalFsdScenario')) {
        if ($null -eq $state.OriginalFsdScenario) { Remove-ItemProperty -LiteralPath $fsdRegistry -Name Session0ScenarioFile -ErrorAction SilentlyContinue }
        else { Set-ItemProperty -LiteralPath $fsdRegistry -Name Session0ScenarioFile -Value $state.OriginalFsdScenario }
        $state.Remove('OriginalFsdScenario'); Save-State
    }
    if ($state.ContainsKey('OriginalFsdSameAsDefault')) {
        if ($null -eq $state.OriginalFsdSameAsDefault) { Remove-ItemProperty -LiteralPath $fsdRegistry -Name Session0SameAsDefault -ErrorAction SilentlyContinue }
        else { Set-ItemProperty -LiteralPath $fsdRegistry -Name Session0SameAsDefault -Value $state.OriginalFsdSameAsDefault }
        $state.Remove('OriginalFsdSameAsDefault'); Save-State
    }
    Invoke-Compose @('stop')
    foreach ($container in @($state.SuspendedContainers | Where-Object { ![string]::IsNullOrWhiteSpace($_) } | Select-Object -Unique)) {
        Invoke-Checked docker @('start',$container)
        $state.SuspendedContainers = @($state.SuspendedContainers | Where-Object { $_ -and $_ -ne $container })
        Save-State
    }
}
function ConvertTo-SnapshotReferences([string]$Text, [string]$Source, [string]$Destination) {
    # Windows paths ignore case and may mix separator styles. Only match a whole
    # directory prefix, not a similarly named sibling such as Data-External.
    $prefix = (($Source.TrimEnd('\','/') -split '[\\/]' | ForEach-Object { [regex]::Escape($_) }) -join '[\\/]')
    $pattern = $prefix + '(?=[\\/]|$|["''\r\n\t])'
    return [regex]::Replace($Text, $pattern, [Text.RegularExpressions.MatchEvaluator]{ param($match) $Destination },
        [Text.RegularExpressions.RegexOptions]::IgnoreCase -bor [Text.RegularExpressions.RegexOptions]::CultureInvariant)
}
function Get-SnapshotWritePath([string]$Path, [string]$SnapshotDirectory) {
    if ([string]::IsNullOrWhiteSpace($Path)) { throw 'Missing sector path in the copied EuroScope profile.' }
    $snapshot = [IO.Path]::GetFullPath($SnapshotDirectory).TrimEnd('\')
    $destination = [IO.Path]::GetFullPath($Path, $snapshot)
    if (!$destination.StartsWith($snapshot + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'Runway destination is outside the copied snapshot. Keep sector files inside EuroScopeDataDirectory and update the source profile to reference them.'
    }
    # A lexically contained path can still escape through a directory junction
    # or a symlink, including a linked existing runway file.
    for ($current = $destination; ; $current = Split-Path $current -Parent) {
        if (Test-Path -LiteralPath $current) {
            if ((Get-Item -LiteralPath $current -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) {
                throw 'Runway destination crosses a reparse point in the copied snapshot.'
            }
        }
        if ($current -eq $snapshot) { break }
    }
    return $destination
}
function New-TestProfile {
    $sourceData = [IO.Path]::GetFullPath($config.EuroScopeDataDirectory).TrimEnd('\')
    $sourceApp = Split-Path ([IO.Path]::GetFullPath($config.EuroScopeExe)) -Parent
    $sourceFsd = Split-Path ([IO.Path]::GetFullPath($config.FsdExe)) -Parent
    $sourceProfile = [IO.Path]::GetFullPath($config.BaseProfile)
    if (!$sourceProfile.StartsWith($sourceData + '\', [StringComparison]::OrdinalIgnoreCase)) {
        throw 'BaseProfile must be inside EuroScopeDataDirectory.'
    }
    # Keep executable paths stable so Windows can reuse its per-program firewall decision.
    # Archive the entire previous copy before making a fresh whole-folder copy.
    $copyRoot = [IO.Path]::GetFullPath((Join-Path $runtime 'euroscope/current'))
    $snapshotRoot = [IO.Path]::GetFullPath((Join-Path $runtime 'euroscope'))
    $ownerFile = Join-Path $copyRoot '.flightstrips-owner'
    if (Test-Path -LiteralPath $copyRoot) {
        $archive = [IO.Path]::GetFullPath((Join-Path $snapshotRoot "history/$([Guid]::NewGuid())"))
        foreach ($target in @($copyRoot,$archive)) {
            if (!$target.StartsWith($snapshotRoot + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Snapshot move escaped the manual-test directory.' }
        }
        foreach ($directory in @($runtime,$snapshotRoot,$copyRoot)) {
            if ((Get-Item -LiteralPath $directory).Attributes -band [IO.FileAttributes]::ReparsePoint) { throw "Refusing to move a snapshot through a reparse point: $directory" }
        }
        if (!(Test-Path -LiteralPath $ownerFile) -or (Get-Content -LiteralPath $ownerFile -Raw).Trim() -ne $root) { throw 'Existing current snapshot is not owned by this worktree.' }
        New-Item -ItemType Directory -Path (Split-Path $archive -Parent) -Force | Out-Null
        Move-Item -LiteralPath $copyRoot -Destination $archive
    }
    $copiedData = Join-Path $copyRoot 'data'
    $copiedApp = Join-Path $copyRoot 'app'
    foreach ($source in @($sourceData,$sourceApp,$sourceFsd)) {
        if ($copyRoot.StartsWith($source + '\', [StringComparison]::OrdinalIgnoreCase)) { throw 'Test destination must be outside the source EuroScope folders.' }
    }
    New-Item -ItemType Directory -Path $copyRoot -Force | Out-Null
    $root | Set-Content -LiteralPath $ownerFile
    Write-Host "Copying complete EuroScope data and installation folders to $copyRoot ..."
    Copy-Item -LiteralPath $sourceData -Destination $copiedData -Recurse
    Copy-Item -LiteralPath $sourceApp -Destination $copiedApp -Recurse
    $copiedFsd = $copiedApp
    if ($sourceFsd -ne $sourceApp) {
        $copiedFsd = Join-Path $copyRoot 'fsd-app'
        Copy-Item -LiteralPath $sourceFsd -Destination $copiedFsd -Recurse
    }
    $state.TestProfile = Join-Path $copiedData $sourceProfile.Substring($sourceData.Length + 1)
    $state.TestEuroScopeExe = Join-Path $copiedApp (Split-Path $config.EuroScopeExe -Leaf)
    $state.TestFsdExe = Join-Path $copiedFsd (Split-Path $config.FsdExe -Leaf)
    $state.TestDataDirectory = $copiedData
    $state.TestPluginDirectory = Join-Path $copiedData 'EKDK/Plugins/FlightStrips-Manual-Test'
    # Rebase absolute references in copied text configuration files only.
    foreach ($file in Get-ChildItem -LiteralPath $copiedData -Recurse -File | Where-Object { $_.Extension -in '.prf','.asr','.ini','.txt','.ese','.sct','.rwy' }) {
        $content = [IO.File]::ReadAllText($file.FullName)
        $updated = ConvertTo-SnapshotReferences $content $sourceData $copiedData
        $updated = ConvertTo-SnapshotReferences $updated $sourceApp $copiedApp
        if ($updated -ne $content) { [IO.File]::WriteAllText($file.FullName,$updated,[Text.UTF8Encoding]::new($false)) }
    }
    New-Item -ItemType Directory -Path $state.TestPluginDirectory -Force | Out-Null
    foreach ($file in Get-ChildItem -LiteralPath (Join-Path $runtime 'plugin') -File) {
        Copy-Item -LiteralPath $file.FullName -Destination $state.TestPluginDirectory -Force
    }
    $testIni = Join-Path $state.TestPluginDirectory 'flightstrips_config.ini'
    $iniContent = [IO.File]::ReadAllText($testIni)
    $testStands = ConvertTo-SnapshotReferences ([IO.Path]::GetFullPath($config.StandsFile)) $sourceData $copiedData
    $testAirlines = ConvertTo-SnapshotReferences ([IO.Path]::GetFullPath($config.AirlinesFile)) $sourceData $copiedData
    $iniContent = [regex]::Replace($iniContent,'(?m)^stands\s*=.*$', { "stands = $testStands" })
    $iniContent = [regex]::Replace($iniContent,'(?m)^airlines\s*=.*$', { "airlines = $testAirlines" })
    [IO.File]::WriteAllText($testIni,$iniContent,[Text.UTF8Encoding]::new($false))
    $state.PluginHashes['flightstrips_config.ini'] = (Get-FileHash -LiteralPath $testIni).Hash
    $state.SourceScenarioPath = $state.ScenarioPath
    # Both applications reference this exact canonical source file, never separate copies.
    $state.ScenarioPath = [IO.Path]::GetFullPath($state.ScenarioPath)
    $state.ScenarioHash = (Get-FileHash -LiteralPath $state.ScenarioPath).Hash
    $hasPlugin = $false
    $lines = foreach ($line in Get-Content -LiteralPath $state.TestProfile) {
        $parts = $line -split "`t",3
        if ($parts.Count -ne 3) { $line; continue }
        if ($parts[0] -eq 'LastSession') { continue }
        if ($parts[2].StartsWith('\') -and !$parts[2].StartsWith('\\')) { $parts[2] = Join-Path $copiedData $parts[2].TrimStart('\') }
        if ($parts[0] -eq 'Plugins' -and $parts[1] -match '^Plugin\d+$' -and $parts[2] -match '(?i)FlightStrips[^\\/]*\.dll$') {
            $parts[2] = Join-Path $state.TestPluginDirectory 'FlightStripsPluginCore.dll'
            $hasPlugin = $true
        }
        $parts -join "`t"
    }
    if (!$hasPlugin) { throw 'BaseProfile must contain a FlightStrips plugin entry. Enable it once in the base profile first.' }
    $last = @{ callsign=$config.Callsign; facility=[string]$config.Facility; rating='4'; server='localhost'; tovatsim='0'; connecttype='2'; range='100'; realname='FlightStrips local test'; certificate='999999'; password='local'; simdatapublish='2'; scenario=$state.ScenarioPath }
    $lines += foreach ($entry in $last.GetEnumerator()) { "LastSession`t$($entry.Key)`t$($entry.Value)" }
    # Preserve accented paths and settings without adding a BOM to the first key.
    $lines | Set-Content -LiteralPath $state.TestProfile -Encoding utf8NoBOM
    $sectorLine = @($lines | Where-Object { $_ -match '^Settings\tsector\t' })[0]
    $testSector = ($sectorLine -split "`t",3)[2]
    if ($state.ArrivalRunways -or $state.DepartureRunways) {
        $runwayLines = @(foreach ($runway in $state.ArrivalRunways) { "ACTIVE_RUNWAY:EKCH:${runway}:0" })
        $runwayLines += @(foreach ($runway in $state.DepartureRunways) { "ACTIVE_RUNWAY:EKCH:${runway}:1" })
        $runwayPath = Get-SnapshotWritePath ([IO.Path]::ChangeExtension($testSector,'.rwy')) $copiedData
        $runwayLines | Set-Content -LiteralPath $runwayPath -Encoding ascii
    }
    Save-State
}

if ($Action -eq 'init') {
    if (Test-Path -LiteralPath $ConfigPath) { throw "Config already exists: $ConfigPath" }
    Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'manual-test.example.json') -Destination $ConfigPath
    Write-Host "Created $ConfigPath. Set the machine paths and scenario paths, then run start."
    exit 0
}
if (!(Test-Path -LiteralPath $ConfigPath)) { throw "Missing $ConfigPath. Run: pwsh ./scripts/manual-test.ps1 init" }
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json -AsHashtable
if ($config.NodeDirectory) { $env:PATH = "$($config.NodeDirectory);$env:PATH" }
New-Item -ItemType Directory -Path $runtime -Force | Out-Null
# Hold an exclusive lock for mutations; status remains usable while start waits for connection.
$lock = $null
try {
    if ($Action -ne 'status') { $lock = [IO.File]::Open((Join-Path $runtime 'launcher.lock'), 'OpenOrCreate', 'ReadWrite', 'None') }
    $state = if (Test-Path -LiteralPath $statePath) { Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json -AsHashtable } else { @{ Processes=@{}; PluginHashes=@{} } }
    if ($Action -eq 'status') { Show-Status; exit 0 }
    $hasOwnedProcesses = @($state.Processes.Values | Where-Object { Get-OwnedProcess $_ }).Count -gt 0
    # Resolve an explicit selection before reset can stop a running test session.
    $selectedScenario = if ($Action -in 'start','reset' -and ($Scenario -or $Action -eq 'reset' -or !$hasOwnedProcesses)) { Resolve-TestScenario $Scenario } else { $null }
    if ($Action -in 'stop','reset') { Stop-Owned; if ($Action -eq 'stop') { exit 0 } }
    if (@($state.Processes.Values | Where-Object { Get-OwnedProcess $_ }).Count -gt 0) {
        if ($state.SourceStamp -ne (Get-SourceStamp) -or $state.ConfigHash -ne (Get-FileHash -LiteralPath $ConfigPath).Hash) { throw 'Source or config changed since launch. Run reset to rebuild and restart this test environment.' }
        if ($selectedScenario -and ($selectedScenario.File -ne $state.ScenarioPath -or
                ($selectedScenario.ArrivalRunways -join ',') -ne ($state.ArrivalRunways -join ',') -or
                ($selectedScenario.DepartureRunways -join ',') -ne ($state.DepartureRunways -join ','))) {
            throw 'A different scenario or runway configuration is running. Use reset -Scenario with the desired file path or preset.'
        }
        Show-Status; exit 0
    }
    foreach ($command in 'docker','go','node','npm.cmd','cmake') { if (!(Get-Command $command -ErrorAction SilentlyContinue)) { throw "Missing prerequisite: $command" } }
    $nodeVersion = [version]((& node --version).TrimStart('v'))
    if ($nodeVersion -lt [version]'22.22.0') { throw "Node $nodeVersion is older than the frontend requires (22.22.0). Set NodeDirectory in $ConfigPath to a supported Node installation." }
    foreach ($key in 'EuroScopeExe','FsdExe','EuroScopeDataDirectory','BaseProfile','StandsFile','AirlinesFile','AircraftFile') {
        if (!(Test-Path -LiteralPath $config[$key])) { throw "Invalid $key in ${ConfigPath}: $($config[$key])" }
    }
    if (Get-Process EuroScope,EuroScopeFsdServer -ErrorAction SilentlyContinue) { throw 'Close the existing EuroScope/FSD instances first. The launcher will not modify an unowned running instance.' }
    $state = New-LaunchState $selectedScenario
    Save-State
    foreach ($container in $config.SuspendContainers) {
        $running = & docker inspect --format '{{.State.Running}}' $container
        if ($LASTEXITCODE -ne 0) { throw "Configured container does not exist: $container" }
        if ($running -eq 'true') {
            $state.SuspendedContainers = @(@($state.SuspendedContainers | Where-Object { ![string]::IsNullOrWhiteSpace($_) }) + $container | Select-Object -Unique); Save-State
            Invoke-Checked docker @('stop',$container)
        }
    }
    Assert-PortFree $config.BackendPort
    Assert-PortFree $config.FrontendPort
    Write-Host "Building plugin from $root ..."
    Invoke-Checked cmake @('-S',"$root/euroscope-plugin",'-B',"$runtime/plugin-build",'-G','Visual Studio 17 2022','-A','Win32')
    Invoke-Checked cmake @('--build',"$runtime/plugin-build",'--config','Debug','--target','FlightStripsPluginCore','FlightStripsLoader','--parallel','4')
    $pluginDirectory = Join-Path $runtime 'plugin'
    New-Item -ItemType Directory -Path $pluginDirectory -Force | Out-Null
    foreach ($dll in 'FlightStripsPlugin.dll','FlightStripsPluginCore.dll') {
        $built = @(Get-ChildItem -LiteralPath "$runtime/plugin-build" -Recurse -File -Filter $dll)
        if ($built.Count -ne 1) { throw "Expected one built $dll, found $($built.Count)." }
        Copy-Item -LiteralPath $built[0].FullName -Destination $pluginDirectory -Force
        $state.PluginHashes[$dll] = (Get-FileHash -LiteralPath $built[0].FullName).Hash
    }
    foreach ($dll in Get-ChildItem -LiteralPath "$root/euroscope-plugin/bin" -Filter '*.dll') {
        Copy-Item -LiteralPath $dll.FullName -Destination $pluginDirectory -Force
        $state.PluginHashes[$dll.Name] = (Get-FileHash -LiteralPath $dll.FullName).Hash
    }
    $ini = Get-Content -LiteralPath "$root/euroscope-plugin/src/config_dev.ini" -Raw
    $ini = $ini -replace '(?m)^baseurl\s*=.*$', "baseurl = ws://localhost:$($config.BackendPort)/euroscopeEvents"
    $ini = [regex]::Replace($ini, '(?m)^stands\s*=.*$', { "stands = $($config.StandsFile)" })
    $ini = [regex]::Replace($ini, '(?m)^airlines\s*=.*$', { "airlines = $($config.AirlinesFile)" })
    $ini | Set-Content -LiteralPath "$pluginDirectory/flightstrips_config.ini" -Encoding ascii
    $state.PluginHashes['flightstrips_config.ini'] = (Get-FileHash -LiteralPath "$pluginDirectory/flightstrips_config.ini").Hash
    if (!(Test-Path -LiteralPath "$pluginDirectory/flightstrips_user.ini") -and $config.PluginUserConfig -and (Test-Path -LiteralPath $config.PluginUserConfig)) {
        Copy-Item -LiteralPath $config.PluginUserConfig -Destination "$pluginDirectory/flightstrips_user.ini"
    }
    New-TestProfile
    Save-State
    Write-Host 'Starting isolated database and applying migrations ...'
    Invoke-Compose @('up','-d','--wait','--wait-timeout','90')
    Push-Location "$root/backend"
    $previousDsn = $env:DATABASE_CONNECTIONSTRING
    $previousDsnFile = $env:DATABASE_CONNECTIONSTRING_FILE
    try {
        $env:DATABASE_CONNECTIONSTRING_FILE = ''
        $env:DATABASE_CONNECTIONSTRING = "postgresql://fs:local-testing-only@localhost:$($config.DatabasePort)/fsdb?sslmode=disable"
        Invoke-Checked go @('run','./cmd/migrate','--migrations','./migrations')
        Invoke-Checked go @('build','-o',"$runtime/server.exe",'./cmd/server')
    } finally { $env:DATABASE_CONNECTIONSTRING=$previousDsn; $env:DATABASE_CONNECTIONSTRING_FILE=$previousDsnFile; Pop-Location }
    $pwsh = (Get-Command pwsh).Source
    $worker = Join-Path $PSScriptRoot 'manual-test-worker.ps1'
    Start-Owned backend $pwsh @('-NoProfile','-File',$worker,'-Root',$root,'-ConfigPath',$ConfigPath,'-Kind','backend','-RunId',$state.RunId) $root | Out-Null
    $health = Wait-Http "http://127.0.0.1:$($config.BackendPort)/healthz" backend
    Push-Location "$root/frontend"
    try {
        $lockHash = (Get-FileHash -LiteralPath 'package-lock.json').Hash
        $dependencyStamp = Join-Path $runtime 'frontend-lock.sha256'
        if (!(Test-Path -LiteralPath 'node_modules/vite/bin/vite.js') -or !(Test-Path -LiteralPath $dependencyStamp) -or (Get-Content -LiteralPath $dependencyStamp -Raw).Trim() -ne $lockHash) {
            Invoke-Checked npm.cmd @('ci','--no-audit','--no-fund')
            $lockHash | Set-Content -LiteralPath $dependencyStamp
        }
    } finally { Pop-Location }
    Start-Owned frontend $pwsh @('-NoProfile','-File',$worker,'-Root',$root,'-ConfigPath',$ConfigPath,'-Kind','frontend','-RunId',$state.RunId) $root | Out-Null
    $marker = Wait-Http "http://127.0.0.1:$($config.FrontendPort)/__manual-test" frontend
    if ($marker.runId -ne $state.RunId -or $marker.root -ne "$root/frontend".Replace('\','/')) { throw 'Frontend identity does not match this launch.' }
    if (!(Test-Path -LiteralPath $fsdRegistry)) { New-Item -Path $fsdRegistry -Force | Out-Null }
    if (!$state.ContainsKey('OriginalFsdScenario')) { $state.OriginalFsdScenario = (Get-ItemProperty -LiteralPath $fsdRegistry -Name Session0ScenarioFile -ErrorAction SilentlyContinue).Session0ScenarioFile }
    if (!$state.ContainsKey('OriginalFsdSameAsDefault')) { $state.OriginalFsdSameAsDefault = (Get-ItemProperty -LiteralPath $fsdRegistry -Name Session0SameAsDefault -ErrorAction SilentlyContinue).Session0SameAsDefault }
    Save-State
    Set-ItemProperty -LiteralPath $fsdRegistry -Name Session0ScenarioFile -Value $state.ScenarioPath
    Set-ItemProperty -LiteralPath $fsdRegistry -Name Session0SameAsDefault -Value '0'
    if (!(Test-ScenarioAgreement)) { throw 'FSD and EuroScope must reference the exact same unchanged scenario file before launch.' }
    Start-Owned fsd $state.TestFsdExe @() (Split-Path $state.TestFsdExe -Parent) -Visible | Out-Null
    # FSD may need its one-time licence dialog. Never treat process startup as readiness.
    $deadline = [DateTime]::UtcNow.AddSeconds(60)
    do { $listener = Get-NetTCPConnection -State Listen -LocalPort 6809 -ErrorAction SilentlyContinue; if (!$listener) { Start-Sleep -Milliseconds 500 } } while (!$listener -and [DateTime]::UtcNow -lt $deadline)
    if (!$listener -or @($listener.OwningProcess) -notcontains $state.Processes.fsd.Id) { throw 'The launched FSD does not own port 6809. Complete its licence dialog and inspect the FSD window.' }
    # Use the database clock so retained rows must receive a new EuroScope observation.
    $state.ObservationStartedAt = Invoke-TestDatabaseQuery 'SELECT statement_timestamp()::text;'
    Save-State
    Start-Owned euroscope $state.TestEuroScopeExe @($state.TestProfile) $state.TestDataDirectory -Visible | Out-Null
    if ($OpenBrowser) { Start-Process "http://localhost:$($config.FrontendPort)/app" }
    Write-Host "Services prepared. In EuroScope click CONNECT, then Connect (localhost, non-VATSIM, Start simulator session)."
    Write-Host "Frontend: http://localhost:$($config.FrontendPort)/app. Sign in normally; authenticate the plugin if prompted."
    Write-Host "Waiting up to $ConnectionTimeoutSeconds seconds for a controller and scenario traffic ..."
    $deadline = [DateTime]::UtcNow.AddSeconds($ConnectionTimeoutSeconds)
    do {
        $summary = Get-SessionSummary
        if ($summary.controllers -gt 0 -and $summary.strips -gt 0) { break }
        Start-Sleep -Seconds 2
    } while ([DateTime]::UtcNow -lt $deadline)
    Show-Status
    if (!($summary.controllers -gt 0 -and $summary.strips -gt 0)) { exit 2 }
} catch {
    Write-Error "Manual-test setup failed: $_`nLogs and partial startup state are preserved in $runtime. Use status or stop."
    exit 1
} finally { if ($lock) { $lock.Dispose() } }
