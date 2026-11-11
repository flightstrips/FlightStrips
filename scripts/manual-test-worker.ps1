#requires -Version 7.0
param([Parameter(Mandatory)][string]$Root, [Parameter(Mandatory)][string]$ConfigPath,
      [Parameter(Mandatory)][ValidateSet('backend','frontend')][string]$Kind,
      [Parameter(Mandatory)][string]$RunId)
$ErrorActionPreference = 'Stop'
$config = Get-Content -LiteralPath $ConfigPath -Raw | ConvertFrom-Json -AsHashtable
if ($config.NodeDirectory) { $env:PATH = "$($config.NodeDirectory);$env:PATH" }
try {
    if ($Kind -eq 'backend') {
        Set-Location (Join-Path $Root 'backend')
        foreach ($entry in $config.BackendEnvironment.GetEnumerator()) { [Environment]::SetEnvironmentVariable($entry.Key, [string]$entry.Value, 'Process') }
        $env:ENVIRONMENT = 'development'
        $env:ENV = 'development'
        $env:TEST_MODE = 'false'
        $env:RECORD_MODE = 'false'
        $env:DATABASE_CONNECTIONSTRING_FILE = ''
        $env:DATABASE_CONNECTIONSTRING = "postgresql://fs:local-testing-only@localhost:$($config.DatabasePort)/fsdb?sslmode=disable"
        $env:AIRPORT_REPOSITORY_ROOT = $Root
        $env:AIRPORT_CONFIG_FILES = Join-Path $Root 'backend/config/airports/ekch.airport.yaml'
        $env:SERVER_ADDR = "127.0.0.1:$($config.BackendPort)"
        $env:ENABLE_VATSIM = 'false'
        $env:ENABLE_VATSIM_TRANSCEIVERS = 'false'
        $env:OTEL_EXPORTER_OTLP_ENDPOINT = ''
        $testData = Join-Path $Root '.manual-test/euroscope/current/data'
        $env:GRPLUGIN_ICAO_AIRCRAFT_JSON = [IO.Path]::GetFullPath($config.AircraftFile).Replace([IO.Path]::GetFullPath($config.EuroScopeDataDirectory).TrimEnd('\'),$testData)
        & (Join-Path $Root '.manual-test/server.exe')
    } else {
        Set-Location (Join-Path $Root 'frontend')
        $env:FLIGHTSTRIPS_MANUAL_TEST_RUN = $RunId
        $env:FLIGHTSTRIPS_MANUAL_TEST_API = "http://localhost:$($config.BackendPort)"
        & node 'node_modules/vite/bin/vite.js' --host 127.0.0.1 --port $config.FrontendPort --strictPort
    }
    exit $LASTEXITCODE
} catch { Write-Error $_; exit 1 }
