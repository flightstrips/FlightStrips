# Task 20b — Immutable release gates

**Depends on:** completed 14–19 and their follow-ups. Can run alongside 20a.

**Outcome:** CI builds a reviewable candidate, records its exact artifacts and source tree, promotes identical images without rebuilding, and gates incompatible client publication on manual cutover.

**Release boundary:** held under [Task 20](20-app-cutover.md). Implement and test the workflows; do not actually publish releases/tags, dispatch cutover, notify users or deploy infrastructure.

## Candidate artifact contract

Keep the unified Release Please PR and existing component version files/tag scheme. Build the backend, frontend and plugin from its verified head tree. Immutable candidate tags include component version, Git tree and workflow run/attempt; a changed head cannot reuse a previous manifest. Repository-owned artifact metadata is deterministic binary Protobuf; optional textproto is review output only. External GitHub/OCI JSON is parsed at the boundary and is not embedded in metadata.

Add `proto/release/v1/candidate.proto`, package `flightstrips.release.v1`, with these exact fields. This is a release artifact schema, separate from cluster operational storage; generate the bindings required by the release helper without modifying the operational protocol revision.

| Message | Fields (number: type name) |
| --- | --- |
| `CandidateManifest` | 1: `uint32 schema_revision`; 2: `string repository`; 3: `uint64 release_pr_number`; 4: `string release_head_sha`; 5: `string release_tree_sha`; 6: `ComponentVersions versions`; 7: `OciArtifact backend`; 8: `OciArtifact frontend`; 9: `PluginArtifact plugin`; 10: `uint32 protocol_revision`; 11: `google.protobuf.Timestamp created_at`; 12: `uint64 workflow_run_id`; 13: `uint32 workflow_run_attempt` |
| `ComponentVersions` | 1: `string backend`; 2: `string frontend`; 3: `string plugin`; 4: `string docs` |
| `OciArtifact` | 1: `string repository`; 2: `string digest`; 3: `string candidate_tag` |
| `PluginArtifact` | 1: `string artifact_name`; 2: `uint64 artifact_id`; 3: `repeated ArtifactFile files` |
| `ArtifactFile` | 1: `string path`; 2: `string sha256`; 3: `uint64 size_bytes` |
| `CutoverReceipt` | 1: `string release_tree_sha`; 2: `string plugin_release_tag`; 3: `string dll_sha256`; 4: `string frontend_release_tag`; 5: `uint64 workflow_run_id`; 6: `CutoverPhase phase`; 7: `google.protobuf.Timestamp recorded_at`; 8: `string discord_message_id` |

`CutoverPhase` values are `CUTOVER_PHASE_UNSPECIFIED = 0` (invalid), `CUTOVER_PHASE_ANNOUNCEMENT_INTENT = 1`, and `CUTOVER_PHASE_COMPLETE = 2`.

- `schema_revision` is exactly 1 and `protocol_revision` exactly 2. Git hashes are full lowercase Git SHA-1 values; OCI digests are `sha256:<64 lowercase hex>`; file hashes are 64 lowercase hex. Reject missing, malformed, unknown or inconsistent metadata. Versions come from the exact tested tree's existing component version files. Repository is the canonical `owner/name`; IDs/attempt are positive. Timestamp is valid UTC.
- Artifact paths are canonical relative paths with no traversal, duplicates or absolute paths, sorted lexically. Record the actual plugin release bundle, including `FlightStripsPlugin.dll`, `CoreDLL` and applicable configuration files. Hash and size every file; verify downloaded artifact origin repository/run/head/tree and all hashes. The manifest artifact is associated with that same workflow run, avoiding a circular manifest self-hash. Workflow lookup must prove origin; user-supplied manifest contents alone never establish trust.
- Store `candidate-manifest.pb` alongside the bundle in the candidate workflow artifacts, with 90-day retention. Keep optional textproto separate and never use it as the authoritative input.
- Record the exact candidate workflow run and attempt in Tasks 21–23's qualification record. Promotion and manual cutover must select those recorded values explicitly, never the newest successful run. Task 24 binds `QUALIFIED_CANDIDATE_RUN_ID` and `QUALIFIED_CANDIDATE_RUN_ATTEMPT`; missing binding blocks writes. A rebuild of the same head is a new candidate and requires affected qualification gates.
- Embedded backend `BUILD_VERSION`/`main.buildVersion` and frontend build identity contain component version plus tested Git tree. Preserve a development fallback. Task 20a consumes the existing backend entrypoint; this task owns Dockerfile/build argument and frontend identity wiring.

