const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const { parseOwnership, isReleasePullRequest, evaluate, run, RELEASE_FILES, REVIEW_CHECK_NAME, REVIEW_CHECK_ID } = require('./review-policy.cjs');

const repository = 'flightstrips/FlightStrips';
const owner = 'frederikrosenberg';
const reviewAppId = 424242;
const reviewApp = { id: reviewAppId, slug: 'flightstrips-review' };
const head = 'a'.repeat(40);
const base = 'b'.repeat(40);
const owners = new Set([owner]);
const ownership = parseOwnership('* @frederikrosenberg');
const sourceFile = { filename: 'backend/main.go', status: 'modified' };
const makePr = (changes = {}) => ({
  number: 1028, state: 'open', user: { login: 'contributor' }, changed_files: 1,
  base: { ref: 'main', sha: base },
  head: { ref: 'feature', sha: head, repo: { full_name: repository } },
  ...changes,
});
const review = (state, commit_id = head, login = owner) => ({ state, commit_id, user: { login } });
const decision = (reviews, pr = makePr(), files = [sourceFile]) => evaluate({ pr, ownership, reviews, files, repository });
const release = () => makePr({
  user: { login: 'github-actions[bot]' },
  head: { ref: 'release-please--branches--main', sha: head, repo: { full_name: repository } },
});

test('global ownership includes new directories and fails closed on unsupported rules', () => {
  assert.deepEqual(parseOwnership('# Owners\n\n* @FrederikRosenberg # all files\n').defaultOwners, owners);
  assert.deepEqual(parseOwnership('* @one @two').defaultOwners, new Set(['one', 'two']));
  for (const text of ['', '/backend/ @one', '* @one\n/frontend/ @two', '* @org/team', '*',
    '* @one\n/**/*.txt @two', '* @one\n/../version.txt @two', '* @one\n* @two']) {
    assert.throws(() => parseOwnership(text), /CODEOWNERS/);
  }
  const parsed = parseOwnership('* @one\n/backend/version.txt @one @two\n/backend/version.txt @three');
  assert.deepEqual(parsed.fileOwners.get('backend/version.txt'), new Set(['three']));
});

test('owner authors are exempt; other authors need a current owner approval', () => {
  assert.equal(decision([], makePr({ user: { login: 'FrederikRosenberg' } })).state, 'success');
  assert.equal(decision([]).state, 'failure');
  assert.equal(decision([review('APPROVED', head, 'someone-else')]).state, 'failure');
  assert.equal(decision([review('APPROVED', head, 'FrederikRosenberg')]).state, 'success');
  assert.equal(decision([review('APPROVED', 'old-head')]).state, 'failure');
  assert.equal(decision([review('APPROVED')], makePr({ head: { sha: 'new-head' } })).state, 'failure');
});

test('comments preserve decisions; dismissal and change requests revoke them', () => {
  assert.equal(decision([review('APPROVED'), review('COMMENTED'), review('PENDING')]).state, 'success');
  assert.equal(decision([review('APPROVED'), review('DISMISSED')]).state, 'failure');
  assert.equal(decision([review('APPROVED'), review('CHANGES_REQUESTED', 'older')]).state, 'failure');
  assert.equal(decision([review('CHANGES_REQUESTED'), review('COMMENTED')]).state, 'failure');
  assert.equal(decision([review('CHANGES_REQUESTED'), review('APPROVED')]).state, 'success');
  const result = evaluate({
    pr: makePr(), ownership: parseOwnership(`* @${owner} @another-owner`), files: [sourceFile], repository,
    reviews: [review('APPROVED'), review('CHANGES_REQUESTED', head, 'another-owner')],
  });
  assert.equal(result.state, 'failure');
});

