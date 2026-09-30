"""Release authority lives in verified GitHub workflow artifacts, never a supplied manifest.

Only the adapters below cross JSON boundaries. All stored metadata is binary protobuf.
"""
import argparse
import base64
import hashlib
import io
import json
import os
from pathlib import Path, PurePosixPath
import re
import subprocess
import tempfile
import zipfile

from google.protobuf import text_format
from release.v1 import candidate_pb2 as pb

WORKFLOW = '.github/workflows/release-candidate.yml'
VERSION_PATHS = dict(backend='backend', frontend='frontend',
                     plugin='euroscope-plugin', docs='docs')
BUNDLE = ('FlightStripsPlugin.dll', 'FlightStripsPluginCore.dll',
          'flightstrips_config.ini', 'flightstrips_config_dev.ini')
SHA = r'[0-9a-f]{40}'
HASH = r'[0-9a-f]{64}'
VERSION = r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?'


class Rejected(RuntimeError):
    pass


def require(condition, reason):
    if not condition:
        raise Rejected(reason)


def matches(pattern, value):
    return re.fullmatch(pattern, value) is not None


def encode(message):
    return message.SerializeToString(deterministic=True)


def decode(kind, data):
    message = kind.FromString(data)
    known = kind()
    known.CopyFrom(message)
    known.DiscardUnknownFields()
    require(encode(known) == data, 'unknown fields or noncanonical protobuf')
    return message


def timestamp(value):
    # ToJsonString validates seconds/nanos, including the UTC range.
    require(value.ToJsonString().endswith('Z'), 'invalid timestamp')


def plugin_name(run, attempt):
    return f'plugin-{run}-{attempt}'


def manifest_name(run, attempt):
    return f'candidate-{run}-{attempt}'


def candidate_tag(version, tree, run, attempt):
    return f'{version}-candidate-{tree}-{run}-{attempt}'


