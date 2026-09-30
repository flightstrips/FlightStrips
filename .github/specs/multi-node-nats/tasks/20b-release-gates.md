# Task 20b — Ordinary release workflow cleanup

**Depends on:** completed 14–19 and their follow-ups. Can run alongside 20a.

**Operator decision:** the user will test on their own machine. This replaces the earlier Task 20b release-candidate framework in full. Build the application locally for qualification; ordinary build/test CI remains useful.

**Integration status:** the superseded framework merged as [PR #825](https://github.com/flightstrips/FlightStrips/pull/825). A follow-up in the existing 20b chat removes that framework and retains the migrator cleanup described below. Task 20b is accepted only after this corrective PR merges; prior candidate evidence does not satisfy the revised task.

**Outcome:** the existing ordinary release workflow supports the NATS-only application without a migrator dependency.

**Release boundary:** held under [Task 20](20-app-cutover.md). Implement the cleanup and open a draft PR into the integration base. Actual release and production cutover happen only after local testing and the operator's decision at Task 24.

## Work and fixed boundaries

- Remove `backend-migrate` build, image publication and infrastructure image-bump references from application release workflows. Remove any workflow dependency on that deleted job. Task 20c owns removal of the SQL runtime/migration executable; Task 21 owns the infrastructure stack/migrator validation cleanup.
- Preserve the current unified Release Please PR, component version files/tag scheme, normal backend/frontend image builds, plugin DLL/config attachment, documentation publication, existing release announcements and infrastructure image-bump **PR** behavior. Do not directly deploy a stack. Production continues using pinned component versions, never a floating image tag.
- Preserve existing component/development build identity. Do not add a tested-tree identity requirement or change frontend identity for this task.
- Remove the candidate build workflows, candidate image tags, staged release artifact machinery, candidate manifests, release Protobuf schemas, digest promotion, custom cutover receipts and manual publication framework introduced by #825. Remove obsolete candidate samples/evidence or label retained history as superseded. Restore ordinary release workflows and version identity while keeping migrator cleanup. Do not add a staging environment. Operational Protobuf contracts remain unchanged.
- Own the minimal release workflow cleanup and any necessary checks. Do not modify runtime/API assembly owned by 20a, Compose/default-entrypoint/SQL retirement owned by 20c, ALB, or unrelated release behavior. Dockerfiles change only if necessary to remove a migrator build reference, not to implement a new release pipeline.
- Do not run publication/notification/deployment workflows while implementing or testing. Existing ordinary release automation runs only when the operator later merges the actual release PR; local testing requires no published artifact.

## Done when

- Workflow inspection and applicable local validation show no remaining application build/publish/image-bump path for `backend-migrate`, no broken dependency on its former job and no remaining candidate/promotion/staging framework from #825.
- Normal backend/frontend/plugin/docs releases and infrastructure image-bump PR behavior are preserved. Explain the precise workflow changes and validation in the PR; no external publication is needed to demonstrate them.
- Open and attach a draft PR into `codex/multi-node-nats-base`. Do not merge, publish, announce or deploy. The user tests the integrated local application after 20c; normal release readiness is recorded there and in Tasks 22–24.

**Starting points:** `.github/workflows/release-please.yml`, other workflow references to `backend-migrate`, and the existing version/image-bump configuration.
