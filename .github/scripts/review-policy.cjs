const REVIEW_CHECK_NAME = 'Code owner review';
const REVIEW_EVENTS_WORKFLOW = '.github/workflows/review-policy-events.yml';
const RELEASE_BRANCH = 'release-please--branches--main';
const RELEASE_REQUIRED_CHECKS = ['AMAN publisher-consumer contract', 'Running-system E2E'];
const RELEASE_FILES = new Set([
  '.release-please-manifest.json',
  ...['backend', 'frontend', 'euroscope-plugin', 'docs'].flatMap(component =>
    [`${component}/CHANGELOG.md`, `${component}/version.txt`]),
]);

function parseOwnership(contents) {
  const rules = contents.split(/\r?\n/)
    .map(line => line.split('#')[0].trim()).filter(Boolean);
  if (!rules.length || rules[0].split(/\s+/)[0] !== '*') {
    throw new Error('Review policy requires a global CODEOWNERS rule first.');
  }
  const fileOwners = new Map();
  let defaultOwners;
  for (const [index, rule] of rules.entries()) {
    const [pattern, ...owners] = rule.split(/\s+/);
    if (owners.length === 0 || owners.some(owner => !/^@[a-z\d](?:[a-z\d-]*[a-z\d])?$/i.test(owner))) {
      throw new Error('CODEOWNERS rules require individual GitHub usernames.');
    }
    const logins = new Set(owners.map(owner => owner.slice(1).toLowerCase()));
    if (index === 0) {
      defaultOwners = logins;
    } else if (/^\/(?:[a-z\d_.-]+\/)*[a-z\d_.-]+$/i.test(pattern) &&
        !pattern.split('/').some(part => part === '.' || part === '..')) {
      // Exact rooted files override the default; the last matching rule wins.
      fileOwners.set(pattern.slice(1), logins);
    } else {
      throw new Error('CODEOWNERS overrides must be exact rooted file paths.');
    }
  }
  return { defaultOwners, fileOwners };
}

function isReleasePullRequest(pr, repository) {
  return pr.base.ref === 'main' && pr.head.ref === RELEASE_BRANCH &&
    pr.head.repo?.full_name.toLowerCase() === repository.toLowerCase() &&
    pr.user.login.toLowerCase() === 'github-actions[bot]';
}

function releaseMetadataOnly(pr, files) {
  return files.length === pr.changed_files && files.length > 0 && files.every(file =>
    RELEASE_FILES.has(file.filename) && !file.previous_filename &&
    ['added', 'modified'].includes(file.status));
}

function evaluate({ pr, ownership, reviews, files, repository }) {
  if (isReleasePullRequest(pr, repository) && !releaseMetadataOnly(pr, files)) {
    return { state: 'failure', description: 'Release PR contains changes beyond generated release metadata; use a regular branch.' };
  }
  if (files.length !== pr.changed_files || files.length === 0) {
    return { state: 'failure', description: 'Unable to verify ownership of every changed file.' };
  }
  // Renames change both paths: a metadata owner cannot approve moving source
  // into a release-owned filename without the source owner's approval.
  const changedPaths = files.flatMap(file => [file.filename, file.previous_filename].filter(Boolean));
  const ownerGroups = changedPaths.map(path => ownership.fileOwners.get(path) || ownership.defaultOwners);
  const owners = new Set(ownerGroups.flatMap(group => [...group]));
  const author = pr.user.login.toLowerCase();

  // Authoring a change exempts it only when the author owns the affected file.
  if (ownerGroups.every(group => group.has(author))) {
    return { state: 'success', description: 'PR author owns all changed files.' };
  }
  // Review events execute the notifier from the PR merge tree. An approval-
  // dependent PR must leave it intact, or it could suppress later revocation
  // events after this check passes. Fully owner-authored PRs need no approval
  // to revoke and may maintain the notifier under the exemption above.
  if (changedPaths.includes(REVIEW_EVENTS_WORKFLOW)) {
    return { state: 'failure', description: 'Review notifier changes require an author who owns every changed file.' };
  }

  // The API returns reviews in chronological order. Comments and pending reviews
  // do not replace a decision; a dismissed decision must not revive an old one.
  const decisions = new Map();
  for (const review of reviews) {
    const login = review.user?.login.toLowerCase();
    if (owners.has(login) && ['APPROVED', 'CHANGES_REQUESTED', 'DISMISSED'].includes(review.state)) {
      decisions.set(login, review);
    }
  }
  if ([...decisions.values()].some(review => review.state === 'CHANGES_REQUESTED')) {
    return { state: 'failure', description: 'A code owner has requested changes.' };
  }
  if (ownerGroups.every(group => group.has(author) || [...group].some(login => {
    const review = decisions.get(login);
    return review?.state === 'APPROVED' && review.commit_id === pr.head.sha;
  }))) {
    return { state: 'success', description: 'Code owners approved the latest commit for all changed files.' };
  }
  return { state: 'failure', description: 'Code owners must approve the latest commit for all changed files.' };
}

