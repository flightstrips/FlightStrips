#requires -Version 7.0
param([string]$DatabaseContainer)
$ErrorActionPreference = 'Stop'
$launcher = Join-Path (Split-Path $PSScriptRoot -Parent) 'manual-test.ps1'
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($launcher, [ref]$null, [ref]$parseErrors)
if ($parseErrors) { throw ($parseErrors | Out-String) }
# Import functions without running the launcher or operating any real test services.
foreach ($name in 'Save-State','New-LaunchState','Stop-Owned','Invoke-TestDatabaseQuery','Get-SessionSummaryQuery','Get-SessionSummary','Resolve-TestScenario','ConvertTo-SnapshotReferences','Get-SnapshotWritePath','New-TestProfile','Test-ScenarioAgreement') {
    $definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
    . ([scriptblock]::Create($definition.Extent.Text))
}
function Assert($Condition, [string]$Message) { if (!$Condition) { throw $Message } }
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "flightstrips-launcher-tests-$([Guid]::NewGuid())"
New-Item -ItemType Directory -Path $testRoot | Out-Null
$root = $testRoot
$runtime = $testRoot
$ConfigPath = Join-Path $testRoot 'config.json'
$statePath = Join-Path $testRoot 'state.json'
'{}' | Set-Content -LiteralPath $ConfigPath
$config = @{ Callsign='EKCH_A_TWR' }
$selectedScenario = @{ Name='tower'; File='scenario.txt'; ArrivalRunways=@('22L'); DepartureRunways=@('22R') }
$events = [Collections.Generic.List[string]]::new()
$backendOwned = $false
$restoreFailure = ''
function Get-SourceStamp { 'fixture' }
function Get-OwnedProcess($Entry) { if ($backendOwned) { return @{ Id=1 } }; return $null }
function Invoke-Compose($Arguments) { $events.Add("compose $($Arguments -join ' ')") }
function Set-ItemProperty { param($LiteralPath,$Name,$Value) $events.Add("restore $Name=$Value") }
function Remove-ItemProperty { param($LiteralPath,$Name,$ErrorAction) $events.Add("remove $Name") }
function Invoke-Checked($Exe,$Arguments) {
    $events.Add("$Exe $($Arguments -join ' ')")
    if ($restoreFailure -and $Arguments[1] -eq $restoreFailure) { throw 'Simulated container restoration failure' }
}

# Repeated early failures must retain cleanup obligations across state-file round trips.
$state = @{ Processes=@{}; SuspendedContainers=@('suspended-service'); OriginalFsdScenario='original.txt'; OriginalFsdSameAsDefault='1'; ObservationStartedAt='2000-01-01T00:00:00Z' }
foreach ($attempt in 1..2) {
    $state = New-LaunchState $selectedScenario
    Save-State
    $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json -AsHashtable
    Assert ($state.SuspendedContainers -contains 'suspended-service') 'Retry lost a suspended container'
    Assert ($state.OriginalFsdScenario -eq 'original.txt' -and $state.OriginalFsdSameAsDefault -eq '1') 'Retry lost original FSD settings'
    Assert (!$state.ContainsKey('ObservationStartedAt')) 'Retry reused an earlier observation boundary'
}
Stop-Owned
Assert ($events -contains 'docker start suspended-service') 'stop did not restore the container after retries'
Assert ($events -contains 'restore Session0ScenarioFile=original.txt') 'stop did not restore original FSD scenario'
Assert ($state.SuspendedContainers.Count -eq 0) 'Successful restoration was not cleared'

# A partial restoration failure must retain only the outstanding restoration.
$state = @{ Processes=@{}; SuspendedContainers=@('first','second') }
$restoreFailure = 'second'
$failed = $false
try { Stop-Owned } catch { $failed = $true }
Assert $failed 'Expected a simulated restoration failure'
$state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json -AsHashtable
Assert ($state.SuspendedContainers.Count -eq 1 -and $state.SuspendedContainers[0] -eq 'second') 'Partial restoration discarded outstanding cleanup'
$restoreFailure = ''
Stop-Owned
Assert ($state.SuspendedContainers.Count -eq 0) 'Retry did not clear completed restoration'