## Promotion and cutover contract

- After the release PR merges, prove repository/run origin, versions and merged Git tree match the tested manifest, then promote the **same** backend/frontend OCI digests to pinned version tags. Do not rebuild. If a pinned tag exists, accept it only when its digest is already identical; otherwise fail before mutation. Do not update application `latest`, build/publish/bump `backend-migrate`, or replace digest proof with a matching tag name.
- Add a manually dispatched cutover job with exact `plugin_release_tag`, `expected_dll_sha256` (the `FlightStripsPlugin.dll` hash) and `dry_run` (boolean, default true) inputs. Verify the tag/tree/component versions against the manifest, all plugin files/origin and promoted image digests. Refuse mismatch before any attachment/announcement. Attach only the verified bundle to its matching release and send only the matching frontend announcement after validation. Preserve existing release contents except the intended verified attachments; retries must detect identical already-attached assets and avoid duplicate announcement through a recorded completion marker.
- Serialize cutover jobs per repository/plugin tag with cancellation disabled. Attach immutable `cutover-<tree>-<run>-intent.pb` to the plugin release before the Discord call, then `cutover-<tree>-<run>-complete.pb` after its confirmed response, using `CutoverReceipt`. Receipts must match the verified manifest and release. A completion receipt makes retries skip announcement; an intent without completion requires operator reconciliation and blocks automatic resend, since an interrupted call may have succeeded. Record the returned Discord message ID in completion; never claim exactly-once delivery without provider proof. Conflicting pre-existing assets/receipts fail; do not overwrite them. Dry run only reads receipts. No additional publication permission is inferred from the intent.
- Dry run performs the same validation with zero release/asset/tag/announcement/infrastructure writes and reports the exact intended artifacts. CI credentials are scoped to the job; no tokens appear in logs or manifests. Actual attachment and announcement authorization occurs at Task 24, not this implementation task.
- Remove automatic DLL attachment and frontend notifications triggered merely by release PR merge **and** the separate release-published Discord workflow. Preserve unrelated documentation release behavior. Infrastructure image-bump automation remains a PR with pinned versions/digests; it cannot update/deploy the stack directly or bypass the reviewed Task 21 stack.
- Record artifact retrieval/promotion/dry-run commands and retention sufficient for Tasks 21–24. An expired/missing artifact requires a new candidate and affected qualification gates; never rebuild an old manifest in place.

## Scope boundaries

Own release/build/publication workflows (including `discord-release.yml`), Release Please integration, release helper code/tests, the new release Protobuf schema, Dockerfiles and frontend build identity. Do not modify runtime/API assembly owned by 20a or Compose/default-entrypoint/SQL deletion owned by 20c. No ALB changes. Do not run actual publication/notification/deployment workflows while testing.

## Done when

- Local tests with GitHub/OCI fixtures demonstrate failure before writes for changed head/tree, wrong origin/run, hash/digest/version drift, a conflicting pinned tag and incorrect manual inputs. Success promotes the recorded digest with no build step. Dry-run tests prove zero writes; retry tests prove safe attachment/announcement completion handling.
- Workflow inspection/test evidence shows that merge/release-published events alone cannot attach an incompatible DLL, announce the frontend, update `latest` or publish/bump a migrator. All automatic publication paths are covered, not only `release-please.yml`.
- Candidate manifests and exact artifact retrieval commands are usable by Tasks 21–24. Combined final-tree application qualification remains 20c and staging qualification remains 21–23.
- Open and attach a draft PR into `codex/multi-node-nats-base` with tests, sample textproto and dry-run evidence. Do not merge, publish, notify or deploy.

**Starting points:** `.github/workflows/release-please.yml`, `.github/workflows/discord-release.yml`, `.release-please-config.json`, backend/frontend Dockerfiles and frontend Vite version definition.

Implementation handoff and local acceptance evidence: [release-gates-evidence.md](../release-gates-evidence.md).
