# Local NATS resource fixture

From `backend/`, run `./testdata/nats/verify.ps1` in PowerShell. It starts three
`nats:2.15.0` servers with separate persistent volumes, waits for JetStream
quorum, verifies resource bootstrap twice and configuration drift, tests durable
writes with one and two nodes stopped, then restarts all three nodes and verifies
the retained resources again. It leaves the fixture running. To stop it while
retaining data, run:

```powershell
docker compose -f docker-compose.nats.yaml -p flightstrips-nats-01 stop
```

The bootstrap command uses a separate local credential:

```powershell
$env:NATS_URLS='nats://bootstrap:bootstrap-local-only@127.0.0.1:4222,nats://bootstrap:bootstrap-local-only@127.0.0.1:4223,nats://bootstrap:bootstrap-local-only@127.0.0.1:4224'
go run ./cmd/nats-bootstrap
```

To exercise the isolated projection and its readiness endpoint without
changing the PostgreSQL application, use the backend fixture credential:

```powershell
$env:NATS_URLS='nats://backend:backend-local-only@127.0.0.1:4222,nats://backend:backend-local-only@127.0.0.1:4223,nats://backend:backend-local-only@127.0.0.1:4224'
go run ./cmd/nats-projection
```

The isolated process serves `/readyz` on `127.0.0.1:8091` by default. It
requires current JetStream metadata, a write quorum, caught-up state replay,
and initialized position/presence watchers. Set `NATS_PROJECTION_LISTEN` to
choose a different test address. The current production server does not start
this process or depend on NATS readiness.

To exercise owner failover and command routing against this fixture, run
`$env:NATS_INTEGRATION='1'; go test ./internal/cluster -run '^TestOwnerFailoverAndLostCoreReply$' -count=1`.
The test uses three independent backend connections and projections. For its
NATS-node fault phase, set `NATS_FAULT_PROJECT` to the exact Compose project name
of a disposable fixture; the test stops and restarts nodes 3 and 2. Set
`NATS_TEST_PORT_BASE` if that fixture maps its three client ports somewhere other
than 4222–4224. Do not run the fault phase against a shared fixture.

To check backend startup against the fixture, set `NATS_VERIFY_RESOURCES=true`
and use `nats://backend:backend-local-only@127.0.0.1:4222` in `NATS_URLS`.
This preparatory flag is off by default; normal production startup still uses
PostgreSQL and makes no NATS connection. The fixture passwords are for local
tests only. Production credentials and TLS are configured with
`NATS_CREDENTIALS_FILE`, `NATS_CA_FILE`, `NATS_CLIENT_CERT_FILE`, and
`NATS_CLIENT_KEY_FILE`; connection and API timeouts use
`NATS_CONNECT_TIMEOUT` and `NATS_REQUEST_TIMEOUT`.
