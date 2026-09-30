import io
import json
import subprocess
import unittest
import warnings
from unittest.mock import patch
import zipfile

import gate as g
from test_gate import REPO


class AdapterTests(unittest.TestCase):
    def test_github_external_json_boundary_and_zero_read_mutations(self):
        with patch('gate.subprocess.run', return_value=subprocess.CompletedProcess([], 0, b'{"id":42}', b'')) as call:
            self.assertEqual(g.GitHub(REPO).get('pulls/42'), {'id': 42})
            args = call.call_args.args[0]
            self.assertEqual(args, ['gh', 'api', '--method', 'GET', f'repos/{REPO}/pulls/42'])

    def test_github_artifact_zip_path_duplicates_and_expiry(self):
        for names in [['a', 'a'], ['../a'], ['/a'], ['C:/a'], ['a\\b'], ['a/./b'], ['dir/']]:
            with self.subTest(names=names):
                out = io.BytesIO()
                with warnings.catch_warnings():
                    warnings.simplefilter('ignore', UserWarning)
                    with zipfile.ZipFile(out, 'w') as z:
                        for n in names:
                            z.writestr(n, b'bytes')
                raw = out.getvalue()
                # Windows' ZIP writer normalizes separators. Craft the on-wire
                # name explicitly so this fixture also tests Linux readers.
                if names == ['a\\b']:
                    raw = raw.replace(b'a/b', b'a\\b')
                with patch.object(g.GitHub, 'raw', return_value=raw):
                    with self.assertRaises(g.Rejected):
                        g.GitHub(REPO).artifact({'id': 10, 'expired': False})
        with patch.object(g.GitHub, 'raw') as call:
            with self.assertRaises(g.Rejected):
                g.GitHub(REPO).artifact({'id': 10, 'expired': True})
            call.assert_not_called()

    def test_registry_missing_tag_distinguished_from_transport_auth_errors(self):
        for error in ['UNAUTHORIZED', 'timeout', 'connection reset']:
            with patch('gate.subprocess.run', return_value=subprocess.CompletedProcess([], 1, '', error)):
                with self.assertRaises(g.Rejected):
                    g.OCI().digest('image:tag')
        with patch('gate.subprocess.run', return_value=subprocess.CompletedProcess([], 1, '', 'MANIFEST_UNKNOWN')):
            self.assertIsNone(g.OCI().digest('image:tag'))

    def test_registry_digest_validation_and_same_digest_tag_command(self):
        digest = 'sha256:' + 'a' * 64
        with patch('gate.subprocess.run', return_value=subprocess.CompletedProcess([], 0, digest + '\n', '')):
            self.assertEqual(g.OCI().digest('image:tag'), digest)
        with patch('gate.subprocess.run', return_value=subprocess.CompletedProcess([], 0, 'wrong', '')):
            with self.assertRaises(g.Rejected):
                g.OCI().digest('image:tag')
        with patch('gate.subprocess.run', return_value=subprocess.CompletedProcess([], 0, b'', b'')) as call:
            g.OCI().promote('image@' + digest, '4.0.0')
            self.assertEqual(call.call_args.args[0], ['crane', 'tag', 'image@' + digest, '4.0.0'])

    def test_discord_wait_confirmation_and_no_retry(self):
        release = {'tag_name': 'frontend/v4.0.0', 'html_url': 'https://github.com/example/release'}
        with patch.dict('os.environ', DISCORD_WEBHOOK_URL='https://discord.com/api/webhooks/fixture/secret'):
            with patch('urllib.request.urlopen') as call:
                call.return_value.__enter__.return_value = io.BytesIO(b'{"id":"1234567890"}')
                self.assertEqual(g.discord_announce(release), '1234567890')
                request = call.call_args.args[0]
                self.assertTrue(request.full_url.endswith('?wait=true'))
                self.assertEqual(json.loads(request.data)['allowed_mentions'], {'parse': []})
            with patch('urllib.request.urlopen', side_effect=TimeoutError('secret must not leak')) as call:
                with self.assertRaisesRegex(g.Rejected, '^Discord outcome unknown'):
                    g.discord_announce(release)
                self.assertEqual(call.call_count, 1)