# Old launch files cannot use retained traffic without a fresh observation boundary.
$state = @{ Processes=@{ backend=@{} }; ControllerCallsign='EKCH_A_TWR' }
$backendOwned = $true
Assert ($null -eq (Get-SessionSummary)) 'Missing observation boundary must fail closed'
Write-Host 'Passed cleanup retry, partial restoration, and legacy-state readiness regressions.'

# Generate the complete copied profile with accented installation, profile,
# sector, and scenario paths. The fixture executables are text and never run.
$sourceData = Join-Path $testRoot 'fixture/données'
$sourceApp = Join-Path $testRoot 'fixture/application-é'
foreach ($directory in @("$sourceData/EKDK/Plugins/FlightStrips",$sourceApp,"$runtime/plugin")) {
    New-Item -ItemType Directory -Path $directory -Force | Out-Null
}
foreach ($name in 'EuroScope.exe','EuroScopeFsdServer.exe') { 'fixture only' | Set-Content -LiteralPath (Join-Path $sourceApp $name) }
$sourceProfile = Join-Path $sourceData 'profilé.prf'
"Settings`tsector`t\EKDK\secteur-é.sct`nSettings`tnote`tRéférence ø`nPlugins`tPlugin0`t\EKDK\Plugins\FlightStrips\FlightStripsPlugin.dll" | Set-Content -LiteralPath $sourceProfile -Encoding utf8NoBOM
'fixture sector' | Set-Content -LiteralPath "$sourceData/EKDK/secteur-é.sct"
"stands = fixture`nairlines = fixture" | Set-Content -LiteralPath "$runtime/plugin/flightstrips_config.ini"
$scenarioFile = Join-Path $testRoot 'fixture/scénario-Rwy22.txt'
'PSEUDOPILOT:ALL' | Set-Content -LiteralPath $scenarioFile
$config = @{ EuroScopeDataDirectory=$sourceData; EuroScopeExe="$sourceApp/EuroScope.exe"; FsdExe="$sourceApp/EuroScopeFsdServer.exe"; BaseProfile=$sourceProfile; StandsFile="$sourceData/EKDK/stands-é.txt"; AirlinesFile="$sourceData/EKDK/airlines-é.txt"; Callsign='EKCH_A_TWR'; Facility=4 }
$selectedScenario = Resolve-TestScenario $scenarioFile
$state = @{ RunId='fixture'; Processes=@{}; PluginHashes=@{}; ScenarioPath=$selectedScenario.File; ArrivalRunways=$selectedScenario.ArrivalRunways; DepartureRunways=$selectedScenario.DepartureRunways }
New-TestProfile
$profileText = Get-Content -LiteralPath $state.TestProfile -Raw
$configuredScenario = (($profileText -split '\r?\n' | Where-Object { $_ -match '^LastSession\tscenario\t' }) -split "`t",3)[2]
Assert ($configuredScenario -ceq $scenarioFile) 'Generated profile corrupted the non-ASCII scenario path'
Assert ($profileText.Contains("Settings`tnote`tRéférence ø")) 'Generated profile corrupted existing non-ASCII settings'
$configuredSector = (($profileText -split '\r?\n' | Where-Object { $_ -match '^Settings\tsector\t' }) -split "`t",3)[2]
Assert (Test-Path -LiteralPath $configuredSector) 'Generated non-ASCII sector path does not resolve'
$fsdRegistry = 'fixture-only'
$fakeFsdSettings = [pscustomobject]@{ Session0ScenarioFile=$scenarioFile; Session0SameAsDefault='0' }
function Get-ItemProperty { param($LiteralPath,$ErrorAction) $fakeFsdSettings }
Assert (Test-ScenarioAgreement) 'Non-ASCII paths failed the shared FSD/EuroScope file check'
Write-Host 'Passed profile generation and same-file verification with non-ASCII paths and existing settings.'