test('release PRs allow only complete, generated metadata and still need owner approval', () => {
  const files = [...RELEASE_FILES].map(filename => ({ filename, status: 'modified' }));
  const pr = { ...release(), changed_files: files.length };
  assert.equal(isReleasePullRequest(pr, repository), true);
  assert.equal(decision([], pr, files).state, 'failure');
  assert.equal(decision([review('APPROVED')], pr, files).state, 'success');
  assert.equal(decision([review('APPROVED', 'old-head')], pr, files).state, 'failure');
  for (const file of [
    { filename: 'backend/main.go', status: 'modified' },
    { filename: '.github/workflows/release-please.yml', status: 'modified' },
    { filename: 'backend/version.txt', status: 'removed' },
    { filename: 'backend/CHANGELOG.md', previous_filename: 'backend/main.go', status: 'renamed' },
  ]) {
    assert.equal(decision([review('APPROVED')], release(), [file]).state, 'failure');
  }
  assert.equal(decision([review('APPROVED')], pr, files.slice(1)).state, 'failure');
  assert.equal(decision([review('APPROVED')], { ...release(), changed_files: 0 }, []).state, 'failure');
});

const configuredOwnership = () => parseOwnership(fs.readFileSync('.github/CODEOWNERS', 'utf8'));
const scopedDecision = (reviews, files, author = 'contributor') => evaluate({
  pr: makePr({ changed_files: files.length, user: { login: author } }),
  ownership: configuredOwnership(), files, reviews, repository,
});

test('Simon co-owns every generated release file and can approve an entire release', () => {
  const ownership = configuredOwnership();
  const config = JSON.parse(fs.readFileSync('.release-please-config.json', 'utf8'));
  const expected = ['.release-please-manifest.json', ...Object.keys(config.packages).flatMap(component =>
    [`${component}/CHANGELOG.md`, `${component}/version.txt`])];
  assert.deepEqual(new Set(ownership.fileOwners.keys()), new Set(expected));
  assert.deepEqual(ownership.defaultOwners, owners);
  for (const path of expected) assert.deepEqual(ownership.fileOwners.get(path), new Set([owner, 'bjerrecs']));
  const files = expected.map(filename => ({ filename, status: 'modified' }));
  const pr = { ...release(), changed_files: files.length };
  const result = evaluate({ pr, ownership, files, reviews: [review('APPROVED', head, 'bjerrecs')], repository });
  assert.equal(result.state, 'success');
  assert.equal(scopedDecision([], files, 'bjerrecs').state, 'success');
  assert.equal(scopedDecision([], files, owner).state, 'success');
});

test('Simon cannot exempt or approve source changes, even when combined with release metadata', () => {
  const metadata = { filename: 'backend/version.txt', status: 'modified' };
  for (const files of [[sourceFile], [sourceFile, metadata]]) {
    assert.equal(scopedDecision([], files, 'bjerrecs').state, 'failure');
    assert.equal(scopedDecision([review('APPROVED', head, 'bjerrecs')], files).state, 'failure');
    assert.equal(scopedDecision([review('APPROVED')], files).state, 'success');
    assert.equal(scopedDecision([review('APPROVED')], files, 'bjerrecs').state, 'success');
  }
  const renamed = { filename: 'backend/version.txt', previous_filename: 'backend/main.go', status: 'renamed' };
  assert.equal(scopedDecision([], [renamed], 'bjerrecs').state, 'failure');
  assert.equal(scopedDecision([review('APPROVED', head, 'bjerrecs')], [renamed]).state, 'failure');
  assert.equal(scopedDecision([review('APPROVED')], [renamed]).state, 'success');
  assert.equal(scopedDecision([review('CHANGES_REQUESTED', head, 'bjerrecs'), review('APPROVED')], [sourceFile]).state, 'success');
});

test('scoped approvals must be current and cannot count after dismissal or requested changes', () => {
  const files = [{ filename: 'frontend/CHANGELOG.md', status: 'modified' }];
  assert.equal(scopedDecision([review('APPROVED', 'old-head', 'bjerrecs')], files).state, 'failure');
  assert.equal(scopedDecision([review('APPROVED', head, 'bjerrecs'), review('COMMENTED', head, 'bjerrecs')], files).state, 'success');
  assert.equal(scopedDecision([review('APPROVED', head, 'bjerrecs'), review('DISMISSED', head, 'bjerrecs')], files).state, 'failure');
  assert.equal(scopedDecision([review('APPROVED'), review('CHANGES_REQUESTED', head, 'bjerrecs')], files).state, 'failure');
});

