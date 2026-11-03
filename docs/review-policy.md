# Code owner review policy

`.github/CODEOWNERS` assigns all files to `@frederikrosenberg`, with `@bjerrecs`
(Simon) as a co-owner of the release manifest and each component's changelog and
version file. The policy supports a global `*` rule followed by exact rooted file
overrides with individual usernames. The last matching rule wins. Unsupported
ownership rules fail closed rather than silently granting an exemption.

The `Code owner review` check run checks every changed file. Each file must
be owned by the PR author or have an owner's approving review whose `commit_id`
matches the PR's current head SHA. Renames check both the old and new paths.
Simon can approve release metadata; a PR also changing source needs that source's
owner to approve it. An incomplete file listing fails the ownership check.
Comments do not erase decisions. Dismissed approvals and approvals of older
commits do not count; an owner's outstanding request for changes blocks approval.

The trusted `Review policy` workflow runs on PR changes, main pushes, manual
dispatch, and completion of `Release Please` or `Review policy events`. The latter
is a read-only notification on review submission, editing or dismissal, including
fork reviews.
The writer only executes the trusted workflow revision and loads CODEOWNERS from
the PR's target commit. It never executes code or downloads artifacts from the PR.
Every run reevaluates all open main PRs so coalesced events and missing fork PR
metadata in workflow-run payloads do not lose updates. PRs sharing a commit must
all satisfy policy before the shared check can pass. The workflow updates the
same check run on each commit instead of appending commit statuses on every
refresh. API failures for one commit do not prevent evaluating the other PRs.

The workflows use the built-in `GITHUB_TOKEN` to read PR data and publish checks.
No custom GitHub App, secrets, variables, or deployment environment are needed.
The required check uses the shared GitHub Actions identity and is intended for
trusted collaborators who do not deliberately spoof workflow check results.

## Review notifier integrity

GitHub evaluates `pull_request_review` workflows from the PR merge tree. To keep
review dismissal and change-request notifications working, any PR that depends
on another owner's approval must leave `.github/workflows/review-policy-events.yml`
unchanged. Changing, deleting, adding or renaming that file fails the gate even
after an owner approves the latest commit. Renames check both the old and new
paths, and incomplete file listings fail before this safeguard is evaluated.

To maintain the notifier, use a separate PR authored by a person who owns every
changed file. Such PRs already qualify for the owner-author exemption and do not
depend on an approval that could later be revoked. This also prevents a partial
owner from combining notifier changes with files that still need another
owner's approval.

## Release PRs

PRs by `github-actions[bot]` from this repository's
`release-please--branches--main` branch targeting `main` skip the component builds,
AMAN contract, airport configuration and browser E2E jobs. Job-level skipping
keeps existing required checks satisfied without allocating build runners.
PRs created using `GITHUB_TOKEN` do not trigger normal PR workflows. Once Release
Please completes, the gate also explicitly reports AMAN and E2E as skipped on the
verified release head, so these PRs do not wait for missing required checks.
Normal PRs, forks with a matching branch name, and merge queue entries still run CI.

The review gate checks every file in a release PR. Only added or modified
`.release-please-manifest.json` and `CHANGELOG.md` / `version.txt` in the four
configured component directories are allowed. Renames, deletions, source changes,
workflow changes and incomplete file listings fail the gate. Release PRs still
need either `@frederikrosenberg` or `@bjerrecs` to approve their latest commit.

Main pushes containing only release metadata skip redundant CI. Actual release
builds and publishing in `release-please.yml` still run after the release is merged.

## Enable enforcement

1. Merge these workflows into `main`. The main-push run creates the checks on
   existing open PRs, including release PRs. The workflow can also be run manually.
2. In the [main ruleset](https://github.com/flightstrips/FlightStrips/rules/300063),
   add `Code owner review` from **GitHub Actions** as a required check once it has
   been reported. For API configuration, its `integration_id` is `15368`.
3. Keep the existing AMAN and E2E required checks, required PRs, and conversation
   resolution. Set native required approvals to zero, code-owner review disabled,
   and last-push approval disabled so owner-authored PRs use the custom exemption.

Any ruleset bypass actors retain their configured ability to bypass requirements.

The workflow uses `pull_request_target` for trusted, write-capable handling of
fork PR metadata. If the repository's Actions event policy blocks that event,
allow this workflow before enabling the required check. The repository currently
has no merge queue rule; enabling a merge queue also requires extending this
review check to validate the queue's synthetic commits.

To reevaluate existing PRs manually, run `Review policy` from the Actions tab or
`gh workflow run review-policy.yml --ref main`.

## Validate locally

```powershell
node --test .github/scripts/review-policy.test.cjs
docker run --rm -v "${PWD}:/repo" -w /repo rhysd/actionlint:1.7.12 -shellcheck=
```