# Equivalent Windows paths must rebase even with different capitalization or
# separator styles; the source runway file must never be rewritten.
$sourceRunway = Join-Path $sourceData 'EKDK/secteur-é.rwy'
'original runway selections' | Set-Content -LiteralPath $sourceRunway
$originalRunwayHash = (Get-FileHash -LiteralPath $sourceRunway).Hash
$mixedCaseSector = $sourceData.ToUpperInvariant().Replace('\','/') + '/EKDK/secteur-é.sct'
"Settings`tsector`t$mixedCaseSector`nPlugins`tPlugin0`t\EKDK\Plugins\FlightStrips\FlightStripsPlugin.dll" | Set-Content -LiteralPath $sourceProfile -Encoding utf8NoBOM
New-TestProfile
Assert ((Get-FileHash -LiteralPath $sourceRunway).Hash -eq $originalRunwayHash) 'Case-variant sector reference overwrote the source runway file'
$profileText = Get-Content -LiteralPath $state.TestProfile -Raw
$configuredSector = (($profileText -split '\r?\n' | Where-Object { $_ -match '^Settings\tsector\t' }) -split "`t",3)[2]
Assert ([IO.Path]::GetFullPath($configuredSector).StartsWith($state.TestDataDirectory + '\', [StringComparison]::OrdinalIgnoreCase)) 'Case-variant sector path was not rebased inside the snapshot'
Write-Host 'Passed case-insensitive Windows path rebasing and original runway preservation.'

# External sector references and relative traversal must fail before a write.
$externalSector = Join-Path $testRoot 'fixture/external.sct'
$externalRunway = [IO.Path]::ChangeExtension($externalSector,'.rwy')
'external sector' | Set-Content -LiteralPath $externalSector
'external runway selections' | Set-Content -LiteralPath $externalRunway
$externalHash = (Get-FileHash -LiteralPath $externalRunway).Hash
"Settings`tsector`t$externalSector`nPlugins`tPlugin0`t\EKDK\Plugins\FlightStrips\FlightStripsPlugin.dll" | Set-Content -LiteralPath $sourceProfile -Encoding utf8NoBOM
$externalRejected = $false
try { New-TestProfile } catch {
    if ($_ -notmatch 'outside the copied snapshot') { throw }
    $externalRejected = $true
}
Assert $externalRejected 'External sector reference was accepted for a runway write'
Assert ((Get-FileHash -LiteralPath $externalRunway).Hash -eq $externalHash) 'External runway file was modified'
$traversalRejected = $false
try { Get-SnapshotWritePath '../outside.rwy' $state.TestDataDirectory | Out-Null } catch {
    if ($_ -notmatch 'outside the copied snapshot') { throw }
    $traversalRejected = $true
}
Assert $traversalRejected 'Relative traversal escaped the snapshot write guard'
$siblingPath = $sourceData + '-external/sector.sct'
Assert ((ConvertTo-SnapshotReferences $siblingPath $sourceData $state.TestDataDirectory) -ceq $siblingPath) 'Rebasing matched a similarly named external directory'
$junction = Join-Path $state.TestDataDirectory 'linked-sector'
New-Item -ItemType Junction -Path $junction -Target (Split-Path $externalSector -Parent) | Out-Null
try {
    $junctionRejected = $false
    try { Get-SnapshotWritePath (Join-Path $junction 'external.rwy') $state.TestDataDirectory | Out-Null } catch {
        if ($_ -notmatch 'reparse point') { throw }
        $junctionRejected = $true
    }
    Assert $junctionRejected 'Directory junction escaped the snapshot write guard'
} finally { Remove-Item -LiteralPath $junction -Force }
Assert ((Get-FileHash -LiteralPath $externalRunway).Hash -eq $externalHash) 'Write-path checks changed the external runway file'
Write-Host 'Passed external-sector rejection, traversal/junction protection, and directory-boundary checks.'

if ($DatabaseContainer) {
    # Exercise the actual SQL in one transaction with temporary tables shadowing
    # application tables. Existing data and running services are not changed.
    $query = (Get-SessionSummaryQuery).Trim().TrimEnd(';')
    $sql = @'
BEGIN;
SET LOCAL TIME ZONE 'Pacific/Auckland';
CREATE TEMP TABLE sessions (id integer, airport text, name text);
CREATE TEMP TABLE controllers (session integer, callsign text, last_seen_euroscope timestamp);
CREATE TEMP TABLE strips (session integer, euroscope_seen_at timestamptz);
CREATE TEMP TABLE readiness_results (name text, summary json);
INSERT INTO sessions VALUES (1,'EKCH','SWEATBOX'),(2,'EKCH','PLAYBACK'),(3,'EKBI','SWEATBOX'),(4,'EKCH','LIVE');
INSERT INTO controllers VALUES
 (1,'EKCH_A_TWR',statement_timestamp() AT TIME ZONE 'UTC'),
 (1,'OTHER_TWR',statement_timestamp() AT TIME ZONE 'UTC'),
 (2,'EKCH_A_TWR',statement_timestamp() AT TIME ZONE 'UTC'),
 (3,'EKCH_A_TWR',statement_timestamp() AT TIME ZONE 'UTC'),
 (4,'EKCH_A_TWR',statement_timestamp() AT TIME ZONE 'UTC');
INSERT INTO strips VALUES (1,statement_timestamp()-interval '2 minutes'),(1,NULL),
 (2,statement_timestamp()),(3,statement_timestamp()),(4,statement_timestamp());
SELECT (statement_timestamp()-interval '1 minute')::text AS started_at \gset
'@
    $sql += "`nINSERT INTO readiness_results SELECT 'retained-or-other-session', summary FROM ($query) q(summary);`n"
    $sql += "INSERT INTO strips VALUES (1,statement_timestamp());`n"
    $sql += "INSERT INTO readiness_results SELECT 'fresh-current-session', summary FROM ($query) q(summary);`n"
    $sql += "UPDATE controllers SET last_seen_euroscope=(statement_timestamp() AT TIME ZONE 'UTC')-interval '45 seconds' WHERE session=1 AND callsign='EKCH_A_TWR';`n"
    $sql += "INSERT INTO readiness_results SELECT 'stale-test-controller', summary FROM ($query) q(summary);`n"
    $sql += "UPDATE controllers SET last_seen_euroscope=(statement_timestamp() AT TIME ZONE 'UTC')-interval '2 minutes' WHERE session=1 AND callsign='EKCH_A_TWR';`n"
    $sql += "INSERT INTO readiness_results SELECT 'prior-launch-controller', summary FROM ($query) q(summary);`n"
    $sql += "SELECT json_agg(json_build_object('name',name,'summary',summary)) FROM readiness_results;`nROLLBACK;"
    $result = $sql | & docker exec -i $DatabaseContainer psql -X -qAt -v ON_ERROR_STOP=1 -v callsign=EKCH_A_TWR -U fs -d fsdb
    if ($LASTEXITCODE -ne 0) { throw 'Database regression query failed' }
    $cases = $result | ConvertFrom-Json
    $retained = $cases | Where-Object name -eq 'retained-or-other-session'
    Assert ($retained.summary.controllers -eq 1 -and $retained.summary.strips -eq 0) 'Retained or different-session traffic satisfied readiness'
    $fresh = $cases | Where-Object name -eq 'fresh-current-session'
    Assert ($fresh.summary.controllers -eq 1 -and $fresh.summary.strips -eq 1 -and $fresh.summary.sessions.Count -eq 1) 'Fresh current-session traffic did not satisfy readiness'
    foreach ($name in 'stale-test-controller','prior-launch-controller') {
        $case = $cases | Where-Object name -eq $name
        Assert ($case.summary.controllers -eq 0 -and $case.summary.strips -eq 0) "$name satisfied readiness through another controller"
    }
    Write-Host 'Passed PostgreSQL regressions for retained traffic, other sessions/airports, fresh traffic, and stale controllers (non-UTC database timezone).'
} else { Write-Host 'SQL integration checks skipped; pass -DatabaseContainer with an existing PostgreSQL container to run them.' }
$resolvedTestRoot = (Resolve-Path -LiteralPath $testRoot).ProviderPath
$tempRoot = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\'
if (!$resolvedTestRoot.StartsWith($tempRoot, [StringComparison]::OrdinalIgnoreCase) -or
    (Split-Path $resolvedTestRoot -Leaf) -notmatch '^flightstrips-launcher-tests-[0-9a-f-]{36}$' -or
    ((Get-Item -LiteralPath $resolvedTestRoot).Attributes -band [IO.FileAttributes]::ReparsePoint)) {
    throw 'Refusing to remove a test fixture outside the owned temporary directory.'
}
Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force
