# Local NATS resource fixture

The default [Windows development setup](../../Readme.md) runs three pinned
`nats:2.15.0` brokers and two real backends. Bootstrap is a separate administrator
operation. Runtime credentials verify resources and cannot repair drift.

From `backend`, `./testdata/nats/verify.ps1` uses the broker-only Compose project
`flightstrips-nats-01`, exercises repeated bootstrap, resource drift, replicated
writes with one/two brokers stopped and retained-resource restart. It leaves that
fixture running. Stop it with:

```powershell
docker compose -f docker-compose.nats.yaml -p flightstrips-nats-01 stop
```

`NATS_URLS` supplies comma-separated backend URLs. TLS/credentials use
`NATS_CREDENTIALS_FILE`, `NATS_CA_FILE`, `NATS_CLIENT_CERT_FILE`,
`NATS_CLIENT_KEY_FILE`; deadlines use `NATS_CONNECT_TIMEOUT` and
`NATS_REQUEST_TIMEOUT`. The committed fixture passwords are local-only.

For administrator bootstrap, set `NATS_URLS` to the three
`nats://bootstrap:bootstrap-local-only@127.0.0.1:4222` through `:4224` URLs and run
`go run ./cmd/nats-bootstrap`. Restore backend credentials before server startup.
`cmd/nats-projection` remains an isolated diagnostic process; set
`NATS_PROJECTION_LISTEN` to avoid real backend ports. The real server always starts
NATS and checks readiness; the former preparatory flag and SQL runtime are retired.

Set `NATS_INTEGRATION=1` for real integration tests, `NATS_TEST_PORT_BASE` for a
non-default fixture port and `NATS_FAULT_PROJECT` only for the exact **disposable**
Compose project the fault tests may stop/restart. Never fault a shared fixture.