function selectPulls(pulls, context) {
  const payload = context.payload || {};
  if (['push', 'schedule', 'workflow_dispatch'].includes(context.eventName)) return pulls;

  const heads = new Set();
  const include = pr => { if (pr?.head?.sha) heads.add(pr.head.sha); };
  if (context.eventName === 'pull_request_target') {
    if (!payload.pull_request?.number) throw new Error('PR event is missing its pull request.');
    include(payload.pull_request);
    include(pulls.find(pr => pr.number === payload.pull_request.number));
    // A synchronized or closed PR may leave other open PRs on its old head.
    if (payload.before) heads.add(payload.before);
  } else if (context.eventName === 'workflow_run') {
    const workflow = payload.workflow_run;
    if (workflow?.name === 'Release Please') {
      const repository = `${context.repo.owner}/${context.repo.repo}`;
      pulls.filter(pr => isReleasePullRequest(pr, repository)).forEach(include);
    } else if (workflow?.name === 'Review policy events') {
      const notified = workflow.pull_requests || [];
      for (const pr of notified) {
        include(pr);
        include(pulls.find(current => current.number === pr.number));
      }
      if (!notified.length) {
        // Fork workflow runs can omit pull_requests. Branch names are only a
        // routing hint: evaluate every matching candidate against the live API,
        // including forks with the same branch name, without trusting PR code.
        if (!workflow.head_sha && !workflow.head_branch) {
          throw new Error('Review notification is missing its PR and head metadata.');
        }
        pulls.filter(pr => pr.head.sha === workflow.head_sha || pr.head.ref === workflow.head_branch)
          .forEach(include);
      }
    } else {
      throw new Error('Unexpected review policy workflow notification.');
    }
  } else {
    throw new Error(`Unsupported review policy event: ${context.eventName}`);
  }
  // Statuses belong to commits. All open PRs sharing an affected head must pass.
  return pulls.filter(pr => heads.has(pr.head.sha));
}

