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
    for (const body of ['feat(strip): improve clearance\nfix(session): reconnect', '']) {
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
        assert.ok(payload.content.includes(url));
        if (body) {
          assert.ok(payload.content.includes('Improve clearance'));
          assert.ok(payload.content.includes('Reconnect'));
        } else {
          assert.ok(payload.content.includes('See full release notes for details.'));
        }
        assert.equal(failures.length, ok ? 0 : 1);
        if (!ok) assert.match(failures[0], /503: unavailable/);
      }
    }
  }
  console.log('Both announcement scripts passed success, empty-note, and webhook-error checks.');
};

if (require.main === module) module.exports().catch(error => { console.error(error); process.exitCode = 1; });
