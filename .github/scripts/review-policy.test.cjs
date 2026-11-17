const assert = require('node:assert/strict');
const fs = require('node:fs');
const test = require('node:test');
const { parseOwnership, isReleasePullRequest, evaluate, run, RELEASE_FILES, REVIEW_CHECK_NAME } = require('./review-policy.cjs');

const repository = 'flightstrips/FlightStrips';
const owner = 'frederikrosenberg';
const actionsApp = { id: 15368, slug: 'github-actions' };
const head = 'a'.repeat(40);
const base = 'b'.repeat(40);
const mainHead = 'd'.repeat(40);
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

function mockGitHub({ prs = [makePr()], reviews = [], files = [sourceFile], checks = [], statuses = [],
  content = '* @frederikrosenberg', getPr, getMain, reviewError, checkError, statusError } = {}) {
  const createdChecks = [];
  const checkRuns = checks.map(check => ({ head_sha: head, ...check }));
  const statusHistory = statuses.map(status => ({ sha: head, context: REVIEW_CHECK_NAME, ...status }));
  const reviewWrites = [];
  const contentReads = [];
  const pagination = [];
  const prReads = [];
  const failures = [];
  const rest = {
    checks: {
      listForRef() {},
      create: async args => {
        if (checkError) checkError(args);
        const check = { ...args, app: actionsApp };
        checkRuns.push(check);
        createdChecks.push(check);
        return { data: check };
      },
    },
    pulls: { list() {}, listReviews() {}, listFiles() {},
      get: async ({ pull_number }) => {
        prReads.push(pull_number);
        return { data: getPr ? getPr(pull_number) : prs.find(pr => pr.number === pull_number) };
      },
    },
    repos: {
      getBranch: async ({ branch }) => {
        assert.equal(branch, 'main', 'Ownership must come from trusted main');
        return { data: getMain ? getMain() : { commit: { sha: mainHead } } };
      },
      getContent: async args => {
        contentReads.push(args);
        return { data: { type: 'file', encoding: 'base64', content: Buffer.from(content).toString('base64') } };
      },
      getCombinedStatusForRef: async args => {
        if (statusError) statusError('list', { sha: args.ref });
        const contexts = new Map();
        for (const status of statusHistory.filter(status => status.sha === args.ref)) {
          if (!contexts.has(status.context)) contexts.set(status.context, status);
        }
        const latest = [...contexts.values()];
        const offset = ((args.page || 1) - 1) * args.per_page;
        return { data: {
          url: `https://api.github.com/repos/${repository}/commits/${args.ref}/status`,
          sha: args.ref, total_count: latest.length,
          statuses: latest.slice(offset, offset + args.per_page),
        } };
      },
      createCommitStatus: async args => {
        if (statusError) statusError('create', args);
        assert.ok(args.description.length <= 140);
        const status = { ...args, id: statusHistory.length + 1 };
        statusHistory.unshift(status);
        reviewWrites.push(status);
        return { data: status };
      },
    },
  };
  const paginate = async (method, args, mapResults) => {
    pagination.push(args);
    if (method === rest.pulls.list) return prs;
    if (method === rest.checks.listForRef) return checkRuns.filter(check => check.head_sha === args.ref);
    if (method === rest.repos.getCombinedStatusForRef) {
      const results = [];
      for (let page = 1; ; page++) {
        const response = await method({ ...args, page });
        // github-script v9 leaves responses with a top-level URL intact.
        // Without a mapping callback, paginate collects combined objects.
        results.push(...(mapResults ? mapResults(response) : [response.data]));
        if (page * args.per_page >= response.data.total_count) return results;
      }
    }
    if (method === rest.pulls.listFiles) return files;
    if (method === rest.pulls.listReviews) {
      if (reviewError) throw new Error('Review API unavailable');
      return typeof reviews === 'function' ? reviews(args.pull_number) : reviews;
    }
    throw new Error('Unexpected API');
  };
  const github = { rest, paginate };
  const context = { repo: { owner: 'flightstrips', repo: 'FlightStrips' }, runId: 123,
    eventName: 'workflow_dispatch', payload: {} };
  const core = { info() {}, error() {}, setFailed(message) { failures.push(message); } };
  return {
    github, context, core, createdChecks, checkRuns, statusHistory, reviewWrites, contentReads, pagination, prReads, failures,
    latest(sha = head) { return statusHistory.find(status => status.sha === sha && status.context === REVIEW_CHECK_NAME); },
  };
}