test('incomplete file listings cannot grant a scoped author exemption or approval', () => {
  for (const author of [owner, 'bjerrecs', 'contributor']) {
    const result = evaluate({
      pr: makePr({ changed_files: 2, user: { login: author } }), ownership: configuredOwnership(), repository,
      files: [{ filename: 'backend/version.txt', status: 'modified' }], reviews: [review('APPROVED')],
    });
    assert.equal(result.state, 'failure');
  }
});

const notifierPath = '.github/workflows/review-policy-events.yml';

test('approvals cannot authorize disabling the notifier on an approval-dependent PR', () => {
  for (const file of [
    { filename: notifierPath, status: 'modified' },
    { filename: notifierPath, status: 'removed' },
    { filename: notifierPath, status: 'added' },
    { filename: '.github/workflows/renamed-events.yml', previous_filename: notifierPath, status: 'renamed' },
    { filename: notifierPath, previous_filename: '.github/workflows/other-events.yml', status: 'renamed' },
  ]) {
    for (const author of ['contributor', 'bjerrecs']) {
      const result = scopedDecision([review('APPROVED')], [sourceFile, file], author);
      assert.equal(result.state, 'failure');
      assert.match(result.description, /Review notifier changes/);
    }
  }
});

test('fully owning authors can maintain the notifier without depending on reviews', () => {
  const files = [sourceFile, { filename: notifierPath, status: 'modified' }];
  assert.equal(scopedDecision([], files, owner).state, 'success');
  const result = evaluate({
    pr: makePr({ changed_files: files.length, user: { login: 'bjerrecs' } }), files, repository,
    ownership: parseOwnership(`* @${owner}\n/${notifierPath} @bjerrecs`), reviews: [review('APPROVED')],
  });
  assert.equal(result.state, 'failure', 'Owning only the notifier must not exempt source files');
  assert.match(result.description, /Review notifier changes/);
});

function mockGitHub({ prs = [makePr()], reviews = [], files = [sourceFile], checks = [], content = '* @frederikrosenberg',
  getPr, reviewError, checkError } = {}) {
  const createdChecks = [];
  const checkWrites = [];
  let nextId = Math.max(0, ...checks.map(check => check.id || 0)) + 1;
  const checkRuns = checks.map(check => ({ head_sha: head, ...check, id: check.id || nextId++ }));
  const contentReads = [];
  const pagination = [];
  const failures = [];
  const checksApi = app => ({
      listForRef() {},
      create: async args => {
        assert.equal(args.name === REVIEW_CHECK_NAME, app.id === reviewAppId, 'Review and CI checks use separate App identities');
        if (checkError) checkError('create', args);
        const check = { ...args, id: nextId++, app, conclusion: args.conclusion || null };
        checkRuns.push(check);
        createdChecks.push({ ...check });
        checkWrites.push({ ...check });
        return { data: check };
      },
      update: async args => {
        const check = checkRuns.find(check => check.id === args.check_run_id);
        assert.ok(check, 'Updating an existing check');
        assert.equal(check.app.id, app.id, 'Only the publishing App can update its check');
        assert.equal(args.head_sha, undefined, 'Updates cannot move a check to a different commit');
        if (checkError) checkError('update', { ...check, ...args });
        Object.assign(check, args);
        if (args.status !== 'completed') check.conclusion = null;
        checkWrites.push({ ...check });
        return { data: check };
      },
    });
  const reviewChecks = checksApi(reviewApp);
  const rest = {
    checks: checksApi({ id: 15368, slug: 'github-actions' }),
    pulls: { list() {}, listReviews() {}, listFiles() {},
      get: async ({ pull_number }) => ({ data: getPr ? getPr(pull_number) : prs.find(pr => pr.number === pull_number) }),
    },
    repos: {
      getContent: async args => {
        contentReads.push(args);
        return { data: { type: 'file', encoding: 'base64', content: Buffer.from(content).toString('base64') } };
      },
      createCommitStatus: async () => { assert.fail('Review refreshes must not append commit statuses'); },
    },
  };
  const paginate = async (method, args) => {
    pagination.push(args);
    if (method === rest.pulls.list) return prs;
    if (method === rest.checks.listForRef || method === reviewChecks.listForRef) {
      if (checkError) checkError('list', { head_sha: args.ref, name: args.check_name });
      return checkRuns.filter(check => check.head_sha === args.ref && (!args.check_name || check.name === args.check_name));
    }
    if (method === rest.pulls.listFiles) return files;
    if (method === rest.pulls.listReviews) {
      if (reviewError) throw new Error('Review API unavailable');
      return typeof reviews === 'function' ? reviews(args.pull_number) : reviews;
    }
    throw new Error('Unexpected API');
  };
  const github = { rest, paginate };
  const reviewGithub = { rest: { checks: reviewChecks }, paginate };
  const context = { repo: { owner: 'flightstrips', repo: 'FlightStrips' }, runId: 123 };
  const core = { info() {}, error() {}, setFailed(message) { failures.push(message); } };
  return {
    github, reviewGithub, reviewAppId, context, core, createdChecks, checkRuns, checkWrites, contentReads, pagination, failures,
    get reviewWrites() { return checkWrites.filter(check => check.name === REVIEW_CHECK_NAME); },
    get createdReleaseChecks() { return createdChecks.filter(check => check.name !== REVIEW_CHECK_NAME); },
  };
}

