const assert = require('node:assert/strict');
const fs = require('node:fs');

// Exercise the scripts shipped in the workflows with the same injected parameter
// names as github-script v9. This also catches incompatible parameter declarations.
const AsyncFunction = Object.getPrototypeOf(async function () {}).constructor;

function readScript(file) {
  const lines = fs.readFileSync(file, 'utf8').split(/\r?\n/);
  const start = lines.findIndex(line => /^\s+script: \|\s*$/.test(line));
  assert.notEqual(start, -1, `No script block in ${file}`);
  const indent = lines[start].search(/\S/);
  const body = [];
  for (const line of lines.slice(start + 1)) {
    if (line.trim() && line.search(/\S/) <= indent) break;
    body.push(line.slice(indent + 2));
  }
  return body.join('\n');
}

module.exports = async function () {
  for (const file of ['.github/workflows/discord-release.yml', '.github/workflows/release-please.yml']) {
    const run = new AsyncFunction('github', 'octokit', 'context', 'core', 'glob', 'io', 'exec',
      'require', '__original_require__', 'getOctokit', 'fetch', 'process', readScript(file));
    const cases = [
      { body: 'feat(strip): improve clearance\nfix(session): reconnect', expected: ['Improve clearance', 'Reconnect'] },
      { body: '', expected: ['See full release notes for details.'] },
      {
        body: Array.from({ length: 8 }, (_, i) => `* **strip:** change ${i} ${'details '.repeat(20)} ([#${i}](https://github.com/flightstrips/FlightStrips/issues/${i})) ([abcdefg](https://github.com/flightstrips/FlightStrips/commit/${'a'.repeat(40)}))`).join('\n'),
        expected: ['change 0', 'More changes in the full release notes.'],
      },
      { body: `feat: ${'✈️'.repeat(2000)}`, expected: ['More changes in the full release notes.'] },
      {
        body: Array.from({ length: 9 }, (_, i) => `feat: change ${i}`).join('\n'),
        expected: ['Change 7', 'More changes in the full release notes.'],
      },
    ];
    for (const { body, expected } of cases) {
      for (const ok of [true, false]) {
        const calls = [];
        const failures = [];
        const tag = 'v1.2.3';
        const url = 'https://example.test/releases/v1.2.3';
        const context = {
          repo: { owner: 'flightstrips', repo: 'FlightStrips' },
          payload: { release: { tag_name: tag, name: 'Test release', body, html_url: url } },
        };
        const process = { env: {
          DISCORD_WEBHOOK_URL: 'https://example.test/webhook',
          RELEASE_TAG: tag, RELEASE_VERSION: '1.2.3', RELEASE_BODY: body, RELEASE_URL: url,
        } };
        const core = { info() {}, setFailed: message => failures.push(message) };
        const fetch = async (target, options) => {
          calls.push({ target, options });
          return { ok, status: 503, text: async () => 'unavailable' };
        };
        await run({}, {}, context, core, {}, {}, {}, require, require, () => {}, fetch, process);
        assert.equal(calls.length, 1);
        assert.equal(calls[0].target, process.env.DISCORD_WEBHOOK_URL);
        assert.equal(calls[0].options.method, 'POST');
        const payload = JSON.parse(calls[0].options.body);
        assert.equal(payload.username, 'FlightStrips Releases');
        assert.ok(payload.content.endsWith(`📦 Full release notes: <${url}>`));
        assert.ok(payload.content.length <= 2000, `${file}: ${payload.content.length} characters exceeds Discord's limit`);
        for (const text of expected) assert.ok(payload.content.includes(text), `${file}: missing ${text}`);
        assert.ok(!payload.content.includes('Change 8'));
        assert.equal(failures.length, ok ? 0 : 1);
        if (!ok) assert.match(failures[0], /503: unavailable/);
      }
    }
  }
  console.log('Both announcement scripts passed success, empty-note, length-limit, and webhook-error checks.');
};

if (require.main === module) module.exports().catch(error => { console.error(error); process.exitCode = 1; });