test('bot release metadata skips required builds but still requires current owner approval', async () => {
  let reviews = [];
  const mock = mockGitHub({ prs: [release()], files: [{ filename: 'backend/version.txt', status: 'modified' }], reviews: () => reviews });
  await run(mock);
  assert.equal(mock.latest().state, 'failure');
  assert.deepEqual(mock.createdChecks.map(check => check.name), ['AMAN publisher-consumer contract', 'Running-system E2E']);
  assert.ok(mock.createdChecks.every(check => check.head_sha === head && check.conclusion === 'skipped'));
  reviews = [review('APPROVED')];
  await run(mock);
  assert.equal(mock.latest().state, 'success');
  assert.equal(mock.createdChecks.length, 2, 'Do not duplicate skipped build checks');
});

test('runtime loads per-file ownership from current main, including older releases and stack branches', async () => {
  for (const [pr, files, expected] of [
    [release(), [{ filename: 'backend/version.txt', status: 'modified' }], 'success'],
    [makePr(), [sourceFile], 'failure'],
    [makePr({ user: { login: 'bjerrecs' }, base: { ref: 'codex/parent', sha: 'unmerged-parent' } }), [sourceFile], 'failure'],
  ]) {
    const mock = mockGitHub({ prs: [pr], files, content: fs.readFileSync('.github/CODEOWNERS', 'utf8'), reviews: [review('APPROVED', head, 'bjerrecs')] });
    await run(mock);
    assert.equal(mock.latest().state, expected);
    assert.equal(mock.contentReads.length, 1);
    assert.equal(mock.contentReads[0].ref, mainHead);
  }
});

test('each stack layer receives its own status; missing approval elsewhere does not fail the updater', async () => {
  const prs = [
    makePr({ number: 1, user: { login: owner } }),
    makePr({ number: 2, base: { ref: 'codex/layer-one', sha: 'parent-one' }, head: { ...makePr().head, sha: '1'.repeat(40) } }),
    makePr({ number: 3, base: { ref: 'codex/layer-two', sha: 'parent-two' }, head: { ...makePr().head, sha: '2'.repeat(40) } }),
  ];
  const mock = mockGitHub({ prs, reviews: number => number === 2 ? [review('APPROVED', prs[1].head.sha)] : [] });
  await run(mock);
  assert.deepEqual(prs.map(pr => mock.latest(pr.head.sha).state), ['success', 'success', 'failure']);
  assert.equal(mock.createdChecks.length, 0, 'Owner results must not inherit an Actions check suite');
  assert.equal(mock.pagination.find(args => args.state === 'open').base, undefined);
  assert.equal(mock.contentReads.length, 1);
  assert.equal(mock.failures.length, 0);
});

test('event and scheduled refreshes update approvals and dismissals without redundant status history', async () => {
  for (const eventName of ['workflow_run', 'schedule']) {
    let reviews = [];
    const mock = mockGitHub({ reviews: () => reviews });
    mock.context.eventName = eventName;
    mock.context.payload = { workflow_run: { name: 'Review policy events', pull_requests: [makePr()] } };
    for (const [updated, expected] of [
      [[], 'failure'], [[review('APPROVED')], 'success'],
      [[review('APPROVED'), review('DISMISSED')], 'failure'],
    ]) {
      reviews = updated;
      await run(mock);
      assert.equal(mock.latest().state, expected);
      assert.equal(mock.latest().context, 'Code owner review');
      assert.equal(mock.latest().target_url, `https://github.com/flightstrips/FlightStrips/actions/runs/${mock.context.runId}`);
      const written = mock.reviewWrites.length;
      mock.context.runId++;
      await run(mock);
      assert.equal(mock.reviewWrites.length, written, 'A changed workflow URL must not append unchanged decisions');
    }
    assert.deepEqual(mock.reviewWrites.map(status => status.state), ['pending', 'failure', 'success', 'failure']);
    assert.equal(mock.failures.length, 0);
  }
});