async function run({ github, context, core }) {
  const repo = context.repo;
  const repository = `${repo.owner}/${repo.repo}`;
  const targetUrl = `${context.serverUrl || 'https://github.com'}/${repository}/actions/runs/${context.runId}`;
  // Title/body edits do not affect ownership, approval or the PR's target.
  if (context.eventName === 'pull_request_target' && context.payload?.action === 'edited' &&
      !context.payload.changes?.base) return;
  const openPulls = await github.paginate(github.rest.pulls.list, {
    ...repo, state: 'open', per_page: 100,
  });
  const pulls = selectPulls(openPulls, context);
  core.info(`Refreshing ${pulls.length} of ${openPulls.length} open PRs (${context.eventName}).`);
  // PR base.sha is the PR's recorded base, not necessarily today's main. Stacks
  // also target unmerged branches, so ownership always comes from trusted main.
  let ownershipPromise;
  const loadOwnership = () => ownershipPromise ||= (async () => {
    const { data: branch } = await github.rest.repos.getBranch({ ...repo, branch: 'main' });
    const { data: file } = await github.rest.repos.getContent({
      ...repo, path: '.github/CODEOWNERS', ref: branch.commit.sha,
    });
    if (file.type !== 'file' || file.encoding !== 'base64') {
      throw new Error('Cannot read CODEOWNERS from main.');
    }
    return { sha: branch.commit.sha, ownership: parseOwnership(Buffer.from(file.content, 'base64').toString('utf8')) };
  })();
  const heads = new Map();
  for (const pr of pulls) {
    if (!heads.has(pr.head.sha)) heads.set(pr.head.sha, []);
    heads.get(pr.head.sha).push(pr);
  }
  const statuses = new Map();
  const publish = async (sha, state, description) => {
    try {
      if (!statuses.has(sha)) {
        // The combined endpoint returns only the latest status per context,
        // so request cost does not grow with this commit's status history.
        // Octokit leaves this response object intact; extract each page's statuses.
        const latest = await github.paginate(github.rest.repos.getCombinedStatusForRef, {
          ...repo, ref: sha, per_page: 100,
        }, response => response.data.statuses);
        // Statuses are returned newest first. Unlike Actions-created check runs,
        // they do not inherit a workflow suite's event or cancellation state.
        statuses.set(sha, latest.find(status => status.context === REVIEW_CHECK_NAME));
      }
      const existing = statuses.get(sha);
      // Commit status history is limited to 1,000 entries per context/SHA.
      // Do not append pending/final pairs on every event or scheduled refresh.
      if ((state === 'pending' && existing) ||
          (existing?.state === state && existing.description === description)) return true;
      const { data: status } = await github.rest.repos.createCommitStatus({
        ...repo, sha, context: REVIEW_CHECK_NAME, state, description, target_url: targetUrl,
      });
      statuses.set(sha, status);
      return true;
    } catch (error) {
      core.error(`Commit ${sha}: unable to publish review check: ${error.message}`);
      core.setFailed('Unable to publish a code owner review check; other PRs will still be evaluated.');
      return false;
    }
  };

  // New heads start pending. Existing heads receive only changed decisions.
  // A publication failure for one head must not prevent checking the others.
  const ready = new Set();
  for (const sha of heads.keys()) {
    if (await publish(sha, 'pending', 'Checking ownership and approval of the latest commit.')) {
      ready.add(sha);
    }
  }

  for (const [sha, listedPulls] of heads) {
    if (!ready.has(sha)) continue;
    try {
      const { ownership, sha: ownershipSha } = await loadOwnership();
      const entries = [];
      for (const listed of listedPulls) {
        const { data: pr } = await github.rest.pulls.get({ ...repo, pull_number: listed.number });
        if (pr.state !== 'open' || pr.head.sha !== sha) {
          entries.push({ pr, metadataOnlyRelease: false, result: {
            state: 'failure', description: 'PR changed during evaluation; refresh the review policy.',
          } });
          continue;
        }
        const reviews = await github.paginate(github.rest.pulls.listReviews, {
          ...repo, pull_number: pr.number, per_page: 100,
        });
        const files = await github.paginate(github.rest.pulls.listFiles, {
          ...repo, pull_number: pr.number, per_page: 100,
        });
        entries.push({
          pr, metadataOnlyRelease: isReleasePullRequest(pr, repository) && releaseMetadataOnly(pr, files),
          result: evaluate({ pr, ownership, reviews, files, repository }),
        });
      }

      // Recheck after API reads. Never publish success for a different revision
      // or target branch from the one whose reviews were evaluated.
      const { data: currentMain } = await github.rest.repos.getBranch({ ...repo, branch: 'main' });
      for (const entry of entries) {
        const { data: latest } = await github.rest.pulls.get({ ...repo, pull_number: entry.pr.number });
        if (latest.state !== 'open' || latest.head.sha !== sha ||
            latest.base.ref !== entry.pr.base.ref || latest.base.sha !== entry.pr.base.sha ||
            currentMain.commit.sha !== ownershipSha) {
          entry.result = { state: 'failure', description: 'PR changed during evaluation; refresh the review policy.' };
          entry.metadataOnlyRelease = false;
        }
      }
      if (entries.some(entry => entry.metadataOnlyRelease)) {
        // GITHUB_TOKEN-created release PRs do not trigger pull_request workflows.
        // Report existing required jobs as skipped without running builds.
        const checks = await github.paginate(github.rest.checks.listForRef, { ...repo, ref: sha, per_page: 100 });
        for (const name of RELEASE_REQUIRED_CHECKS) {
          if (checks.some(check => check.name === name && check.app?.slug === 'github-actions' && check.conclusion === 'skipped')) continue;
          await github.rest.checks.create({
            ...repo, name, head_sha: sha, status: 'completed', conclusion: 'skipped',
            details_url: targetUrl,
            output: { title: 'Release metadata only', summary: 'Verified Release Please metadata; builds are skipped. Code owner review is still required.' },
          });
        }
      }
      // Checks belong to commits, so PRs sharing a head must all satisfy policy.
      const failed = entries.find(entry => entry.result.state !== 'success');
      const { state, description } = (failed || entries[0]).result;
      await publish(sha, state, description);
      for (const entry of entries) core.info(`PR #${entry.pr.number} (${sha}): ${entry.result.description}`);
      // Policy failures belong to each PR's check. A batch refresh must not fail
      // the triggering owner's workflow because another PR awaits approval.
    } catch (error) {
      core.error(`Commit ${sha}: ${error.message}`);
      await publish(sha, 'error', 'Unable to verify review policy; see workflow logs.');
      core.setFailed('Unable to verify a code owner review; other PRs will still be evaluated.');
    }
  }
}

module.exports = { REVIEW_CHECK_NAME, RELEASE_BRANCH, RELEASE_FILES, parseOwnership, isReleasePullRequest, releaseMetadataOnly, evaluate, selectPulls, run };
