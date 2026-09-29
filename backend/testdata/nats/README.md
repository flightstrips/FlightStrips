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

To check backend startup against the fixture, set `NATS_VERIFY_RESOURCES=true`
and use `nats://backend:backend-local-only@127.0.0.1:4222` in `NATS_URLS`.
This preparatory flag is off by default; normal production startup still uses
PostgreSQL and makes no NATS connection. The fixture passwords are for local
tests only. Production credentials and TLS are configured with
`NATS_CREDENTIALS_FILE`, `NATS_CA_FILE`, `NATS_CLIENT_CERT_FILE`, and
`NATS_CLIENT_KEY_FILE`; connection and API timeouts use
`NATS_CONNECT_TIMEOUT` and `NATS_REQUEST_TIMEOUT`.
