# Backend testing support

The SQL E2E/replay/testcontainers runner and its JSON socket fixtures are retired.
SAT scenario/replay tools remain disabled. Pure shared-policy tests use `go test ./...`.

`natscluster` provides isolated three-broker helpers for real NATS integration
checks. `recorder` remains a diagnostic interface used by provider policy; it is
not a runtime storage path or a replay service. See [local development](../../Readme.md)
and [Task 20c evidence](../../../.github/specs/multi-node-nats/tasks/20c-sql-retirement-evidence.md).
