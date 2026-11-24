# A keyboard-driven stand-in for GET /api/gsx/stand, for flying the GSX handler
# without a controller.
#
#     powershell -ExecutionPolicy Bypass -File gsx-client\mock\api-tui.ps1
#
# Then point the deployed handler at it:
#
#     gsx-client/mock/deploy.sh http://localhost:8787
#
# TEST SETUP: the aircraft must be parked at Gate D2 at EKCH, with the
# EKCH-Simnord-Sonnich GSX profile active. The options below are D2's pushback
# routes; the real server translates a controller's point into the routes of the
# pilot's own stand, and this mock does not know which stand you are on. Parked
# anywhere else, the handler will answer "no route to it from this stand".

param([int]$Port = 8787)

$TestStand = "Gate D2"
$TestAirport = "EKCH"

# Key -> the release point a controller would assign. D2 has a route to each in
# both EKCH Simnord profiles (Sonnich calls them "S1 Face N", RW Profiles
# "Release Point S1 (NORTH)"). Each is sent as the real server sends it to an
# unmapped scenery: the bare point and where it is, so the handler has to find
# the route by position - the path every profile relies on.
$Points = [ordered]@{
    "1" = "S1"
    "2" = "S2"
    "3" = "R1"
    "4" = "S3"
}

# Positions and the match radius, from the server's own config.
$ConfigPath = Join-Path $PSScriptRoot "..\..\backend\config\ekch\gsx_sceneries.json"
$Config = Get-Content -Raw -Path $ConfigPath | ConvertFrom-Json

$state = @{ Route = $null; Revision = "none"; Since = Get-Date }
$requests = New-Object System.Collections.Generic.List[string]
$lastAirport = $null

function Set-Assignment($route) {
    $state.Route = $route
    $state.Revision = if ($route) { "push:" + $route } else { "none" }
    $state.Since = Get-Date
}

function ConvertTo-JsonString([string]$value) {
    # One string as a JSON literal, quotes and backslashes escaped.
    return ConvertTo-Json -InputObject $value -Compress
}

function Get-Body {
    # Assembled by hand so an empty assignment is "null" rather than a missing
    # field. Each string still goes through the serializer.
    $point = "null"
    $target = "null"
    if ($state.Route) {
        $point = ConvertTo-JsonString $state.Route
        $spot = $Config.release_points.($state.Route)
        if ($spot) {
            $avoid = @($Config.release_points.PSObject.Properties |
                Where-Object { $_.Name -ne $state.Route } |
                ForEach-Object { $_.Value })
            $target = ConvertTo-Json -Compress -Depth 4 -InputObject @{
                lat = $spot.lat
                lon = $spot.lon
                heading = $null
                radius_m = $Config.release_point_radius_m
                avoid = $avoid
            }
        }
    }
    return '{"stand":null,"pushback":null,"point":' + $point + ',"target":' + $target +
        ',"revision":' + (ConvertTo-JsonString $state.Revision) + '}'
}

function Show-Screen {
    Clear-Host
    Write-Host "GSX stand feed - mock" -ForegroundColor Cyan
    Write-Host "http://localhost:$Port/api/gsx/stand"
    Write-Host ""
    Write-Host "  Test setup: park at $TestStand, $TestAirport (EKCH-Simnord-Sonnich profile)" -ForegroundColor Yellow
    if ($lastAirport -and $lastAirport -ne $TestAirport) {
        Write-Host "  The handler is polling from $lastAirport, not $TestAirport" -ForegroundColor Red
    }
    Write-Host ""
    if ($state.Route) {
        Write-Host ("  Pushback  {0}" -f $state.Route) -ForegroundColor Green
    } else {
        Write-Host "  Pushback  none" -ForegroundColor DarkGray
    }
    Write-Host ("  Since     {0:HH:mm:ss}" -f $state.Since)
    Write-Host ""
    foreach ($key in $Points.Keys) {
        Write-Host ("  [{0}] assign {1}" -f $key, $Points[$key])
    }
    Write-Host "  [R] reset (nothing assigned)"
    Write-Host "  [Q] quit"
    Write-Host ""
    Write-Host "Recent requests" -ForegroundColor Cyan
    if ($requests.Count -eq 0) {
        Write-Host "  none yet - is API_BASE in the deployed handler pointing here?" -ForegroundColor DarkGray
    }
    foreach ($line in $requests) { Write-Host "  $line" }
}

$listener = New-Object System.Net.HttpListener
$listener.Prefixes.Add("http://localhost:$Port/")
try {
    $listener.Start()
} catch {
    Write-Host "Could not listen on port $Port - is another mock still running? $_" -ForegroundColor Red
    exit 1
}

Show-Screen
$pending = $listener.GetContextAsync()
try {
    while ($true) {
        $dirty = $false

        if ($pending.Wait(100)) {
            $context = $pending.Result
            $pending = $listener.GetContextAsync()

            # Collapse doubled slashes: an API_BASE with a trailing slash asks
            # for //api/gsx/stand, and a 404 there looks like "nothing assigned".
            $path = $context.Request.Url.AbsolutePath -replace "/{2,}", "/"
            $response = $context.Response
            if ($path -eq "/api/gsx/stand") {
                $body = Get-Body
                $status = 200
                $query = $context.Request.QueryString
                $lastAirport = $query["icao"]
                $shown = if ($state.Route) { $state.Route } else { "none" }
                $line = "{0:HH:mm:ss}  {1} at {2} -> {3}" -f (Get-Date), $query["callsign"], $query["icao"], $shown
            } else {
                $body = '{"error":"try /api/gsx/stand"}'
                $status = 404
                $line = "{0:HH:mm:ss}  404 {1}" -f (Get-Date), $path
            }
            # No ETag, on purpose: a 304 would hide a change made here.
            $bytes = [System.Text.Encoding]::UTF8.GetBytes($body)
            $response.StatusCode = $status
            $response.ContentType = "application/json"
            $response.Headers.Add("Cache-Control", "no-store")
            $response.ContentLength64 = $bytes.Length
            $response.OutputStream.Write($bytes, 0, $bytes.Length)
            $response.Close()

            $requests.Insert(0, $line)
            while ($requests.Count -gt 10) { $requests.RemoveAt($requests.Count - 1) }
            $dirty = $true
        }

        while ([Console]::KeyAvailable) {
            $key = [Console]::ReadKey($true).KeyChar.ToString().ToUpper()
            if ($Points.Contains($key)) {
                Set-Assignment $Points[$key]
                $dirty = $true
            } elseif ($key -eq "R") {
                Set-Assignment $null
                $dirty = $true
            } elseif ($key -eq "Q") {
                return
            }
        }

        if ($dirty) { Show-Screen }
    }
} finally {
    $listener.Stop()
    $listener.Close()
}
