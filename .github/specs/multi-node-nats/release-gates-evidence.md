# Task 20b release gates and artifact handoff

This implementation is held on the integration base until coordinated Task 24.
The commands below are an operator handoff, not permission to build, publish,
dispatch, notify or deploy during Task 20b. No real release workflow was run for
this acceptance evidence.

## Authority and qualification record

The unified Release Please PR remains `release-please--branches--main`, with
`plugin/v<version>`, `backend/v<version>`, `frontend/v<version>` and
`docs/v<version>` tags and existing component `version.txt` files. Each candidate
build is manually dispatched **on the release PR head branch**, not `main`.
The helper verifies the open, same-repository bot PR, pending label, exact run
head/branch, main-reviewed workflow/helper/schema, version files and Release
Please manifest before building. It checks again when creating the manifest.

Candidate image tags are `<version>-candidate-<full-tree>-<run>-<attempt>`.
An existing candidate tag cannot be overwritten; retry the whole workflow with
a new attempt. Both container build identities and OCI labels contain
`<component-version>+<tested-tree>`. Candidate images target Linux amd64, matching
the application stack; the immutable digest is the actual image manifest digest.
No extra JSON metadata or cluster protocol revision is introduced.

Each successful run retains two immutable GitHub artifacts for 90 days:

- `plugin-<run>-<attempt>`: the loader DLL, Core DLL, production and development
  configuration files, all recorded with hash and size in the manifest.
- `candidate-<run>-<attempt>`: `candidate-manifest.pb` and a separate review-only
  `candidate-manifest.textproto`. This separate artifact avoids a circular hash.

Tasks 21–23 must record the **candidate run ID and attempt**, release PR number,
head and tree, all versions, both OCI digests, plugin artifact ID, DLL hash and
reviewed staging stack revision. A successful rebuild of the same head is still
a different candidate. Never select “the latest successful run” as qualification
evidence. At Task 24, record that exact run/attempt in repository variables
`QUALIFIED_CANDIDATE_RUN_ID` and `QUALIFIED_CANDIDATE_RUN_ATTEMPT` before merging
the release PR. Missing variables block promotion. Manual cutover uses the same
binding. Changing either variable requires rechecking the qualification record;
the variables do not themselves authorize staging or production actions.

Artifact lookup checks the repository, candidate workflow ID/path, dispatch
event, run/attempt, head branch/SHA, successful conclusion, artifact origin IDs
and SHA, manifest source/version/tree and every downloaded file. A local or
user-supplied manifest never establishes provenance. Expiry, deletion or drift
requires a new candidate and affected qualification gates, never rebuilding an
old manifest in place. Retain artifacts through Task 24; complete qualification
within the 90-day window or rebuild and requalify.

## Retrieval and read-only validation

Install Python 3.11 with `python -m pip install -r scripts/cluster-proto-requirements.txt`,
authenticated GitHub CLI and `crane` (candidate CI installs setup-crane v0.4).
For schema regeneration/checks use protoc 26.1:

```powershell
protoc -I proto --python_out=scripts/release proto/release/v1/candidate.proto
python scripts/release/check_generated.py
```

These PowerShell commands retrieve the exact artifacts. Replace the example
IDs with the recorded qualification values; the samples in `release-samples`
are synthetic and cannot be published.

```powershell
$repo = 'flightstrips/FlightStrips'
$releasePr = 42
$candidateRun = 100
$candidateAttempt = 1
gh run download $candidateRun -R $repo -n "candidate-$candidateRun-$candidateAttempt" -D candidate/manifest
gh run download $candidateRun -R $repo -n "plugin-$candidateRun-$candidateAttempt" -D candidate/bundle
python scripts/release/gate.py inspect --repository $repo --pr $releasePr --candidate-run $candidateRun --candidate-attempt $candidateAttempt
```

`inspect` verifies the still-open PR and remote artifact/run provenance, all
plugin bytes, and candidate image digests/labels with zero writes. Its scalar
output supplies the exact image references and plugin artifact ID. For Tasks
21–23 pull those references literally, without a tag lookup:

```powershell
docker pull ghcr.io/flightstrips/backend-api@sha256:<64-hex-backend-digest>
docker pull ghcr.io/flightstrips/frontend@sha256:<64-hex-frontend-digest>
```

After the PR merges, this command performs the complete promotion validation
without writes (the default is dry run). It proves the exact merge commit's
tree and component versions match the manifest and checks both pinned tags
before any potential write:

```powershell
python scripts/release/gate.py promote --repository $repo --sha <full-release-PR-merge-SHA> --candidate-run $candidateRun --candidate-attempt $candidateAttempt
```