test('bot-created release PRs report skipped required CI without needing PR workflow events', async () => {
  const pr = release();
  const files = [{ filename: 'backend/version.txt', status: 'modified' }];
  const mock = mockGitHub({ prs: [pr], files });
  mock.context.payload = { workflow_run: { name: 'Release Please', pull_requests: [] } };
  await run(mock);
  assert.equal(mock.reviewWrites.at(-1).conclusion, 'failure', 'Release bot still needs owner approval');
  assert.deepEqual(mock.createdReleaseChecks.map(check => check.name), ['AMAN publisher-consumer contract', 'Running-system E2E']);
  assert.ok(mock.createdReleaseChecks.every(check => check.head_sha === head && check.conclusion === 'skipped'));

  const existing = mock.createdChecks;
  const again = mockGitHub({ prs: [pr], files, checks: existing, reviews: [review('APPROVED')] });
  await run(again);
  assert.equal(again.reviewWrites.at(-1).conclusion, 'success');
  assert.equal(again.createdChecks.length, 0, 'Reuse the review check and skipped check runs');
});

test('runtime uses per-file ownership from the target branch for release and ordinary PRs', async () => {
  const content = fs.readFileSync('.github/CODEOWNERS', 'utf8');
  for (const [pr, files, expected] of [
    [release(), [{ filename: 'backend/version.txt', status: 'modified' }], 'success'],
    [makePr(), [sourceFile], 'failure'],
    [makePr({ user: { login: 'bjerrecs' } }), [sourceFile], 'failure'],
  ]) {
    const mock = mockGitHub({ prs: [pr], files, content, reviews: [review('APPROVED', head, 'bjerrecs')] });
    await run(mock);
    assert.equal(mock.reviewWrites.at(-1).conclusion, expected);
    assert.ok(mock.contentReads.every(read => read.ref === base));
  }
});

test('CI is never waived for source changes hidden in the release branch or for ordinary PRs', async () => {
  const unsafe = mockGitHub({
    prs: [release()], reviews: [review('APPROVED')],
    files: [{ filename: 'backend/main.go', status: 'modified' }],
  });
  await run(unsafe);
  assert.equal(unsafe.reviewWrites.at(-1).conclusion, 'failure');
  assert.equal(unsafe.createdReleaseChecks.length, 0);
  const ordinary = mockGitHub({ reviews: [review('APPROVED')] });
  await run(ordinary);
  assert.equal(ordinary.createdReleaseChecks.length, 0);
});

