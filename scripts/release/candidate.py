"""Release artifacts are data; this tool always runs from reviewed publisher source."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import zipfile

REPO = 'HitKeep/hitkeep'
WORKFLOW = '.github/workflows/release.yml'
CHECKS = ('native', 'archives', 'tracker', 'chart', 'docker', 'compose', 'helm', 'docs', 'assistant-package', 'source-archive')
CHANNELS = ('github', 'npm', 'images', 'helm')
SHA = re.compile(r'[a-f0-9]{40}')
DIGEST = re.compile(r'sha256:[a-f0-9]{64}')


def version(value):
    if not isinstance(value, str) or not re.fullmatch(r'v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', value):
        raise ValueError('expected numeric stable SemVer')
    return tuple(map(int, value.lstrip('v').split('.')))


def may_promote(requested, current):
    return version(requested) >= version(current)


def require_matching(existing, expected, name):
    if existing != expected:
        raise ValueError('immutable content conflict: ' + name)


def required_files(ver):
    files = {f'hitkeep{cloud}-linux-{arch}': 'binary' for cloud in ('', '-cloud') for arch in ('amd64', 'arm64')}
    files.update({f'hitkeep_{ver}_Linux_{arch}.tar.gz': 'archive' for arch in ('amd64', 'arm64')})
    files.update({'hitkeep-configuration.json': 'configuration', 'hitkeep.example.yaml': 'configuration',
                  'hitkeep-configuration-manifest.json': 'configuration', 'SHA256SUMS': 'checksums',
                  'verified-tracker.tgz': 'tracker', f'hitkeep-{ver}.tgz': 'chart',
                  f'hitkeep-analytics-{ver}.zip': 'assistant-package', f'hitkeep-source-{ver}.tar.gz': 'source-archive'})
    return files


def validate_manifest(m, root=None):
    version(m['tag'])
    ver = m['tag'][1:]
    if not m['tag'].startswith('v') or m.get('schema_version') != 1:
        raise ValueError('unsupported candidate schema or tag')
    if not all(SHA.fullmatch(m.get(key, '')) for key in ('source_commit', 'producer_workflow_sha', 'docs_revision')):
        raise ValueError('invalid immutable source identity')
    if not DIGEST.fullmatch(m.get('image_digest', '')):
        raise ValueError('invalid candidate image digest')
    if any(type(m.get(key)) is not int or m[key] < 1 for key in ('producer_run_id', 'producer_run_attempt')):
        raise ValueError('invalid producer attempt')
    if m.get('checks') != dict.fromkeys(CHECKS, 'success'):
        raise ValueError('missing mandatory candidate check')
    expected = required_files(ver)
    seen = set()
    for item in m['artifacts']:
        name = item['name']
        if name in seen or name not in expected or item['type'] != expected[name]:
            raise ValueError('unexpected or duplicate candidate artifact')
        seen.add(name)
        if not re.fullmatch(r'[a-f0-9]{64}', item['sha256']):
            raise ValueError('invalid artifact digest')
        if root is None:
            continue
        path = root / name
        if path.is_symlink() or not path.is_file() or hashlib.sha256(path.read_bytes()).hexdigest() != item['sha256']:
            raise ValueError('candidate artifact digest mismatch: ' + name)
        if item['type'] == 'tracker':
            with tarfile.open(path) as archive:
                package = json.load(archive.extractfile('package/package.json'))
            if package.get('name') != '@hitkeep/tracker' or package.get('version') != ver:
                raise ValueError('tracker version mismatch')
        if item['type'] == 'chart':
            with tarfile.open(path) as archive:
                chart = archive.extractfile('hitkeep/Chart.yaml').read().decode()
            for key in ('version', 'appVersion'):
                values = re.findall(r'^' + key + r':\s*[\"\']?([^\s\"\']+)', chart, re.M)
                if values != [ver]:
                    raise ValueError('chart version mismatch')
    if seen != expected.keys():
        raise ValueError('incomplete candidate inventory')


def validate_run(run, run_id, attempt):
    if (run.get('id') != run_id or run.get('run_attempt') != attempt or
            run.get('repository', {}).get('full_name') != REPO or
            run.get('head_repository', {}).get('full_name') != REPO or
            run.get('path') != WORKFLOW or run.get('head_branch') != 'main' or
            run.get('event') not in ('push', 'workflow_dispatch') or
            run.get('status') != 'completed' or run.get('conclusion') != 'success' or
            not SHA.fullmatch(run.get('head_sha', ''))):
        raise ValueError('untrusted or unsuccessful producer attempt')


def select_artifact(items, run):
    matches = [item for item in items if item['name'] == f"release-candidate-{run['id']}-{run['run_attempt']}"]
    if len(matches) != 1 or matches[0].get('expired') is not False or not DIGEST.fullmatch(matches[0].get('digest', '')):
        raise ValueError('missing, ambiguous or expired candidate; explicitly validate a new candidate')
    artifact = matches[0]
    identity = artifact.get('workflow_run', {})
    if identity.get('id') != run['id'] or identity.get('head_sha') != run['head_sha']:
        raise ValueError('artifact producer identity mismatch')
    return artifact


def extract_candidate(raw, digest, root):
    if 'sha256:' + hashlib.sha256(raw).hexdigest() != digest:
        raise ValueError('candidate ZIP digest mismatch')
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        names = archive.namelist()
        if len(names) != len(set(names)) or any(not re.fullmatch(r'[A-Za-z0-9_.-]+', name) or name in ('.', '..') for name in names):
            raise ValueError('unsafe candidate ZIP member')
        if any((entry.external_attr >> 16) & 0o170000 == 0o120000 for entry in archive.infolist()):
            raise ValueError('candidate ZIP contains a symlink')
        root.mkdir(parents=True, exist_ok=False)
        archive.extractall(root)


def command(*args, binary=False):
    result = subprocess.run(args, check=True, stdout=subprocess.PIPE)
    return result.stdout if binary else result.stdout.decode().strip()


def api(path, *args, binary=False):
    return command('gh', 'api', f'repos/{REPO}/{path}', *args, binary=binary)


def json_api(path):
    return json.loads(api(path))


def verify_attestation(path, manifest):
    verified = json.loads(command('gh', 'attestation', 'verify', str(path), '--repo', REPO,
                                  '--signer-workflow', REPO + '/' + WORKFLOW,
                                  '--source-digest', manifest['producer_workflow_sha'],
                                  '--source-ref', 'refs/heads/main', '--format', 'json'))
    expected = f"https://github.com/{REPO}/actions/runs/{manifest['producer_run_id']}/attempts/{manifest['producer_run_attempt']}"
    for result in verified:
        result = result.get('verificationResult', {})
        statement = result.get('statement', {})
        claimed = statement.get('predicate', {}).get('runDetails', {}).get('metadata', {}).get('invocationId')
        certificate = result.get('signature', {}).get('certificate', {})
        invocation = certificate.get('runInvocationURI', claimed)
        if invocation == expected and claimed in (None, expected):
            return
    raise ValueError('attestation does not certify the exact producer attempt')


def verify(run_id, attempt, root, automatic=False):
    run = json_api(f'actions/runs/{run_id}/attempts/{attempt}')
    validate_run(run, run_id, attempt)
    artifacts = json_api(f'actions/runs/{run_id}/artifacts?per_page=100')['artifacts']
    if automatic and not any(item['name'].startswith('release-candidate-') for item in artifacts):
        return None
    artifact = select_artifact(artifacts, run)
    raw = api(f"actions/artifacts/{artifact['id']}/zip", binary=True)
    extract_candidate(raw, artifact['digest'], root)
    m = json.loads((root / 'release-candidate.json').read_text())
    validate_manifest(m, root)
    if (m['producer_run_id'], m['producer_run_attempt'], m['producer_workflow_sha']) != (run_id, attempt, run['head_sha']):
        raise ValueError('candidate attempt mismatch')
    # Ancestry is not review approval; activation also requires main review/CI protections.
    for sha in {run['head_sha'], m['source_commit']}:
        comparison = json_api(f'compare/{sha}...main')
        if comparison.get('status') not in ('ahead', 'identical'):
            raise ValueError('candidate source is outside protected main history')
    tag = json_api(f"git/ref/tags/{m['tag']}")['object']
    if tag['type'] == 'tag':
        tag = json_api(f"git/tags/{tag['sha']}")['object']
    if tag.get('type') != 'commit' or tag['sha'] != m['source_commit']:
        raise ValueError('release tag changed')
    verify_attestation(root / 'release-candidate.json', m)
    return m


def public_candidate(tag, source_sha, root):
    version(tag)
    if not SHA.fullmatch(source_sha):
        raise ValueError('invalid release source SHA')
    release = json_api('releases/tags/' + tag)
    if release['draft'] or release['prerelease'] or release['tag_name'] != tag:
        raise ValueError('release is not public and stable')
    assets = [a for a in release['assets'] if a['name'] == 'release-candidate.json']
    if len(assets) != 1:
        raise ValueError('public release has no unique candidate evidence')
    path = root / 'release-candidate.json'
    path.write_bytes(api(f"releases/assets/{assets[0]['id']}", '-H', 'Accept: application/octet-stream', binary=True))
    manifest = json.loads(path.read_text())
    validate_manifest(manifest)
    if manifest['tag'] != tag or manifest['source_commit'] != source_sha:
        raise ValueError('public release candidate identity mismatch')
    run_id, attempt = manifest['producer_run_id'], manifest['producer_run_attempt']
    run = json_api(f'actions/runs/{run_id}/attempts/{attempt}')
    validate_run(run, run_id, attempt)
    if run['head_sha'] != manifest['producer_workflow_sha']:
        raise ValueError('public candidate workflow revision mismatch')
    tag_object = json_api('git/ref/tags/' + tag)['object']
    if tag_object['type'] == 'tag':
        tag_object = json_api('git/tags/' + tag_object['sha'])['object']
    if tag_object['type'] != 'commit' or tag_object['sha'] != source_sha:
        raise ValueError('public release tag changed')
    verify_attestation(path, manifest)
    return manifest


def assemble(root):
    m = {key: os.environ[key.upper()] for key in ('tag', 'source_commit', 'producer_workflow_sha', 'image_digest', 'docs_revision')}
    m.update(schema_version=1, producer_run_id=int(os.environ['GITHUB_RUN_ID']),
             producer_run_attempt=int(os.environ['GITHUB_RUN_ATTEMPT']), checks=json.loads(os.environ['CANDIDATE_CHECKS']), artifacts=[])
    if os.environ['TESTED_IMAGE_DIGEST'] != m['image_digest']:
        raise ValueError('tested image differs from candidate')
    for name, kind in required_files(m['tag'][1:]).items():
        m['artifacts'].append({'name': name, 'type': kind, 'sha256': hashlib.sha256((root / name).read_bytes()).hexdigest()})
    validate_manifest(m, root)
    (root / 'release-candidate.json').write_text(json.dumps(m, sort_keys=True, indent=2) + '\n')


def github(m, root):
    release = json_api(f"releases/tags/{m['tag']}")
    if release['prerelease'] or release['tag_name'] != m['tag']:
        raise ValueError('stable candidate points to prerelease')
    files = [item['name'] for item in m['artifacts']] + ['release-candidate.json']
    existing = {item['name']: item for item in release['assets']}
    if set(existing) - set(files):
        raise ValueError('unexpected draft assets; review inventory')
    for name in files:
        if name in existing:
            raw = api(f"releases/assets/{existing[name]['id']}", '-H', 'Accept: application/octet-stream', binary=True)
            require_matching(raw, (root / name).read_bytes(), name)
    for name in files:
        if name not in existing:
            if not release['draft']:
                raise ValueError('public release inventory incomplete')
            command('gh', 'release', 'upload', m['tag'], str(root / name), '--repo', REPO)
    # Read back the complete asset set before the one-way undraft operation.
    final = json_api(f"releases/tags/{m['tag']}")
    if {item['name'] for item in final['assets']} != set(files):
        raise ValueError('incomplete release assets')
    for item in final['assets']:
        raw = api(f"releases/assets/{item['id']}", '-H', 'Accept: application/octet-stream', binary=True)
        require_matching(raw, (root / item['name']).read_bytes(), item['name'])
    if release['draft']:
        releases = json.loads(command('gh', 'api', '--paginate', '--slurp', f'repos/{REPO}/releases?per_page=100'))
        newer = any(not r['draft'] and not r['prerelease'] and version(r['tag_name']) > version(m['tag']) for page in releases for r in page if re.fullmatch(r'v[0-9]+\.[0-9]+\.[0-9]+', r['tag_name']))
        command('gh', 'release', 'edit', m['tag'], '--repo', REPO, '--draft=false', '--latest=' + str(not newer).lower())
        published = json_api(f"releases/tags/{m['tag']}")
        if published['draft'] or published['prerelease'] or published['tag_name'] != m['tag']:
            raise ValueError('GitHub release publication was not verified')
        if not newer and json_api('releases/latest')['tag_name'] != m['tag']:
            raise ValueError('GitHub latest release was not verified')


def optional_command(*args):
    result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        # Only an explicit registry not-found response is absence; outages fail closed.
        error = result.stderr.decode()
        if not (any(marker in error for marker in ('E404', 'MANIFEST_UNKNOWN', 'manifest unknown', 'not found: manifest')) or
                re.search(r'(?im)^error: (?:failed to perform "FetchReference" on source: )?\S+: not found\s*$', error)):
            raise ValueError('registry read failed')
        return None
    return result.stdout.decode().strip()


def npm(m, root):
    ver = m['tag'][1:]
    cli = version(command('npm', '--version'))
    if cli < (11, 21, 0) or ((12, 0, 0) <= cli < (12, 2, 0)):
        raise ValueError('npm OIDC dist-tag capability unavailable; upgrade pinned CLI before activation')
    spec = '@hitkeep/tracker@' + ver
    metadata = optional_command('npm', 'view', spec, 'dist', '--json')
    if metadata is not None:
        dist = json.loads(metadata)
        import urllib.request
        with urllib.request.urlopen(dist['tarball']) as response:
            require_matching(response.read(), (root / 'verified-tracker.tgz').read_bytes(), spec)
    tags = json.loads(command('npm', 'view', '@hitkeep/tracker', 'dist-tags', '--json'))
    if metadata is None:
        command('npm', 'publish', str(root / 'verified-tracker.tgz'), '--access', 'public', '--ignore-scripts', '--tag', 'release-' + ver.replace('.', '-'))
    if tags.get('latest') != ver and (not tags.get('latest') or may_promote(ver, tags['latest'])):
        command('npm', 'dist-tag', 'add', spec, 'latest')


def image_version(image):
    raw = optional_command('docker', 'buildx', 'imagetools', 'inspect', image, '--format', '{{json .Image}}')
    if raw is None:
        return None
    values = set()
    def visit(value):
        if isinstance(value, dict):
            labels = value.get('config', {}).get('Labels', {})
            if 'org.opencontainers.image.version' in labels:
                values.add(labels['org.opencontainers.image.version'])
            for child in value.values():
                visit(child)
        elif isinstance(value, list):
            for child in value:
                visit(child)
    visit(json.loads(raw))
    if len(values) != 1:
        raise ValueError('mutable image has no unambiguous version label')
    return values.pop()


def images(m, root):
    source = 'ghcr.io/hitkeep/hitkeep@' + m['image_digest']
    repositories = ('ghcr.io/hitkeep/hitkeep', os.environ['DOCKERHUB_REPOSITORY'])
    if not re.fullmatch(r'[a-z0-9_-]+/hitkeep', repositories[1]):
        raise ValueError('unexpected Docker Hub destination')
    ver = m['tag'][1:]
    for repo in repositories:
        immutable = repo + ':' + ver
        existing = optional_command('docker', 'buildx', 'imagetools', 'inspect', immutable, '--format', '{{.Manifest.Digest}}')
        if existing is not None:
            require_matching(existing, m['image_digest'], immutable)
        else:
            command('docker', 'buildx', 'imagetools', 'create', '--tag', immutable, source)
            actual = command('docker', 'buildx', 'imagetools', 'inspect', immutable, '--format', '{{.Manifest.Digest}}')
            require_matching(actual, m['image_digest'], immutable)
        for channel in ('latest', ver.split('.')[0], '.'.join(ver.split('.')[:2])):
            target = repo + ':' + channel
            current_digest = optional_command('docker', 'buildx', 'imagetools', 'inspect', target, '--format', '{{.Manifest.Digest}}')
            if current_digest == m['image_digest']:
                continue
            current = image_version(target)
            if current is not None and version(current) == version(ver):
                raise ValueError('same image version has different digest')
            if current is None or may_promote(ver, current):
                command('docker', 'buildx', 'imagetools', 'create', '--tag', target, source)
                actual = command('docker', 'buildx', 'imagetools', 'inspect', target, '--format', '{{.Manifest.Digest}}')
                require_matching(actual, m['image_digest'], target)


def helm(m, root):
    ver = m['tag'][1:]
    name = 'hitkeep-' + ver + '.tgz'
    ref = 'oci://ghcr.io/hitkeep/charts/hitkeep'
    with tempfile.TemporaryDirectory() as temp:
        found = optional_command('helm', 'pull', ref, '--version', ver, '--destination', temp)
        if found is not None:
            require_matching((Path(temp) / name).read_bytes(), (root / name).read_bytes(), name)
        else:
            command('helm', 'push', str(root / name), 'oci://ghcr.io/hitkeep/charts')


def publish_selected(channel, m, root, publishers=None):
    publishers = publishers if publishers is not None else {name: globals()[name] for name in CHANNELS}
    selected = CHANNELS if channel == 'all' else (channel,)
    if any(name not in CHANNELS for name in selected):
        raise ValueError('unknown publication channel')
    for name in selected:
        publishers[name](m, root)
        print(name + ': published or already matching', flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('action', choices=('assemble', 'verify', 'publish', 'public'))
    parser.add_argument('--directory', type=Path, required=True)
    parser.add_argument('--run-id', type=int)
    parser.add_argument('--attempt', type=int)
    parser.add_argument('--automatic', action='store_true')
    parser.add_argument('--tag')
    parser.add_argument('--source-sha')
    parser.add_argument('--channel', choices=('all',) + CHANNELS, default='all')
    args = parser.parse_args()
    if args.action == 'public':
        if not args.tag or not args.source_sha:
            parser.error('public requires --tag and --source-sha')
        args.directory.mkdir(parents=True, exist_ok=True)
        manifest = public_candidate(args.tag, args.source_sha, args.directory)
        digests = {item['name']: item['sha256'] for item in manifest['artifacts']}
        fields = {'tag': manifest['tag'], 'source_sha': manifest['source_commit'],
                  'configuration_sha256': digests['hitkeep-configuration.json'],
                  'manifest_sha256': digests['hitkeep-configuration-manifest.json'],
                  'artifact_sha256': digests['hitkeep-cloud-linux-arm64'],
                  'package_sha256': digests[f"hitkeep-analytics-{manifest['tag'][1:]}.zip"],
                  'archive_sha256': digests[f"hitkeep-source-{manifest['tag'][1:]}.tar.gz"]}
        print(json.dumps(fields, sort_keys=True))
        if os.environ.get('GITHUB_OUTPUT'):
            with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
                for name, value in fields.items():
                    output.write(f'{name}={value}\n')
        return
    if args.action == 'assemble':
        assemble(args.directory)
        return
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp) / 'candidate'
        m = verify(args.run_id, args.attempt, root, args.automatic)
        if m is None:
            print('release-please-only run: no candidate')
            return
        if args.action == 'publish':
            publish_selected(args.channel, m, root)
        else:
            args.directory.mkdir(parents=True, exist_ok=True)
            import shutil
            shutil.copytree(root, args.directory, dirs_exist_ok=True)
            if os.environ.get('GITHUB_OUTPUT'):
                with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
                    output.write(f"ready=true\ntag={m['tag']}\nsource_sha={m['source_commit']}\n")


if __name__ == '__main__':
    main()
