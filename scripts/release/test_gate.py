"""No real GitHub, registry, release or Discord writes: adapters are fixtures."""
import copy
import hashlib
from pathlib import Path
import unittest

from google.protobuf import text_format
import gate as g

REPO = 'flightstrips/FlightStrips'
HEAD, TREE, MERGE = 'a' * 40, 'b' * 40, 'c' * 40
RUN, ATTEMPT = 100, 1


def fixture():
    files = {name: ('fixture bytes: ' + name).encode() for name in g.BUNDLE}
    m = g.pb.CandidateManifest(schema_revision=1, repository=REPO, release_pr_number=42,
        release_head_sha=HEAD, release_tree_sha=TREE, protocol_revision=2,
        workflow_run_id=RUN, workflow_run_attempt=ATTEMPT)
    m.created_at.FromJsonString('2026-09-30T12:00:00Z')
    for k in g.VERSION_PATHS:
        setattr(m.versions, k, '4.0.0')
    for k, digit, suffix in [('backend', 'd', 'backend-api'), ('frontend', 'e', 'frontend')]:
        image = getattr(m, k)
        image.repository = 'ghcr.io/flightstrips/' + suffix
        image.digest = 'sha256:' + digit * 64
        image.candidate_tag = g.candidate_tag('4.0.0', TREE, RUN, ATTEMPT)
    m.plugin.artifact_name = g.plugin_name(RUN, ATTEMPT)
    m.plugin.artifact_id = 201
    for name, data in sorted(files.items()):
        m.plugin.files.add(path=name, sha256=hashlib.sha256(data).hexdigest(), size_bytes=len(data))
    return m, files


class FakeOCI:
    def __init__(self, m, pinned=False):
        self.writes = []
        self.digests, self.labels = {}, {}
        for k in ('backend', 'frontend'):
            image = getattr(m, k)
            self.digests[f'{image.repository}:{image.candidate_tag}'] = image.digest
            self.digests[f'{image.repository}@{image.digest}'] = image.digest
            if pinned:
                self.digests[f'{image.repository}:{getattr(m.versions, k)}'] = image.digest
            self.labels[f'{image.repository}@{image.digest}'] = {
                'org.opencontainers.image.source': f'https://github.com/{m.repository}',
                'org.opencontainers.image.revision': m.release_tree_sha,
                'org.opencontainers.image.version': getattr(m.versions, k) + '+' + m.release_tree_sha}

    def digest(self, ref):
        return self.digests.get(ref)

    def identity(self, ref):
        return self.labels[ref]

    def promote(self, ref, tag):
        self.writes.append((ref, tag))
        repo, digest = ref.split('@')
        self.digests[f'{repo}:{tag}'] = digest