def validate(m, repository):
    require(m.schema_revision == 1 and m.protocol_revision == 2, 'schema/protocol revision')
    require(m.repository == repository and matches(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', repository), 'repository')
    require(matches(SHA, m.release_head_sha) and matches(SHA, m.release_tree_sha), 'Git identity')
    require(m.release_pr_number > 0 and m.workflow_run_id > 0 and m.workflow_run_attempt > 0, 'positive IDs')
    require(m.HasField('created_at'), 'missing timestamp')
    timestamp(m.created_at)
    for component in VERSION_PATHS:
        require(matches(VERSION, getattr(m.versions, component)), 'component version')
    for component, image in [('backend', m.backend), ('frontend', m.frontend)]:
        suffix = 'backend-api' if component == 'backend' else 'frontend'
        require(image.repository == f'ghcr.io/{repository.split("/")[0].lower()}/{suffix}', 'image repository')
        require(matches('sha256:' + HASH, image.digest), 'OCI digest')
        require(image.candidate_tag == candidate_tag(getattr(m.versions, component),
                m.release_tree_sha, m.workflow_run_id, m.workflow_run_attempt), 'candidate tag')
    require(m.plugin.artifact_id > 0 and m.plugin.artifact_name == plugin_name(
        m.workflow_run_id, m.workflow_run_attempt), 'plugin artifact identity')
    paths = [f.path for f in m.plugin.files]
    require(paths == sorted(set(paths)) and set(paths) == set(BUNDLE), 'complete sorted plugin bundle')
    for f in m.plugin.files:
        p = PurePosixPath(f.path)
        require(not p.is_absolute() and str(p) == f.path and '..' not in p.parts
                and '\\' not in f.path and ':' not in f.path, 'artifact path')
        require(matches(HASH, f.sha256) and f.size_bytes > 0, 'file hash/size')


class GitHub:
    def __init__(self, repository):
        self.repository = repository

    def raw(self, path, method='GET', data=None, binary=False, upload=False):
        args = ['gh', 'api', '--method', method, path]
        if binary:
            args += ['-H', 'Accept: application/octet-stream']
        if upload:
            args += ['--hostname', 'uploads.github.com', '-H', 'Content-Type: application/octet-stream']
        if data is not None:
            with tempfile.NamedTemporaryFile(delete=False) as f:
                f.write(data)
                filename = f.name
            args += ['--input', filename]
        try:
            result = subprocess.run(args, capture_output=True)
            require(result.returncode == 0, f'GitHub {method} failed (response withheld)')
            return result.stdout
        finally:
            if data is not None:
                os.unlink(filename)

    def get(self, path):
        return json.loads(self.raw(f'repos/{self.repository}/{path}'))

    def pages(self, path, key=None):
        result = []
        for page in range(1, 1001):
            data = self.get(f'{path}{"&" if "?" in path else "?"}per_page=100&page={page}')
            items = data[key] if key else data
            result.extend(items)
            if len(items) < 100:
                return result
        raise Rejected('pagination limit')

    def content(self, path, ref):
        data = self.get(f'contents/{path}?ref={ref}')
        return base64.b64decode(data['content'])

    def tree(self, sha):
        return self.get(f'git/commits/{sha}')['tree']['sha']

    def versions(self, sha):
        versions = {k: self.content(f'{p}/version.txt', sha).decode().strip()
                    for k, p in VERSION_PATHS.items()}
        recorded = json.loads(self.content('.release-please-manifest.json', sha))
        require(all(recorded[p] == versions[k] for k, p in VERSION_PATHS.items()), 'version files drift')
        return versions

    def artifact(self, artifact):
        require(not artifact['expired'], 'expired artifact: build and qualify a new candidate')
        raw = self.raw(f'repos/{self.repository}/actions/artifacts/{artifact["id"]}/zip')
        with zipfile.ZipFile(io.BytesIO(raw)) as z:
            names = z.namelist()
            require(all(i.orig_filename == i.filename for i in z.infolist()), 'normalized ZIP paths')
            require(len(names) == len(set(names)), 'duplicate ZIP paths')
            require(all(not n.endswith('/') and str(PurePosixPath(n)) == n and
                        not PurePosixPath(n).is_absolute() and '..' not in PurePosixPath(n).parts
                        and '\\' not in n and ':' not in n for n in names), 'unsafe ZIP paths')
            return {n: z.read(n) for n in names}

    def asset(self, asset):
        return self.raw(f'repos/{self.repository}/releases/assets/{asset["id"]}', binary=True)

    def attach(self, release, name, data):
        self.raw(f'repos/{self.repository}/releases/{release["id"]}/assets?name={name}',
                 method='POST', data=data, upload=True)


class OCI:
    def digest(self, ref):
        p = subprocess.run(['crane', 'digest', ref], capture_output=True, text=True)
        if p.returncode:
            # A transport/auth failure must never be interpreted as an absent tag.
            if 'MANIFEST_UNKNOWN' in p.stderr or 'NAME_UNKNOWN' in p.stderr:
                return None
            raise Rejected('OCI digest lookup failed')
        result = p.stdout.strip()
        require(matches('sha256:' + HASH, result), 'registry digest response')
        return result

    def identity(self, ref):
        p = subprocess.run(['crane', 'config', ref], capture_output=True, text=True)
        require(p.returncode == 0, 'OCI config lookup failed')
        return json.loads(p.stdout)['config']['Labels']

    def promote(self, ref, tag):
        p = subprocess.run(['crane', 'tag', ref, tag], capture_output=True)
        require(p.returncode == 0, 'OCI promotion failed')


def release_pr(gh, number, merged=False):
    pr = gh.get(f'pulls/{number}')
    require(pr['base']['ref'] == 'main' and pr['base']['repo']['full_name'] == gh.repository
            and pr['head']['repo']['full_name'] == gh.repository
            and pr['head']['ref'] == 'release-please--branches--main'
            and pr['user']['login'] in ('github-actions[bot]', 'release-please[bot]'), 'unified Release Please PR origin')
    require(pr['merged'] if merged else pr['state'] == 'open', 'release PR state')
    if not merged:
        require('autorelease: pending' in [l['name'] for l in pr['labels']], 'release PR label')
    return pr


def verify_workflow(gh, sha):
    require(gh.content(WORKFLOW, sha) == gh.content(WORKFLOW, 'main'), 'candidate workflow differs from main')
    for path in ('scripts/release/gate.py', 'scripts/release/release/v1/candidate_pb2.py',
                 'proto/release/v1/candidate.proto'):
        require(gh.content(path, sha) == gh.content(path, 'main'), 'release helper/schema differs from main')


def prepare(gh, number, run_id, attempt):
    pr = release_pr(gh, number)
    head = pr['head']['sha']
    verify_workflow(gh, head)
    run = gh.get(f'actions/runs/{run_id}/attempts/{attempt}')
    require(run['id'] == run_id and run['run_attempt'] == attempt
            and run['repository']['full_name'] == gh.repository
            and run['head_sha'] == head and run['event'] == 'workflow_dispatch'
            and run['path'] == WORKFLOW and run['head_repository']['full_name'] == gh.repository
            and run['head_branch'] == pr['head']['ref'], 'candidate dispatch must run on exact release PR head')
    versions = gh.versions(head)
    tree = gh.tree(head)
    values = dict(head=head, tree=tree, **versions)
    for component in ('backend', 'frontend'):
        values[component + '_candidate'] = candidate_tag(versions[component], tree, run_id, attempt)
    return values


def verify_files(m, files):
    require(set(files) == {f.path for f in m.plugin.files}, 'downloaded bundle paths')
    for f in m.plugin.files:
        require(len(files[f.path]) == f.size_bytes and hashlib.sha256(files[f.path]).hexdigest() == f.sha256,
                'plugin bytes drift')


def verify_images(m, oci, pinned=False, destinations=True):
    for component, image in [('backend', m.backend), ('frontend', m.frontend)]:
        require(oci.digest(f'{image.repository}:{image.candidate_tag}') == image.digest, 'candidate digest drift')
        require(oci.digest(f'{image.repository}@{image.digest}') == image.digest, 'missing image digest')
        labels = oci.identity(f'{image.repository}@{image.digest}')
        require(labels.get('org.opencontainers.image.source') == f'https://github.com/{m.repository}'
                and labels.get('org.opencontainers.image.revision') == m.release_tree_sha
                and labels.get('org.opencontainers.image.version') == getattr(m.versions, component) + '+' + m.release_tree_sha,
                'image build identity drift')
        if destinations:
            existing = oci.digest(f'{image.repository}:{getattr(m.versions, component)}')
            require(existing == image.digest if pinned else existing in (None, image.digest), 'pinned tag conflict/drift')


def load_candidate(gh, number, run_id, attempt, merged=True):
    require(run_id > 0 and attempt > 0, 'explicit qualified candidate run and attempt required')
    pr = release_pr(gh, number, merged=merged)
    head = pr['head']['sha']
    verify_workflow(gh, head)
    workflow_id = gh.get('actions/workflows/release-candidate.yml')['id']
    run = gh.get(f'actions/runs/{run_id}/attempts/{attempt}')
    require(run['id'] == run_id and run['run_attempt'] == attempt
            and run['conclusion'] == 'success' and run['head_sha'] == head
            and run['event'] == 'workflow_dispatch' and run['path'] == WORKFLOW
            and run['workflow_id'] == workflow_id and run['repository']['full_name'] == gh.repository
            and run['head_repository']['full_name'] == gh.repository
            and run['head_branch'] == pr['head']['ref'], 'candidate workflow/run origin')
    artifacts = gh.pages(f'actions/runs/{run["id"]}/artifacts', 'artifacts')
    def one(name):
        found = [a for a in artifacts if a['name'] == name]
        require(len(found) == 1, 'missing/ambiguous workflow artifact')
        a = found[0]
        require(a['workflow_run']['id'] == run['id'] and a['workflow_run']['head_sha'] == head
                and a['workflow_run']['repository_id'] == run['repository']['id']
                and a['workflow_run']['head_repository_id'] == run['repository']['id'], 'artifact origin')
        return a
    manifest = gh.artifact(one(manifest_name(run['id'], run['run_attempt'])))
    require(set(manifest) == {'candidate-manifest.pb', 'candidate-manifest.textproto'}, 'manifest artifact files')
    m = decode(pb.CandidateManifest, manifest['candidate-manifest.pb'])
    validate(m, gh.repository)
    require(m.workflow_run_id == run['id'] and m.workflow_run_attempt == run['run_attempt']
            and m.release_pr_number == number and m.release_head_sha == head
            and m.release_tree_sha == gh.tree(head), 'manifest source/run drift')
    if merged:
        require(m.release_tree_sha == gh.tree(pr['merge_commit_sha']), 'merged tree differs from tested tree')
    versions = gh.versions(pr['merge_commit_sha'] if merged else head)
    require(all(getattr(m.versions, k) == v for k, v in versions.items()), 'merged versions drift')
    plugin = one(m.plugin.artifact_name)
    require(plugin['id'] == m.plugin.artifact_id, 'plugin artifact ID drift')
    files = gh.artifact(plugin)
    verify_files(m, files)
    return m, files, pr


def merged_pr(gh, sha):
    prs = gh.pages(f'commits/{sha}/pulls')
    matches_pr = [p for p in prs if p['merged_at'] and p['merge_commit_sha'] == sha
                  and p['head']['ref'] == 'release-please--branches--main']
    require(len(matches_pr) == 1, 'commit is not the exact unified release PR merge')
    return matches_pr[0]['number']


def promote(m, files, oci, dry_run=True):
    # Validate BOTH destinations before the first write.
    verify_files(m, files)
    verify_images(m, oci)
    refs = {}
    for component, image in [('backend', m.backend), ('frontend', m.frontend)]:
        version = getattr(m.versions, component)
        if not dry_run and oci.digest(f'{image.repository}:{version}') != image.digest:
            oci.promote(f'{image.repository}@{image.digest}', version)
            require(oci.digest(f'{image.repository}:{version}') == image.digest, 'promotion readback drift')
        refs[component + '_image'] = f'{image.repository}:{version}@{image.digest}'
        refs[component + '_version'] = version
    return refs


def release_for(gh, tag, m):
    ref = gh.get(f'git/ref/tags/{tag}')['object']
    seen = set()
    while ref['type'] == 'tag':
        require(ref['sha'] not in seen, 'tag cycle')
        seen.add(ref['sha'])
        ref = gh.get(f'git/tags/{ref["sha"]}')['object']
    require(ref['type'] == 'commit' and gh.tree(ref['sha']) == m.release_tree_sha, 'release tag tree drift')
    require(all(getattr(m.versions, k) == v for k, v in gh.versions(ref['sha']).items()), 'release tag versions drift')
    release = gh.get(f'releases/tags/{tag}')
    require(release['tag_name'] == tag and not release['draft'], 'matching published release required')
    release['assets'] = gh.pages(f'releases/{release["id"]}/assets')
    return release


def receipt(m, tag, dll, run, phase, message_id=''):
    r = pb.CutoverReceipt(release_tree_sha=m.release_tree_sha, plugin_release_tag=tag,
        dll_sha256=dll, frontend_release_tag='frontend/v' + m.versions.frontend,
        workflow_run_id=run, phase=phase, discord_message_id=message_id)
    r.recorded_at.GetCurrentTime()
    return r


def cutover(m, files, gh, oci, tag, dll, run, discord, dry_run=True):
    validate(m, gh.repository)
    require(run > 0 and tag == 'plugin/v' + m.versions.plugin and matches(HASH, dll), 'manual inputs')
    verify_files(m, files)
    require(hashlib.sha256(files['FlightStripsPlugin.dll']).hexdigest() == dll, 'expected DLL hash drift')
    verify_images(m, oci, pinned=True)
    plugin = release_for(gh, tag, m)
    frontend = release_for(gh, 'frontend/v' + m.versions.frontend, m)
    assets = plugin['assets']
    names = [a['name'] for a in assets]
    require(len(names) == len(set(names)), 'duplicate release assets')
    attach = []
    intents, completes = {}, {}
    for a in assets:
        name = a['name']
        if name in files:
            require(gh.asset(a) == files[name], 'conflicting existing plugin asset')
        elif name.startswith('cutover-'):
            r = decode(pb.CutoverReceipt, gh.asset(a))
            require(r.HasField('recorded_at'), 'missing receipt timestamp')
            timestamp(r.recorded_at)
            require(r.release_tree_sha == m.release_tree_sha and r.plugin_release_tag == tag
                    and r.dll_sha256 == dll and r.frontend_release_tag == frontend['tag_name']
                    and r.workflow_run_id > 0 and r.phase in (1, 2), 'conflicting receipt')
            suffix = 'intent' if r.phase == 1 else 'complete'
            require(name == f'cutover-{m.release_tree_sha}-{r.workflow_run_id}-{suffix}.pb', 'receipt filename')
            require((r.phase == 1 and not r.discord_message_id) or
                    (r.phase == 2 and matches(r'[1-9][0-9]*', r.discord_message_id)), 'receipt message ID')
            origin = gh.get(f'actions/runs/{r.workflow_run_id}')
            require(origin['id'] == r.workflow_run_id and origin['path'] == '.github/workflows/release-cutover.yml'
                    and origin['event'] == 'workflow_dispatch' and origin['head_branch'] == 'main'
                    and origin['repository']['full_name'] == gh.repository
                    and origin['head_repository']['full_name'] == gh.repository, 'receipt workflow origin')
            (intents if r.phase == 1 else completes)[r.workflow_run_id] = r
    require(len(intents) <= 1 and len(completes) <= 1, 'multiple announcement receipts')
    require(all(k in intents for k in completes), 'completion without durable intent')
    require(all((intents[k].recorded_at.seconds, intents[k].recorded_at.nanos) <=
                (completes[k].recorded_at.seconds, completes[k].recorded_at.nanos)
                for k in completes), 'receipt time order')
    require(not any(k not in completes for k in intents),
            'announcement outcome uncertain: operator reconciliation required; automatic resend blocked')
    for name, data in sorted(files.items()):
        if name not in names:
            attach.append((name, data))
    report = dict(plugin_release_tag=tag, frontend_release_tag=frontend['tag_name'],
                  dll_sha256=dll, tree=m.release_tree_sha,
                  attachments=','.join(n for n, _ in attach), announcement='skip' if completes else 'send')
    if dry_run:
        return report
    for name, data in attach:
        gh.attach(plugin, name, data)
    if completes:
        return report
    intent = receipt(m, tag, dll, run, pb.CUTOVER_PHASE_ANNOUNCEMENT_INTENT)
    intent_name = f'cutover-{m.release_tree_sha}-{run}-intent.pb'
    gh.attach(plugin, intent_name, encode(intent))
    # Read back the durable intent before making the external call.
    found = [a for a in gh.pages(f'releases/{plugin["id"]}/assets') if a['name'] == intent_name]
    require(len(found) == 1 and gh.asset(found[0]) == encode(intent), 'intent not durably confirmed')
    message_id = discord(frontend)
    require(matches(r'[1-9][0-9]*', message_id), 'Discord confirmation missing: reconciliation required')
    complete = receipt(m, tag, dll, run, pb.CUTOVER_PHASE_COMPLETE, message_id)
    gh.attach(plugin, f'cutover-{m.release_tree_sha}-{run}-complete.pb', encode(complete))
    return report


def discord_announce(frontend):
    # Never retry: a timeout/error may occur after the provider accepted the message.
    import urllib.request
    url = os.environ['DISCORD_WEBHOOK_URL']
    require(url.startswith('https://discord.com/api/webhooks/') and '?' not in url, 'Discord webhook URL')
    content = f'FlightStrips frontend {frontend["tag_name"]}\n{frontend["html_url"]}'
    req = urllib.request.Request(url + '?wait=true',
        data=json.dumps({'content': content, 'allowed_mentions': {'parse': []}}).encode(),
        headers={'Content-Type': 'application/json'}, method='POST')
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)['id']
    except Exception:
        raise Rejected('Discord outcome unknown: reconcile durable intent; no automatic resend') from None


def output(values):
    for k, v in values.items():
        require('\n' not in str(v) and '\r' not in str(v), 'output newline')
        print(f'{k}={v}')
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as f:
            for k, v in values.items():
                f.write(f'{k}={v}\n')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('command', choices=['prepare', 'guard-tags', 'manifest', 'promote', 'cutover', 'inspect'])
    parser.add_argument('--repository', default=os.environ.get('GITHUB_REPOSITORY'))
    parser.add_argument('--pr', type=int)
    parser.add_argument('--run', type=int, default=int(os.environ.get('GITHUB_RUN_ID', '0')))
    parser.add_argument('--attempt', type=int, default=int(os.environ.get('GITHUB_RUN_ATTEMPT', '0')))
    parser.add_argument('--candidate-run', type=int, default=int(os.environ.get('QUALIFIED_CANDIDATE_RUN_ID') or '0'))
    parser.add_argument('--candidate-attempt', type=int, default=int(os.environ.get('QUALIFIED_CANDIDATE_RUN_ATTEMPT') or '0'))
    parser.add_argument('--sha')
    parser.add_argument('--plugin-dir', type=Path)
    parser.add_argument('--artifact-id', type=int)
    parser.add_argument('--backend-digest')
    parser.add_argument('--frontend-digest')
    parser.add_argument('--output-dir', type=Path, default=Path('candidate'))
    parser.add_argument('--plugin-release-tag')
    parser.add_argument('--expected-dll-sha256')
    parser.add_argument('--dry-run', choices=['true', 'false'], default='true')
    args = parser.parse_args()
    gh, oci = GitHub(args.repository), OCI()
    if args.command in ('prepare', 'guard-tags'):
        values = prepare(gh, args.pr, args.run, args.attempt)
        if args.command == 'guard-tags':
            for component, suffix in [('backend', 'backend-api'), ('frontend', 'frontend')]:
                repo = f'ghcr.io/{gh.repository.split("/")[0].lower()}/{suffix}'
                require(oci.digest(repo + ':' + values[component + '_candidate']) is None,
                        'immutable candidate tag already exists; use a new workflow attempt')
        output(values)
    elif args.command == 'manifest':
        v = prepare(gh, args.pr, args.run, args.attempt)
        m = pb.CandidateManifest(schema_revision=1, repository=gh.repository,
            release_pr_number=args.pr, release_head_sha=v['head'], release_tree_sha=v['tree'],
            protocol_revision=2, workflow_run_id=args.run, workflow_run_attempt=args.attempt)
        m.created_at.GetCurrentTime()
        for k in VERSION_PATHS:
            setattr(m.versions, k, v[k])
        for k, digest in [('backend', args.backend_digest), ('frontend', args.frontend_digest)]:
            image = getattr(m, k)
            image.repository = f'ghcr.io/{gh.repository.split("/")[0].lower()}/{"backend-api" if k == "backend" else k}'
            image.digest, image.candidate_tag = digest, v[k + '_candidate']
        m.plugin.artifact_name = plugin_name(args.run, args.attempt)
        m.plugin.artifact_id = args.artifact_id
        files = {p.name: p.read_bytes() for p in args.plugin_dir.iterdir() if p.is_file()}
        for name, data in sorted(files.items()):
            m.plugin.files.add(path=name, sha256=hashlib.sha256(data).hexdigest(), size_bytes=len(data))
        validate(m, gh.repository)
        verify_images(m, oci, destinations=False)
        # Re-download by artifact ID, not just the local build workspace.
        artifact = gh.get(f'actions/artifacts/{m.plugin.artifact_id}')
        require(artifact['name'] == m.plugin.artifact_name and artifact['workflow_run']['id'] == args.run
                and artifact['workflow_run']['head_sha'] == m.release_head_sha, 'uploaded artifact origin')
        verify_files(m, gh.artifact(artifact))
        args.output_dir.mkdir(parents=True, exist_ok=False)
        (args.output_dir / 'candidate-manifest.pb').write_bytes(encode(m))
        (args.output_dir / 'candidate-manifest.textproto').write_text(text_format.MessageToString(m), encoding='utf-8')
    else:
        if args.command == 'cutover':
            require(matches('plugin/v' + VERSION, args.plugin_release_tag or ''), 'exact plugin tag')
            ref = gh.get(f'git/ref/tags/{args.plugin_release_tag}')['object']
            if ref['type'] == 'tag':
                ref = gh.get(f'git/tags/{ref["sha"]}')['object']
            require(ref['type'] == 'commit', 'release tag commit')
            number = merged_pr(gh, ref['sha'])
        else:
            number = args.pr or merged_pr(gh, args.sha)
        m, files, pr = load_candidate(gh, number, args.candidate_run, args.candidate_attempt,
                                     merged=args.command != 'inspect')
        if args.sha:
            require(pr['merge_commit_sha'] == args.sha, 'promotion commit drift')
        if args.command == 'cutover':
            output(cutover(m, files, gh, oci, args.plugin_release_tag, args.expected_dll_sha256,
                           args.run, discord_announce, args.dry_run == 'true'))
        elif args.command == 'inspect':
            verify_images(m, oci, destinations=False)
            output(dict(tree=m.release_tree_sha, head=m.release_head_sha,
                        backend_image=f'{m.backend.repository}@{m.backend.digest}',
                        frontend_image=f'{m.frontend.repository}@{m.frontend.digest}',
                        dll_sha256=hashlib.sha256(files['FlightStripsPlugin.dll']).hexdigest(),
                        plugin_artifact_id=m.plugin.artifact_id,
                        candidate_run=m.workflow_run_id, candidate_attempt=m.workflow_run_attempt))
        else:
            output(promote(m, files, oci, args.dry_run == 'true'))


if __name__ == '__main__':
    try:
        main()
    except Exception as e:
        # Boundary exceptions can contain secrets/URLs; report only safe validation errors.
        print(str(e) if isinstance(e, Rejected) else 'release validation failed (details withheld)')
        raise SystemExit(1)
