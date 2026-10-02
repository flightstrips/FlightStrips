# Task 21a — JetStream encryption-key rotation qualification

**Depends on:** prepared infrastructure PR #29 and Task 22's encrypted restore evidence (#829). Can run alongside Task 23. Follow up in the existing infrastructure chat/branch; PR #29 stays draft and unmerged.

**Outcome:** the failed rotation probe is explained and replaced by a demonstrated safe rotation and recovery procedure. The operations contract requires tested key rotation; this unresolved check blocks Task 24.

## Contracts and work

- Reproduce and diagnose `docs/task21-rotation-failure.json` from infrastructure PR #29 on disposable encrypted three-node stores. Consult pinned server source and official NATS documentation; distinguish invalid configuration/sequence from a server defect. Keep the failure evidence with an explicit superseding result.
- Qualify the documented NATS-supported procedure on the pinned 2.15.0 version, including original-key encrypted backup/restore, rotation to a new versioned key, full broker/backend shutdown/restart, and subsequent backup/restore using the correct retained key versions. Preserve accepted records, sequences, resource metadata, objects and operational outcomes. Do not declare successful boot alone a pass.
- Fix any sequencing/configuration/tooling defect in the infrastructure PR and document exact key/secret/config transitions, downtime/admission requirements, verification and recovery. Keys remain confidential and old keys stay available until corresponding stores/backups expire. Never overwrite the sole working backup/store or dispose of the original key before verification.
- Use existing reviewed NATS backup/restore primitives and application typed storage contracts. Do not introduce an opaque application export format, weaken encryption/TLS, silently change the pinned version, or substitute a failed probe with a claimed pass. If the pin cannot support the required procedure, record the source-supported diagnosis and propose the specific corrected pin/procedure with affected plan/test updates in a reviewable PR; production remains blocked until qualified. A changed broker pin/config invalidates affected Task 22/23 results and must be reported for reruns.
- Every fault/rotation/restore targets owned disposable processes/containers with separate volumes, ports, test keys and verified labels/PIDs. Leave live/default development and production resources untouched. No release candidates, staging requirement, deployment, announcement or PR merge.

## Done when

- A repeatable local rotation test verifies before/after state and recovery, plus failed/interrupted rotation recovery using retained original keys/backup. Source/version/configuration, commands and checksums are recorded without secrets.
- PR #29 and its README clearly identify the passing procedure, superseded failed probe and remaining physical-host/operator provisioning checks. Rotation has an honest pass/fail result; unresolved data authentication/corruption errors still block Task 24.
- Push the reviewed changes and attach the updated draft PR. Do not merge or deploy it. Local qualification is not production-host acceptance.
