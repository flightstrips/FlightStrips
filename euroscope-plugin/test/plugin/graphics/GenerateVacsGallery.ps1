param(
    [string]$PreviewDirectory = (Join-Path $PSScriptRoot "../../../build/bin/native-preview"),
    [string]$OutputDirectory = (Join-Path $PSScriptRoot "../../../../docs/pr-assets/vacs")
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing
[System.IO.Directory]::CreateDirectory($OutputDirectory) | Out-Null

function Write-Gallery {
    param([string]$Filename, [string]$Title, [array]$States, [int]$Columns)
    $cellWidth = 204
    $cellHeight = 412
    $rows = [int][Math]::Ceiling($States.Count / $Columns)
    $image = New-Object System.Drawing.Bitmap ($Columns * $cellWidth), (80 + $rows * $cellHeight)
    $canvas = [System.Drawing.Graphics]::FromImage($image)
    $titleFont = New-Object System.Drawing.Font "Segoe UI", 16
    $labelFont = New-Object System.Drawing.Font "Segoe UI", 10
    $noteFont = New-Object System.Drawing.Font "Segoe UI", 8
    try {
        $canvas.Clear([System.Drawing.Color]::FromArgb(240, 242, 245))
        $canvas.DrawString($Title, $titleFont, [System.Drawing.Brushes]::Black, 16, 10)
        $canvas.DrawString("Production GDI+ renderer / EuroScope 9 pt / 1:1 pixels", $noteFont, [System.Drawing.Brushes]::DimGray, 16, 40)
        $canvas.DrawString("Fixture states, not live EuroScope screenshots", $noteFont, [System.Drawing.Brushes]::DimGray, 16, 56)
        for ($index = 0; $index -lt $States.Count; $index++) {
            $state = $States[$index]
            $left = 16 + ($index % $Columns) * $cellWidth
            $top = 80 + [int][Math]::Floor($index / $Columns) * $cellHeight
            $canvas.DrawString($state[1], $labelFont, [System.Drawing.Brushes]::Black, $left, $top)
            $canvas.DrawString("Expanded", $noteFont, [System.Drawing.Brushes]::DimGray, $left, ($top + 21))
            foreach ($mode in @("expanded", "mini")) {
                $source = [System.Drawing.Image]::FromFile((Join-Path $PreviewDirectory "state-$($state[0])-$mode.png"))
                try {
                    $offset = if ($mode -eq "expanded") { 39 } else { 356 }
                    $canvas.DrawImageUnscaled($source, $left, ($top + $offset))
                } finally {
                    $source.Dispose()
                }
            }
            $canvas.DrawString("Mini", $noteFont, [System.Drawing.Brushes]::DimGray, $left, ($top + 338))
        }
        $image.Save((Join-Path $OutputDirectory $Filename), [System.Drawing.Imaging.ImageFormat]::Png)
    } finally {
        $noteFont.Dispose()
        $labelFont.Dispose()
        $titleFont.Dispose()
        $canvas.Dispose()
        $image.Dispose()
    }
}

Write-Gallery "connection-states.png" "VACS connection and readiness" @(
    ,@("disabled", "Disabled")
    ,@("unavailable", "VACS unavailable")
    ,@("reconnecting", "Reconnecting")
    ,@("unauthenticated", "Unauthenticated")
    ,@("disconnected", "Position disconnected")
    ,@("ambiguous", "Ambiguous position")
    ,@("synchronizing", "Synchronizing")
    ,@("idle", "Idle / ready")
) 4
Write-Gallery "call-states.png" "VACS call states and full caller identity" @(
    ,@("incoming", "Incoming")
    ,@("incoming-queued", "Incoming / two waiting")
    ,@("outgoing", "Outgoing")
    ,@("connecting", "Connecting")
    ,@("connected", "Connected")
) 3
Write-Gallery "pending-states.png" "Pending commands and mini visibility" @(
    ,@("idle-pending", "Starting a call / pending")
    ,@("incoming-pending", "Answer or reject / pending")
    ,@("outgoing-pending", "Cancel / pending")
    ,@("connecting-pending", "End connecting / pending")
    ,@("connected-pending", "End connected / pending")
    ,@("mini-hidden", "Show in mini unchecked")
) 3