test('review events update the same head from failure to success and back after dismissal', async () => {
  let reviews = [];
  const mock = mockGitHub({ reviews: () => reviews });
  for (const [updated, expected] of [
    [[], 'failure'], [[review('APPROVED')], 'success'],
    [[review('APPROVED'), review('DISMISSED')], 'failure'],
  ]) {
    reviews = updated;
    await run(mock);
    assert.equal(mock.reviewWrites.at(-2).status, 'in_progress');
    assert.equal(mock.reviewWrites.at(-2).conclusion, null);
    assert.equal(mock.reviewWrites.at(-1).conclusion, expected);
    assert.equal(mock.reviewWrites.at(-1).head_sha, head);
    assert.equal(mock.reviewWrites.at(-1).name, 'Code owner review');
  }
  assert.equal(mock.createdChecks.length, 1);
  assert.equal(new Set(mock.reviewWrites.map(check => check.id)).size, 1);
  assert.ok(mock.contentReads.every(read => read.ref === base && read.path === '.github/CODEOWNERS'));
  assert.ok(mock.pagination.every(args => args.per_page === 100));
});

test('runtime cannot leave a successful review for a PR that suppresses review notifications', async () => {
  const content = fs.readFileSync('.github/CODEOWNERS', 'utf8');
  for (const file of [
    { filename: notifierPath, status: 'modified' },
    { filename: notifierPath, status: 'removed' },
    { filename: '.github/workflows/renamed-events.yml', previous_filename: notifierPath, status: 'renamed' },
  ]) {
    const mock = mockGitHub({
      content, files: [file], reviews: [review('APPROVED')],
      checks: [{ id: 10, head_sha: head, name: REVIEW_CHECK_NAME, external_id: REVIEW_CHECK_ID,
        app: reviewApp, status: 'completed', conclusion: 'success' }],
    });
    await run(mock);
    assert.equal(mock.createdChecks.length, 0, 'Update the existing dedicated App check');
    assert.equal(mock.checkRuns[0].conclusion, 'failure');
    assert.match(mock.checkRuns[0].output.summary, /Review notifier changes/);
    assert.ok(mock.reviewWrites.every(check => check.conclusion !== 'success'));
  }
});

test('the notifier retains all revocation triggers and the trusted writer listens to its completion', () => {
  const notifier = fs.readFileSync(notifierPath, 'utf8');
  assert.match(notifier, /^name: Review policy events\r?$/m);
  assert.match(notifier, /pull_request_review:\s*types: \[submitted, edited, dismissed\]/);
  const writer = fs.readFileSync('.github/workflows/review-policy.yml', 'utf8');
  assert.match(writer, /workflow_run:\s*workflows: \[Review policy events, Release Please\]\s*types: \[completed\]/);
});

test('600 refreshes of an unchanged head reuse one check without appending commit statuses', async () => {
  const mock = mockGitHub({ reviews: [review('APPROVED')] });
  for (let refresh = 0; refresh < 600; refresh++) await run(mock);
  assert.equal(mock.createdChecks.length, 1);
  assert.equal(mock.checkRuns.length, 1);
  assert.equal(mock.checkRuns[0].conclusion, 'success');
  assert.equal(new Set(mock.reviewWrites.map(check => check.id)).size, 1);
  assert.equal(mock.failures.length, 0);
});

test('check reuse requires the policy identifier and dedicated App ownership', async () => {
  const mock = mockGitHub({ reviews: [review('APPROVED')], checks: [
    { id: 10, name: REVIEW_CHECK_NAME, external_id: REVIEW_CHECK_ID, app: { id: 15368, slug: 'github-actions' }, conclusion: 'success' },
    { id: 11, name: REVIEW_CHECK_NAME, external_id: 'another-workflow', app: reviewApp, conclusion: 'success' },
  ] });
  await run(mock);
  await run(mock);
  assert.equal(mock.createdChecks.length, 1);
  assert.equal(mock.createdChecks[0].external_id, REVIEW_CHECK_ID);
  assert.ok(mock.reviewWrites.every(check => check.id === mock.createdChecks[0].id));
});