class FakeGitHub:
    repository = REPO

    def __init__(self, m, files):
        self.m = m
        self.files = files
        self.writes = []
        self.assets = []
        self.bytes = {}
        self.fail_complete = False
        repo = {'full_name': REPO, 'id': 77}
        pr = {'number': 42, 'base': {'ref': 'main', 'repo': repo},
              'head': {'sha': HEAD, 'ref': 'release-please--branches--main', 'repo': repo},
              'user': {'login': 'github-actions[bot]'}, 'merged': True, 'state': 'closed',
              'labels': [{'name': 'autorelease: pending'}], 'merge_commit_sha': MERGE,
              'merged_at': '2026-09-30T12:00:00Z'}
        run = {'id': RUN, 'run_attempt': ATTEMPT, 'workflow_id': 9, 'head_sha': HEAD,
               'event': 'workflow_dispatch', 'path': g.WORKFLOW, 'repository': repo,
               'head_repository': repo, 'head_branch': pr['head']['ref'], 'conclusion': 'success'}
        self.routes = {'pulls/42': pr, 'actions/workflows/release-candidate.yml': {'id': 9},
                       f'actions/runs/{RUN}/attempts/{ATTEMPT}': run,
                       f'git/commits/{HEAD}': {'tree': {'sha': TREE}},
                       f'git/commits/{MERGE}': {'tree': {'sha': TREE}}}
        self.contents = {(g.WORKFLOW, HEAD): b'trusted workflow',
                         (g.WORKFLOW, 'main'): b'trusted workflow'}
        for path in ('scripts/release/gate.py', 'scripts/release/release/v1/candidate_pb2.py',
                     'proto/release/v1/candidate.proto'):
            self.contents[path, HEAD] = b'trusted helper'
            self.contents[path, 'main'] = b'trusted helper'
        self.version_data = {HEAD: {k: '4.0.0' for k in g.VERSION_PATHS},
                             MERGE: {k: '4.0.0' for k in g.VERSION_PATHS}}
        origin = {'id': RUN, 'head_sha': HEAD, 'repository_id': 77, 'head_repository_id': 77}
        self.artifacts = [dict(id=200, name=g.manifest_name(RUN, ATTEMPT), expired=False, workflow_run=copy.deepcopy(origin)),
                          dict(id=201, name=g.plugin_name(RUN, ATTEMPT), expired=False, workflow_run=copy.deepcopy(origin))]
        self.manifest = {'candidate-manifest.pb': g.encode(m), 'candidate-manifest.textproto': b'not authoritative'}
        for tag, rid in [('plugin/v4.0.0', 301), ('frontend/v4.0.0', 302)]:
            self.routes[f'git/ref/tags/{tag}'] = {'object': {'type': 'commit', 'sha': MERGE}}
            self.routes[f'releases/tags/{tag}'] = {'id': rid, 'tag_name': tag, 'draft': False,
                'html_url': f'https://github.com/{REPO}/releases/tag/{tag}'}
        self.routes['actions/runs/500'] = {'id': 500, 'path': '.github/workflows/release-cutover.yml',
            'event': 'workflow_dispatch', 'head_branch': 'main', 'repository': repo, 'head_repository': repo}

    def get(self, path):
        return copy.deepcopy(self.routes[path])

    def pages(self, path, key=None):
        if path == f'actions/runs/{RUN}/artifacts':
            return copy.deepcopy(self.artifacts)
        if path == f'commits/{MERGE}/pulls':
            return [self.get('pulls/42')]
        if path.startswith('releases/'):
            return copy.deepcopy(self.assets if path == 'releases/301/assets' else [])
        raise AssertionError(path)

    def content(self, path, sha):
        return self.contents.get((path, sha), b'changed source workflow')

    def tree(self, sha):
        return self.get(f'git/commits/{sha}')['tree']['sha']

    def versions(self, sha):
        return self.version_data[sha]

    def artifact(self, a):
        g.require(not a['expired'], 'expired artifact')
        return self.manifest if a['id'] == 200 else self.files

    def asset(self, a):
        return self.bytes[a['id']]

    def attach(self, release, name, data):
        if self.fail_complete and name.endswith('-complete.pb'):
            raise g.Rejected('upload outcome unknown')
        self.writes.append((name, data))
        self.add_asset(name, data)

    def add_asset(self, name, data):
        i = 1000 + len(self.assets)
        self.assets.append({'id': i, 'name': name})
        self.bytes[i] = data


