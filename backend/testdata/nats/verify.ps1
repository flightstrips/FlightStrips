$ErrorActionPreference = 'Stop'
$compose = 'docker-compose.nats.yaml'
$project = 'flightstrips-nats-01'

function Run-GoTest([string]$name) {
    go test -count=1 -run $name ./internal/natsresources
    if ($LASTEXITCODE -ne 0) { throw "Go integration test $name failed" }
}

try {
    docker compose -f $compose -p $project up -d
    if ($LASTEXITCODE -ne 0) { throw 'Could not start NATS fixture' }
    $env:NATS_INTEGRATION = '1'
    Run-GoTest 'TestClusterResources|TestPrepareWriteProbe'
    docker compose -f $compose -p $project stop nats-3
    if ($LASTEXITCODE -ne 0) { throw 'Could not stop one NATS node' }
    $env:NATS_EXPECT_WRITE = '1'
    Run-GoTest '^TestDurableWrite$'
    docker compose -f $compose -p $project stop nats-2
    if ($LASTEXITCODE -ne 0) { throw 'Could not stop second NATS node' }
    $env:NATS_EXPECT_WRITE = '0'
    Run-GoTest '^TestDurableWrite$'
    docker compose -f $compose -p $project stop
    if ($LASTEXITCODE -ne 0) { throw 'Could not stop NATS fixture for restart' }
    docker compose -f $compose -p $project up -d
    if ($LASTEXITCODE -ne 0) { throw 'Could not restart NATS fixture' }
    Run-GoTest '^TestClusterResources$'
    $env:NATS_EXPECT_WRITE = '1'
    Run-GoTest '^TestDurableWrite$'
} finally {
    Remove-Item Env:NATS_INTEGRATION, Env:NATS_EXPECT_WRITE -ErrorAction SilentlyContinue
    docker compose -f $compose -p $project up -d | Out-Null
}