test('same-named successful Actions checks cannot turn an unapproved review check green', async () => {
  const spoofed = [head, 'd'.repeat(40)].map((head_sha, index) => ({
    id: index + 10, head_sha, name: REVIEW_CHECK_NAME, external_id: REVIEW_CHECK_ID,
    app: { id: 15368, slug: 'github-actions' }, status: 'completed', conclusion: 'success',
  }));
  const mock = mockGitHub({ checks: spoofed });
  await run(mock);
  assert.equal(mock.createdChecks.length, 1);
  assert.equal(mock.createdChecks[0].app.id, reviewAppId);
  assert.equal(mock.reviewWrites.at(-1).conclusion, 'failure');
  assert.ok(mock.reviewWrites.every(check => check.app.id === reviewAppId));
  assert.ok(mock.pagination.filter(args => args.check_name === REVIEW_CHECK_NAME)
    .every(args => args.app_id === reviewAppId));
  assert.ok(mock.checkRuns.filter(check => check.app.id === 15368).every(check => check.conclusion === 'success'));
});

test('missing or shared publisher configuration fails before publishing any checks', async () => {
  for (const changes of [
    { reviewGithub: undefined }, { reviewAppId: undefined }, { reviewAppId: 'invalid' },
    { reviewAppId: 0 }, { reviewAppId: 15368 }, { reviewAppId: '15368' },
  ]) {
    const mock = mockGitHub();
    await assert.rejects(run({ ...mock, ...changes }), /dedicated review GitHub App/);
    assert.equal(mock.checkWrites.length, 0);
    assert.equal(mock.pagination.length, 0);
  }
  const mock = mockGitHub();
  await assert.rejects(run({ ...mock, reviewGithub: mock.github }), /dedicated review GitHub App/);
  assert.equal(mock.checkWrites.length, 0);
});

test('a token publishing as the wrong App cannot produce a passing review check', async () => {
  const mock = mockGitHub({ reviews: [review('APPROVED')] });
  const create = mock.reviewGithub.rest.checks.create;
  mock.reviewGithub.rest.checks.create = async args => {
    const { data } = await create(args);
    return { data: { ...data, app: { id: 15368, slug: 'github-actions' } } };
  };
  await run(mock);
  assert.ok(mock.failures.some(message => /Unable to publish/.test(message)));
  assert.ok(mock.reviewWrites.every(check => check.conclusion !== 'success'));
});

test('the trusted workflow uses its protected environment and a separate App token', async () => {
  const workflow = fs.readFileSync('.github/workflows/review-policy.yml', 'utf8');
  assert.match(workflow, /if: github\.ref == 'refs\/heads\/main'/);
  assert.match(workflow, /environment: review-policy/);
  assert.match(workflow, /ref: \$\{\{ github\.workflow_sha \}\}/);
  assert.match(workflow, /private-key: \$\{\{ secrets\.REVIEW_POLICY_PRIVATE_KEY \}\}/);
  assert.match(workflow, /REVIEW_POLICY_APP_TOKEN: \$\{\{ steps\.review-token\.outputs\.token \}\}/);
  const scriptLines = workflow.split('          script: |')[1].split(/\r?\n/).slice(1);
  const script = [];
  for (const line of scriptLines) {
    if (line.trim() && !line.startsWith('            ')) break;
    script.push(line.slice(12));
  }
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const execute = new AsyncFunction('github', 'context', 'core', 'getOctokit', 'process', 'require', script.join('\n'));
  const requireFromRoot = require('node:module').createRequire(require('node:path').resolve('github-script.cjs'));
  const mock = mockGitHub({ reviews: [review('APPROVED')] });
  await execute(mock.github, mock.context, mock.core, token => {
    assert.equal(token, 'dedicated-app-token');
    return mock.reviewGithub;
  }, { env: { REVIEW_POLICY_APP_TOKEN: 'dedicated-app-token', REVIEW_POLICY_APP_ID: String(reviewAppId) } }, requireFromRoot);
  assert.equal(mock.reviewWrites.at(-1).conclusion, 'success');
  assert.equal(mock.reviewWrites.at(-1).app.id, reviewAppId);
});