Only Task 24 authorizes actual promotion. The release workflow then executes
the same helper with `--dry-run false`, using the recorded variables. The helper
uses `crane tag <repository>@<recorded-digest> <version>` and reads back the
digest; it has no rebuild step. An identical pinned tag is a no-op; a conflicting
pinned tag or identity rejects before writes. Infrastructure automation opens
a PR containing `repository:version@sha256:digest`, does not bump a migrator and
does not deploy. Keep that PR unmerged and incorporate matching references into
the reviewed Task 21 stack PR.

## Manual cutover and announcement uncertainty

With images already promoted and both matching releases present, dry-run the
manual workflow on `main` with exact `plugin_release_tag`,
`expected_dll_sha256` and `dry_run=true` inputs. Local equivalent:

```powershell
python scripts/release/gate.py cutover --repository $repo --plugin-release-tag plugin/v4.0.0 --expected-dll-sha256 <64-hex-DLL-hash> --candidate-run $candidateRun --candidate-attempt $candidateAttempt
```

The default dry run reads and verifies all origins, tags/trees/versions, images,
bundle hashes, existing attachments and receipts and reports intended files and
the exact frontend release. It makes zero release, tag, OCI, notification or
infrastructure writes and does not need the Discord secret. Only the manual
workflow with `dry_run=false` on `main` can reach the publication job; configure
the `coordinated-cutover` environment with Task 24 required operator reviewers
and the Discord webhook secret before activation. This implementation does not
create or configure that production environment.

Cutover jobs serialize per repository/plugin tag with cancellation disabled.
Identical already-attached files are preserved, conflicting bytes fail, and
unrelated release contents remain intact. Before Discord, upload and read back
immutable `cutover-<tree>-<cutover-run>-intent.pb`. The webhook uses `wait=true`,
disables mentions and is called once. After a confirmed returned message ID,
upload `cutover-<tree>-<cutover-run>-complete.pb`. Receipts use only the fixed
`CutoverReceipt` fields and match the manifest, tags, DLL, workflow origin and
filenames. Completion allows retries to finish missing attachments without
announcing again.

An intent without a valid completion blocks **both dry-run validation and
automatic resend**. This includes timeout, interruption, unconfirmed intent
upload/readback or a lost completion upload response. Inspect the immutable
receipt and reconcile the actual Discord message with the provider before any
operator action. If delivery is proven, an authorized operator can add the
matching completion receipt with that message ID, original cutover run ID and
valid UTC time (encode using `CutoverReceipt` and deterministic serialization;
the review sample shows the fields). Do not overwrite/delete intent or assets
to force a retry. If delivery cannot be proven, leave cutover blocked pending
explicit operator reconciliation; no exactly-once delivery is claimed.

## Acceptance evidence (local, 2026-09-30)

`python -m unittest discover -s scripts/release -p 'test_*.py' -v` passes 25 tests
with additional rejection subcases. Fake GitHub/OCI adapters and mocked CLI,
ZIP and Discord boundaries cover source/head/tree/version/run/repository drift,
unknown fields and noncanonical protobuf, path traversal/duplicate archive
entries, missing/expired artifacts, hash/size drift, image label/digest drift,
second-destination pinned tag conflicts before writes, digest-only promotion
and retry, dry-run zero writes, wrong manual inputs, existing asset conflicts,
receipt completion retry and unknown Discord/lost completion outcomes. No
registry login or live GitHub/Discord mutation is used by these tests.

`python scripts/release/check_generated.py` passes with protoc 26.1 and protobuf
5.26.1. `actionlint` 1.7.7 passes on all seven affected/new workflows
(shellcheck/pyflakes disabled on this Windows host).
The frontend production build passes with
`BUILD_VERSION=2.0.0+bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb`; the exact string
is present in the generated JS. Loading the Vite build config without a supplied
identity returns `2.0.0+dev`. Existing Vite chunk-size warnings remain.

The synthetic [manifest](release-samples/candidate-manifest.textproto),
[intent](release-samples/cutover-intent.textproto),
[completion](release-samples/cutover-complete.textproto) and
[zero-write dry-run report](release-samples/dry-run.txt) are review output only.
Both automatic announcement paths are gated: Release Please contains no client
attachment/Discord step, and the release-published Discord workflow accepts
only `docs/v` tags. Application `latest` and migrator build/publish/bump paths
are absent; ordinary plugin CI only builds/tests and candidate CI alone retains
the complete verified plugin bundle. Unrelated docs image/release behavior remains.

Full runtime/container/plugin qualification belongs to 20c and Tasks 21–23.
Task 20b has not built or published a real candidate, dispatched cutover,
attached release assets, notified Discord or deployed anything.