test('600 global refreshes do not exhaust the 1,000-status per-commit limit', async () => {
  const mock = mockGitHub({ reviews: [review('APPROVED')] });
  for (let refresh = 0; refresh < 600; refresh++) {
    mock.context.runId++;
    await run(mock);
  }
  assert.deepEqual(mock.reviewWrites.map(status => status.state), ['pending', 'success']);
  assert.equal(mock.createdChecks.length, 0);
  assert.equal(mock.failures.length, 0);
});

test('success is reported independently of existing canceled workflow check suites', async () => {
  const legacy = { name: REVIEW_CHECK_NAME, conclusion: 'success', check_suite: { conclusion: 'cancelled' } };
  const mock = mockGitHub({ reviews: [review('APPROVED')], checks: [legacy] });
  await run(mock);
  assert.equal(mock.latest().state, 'success');
  assert.equal(mock.createdChecks.length, 0);
  assert.equal(mock.checkRuns[0].check_suite.conclusion, 'cancelled');
});

test('status history uses the newest matching context and preserves unrelated statuses', async () => {
  const mock = mockGitHub({ reviews: [review('APPROVED')], statuses: [
    { context: 'Unrelated', state: 'failure' },
    { state: 'failure', description: 'Review dismissed' },
    { state: 'success', description: 'Code owners approved the latest commit for all changed files.' },
  ] });
  await run(mock);
  assert.equal(mock.latest().state, 'success');
  assert.equal(mock.reviewWrites.length, 1);
  assert.equal(mock.statusHistory.find(status => status.context === 'Unrelated').state, 'failure');
});

test('a new head receives a separate status without moving old results', async () => {
  const prs = [makePr({ user: { login: owner } })];
  const mock = mockGitHub({ prs });
  await run(mock);
  prs[0] = makePr({ user: { login: owner }, head: { ...prs[0].head, sha: 'c'.repeat(40) } });
  await run(mock);
  assert.equal(mock.latest(head).state, 'success');
  assert.equal(mock.latest(prs[0].head.sha).state, 'success');
  assert.deepEqual(mock.reviewWrites.map(status => status.sha), [head, head, prs[0].head.sha, prs[0].head.sha]);
});

test('lookup and publication failures are isolated to their commit', async () => {
  const failedSha = 'c'.repeat(40);
  const prs = [makePr({ number: 1, user: { login: owner }, head: { ...makePr().head, sha: failedSha } }),
    makePr({ number: 2, user: { login: owner } })];
  for (const phase of ['list', 'pending', 'success']) {
    const mock = mockGitHub({ prs, statusError: (operation, status) => {
      if (status.sha === failedSha && (operation === phase || status.state === phase)) throw new Error('Status API unavailable');
    } });
    await run(mock);
    assert.equal(mock.latest(head).state, 'success');
    assert.notEqual(mock.latest(failedSha)?.state, 'success');
    assert.ok(mock.failures.some(message => /Unable to publish/.test(message)));
  }
});

test('read failures revoke previous success and do not abort other heads', async () => {
  const failedSha = 'c'.repeat(40);
  const prs = [makePr({ number: 1, head: { ...makePr().head, sha: failedSha } }), makePr({ number: 2, user: { login: owner } })];
  for (const failedRead of [1, 2]) {
    let reads = 0;
    const mock = mockGitHub({ prs, statuses: [{ sha: failedSha, state: 'success' }], getPr: number => {
      if (number === 1 && ++reads === failedRead) throw new Error('PR API unavailable');
      return prs.find(pr => pr.number === number);
    } });
    await run(mock);
    assert.equal(mock.latest(failedSha).state, 'error');
    assert.equal(mock.latest(head).state, 'success');
    assert.equal(mock.failures.length, 1);
  }
  for (const options of [{ reviewError: true }, { content: '/backend/ @someone' }]) {
    const mock = mockGitHub({ ...options, statuses: [{ state: 'success' }] });
    await run(mock);
    assert.equal(mock.latest().state, 'error');
    assert.equal(mock.failures.length, 1);
  }
});