test('pushing a new head creates a separate check without moving or reusing the old one', async () => {
  const prs = [makePr({ user: { login: owner } })];
  const mock = mockGitHub({ prs });
  await run(mock);
  prs[0] = makePr({ user: { login: owner }, head: { ...prs[0].head, sha: 'c'.repeat(40) } });
  await run(mock);
  assert.equal(mock.createdChecks.length, 2);
  assert.deepEqual(mock.checkRuns.map(check => check.head_sha), [head, 'c'.repeat(40)]);
  assert.ok(mock.checkRuns.every(check => check.conclusion === 'success'));
});

test('lookup, creation and update failures for one head do not block another PR', async () => {
  const failedSha = 'c'.repeat(40);
  const broken = makePr({ number: 1, user: { login: owner }, head: { ...makePr().head, sha: failedSha } });
  const healthy = makePr({ number: 2, user: { login: owner } });
  for (const phase of ['lookup', 'create', 'start-update', 'finish-update']) {
    const checks = phase.endsWith('update') ? [{
      id: 40, head_sha: failedSha, name: REVIEW_CHECK_NAME, external_id: REVIEW_CHECK_ID,
      app: reviewApp, status: 'completed', conclusion: 'success',
    }] : [];
    const mock = mockGitHub({ prs: [broken, healthy], checks, checkError: (operation, check) => {
      if (check.head_sha !== failedSha) return;
      if ((phase === 'lookup' && operation === 'list') || (phase === 'create' && operation === 'create') ||
          (phase === 'start-update' && operation === 'update' && check.status === 'in_progress') ||
          (phase === 'finish-update' && operation === 'update' && check.status === 'completed')) {
        throw new Error('Check publication unavailable');
      }
    } });
    await run(mock);
    assert.ok(mock.reviewWrites.some(check => check.head_sha === head && check.conclusion === 'success'), phase);
    assert.ok(!mock.reviewWrites.some(check => check.head_sha === failedSha && check.conclusion === 'success'), phase);
    assert.ok(mock.failures.some(message => /Unable to publish/.test(message)), phase);
    if (phase === 'finish-update') assert.equal(mock.checkRuns.find(check => check.head_sha === failedSha).status, 'in_progress');
  }
});

test('PR read failures during evaluation and final verification are isolated to their head', async () => {
  const failedSha = 'c'.repeat(40);
  const prs = [
    makePr({ number: 1, user: { login: owner }, head: { ...makePr().head, sha: failedSha } }),
    makePr({ number: 2, user: { login: owner } }),
  ];
  for (const failedRead of [1, 2]) {
    let reads = 0;
    const mock = mockGitHub({ prs, getPr: number => {
      if (number === 1 && ++reads === failedRead) throw new Error('PR API unavailable');
      return prs.find(pr => pr.number === number);
    } });
    await run(mock);
    assert.equal(mock.checkRuns.find(check => check.head_sha === failedSha).conclusion, 'failure');
    assert.equal(mock.checkRuns.find(check => check.head_sha === head).conclusion, 'success');
    assert.equal(mock.failures.length, 1);
  }
});

test('a release skip publication error does not abort reviews for other heads', async () => {
  const failedSha = 'c'.repeat(40);
  const pr = release();
  const mock = mockGitHub({
    prs: [{ ...pr, number: 1, head: { ...pr.head, sha: failedSha } }, makePr({ number: 2, user: { login: owner } })],
    files: [{ filename: 'backend/version.txt', status: 'modified' }],
    checkError: (operation, check) => {
      if (operation === 'create' && check.name === 'AMAN publisher-consumer contract') throw new Error('Release check unavailable');
    },
  });
  await run(mock);
  assert.equal(mock.checkRuns.find(check => check.head_sha === failedSha).conclusion, 'failure');
  assert.equal(mock.checkRuns.find(check => check.head_sha === head).conclusion, 'success');
  assert.equal(mock.failures.length, 1);
});

test('fork review notifications work without PR metadata or artifacts in the event payload', async () => {
  const pr = makePr({ head: { sha: head, ref: 'feature', repo: { full_name: 'contributor/FlightStrips' } } });
  const mock = mockGitHub({ prs: [pr], reviews: [review('APPROVED')] });
  mock.context.payload = { workflow_run: { pull_requests: [] } };
  await run(mock);
  assert.equal(mock.reviewWrites.at(-1).conclusion, 'success');
});