class GateTests(unittest.TestCase):
    def setUp(self):
        self.m, self.files = fixture()
        self.gh = FakeGitHub(self.m, self.files)
        self.oci = FakeOCI(self.m)
        self.calls = []
        self.dll = hashlib.sha256(self.files['FlightStripsPlugin.dll']).hexdigest()

    def load(self, merged=True):
        return g.load_candidate(self.gh, 42, RUN, ATTEMPT, merged)

    def call_cutover(self, dry=True):
        return g.cutover(self.m, self.files, self.gh, self.oci, 'plugin/v4.0.0',
                         self.dll, 500, self.discord, dry_run=dry)

    def discord(self, release):
        self.calls.append(release['tag_name'])
        return '1234567890'

    def assert_no_writes(self):
        self.assertEqual(self.gh.writes, [])
        self.assertEqual(self.oci.writes, [])
        self.assertEqual(self.calls, [])

    def test_binary_roundtrip_and_schema_contract(self):
        g.validate(g.decode(g.pb.CandidateManifest, g.encode(self.m)), REPO)
        expected = {
            'CandidateManifest': ['schema_revision', 'repository', 'release_pr_number', 'release_head_sha',
                'release_tree_sha', 'versions', 'backend', 'frontend', 'plugin', 'protocol_revision',
                'created_at', 'workflow_run_id', 'workflow_run_attempt'],
            'ComponentVersions': ['backend', 'frontend', 'plugin', 'docs'],
            'OciArtifact': ['repository', 'digest', 'candidate_tag'],
            'PluginArtifact': ['artifact_name', 'artifact_id', 'files'],
            'ArtifactFile': ['path', 'sha256', 'size_bytes'],
            'CutoverReceipt': ['release_tree_sha', 'plugin_release_tag', 'dll_sha256', 'frontend_release_tag',
                'workflow_run_id', 'phase', 'recorded_at', 'discord_message_id']}
        for name, fields in expected.items():
            desc = getattr(g.pb, name).DESCRIPTOR
            self.assertEqual([(f.number, f.name) for f in desc.fields], list(enumerate(fields, 1)))
        self.assertEqual([(v.name, v.number) for v in g.pb.CutoverPhase.DESCRIPTOR.values],
                         [('CUTOVER_PHASE_UNSPECIFIED', 0), ('CUTOVER_PHASE_ANNOUNCEMENT_INTENT', 1),
                          ('CUTOVER_PHASE_COMPLETE', 2)])

    def test_unknown_fields_and_noncanonical_wire_rejected(self):
        with self.assertRaises(g.Rejected):
            g.decode(g.pb.CandidateManifest, g.encode(self.m) + b'\xa0\x06\x01')
        with self.assertRaises(g.Rejected):
            g.decode(g.pb.CandidateManifest, g.encode(self.m) + b'\x08\x01')

    def test_metadata_rejections(self):
        for field, bad in [('schema_revision', 2), ('protocol_revision', 1), ('repository', 'attacker/repo'),
                           ('release_head_sha', 'A' * 40), ('release_tree_sha', 'short'),
                           ('workflow_run_id', 0), ('workflow_run_attempt', 0), ('release_pr_number', 0)]:
            with self.subTest(field=field):
                m = copy.deepcopy(self.m)
                setattr(m, field, bad)
                with self.assertRaises(g.Rejected):
                    g.validate(m, REPO)
        for path in ('../FlightStripsPlugin.dll', '/FlightStripsPlugin.dll', 'a\\b', 'a/./b', 'C:/a'):
            m = copy.deepcopy(self.m)
            m.plugin.files[0].path = path
            with self.assertRaises(g.Rejected):
                g.validate(m, REPO)
        m = copy.deepcopy(self.m)
        m.ClearField('created_at')
        with self.assertRaises(g.Rejected):
            g.validate(m, REPO)
        self.assert_no_writes()

    def test_load_proves_origin_and_ignores_textproto(self):
        m, files, pr = self.load()
        self.assertEqual(g.encode(m), g.encode(self.m))
        self.assertEqual(files, self.files)
        self.assertEqual(g.merged_pr(self.gh, MERGE), 42)
        self.assert_no_writes()

    def test_staging_load_on_open_release_pr(self):
        self.gh.routes['pulls/42'].update(merged=False, state='open')
        self.load(merged=False)
        v = g.prepare(self.gh, 42, RUN, ATTEMPT)
        self.assertEqual(v['tree'], TREE)
        self.assert_no_writes()

    def test_candidate_origin_source_and_version_rejections_before_writes(self):
        mutations = [
            lambda: self.gh.routes['pulls/42']['head'].update(sha='f' * 40),
            lambda: self.gh.routes['pulls/42']['head']['repo'].update(full_name='wrong/repo'),
            lambda: self.gh.routes['pulls/42']['user'].update(login='human'),
            lambda: self.gh.routes['pulls/42']['base'].update(ref='other'),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(head_sha='f' * 40),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(id=999),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(run_attempt=2),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(path='evil.yml'),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(event='pull_request'),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(workflow_id=8),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1'].update(conclusion='failure'),
            lambda: self.gh.routes[f'actions/runs/{RUN}/attempts/1']['repository'].update(full_name='wrong/repo'),
            lambda: self.gh.routes[f'git/commits/{MERGE}']['tree'].update(sha='f' * 40),
            lambda: self.gh.version_data[MERGE].update(frontend='5.0.0'),
            lambda: self.gh.contents.update({(g.WORKFLOW, HEAD): b'tampered workflow'}),
            lambda: self.gh.artifacts[0]['workflow_run'].update(id=999),
            lambda: self.gh.artifacts[1]['workflow_run'].update(head_sha='f' * 40),
            lambda: self.gh.artifacts[1]['workflow_run'].update(repository_id=88),
            lambda: self.gh.artifacts[1].update(expired=True),
            lambda: self.gh.artifacts[1].update(id=999),
            lambda: self.gh.files.update({'FlightStripsPluginCore.dll': b'wrong bytes'})]
        for i, mutate in enumerate(mutations):
            with self.subTest(case=i):
                self.setUp()
                mutate()
                with self.assertRaises(g.Rejected):
                    self.load()
                self.assert_no_writes()

    def test_manifest_inconsistency_rejected(self):
        for field, value in [('workflow_run_id', 999), ('workflow_run_attempt', 2), ('release_pr_number', 43),
                             ('release_head_sha', 'f' * 40), ('release_tree_sha', 'f' * 40)]:
            with self.subTest(field=field):
                self.setUp()
                setattr(self.m, field, value)
                self.gh.manifest['candidate-manifest.pb'] = g.encode(self.m)
                with self.assertRaises(g.Rejected):
                    self.load()
                self.assert_no_writes()

    def test_missing_ambiguous_artifacts_and_hash_size_drift(self):
        changes = [
            lambda: self.gh.artifacts.pop(),
            lambda: self.gh.artifacts.append(copy.deepcopy(self.gh.artifacts[0])),
            lambda: self.files.pop('flightstrips_config.ini'),
            lambda: self.files.update({'FlightStripsPlugin.dll': b'x' * len(self.files['FlightStripsPlugin.dll'])}),
            lambda: self.m.plugin.files[0].ClearField('size_bytes'),
            lambda: setattr(self.m.plugin.files[0], 'sha256', 'A' * 64),
            lambda: self.m.plugin.files.add().CopyFrom(self.m.plugin.files[0])]
        for i, mutate in enumerate(changes):
            with self.subTest(case=i):
                self.setUp()
                mutate()
                self.gh.manifest['candidate-manifest.pb'] = g.encode(self.m)
                with self.assertRaises(g.Rejected):
                    self.load()
                self.assert_no_writes()

    def test_explicit_qualification_required(self):
        with self.assertRaises(g.Rejected):
            g.load_candidate(self.gh, 42, 0, 0)
        self.assert_no_writes()

    def test_same_digest_promotion_and_retry(self):
        m, files, _ = self.load()
        result = g.promote(m, files, self.oci, dry_run=False)
        self.assertEqual(len(self.oci.writes), 2)
        self.assertTrue(all('@sha256:' in ref for ref, tag in self.oci.writes))
        self.assertTrue(result['backend_image'].endswith('@' + m.backend.digest))
        g.promote(m, files, self.oci, dry_run=False)
        self.assertEqual(len(self.oci.writes), 2)

    def test_promotion_dry_run_zero_writes(self):
        g.promote(self.m, self.files, self.oci)
        self.assert_no_writes()

    def test_digest_identity_and_second_destination_drift_precede_writes(self):
        changes = [
            lambda: self.oci.digests.update({f'{self.m.frontend.repository}:4.0.0': 'sha256:' + 'f' * 64}),
            lambda: self.oci.digests.update({f'{self.m.frontend.repository}:{self.m.frontend.candidate_tag}': 'sha256:' + 'f' * 64}),
            lambda: self.oci.labels[f'{self.m.frontend.repository}@{self.m.frontend.digest}'].update({'org.opencontainers.image.revision': 'f' * 40}),
            lambda: self.oci.labels[f'{self.m.frontend.repository}@{self.m.frontend.digest}'].update({'org.opencontainers.image.version': '5.0.0+' + TREE}),
            lambda: self.oci.labels[f'{self.m.frontend.repository}@{self.m.frontend.digest}'].update({'org.opencontainers.image.source': 'https://github.com/wrong/repo'}),
            lambda: self.files.update({'FlightStripsPlugin.dll': b'wrong'})]
        for i, mutate in enumerate(changes):
            with self.subTest(case=i):
                self.setUp()
                mutate()
                with self.assertRaises(g.Rejected):
                    g.promote(self.m, self.files, self.oci, dry_run=False)
                self.assert_no_writes()

    def test_cutover_dry_run_zero_writes_reports_exact_bundle(self):
        self.oci = FakeOCI(self.m, pinned=True)
        report = self.call_cutover()
        self.assertEqual(report['dll_sha256'], self.dll)
        self.assertEqual(set(report['attachments'].split(',')), set(g.BUNDLE))
        self.assertEqual(report['frontend_release_tag'], 'frontend/v4.0.0')
        self.assert_no_writes()

    def test_cutover_input_and_release_drift_before_writes(self):
        self.oci = FakeOCI(self.m, pinned=True)
        for tag, dll in [('plugin/v4.0.1', self.dll), ('plugin/v4.0.0', 'f' * 64)]:
            with self.assertRaises(g.Rejected):
                g.cutover(self.m, self.files, self.gh, self.oci, tag, dll, 500, self.discord, False)
        self.gh.routes[f'git/commits/{MERGE}']['tree']['sha'] = 'f' * 40
        with self.assertRaises(g.Rejected):
            self.call_cutover(False)
        self.assert_no_writes()

    def test_cutover_promoted_digest_required(self):
        with self.assertRaises(g.Rejected):
            self.call_cutover(False)
        self.assert_no_writes()

    def test_conflicting_assets_fail_before_any_attachment(self):
        self.oci = FakeOCI(self.m, pinned=True)
        self.gh.add_asset('FlightStripsPluginCore.dll', b'wrong')
        with self.assertRaises(g.Rejected):
            self.call_cutover(False)
        self.assert_no_writes()

    def test_cutover_completion_retry_preserves_unrelated_assets(self):
        self.oci = FakeOCI(self.m, pinned=True)
        self.gh.add_asset('release-notes.txt', b'unrelated')
        self.call_cutover(False)
        self.assertEqual(self.calls, ['frontend/v4.0.0'])
        self.assertEqual(len(self.gh.writes), 6)
        complete = g.decode(g.pb.CutoverReceipt, self.gh.writes[-1][1])
        self.assertEqual(complete.discord_message_id, '1234567890')
        self.call_cutover(False)
        self.assertEqual(len(self.gh.writes), 6)
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(self.gh.bytes[1000], b'unrelated')
        # Retry can complete a missing attachment without announcing again.
        self.gh.assets = [a for a in self.gh.assets if a['name'] != 'flightstrips_config_dev.ini']
        self.call_cutover(False)
        self.assertEqual(len(self.gh.writes), 7)
        self.assertEqual(len(self.calls), 1)

    def test_unknown_discord_and_lost_completion_block_resend(self):
        for failure in ('discord', 'completion'):
            with self.subTest(failure=failure):
                self.setUp()
                self.oci = FakeOCI(self.m, pinned=True)
                if failure == 'discord':
                    def uncertain(release):
                        self.calls.append(release['tag_name'])
                        raise g.Rejected('unknown Discord outcome')
                    self.discord = uncertain
                else:
                    self.gh.fail_complete = True
                with self.assertRaises(g.Rejected):
                    self.call_cutover(False)
                writes = len(self.gh.writes)
                with self.assertRaisesRegex(g.Rejected, 'reconciliation'):
                    self.call_cutover(False)
                self.assertEqual(len(self.gh.writes), writes)
                self.assertEqual(len(self.calls), 1)

    def test_conflicting_receipt_and_completion_without_intent(self):
        self.oci = FakeOCI(self.m, pinned=True)
        r = g.receipt(self.m, 'plugin/v4.0.0', self.dll, 500, 2, '123')
        self.gh.add_asset(f'cutover-{TREE}-500-complete.pb', g.encode(r))
        with self.assertRaises(g.Rejected):
            self.call_cutover(False)
        self.assert_no_writes()
        r.dll_sha256 = 'f' * 64
        self.gh.bytes[1000] = g.encode(r)
        with self.assertRaises(g.Rejected):
            self.call_cutover(False)
        self.assert_no_writes()

    def test_publication_paths(self):
        root = Path(__file__).resolve().parents[2]
        release = (root / '.github/workflows/release-please.yml').read_text()
        discord = (root / '.github/workflows/discord-release.yml').read_text(encoding='utf-8')
        self.assertNotIn('backend-migrate', release)
        self.assertNotIn('DISCORD_WEBHOOK_URL', release)
        self.assertNotIn('action-gh-release', release)
        self.assertNotIn('backend-api:latest', release)
        self.assertNotIn('frontend:latest', release)
        self.assertIn('docs:latest', release)
        self.assertIn('gh pr create', release)
        self.assertIn('BACKEND_IMAGE', release)
        self.assertIn("startsWith(github.event.release.tag_name, 'docs/v')", discord)
        candidate = (root / g.WORKFLOW).read_text()
        self.assertNotIn(':latest', candidate)
        self.assertIn('retention-days: 90', candidate)
        cutover = (root / '.github/workflows/release-cutover.yml').read_text()
        self.assertIn('default: true', cutover)
        self.assertIn('cancel-in-progress: false', cutover)
        self.assertIn("github.ref == 'refs/heads/main'", cutover)
        self.assertNotIn('Dockerfile.Migrate', (root / '.github/workflows/build-backend.yml').read_text())
        self.assertNotIn('upload-artifact', (root / '.github/workflows/build-plugin.yml').read_text())


if __name__ == '__main__':
    unittest.main()