test('changes to main, head, target or PR state during evaluation cannot receive success', async () => {
  const initial = makePr();
  for (const latest of [
    makePr({ head: { ...initial.head, sha: 'new-head' } }),
    makePr({ base: { ...initial.base, sha: 'new-base' } }),
    makePr({ base: { ...initial.base, ref: 'other' } }), makePr({ state: 'closed' }),
  ]) {
    let reads = 0;
    const mock = mockGitHub({ reviews: [review('APPROVED')], getPr: () => ++reads === 1 ? initial : latest });
    await run(mock);
    assert.equal(mock.latest().state, 'failure');
  }
  let reads = 0;
  const mock = mockGitHub({ reviews: [review('APPROVED')], getMain: () => ({ commit: { sha: ++reads === 1 ? mainHead : 'new-main' } }) });
  await run(mock);
  assert.equal(mock.latest().state, 'failure');
});

test('shared heads cannot borrow author exemptions; notifier changes revoke existing success', async () => {
  const shared = mockGitHub({ prs: [makePr({ number: 1, user: { login: owner } }), makePr({ number: 2 })] });
  await run(shared);
  assert.equal(shared.latest().state, 'failure');
  const notifier = mockGitHub({ files: [{ filename: notifierPath, status: 'removed' }], reviews: [review('APPROVED')], statuses: [{ state: 'success' }] });
  await run(notifier);
  assert.equal(notifier.latest().state, 'failure');
  assert.match(notifier.latest().description, /Review notifier changes/);
});

test('unsafe release changes never waive CI; skip publication failure fails only that head', async () => {
  const unsafe = mockGitHub({ prs: [release()], reviews: [review('APPROVED')] });
  await run(unsafe);
  assert.equal(unsafe.latest().state, 'failure');
  assert.equal(unsafe.createdChecks.length, 0);
  const failedSha = 'c'.repeat(40);
  const pr = release();
  const mock = mockGitHub({
    prs: [{ ...pr, number: 1, head: { ...pr.head, sha: failedSha } }, makePr({ number: 2, user: { login: owner } })],
    files: [{ filename: 'backend/version.txt', status: 'modified' }], checkError: () => { throw new Error('Check API unavailable'); },
  });
  await run(mock);
  assert.equal(mock.latest(failedSha).state, 'error');
  assert.equal(mock.latest(head).state, 'success');
});