test('API failures and invalid ownership cannot leave the evaluated commit green', async () => {
  for (const options of [{ reviewError: true }, { content: '/backend/ @someone' }]) {
    const mock = mockGitHub(options);
    await run(mock);
    assert.deepEqual(mock.reviewWrites.map(check => check.conclusion), [null, 'failure']);
    assert.equal(mock.failures.length, 1);
  }
  const failedRead = mockGitHub({ getPr: () => { throw new Error('PR API unavailable'); } });
  await run(failedRead);
  assert.deepEqual(failedRead.reviewWrites.map(check => check.conclusion), [null, 'failure']);
  assert.equal(failedRead.failures.length, 1);
});

test('pushes and target changes during evaluation cannot receive success', async () => {
  const initial = makePr();
  for (const latest of [
    makePr({ head: { ...initial.head, sha: 'new-head' } }),
    makePr({ base: { ...initial.base, sha: 'new-base' } }),
    makePr({ base: { ...initial.base, ref: 'other' } }),
    makePr({ state: 'closed' }),
  ]) {
    let reads = 0;
    const mock = mockGitHub({ reviews: [review('APPROVED')], getPr: () => ++reads === 1 ? initial : latest });
    await run(mock);
    assert.equal(mock.reviewWrites.at(-1).conclusion, 'failure');
    assert.ok(mock.reviewWrites.every(check => check.head_sha === head));
  }
});

test('PRs sharing a head cannot borrow an owner authorship exemption', async () => {
  const prs = [makePr({ number: 1, user: { login: owner } }), makePr({ number: 2 })];
  const mock = mockGitHub({ prs });
  await run(mock);
  assert.deepEqual(mock.reviewWrites.map(check => check.conclusion), [null, 'failure']);
});

const ciWorkflows = [
  'build-backend', 'build-frontend', 'build-docs', 'build-plugin',
  'check-aman-contract', 'check-airport-config', 'check-e2e',
];

test('all build jobs skip real release PRs and run for ordinary PRs, forks, pushes and merge groups', () => {
  const source = pr => ({
    event_name: 'pull_request', base_ref: pr.base.ref, head_ref: pr.head.ref,
    repository, event: { pull_request: pr },
  });
  const real = release();
  const contexts = [
    [source(real), false],
    [source(makePr()), true],
    [source({ ...real, user: { login: 'contributor' } }), true],
    [source({ ...real, base: { ...real.base, ref: 'feature' } }), true],
    [source({ ...real, head: { ...real.head, repo: { full_name: 'fork/FlightStrips' } } }), true],
    [{ event_name: 'push', event: {}, repository }, true],
    [{ event_name: 'merge_group', event: {}, repository }, true],
  ];
  let checkedJobs = 0;
  for (const name of ciWorkflows) {
    const text = fs.readFileSync(`.github/workflows/${name}.yml`, 'utf8');
    const jobs = text.split(/^jobs:\s*$/m)[1].split(/^  [\w-]+:\s*$/m).slice(1);
    for (const job of jobs) {
      const match = job.match(/^    if: >-\r?\n((?:      [^\r\n]+\r?\n)+)/m);
      assert.ok(match, `Missing release condition in ${name}`);
      const shouldRun = new Function('github', `return (${match[1].trim()});`);
      for (const [github, expected] of contexts) assert.equal(shouldRun(github), expected, name);
      checkedJobs++;
    }
  }
  assert.equal(checkedJobs, 10);
});

test('release identity in the review gate matches the CI exemption', () => {
  const pr = release();
  assert.equal(isReleasePullRequest(pr, repository), true);
  assert.equal(isReleasePullRequest({ ...pr, user: { login: owner } }, repository), false);
  assert.equal(isReleasePullRequest({ ...pr, head: { ...pr.head, repo: { full_name: 'fork/FlightStrips' } } }, repository), false);
  assert.equal(isReleasePullRequest({ ...pr, base: { ...pr.base, ref: 'other' } }, repository), false);
});