test('review notifier and fallback remain enabled; manual validation selects code only explicitly', async () => {
  const notifier = fs.readFileSync(notifierPath, 'utf8');
  assert.match(notifier, /pull_request_review:\s*types: \[submitted, edited, dismissed\]/);
  const writer = fs.readFileSync('.github/workflows/review-policy.yml', 'utf8');
  assert.match(writer, /workflow_run:\s*workflows: \[Review policy events, Release Please\]\s*types: \[completed\]/);
  assert.match(writer, /schedule:\s*(?:#[^\r\n]*\s*)?- cron: '17 \* \* \* \*'/);
  assert.match(writer, /push:\s*branches: \[main\]\s*paths:\s*- '\.github\/CODEOWNERS'\s*- '\.github\/scripts\/review-policy.cjs'\s*- '\.github\/workflows\/review-policy\*\.yml'/);
  assert.match(writer, /concurrency:\s*group: review-policy\s*cancel-in-progress: false\s*queue: max/);
  assert.match(writer, /pull_request_target:\s*branches: \[main\]/);
  assert.match(writer, /statuses: write/);
  assert.match(writer, /ref: \$\{\{ github.event_name == 'workflow_dispatch' && github.sha \|\| 'main' \}\}/);
  assert.doesNotMatch(writer, /environment:|secrets\.|vars\.|create-github-app-token/);
  const script = writer.split('          script: |')[1].trim();
  const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;
  const requireFromRoot = require('node:module').createRequire(require('node:path').resolve('github-script.cjs'));
  const mock = mockGitHub({ reviews: [review('APPROVED')] });
  await new AsyncFunction('github', 'context', 'core', 'require', script)(mock.github, mock.context, mock.core, requireFromRoot);
  assert.equal(mock.latest().state, 'success');
});

test('PR events read reviews and files only for affected heads, independent of unrelated PR count', async () => {
  const target = makePr();
  const unrelated = Array.from({ length: 100 }, (_, i) => makePr({
    number: i + 1, head: { ...target.head, sha: `unrelated-${i}` },
  }));
  const mock = mockGitHub({ prs: [target, ...unrelated], reviews: [review('APPROVED')] });
  mock.context.eventName = 'pull_request_target';
  mock.context.payload = { action: 'synchronize', pull_request: target };
  await run(mock);
  assert.equal(mock.latest().state, 'success');
  assert.deepEqual(mock.prReads, [target.number, target.number]);
  assert.equal(mock.pagination.length, 4, 'One open-PR listing, one current-status read, reviews and files');
  assert.equal(mock.contentReads.length, 1);
  assert.ok(mock.reviewWrites.every(status => status.sha === head));
});

test('targeted review refreshes evaluate every open PR sharing a head', async () => {
  const target = makePr({ number: 1, user: { login: owner } });
  const sibling = makePr({ number: 2 });
  const unrelated = makePr({ number: 3, head: { ...target.head, sha: 'unrelated' } });
  const mock = mockGitHub({ prs: [target, sibling, unrelated] });
  mock.context.eventName = 'workflow_run';
  mock.context.payload = { workflow_run: { name: 'Review policy events', pull_requests: [target] } };
  await run(mock);
  assert.equal(mock.latest().state, 'failure', 'A shared commit cannot borrow the target author exemption');
  assert.deepEqual(mock.prReads, [1, 2, 1, 2]);
  assert.equal(mock.latest('unrelated'), undefined);
});

test('delayed review notifications follow the current PR head and preserve old shared heads', async () => {
  const previous = makePr();
  const current = makePr({ head: { ...previous.head, sha: 'new-head' } });
  const sibling = makePr({ number: 2 });
  const mock = mockGitHub({ prs: [current, sibling] });
  mock.context.eventName = 'workflow_run';
  mock.context.payload = { workflow_run: { name: 'Review policy events', pull_requests: [previous] } };
  await run(mock);
  assert.equal(mock.latest('new-head').state, 'failure');
  assert.equal(mock.latest(head).state, 'failure');
});

test('synchronize and closed events refresh PRs remaining on the old shared head', async () => {
  const previous = makePr();
  const sibling = makePr({ number: 2 });
  const current = makePr({ head: { ...previous.head, sha: 'new-head' } });
  for (const action of ['synchronize', 'closed']) {
    const mock = mockGitHub({ prs: action === 'closed' ? [sibling] : [current, sibling] });
    mock.context.eventName = 'pull_request_target';
    mock.context.payload = { action, before: head, pull_request: action === 'closed' ? previous : current };
    await run(mock);
    assert.equal(mock.latest(head).state, 'failure');
    assert.equal(mock.latest('new-head')?.state, action === 'closed' ? undefined : 'failure');
  }
});

test('fork review notifications without PR metadata route by branch without sweeping unrelated PRs', async () => {
  const fork = makePr({ head: { ...makePr().head, repo: { full_name: 'fork/FlightStrips' } } });
  const unrelated = makePr({ number: 2, head: { ...fork.head, ref: 'other', sha: 'unrelated' } });
  for (const head_sha of [head, 'merge-commit']) {
    const mock = mockGitHub({ prs: [fork, unrelated], reviews: [review('APPROVED')] });
    mock.context.eventName = 'workflow_run';
    mock.context.payload = { workflow_run: { name: 'Review policy events', pull_requests: [], head_sha, head_branch: 'feature' } };
    await run(mock);
    assert.equal(mock.latest().state, 'success');
    assert.deepEqual(mock.prReads, [fork.number, fork.number]);
    assert.equal(mock.latest('unrelated'), undefined);
  }
});

test('Release Please completion refreshes only the real release PR and its shared head', async () => {
  const pr = release();
  const sibling = makePr({ number: 3 });
  const unrelated = makePr({ number: 2, head: { ...pr.head, sha: 'unrelated' }, user: { login: owner } });
  const mock = mockGitHub({ prs: [pr, sibling, unrelated], files: [{ filename: 'backend/version.txt', status: 'modified' }] });
  mock.context.eventName = 'workflow_run';
  mock.context.payload = { workflow_run: { name: 'Release Please', pull_requests: [] } };
  await run(mock);
  assert.deepEqual(mock.prReads, [pr.number, sibling.number, pr.number, sibling.number]);
  assert.equal(mock.latest().state, 'failure');
  assert.equal(mock.createdChecks.length, 2);
  assert.equal(mock.latest('unrelated'), undefined);
});

test('notifications with no remaining target do not read ownership, reviews or statuses', async () => {
  for (const workflow_run of [
    { name: 'Release Please' },
    { name: 'Review policy events', pull_requests: [{ number: 99, head: { sha: 'closed-head' } }] },
    { name: 'Review policy events', pull_requests: [], head_sha: 'closed-head', head_branch: 'closed-branch' },
  ]) {
    const mock = mockGitHub();
    mock.context.eventName = 'workflow_run';
    mock.context.payload = { workflow_run };
    await run(mock);
    assert.equal(mock.pagination.length, 1, 'Only the lightweight open-PR list is needed');
    assert.equal(mock.contentReads.length, 0);
    assert.equal(mock.prReads.length, 0);
    assert.equal(mock.reviewWrites.length, 0);
  }
});

test('title and body edits make no API calls; retargeting still refreshes the PR', async () => {
  for (const field of ['title', 'body']) {
    const mock = mockGitHub();
    mock.context.eventName = 'pull_request_target';
    mock.context.payload = { action: 'edited', pull_request: makePr(), changes: { [field]: { from: 'previous' } } };
    await run(mock);
    assert.equal(mock.pagination.length, 0);
    assert.equal(mock.prReads.length, 0);
    assert.equal(mock.contentReads.length, 0);
  }
  const mock = mockGitHub();
  mock.context.eventName = 'pull_request_target';
  mock.context.payload = { action: 'edited', pull_request: makePr(), changes: { base: { ref: { from: 'old-base' } } } };
  await run(mock);
  assert.equal(mock.latest().state, 'failure');
});

test('unroutable notifications fail without evaluating every open PR', async () => {
  for (const payload of [{}, { workflow_run: { name: 'Unexpected' } }, { workflow_run: { name: 'Review policy events' } }]) {
    const mock = mockGitHub();
    mock.context.eventName = 'workflow_run';
    mock.context.payload = payload;
    await assert.rejects(run(mock), /notification/);
    assert.equal(mock.prReads.length, 0);
    assert.equal(mock.reviewWrites.length, 0);
  }
});

test('current status lookup does not paginate through thousands of historical decisions', async () => {
  const description = 'Code owners approved the latest commit for all changed files.';
  const mock = mockGitHub({ reviews: [review('APPROVED')], statuses: [
    { state: 'success', description },
    ...Array.from({ length: 2000 }, () => ({ state: 'failure', description: 'Old decision' })),
  ] });
  // No historical-status endpoint is provided by the mock: using it must fail.
  await run(mock);
  assert.equal(mock.latest().state, 'success');
  assert.equal(mock.reviewWrites.length, 0);
  assert.equal(mock.pagination.filter(args => args.ref === head).length, 1);
  assert.equal(mock.failures.length, 0);
});

test('nested combined-status pages preserve an existing decision beyond the first page', async () => {
  const description = 'Code owners approved the latest commit for all changed files.';
  const mock = mockGitHub({ reviews: [review('APPROVED')], statuses: [
    ...Array.from({ length: 101 }, (_, i) => ({ context: `Other check ${i}`, state: 'success' })),
    { state: 'success', description },
  ] });
  await run(mock);
  await run(mock);
  assert.equal(mock.latest().description, description);
  assert.equal(mock.reviewWrites.length, 0, 'Unchanged nested statuses must not produce pending/final pairs');
  assert.equal(mock.failures.length, 0);
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
