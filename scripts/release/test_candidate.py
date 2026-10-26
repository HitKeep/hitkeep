import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile

SPEC = importlib.util.spec_from_file_location('candidate', Path(__file__).with_name('candidate.py'))
candidate = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(candidate)

VERIFIED_ATTESTATION = json.dumps([{'verificationResult': {'statement': {'predicate': {'runDetails': {
    'metadata': {'invocationId': 'https://github.com/HitKeep/hitkeep/actions/runs/12/attempts/2'}}}}}}])


class CandidateTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.manifest = {'schema_version': 1, 'tag': 'v2.10.0', 'source_commit': 'a' * 40,
                         'producer_workflow_sha': 'a' * 40, 'producer_run_id': 12,
                         'producer_run_attempt': 2, 'image_digest': 'sha256:' + 'b' * 64,
                         'docs_revision': 'c' * 40, 'checks': {name: 'success' for name in candidate.CHECKS},
                         'artifacts': []}
        for name, kind in candidate.required_files('2.10.0').items():
            data = b'fixture'
            if kind in ('tracker', 'chart'):
                buf = io.BytesIO()
                with tarfile.open(fileobj=buf, mode='w:gz') as archive:
                    entry = 'package/package.json' if kind == 'tracker' else 'hitkeep/Chart.yaml'
                    payload = b'{"name":"@hitkeep/tracker","version":"2.10.0"}' if kind == 'tracker' else b'name: hitkeep\nversion: 2.10.0\nappVersion: 2.10.0\n'
                    info = tarfile.TarInfo(entry)
                    info.size = len(payload)
                    archive.addfile(info, io.BytesIO(payload))
                data = buf.getvalue()
            (self.root / name).write_bytes(data)
            self.manifest['artifacts'].append({'name': name, 'type': kind, 'sha256': hashlib.sha256(data).hexdigest()})
        self.run = {'id': 12, 'run_attempt': 2, 'repository': {'full_name': 'HitKeep/hitkeep'},
                    'head_repository': {'full_name': 'HitKeep/hitkeep'}, 'path': '.github/workflows/release.yml',
                    'event': 'push', 'head_branch': 'main', 'head_sha': 'a' * 40,
                    'status': 'completed', 'conclusion': 'success'}

    def test_distribution_assets_and_checks_are_mandatory(self):
        expected = {'hitkeep-analytics-2.10.0.zip': 'assistant-package',
                    'hitkeep-source-2.10.0.tar.gz': 'source-archive'}
        for name, kind in expected.items():
            with self.subTest(name=name):
                self.assertEqual(candidate.required_files('2.10.0').get(name), kind)
                self.assertIn(kind, candidate.CHECKS)
                manifest = copy.deepcopy(self.manifest)
                manifest['artifacts'] = [item for item in manifest['artifacts'] if item['name'] != name]
                with self.assertRaisesRegex(ValueError, 'incomplete candidate inventory'):
                    candidate.validate_manifest(manifest, self.root)

    def test_public_candidate_refuses_untrusted_evidence_before_delivery(self):
        from unittest.mock import patch
        release = {'draft': False, 'prerelease': False, 'tag_name': 'v2.10.0',
                   'assets': [{'name': 'release-candidate.json', 'id': 1}]}
        cases = ('draft', 'prerelease', 'missing', 'duplicate', 'tag', 'source', 'producer', 'attempt', 'workflow', 'moved-tag', 'signature')
        for fault in cases:
            with self.subTest(fault=fault):
                manifest, public, run = copy.deepcopy(self.manifest), copy.deepcopy(release), copy.deepcopy(self.run)
                tag = {'object': {'type': 'commit', 'sha': manifest['source_commit']}}
                if fault in ('draft', 'prerelease'):
                    public[fault] = True
                if fault == 'missing':
                    public['assets'] = []
                if fault == 'duplicate':
                    public['assets'] *= 2
                if fault == 'tag':
                    manifest['tag'] = 'v2.9.0'
                if fault == 'source':
                    manifest['source_commit'] = 'd' * 40
                if fault == 'producer':
                    run['repository']['full_name'] = 'attacker/repo'
                if fault == 'attempt':
                    run['run_attempt'] = 1
                if fault == 'workflow':
                    run['head_sha'] = 'd' * 40
                if fault == 'moved-tag':
                    tag['object']['sha'] = 'd' * 40
                def api(path):
                    if path.startswith('releases/'):
                        return public
                    if path.startswith('actions/'):
                        return run
                    return tag
                with patch.object(candidate, 'json_api', side_effect=api), \
                        patch.object(candidate, 'api', return_value=json.dumps(manifest).encode()), \
                        patch.object(candidate, 'command', side_effect=ValueError('bad signature') if fault == 'signature' else None), \
                        self.assertRaises(ValueError):
                    candidate.public_candidate('v2.10.0', 'a' * 40, self.root)

    def test_public_candidate_preserves_signer_source_and_attempt(self):
        from unittest.mock import patch
        release = {'draft': False, 'prerelease': False, 'tag_name': 'v2.10.0',
                   'assets': [{'name': 'release-candidate.json', 'id': 1}]}
        with patch.object(candidate, 'json_api', side_effect=[release, self.run, {'object': {'type': 'commit', 'sha': 'a' * 40}}]), \
                patch.object(candidate, 'api', return_value=json.dumps(self.manifest).encode()), \
                patch.object(candidate, 'command', return_value=VERIFIED_ATTESTATION) as command:
            self.assertEqual(candidate.public_candidate('v2.10.0', 'a' * 40, self.root), self.manifest)
        args = command.call_args.args
        self.assertEqual(args[:3], ('gh', 'attestation', 'verify'))
        self.assertEqual(args[args.index('--signer-workflow') + 1], 'HitKeep/hitkeep/.github/workflows/release.yml')
        self.assertEqual(args[args.index('--source-ref') + 1], 'refs/heads/main')
        self.assertEqual(args[args.index('--source-digest') + 1], 'a' * 40)
        self.assertEqual(args[args.index('--format') + 1], 'json')

    def test_signed_certificate_and_predicate_cannot_disagree(self):
        from unittest.mock import patch
        expected = 'https://github.com/HitKeep/hitkeep/actions/runs/12/attempts/2'
        wrong = expected.replace('/attempts/2', '/attempts/1')
        for certificate_uri, predicate_uri in ((wrong, expected), (expected, wrong)):
            verified = json.loads(VERIFIED_ATTESTATION)
            result = verified[0]['verificationResult']
            result['signature'] = {'certificate': {'runInvocationURI': certificate_uri}}
            result['statement']['predicate']['runDetails']['metadata']['invocationId'] = predicate_uri
            with self.subTest(certificate=certificate_uri), patch.object(candidate, 'command', return_value=json.dumps(verified)), self.assertRaises(ValueError):
                candidate.verify_attestation(self.root / 'release-candidate.json', self.manifest)
        certificate_only = [{'verificationResult': {'signature': {'certificate': {'runInvocationURI': expected}}}}]
        with patch.object(candidate, 'command', return_value=json.dumps(certificate_only)):
            candidate.verify_attestation(self.root / 'release-candidate.json', self.manifest)

    def test_public_candidate_rejects_another_signed_attempt(self):
        from unittest.mock import patch
        release = {'draft': False, 'prerelease': False, 'tag_name': 'v2.10.0',
                   'assets': [{'name': 'release-candidate.json', 'id': 1}]}
        for invocation in ('https://github.com/HitKeep/hitkeep/actions/runs/12/attempts/1',
                           'https://github.com/HitKeep/hitkeep/actions/runs/13/attempts/2',
                           'https://github.com/attacker/repo/actions/runs/12/attempts/2', ''):
            verified = json.loads(VERIFIED_ATTESTATION)
            verified[0]['verificationResult']['statement']['predicate']['runDetails']['metadata']['invocationId'] = invocation
            with self.subTest(invocation=invocation), \
                    patch.object(candidate, 'json_api', side_effect=[release, self.run, {'object': {'type': 'commit', 'sha': 'a' * 40}}]), \
                    patch.object(candidate, 'api', return_value=json.dumps(self.manifest).encode()), \
                    patch.object(candidate, 'command', return_value=json.dumps(verified)), self.assertRaises(ValueError):
                candidate.public_candidate('v2.10.0', 'a' * 40, self.root)

    def test_valid_candidate(self):
        candidate.validate_manifest(self.manifest, self.root)
        candidate.validate_run(self.run, 12, 2)

    def test_missing_mandatory_checks(self):
        for check in candidate.CHECKS:
            with self.subTest(check=check):
                manifest = copy.deepcopy(self.manifest)
                manifest['checks'][check] = 'skipped'
                with self.assertRaises(ValueError):
                    candidate.validate_manifest(manifest, self.root)

    def test_bad_artifact_names_and_types(self):
        for change in ({'name': '../escape'}, {'name': '/absolute'}, {'name': 'unknown'}, {'type': 'script'}, {'name': ''}):
            manifest = copy.deepcopy(self.manifest)
            manifest['artifacts'][0].update(change)
            with self.subTest(change=change), self.assertRaises(ValueError):
                candidate.validate_manifest(manifest, self.root)

    def test_missing_duplicate_and_tampered_artifacts(self):
        for mode in ('missing', 'duplicate', 'tampered'):
            manifest = copy.deepcopy(self.manifest)
            if mode == 'missing':
                manifest['artifacts'].pop()
            elif mode == 'duplicate':
                manifest['artifacts'].append(manifest['artifacts'][0])
            else:
                manifest['artifacts'][0]['sha256'] = '0' * 64
            with self.subTest(mode=mode), self.assertRaises(ValueError):
                candidate.validate_manifest(manifest, self.root)

    def test_symlink_rejected(self):
        path = self.root / self.manifest['artifacts'][0]['name']
        path.unlink()
        path.symlink_to('/etc/hosts')
        with self.assertRaises(ValueError):
            candidate.validate_manifest(self.manifest, self.root)

    def test_package_versions_are_bound(self):
        for tag in ('v2.9.0', 'v2.10.0-beta', 'v02.10.0'):
            manifest = copy.deepcopy(self.manifest)
            manifest['tag'] = tag
            with self.subTest(tag=tag), self.assertRaises(ValueError):
                candidate.validate_manifest(manifest, self.root)

    def test_embedded_tracker_and_chart_versions_are_checked(self):
        for name, entry, payload in (
                ('verified-tracker.tgz', 'package/package.json', b'{"name":"@hitkeep/tracker","version":"2.9.0"}'),
                ('hitkeep-2.10.0.tgz', 'hitkeep/Chart.yaml', b'version: 2.10.0\nappVersion: 2.9.0\n')):
            with self.subTest(name=name):
                manifest = copy.deepcopy(self.manifest)
                path = self.root / name
                original = path.read_bytes()
                with tarfile.open(path, 'w:gz') as archive:
                    info = tarfile.TarInfo(entry)
                    info.size = len(payload)
                    archive.addfile(info, io.BytesIO(payload))
                for item in manifest['artifacts']:
                    if item['name'] == name:
                        item['sha256'] = hashlib.sha256(path.read_bytes()).hexdigest()
                with self.assertRaisesRegex(ValueError, 'version mismatch'):
                    candidate.validate_manifest(manifest, self.root)
                path.write_bytes(original)

    def test_untrusted_producers(self):
        cases = [('event', 'pull_request'), ('head_branch', 'untrusted'), ('path', '.github/workflows/snapshot.yml'),
                 ('conclusion', 'failure'), ('status', 'in_progress'), ('id', 13), ('run_attempt', 1),
                 ('repository', {'full_name': 'attacker/repo'}), ('head_repository', {'full_name': 'attacker/repo'})]
        for key, value in cases:
            run = copy.deepcopy(self.run)
            run[key] = value
            with self.subTest(key=key), self.assertRaises(ValueError):
                candidate.validate_run(run, 12, 2)

    def test_artifact_evidence_missing_expired_duplicate_wrong_digest(self):
        artifact = {'id': 7, 'name': 'release-candidate-12-2', 'expired': False, 'digest': 'sha256:' + 'd' * 64,
                    'workflow_run': {'id': 12, 'head_sha': 'a' * 40}}
        self.assertEqual(candidate.select_artifact([artifact], self.run)['id'], 7)
        for items in ([], [artifact, artifact], [dict(artifact, expired=True)], [dict(artifact, digest='bad')]):
            with self.subTest(items=items), self.assertRaises(ValueError):
                candidate.select_artifact(items, self.run)

    def test_zip_digest_and_member_validation_before_extract(self):
        for name in ('../escape', '/absolute', 'release-candidate.json'):
            buf = io.BytesIO()
            with zipfile.ZipFile(buf, 'w') as archive:
                archive.writestr(name, '{}')
            raw = buf.getvalue()
            with self.subTest(name=name), self.assertRaises(ValueError):
                candidate.extract_candidate(raw, 'sha256:' + '0' * 64, self.root / 'out')
            if name != 'release-candidate.json':
                with self.assertRaises(ValueError):
                    candidate.extract_candidate(raw, 'sha256:' + hashlib.sha256(raw).hexdigest(), self.root / 'out')

    def test_source_archive_reuses_dashboard_and_keeps_exact_source(self):
        repo = Path(__file__).resolve().parents[2]
        workflow = (repo / '.github/workflows/pipeline.yml').read_text()
        step = workflow.split('      - name: Package exact source with existing dashboard assets\n', 1)[1]
        block = step.split('        run: |\n', 1)[1].split('\n      - ', 1)[0]
        script = '\n'.join(line.removeprefix('          ') for line in block.splitlines())
        source = self.root / 'source'
        source.mkdir()
        for name, data in {'go.mod': 'module example.test/source\n', 'go.sum': 'fixture\n', 'LICENSE': 'fixture\n',
                           'cmd/hitkeep/main.go': 'package main\n', 'public/embed.go': 'package public\n',
                           'frontend/dashboard/package.json': '{"version":"2.10.0"}'}.items():
            path = source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(data)
        for args in (['init', '-q'], ['add', '.'], ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', 'commit', '-qm', 'fixture']):
            subprocess.run(['git', *args], cwd=source, check=True, capture_output=True)
        sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=source, text=True).strip()
        assets = source / 'artifacts/public'
        assets.mkdir(parents=True)
        with tarfile.open(assets / 'public-assets.tar.gz', 'w:gz') as archive:
            for name, payload in {'index.html': b'<script src="main-fixture.js"></script>', 'main-fixture.js': b'dashboard',
                                  'hk.js': b'tracker', 'hk-vitals.js': b'vitals'}.items():
                info = tarfile.TarInfo(name)
                info.size = len(payload)
                archive.addfile(info, io.BytesIO(payload))
        hk = source / 'hk'
        hk.write_text('#!/bin/sh\nset -eu\ntest "$1 $2" = "ci restore-dashboard"\ntar -xzf "$4" -C public\n')
        hk.chmod(0o755)
        env = dict(os.environ, VERSION='v2.10.0', SOURCE_SHA=sha, RUNNER_TEMP=str(self.root), GITHUB_OUTPUT=str(self.root / 'output'))
        def package():
            result = subprocess.run(['bash', '-euo', 'pipefail', '-c', script], cwd=source, env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            return (self.root / 'source-package/hitkeep-source-2.10.0.tar.gz').read_bytes()
        first = package()
        self.assertEqual(first, package())
        with tarfile.open(fileobj=io.BytesIO(first), mode='r:gz') as archive:
            names = archive.getnames()
            self.assertEqual(len(names), len(set(names)))
            for name in ('public/embed.go', 'public/index.html', 'public/hk.js', 'public/hk-vitals.js', 'public/main-fixture.js', 'go.mod'):
                self.assertEqual(archive.extractfile('hitkeep-2.10.0/' + name).read(), (source / name).read_bytes())
            self.assertNotIn('hitkeep-2.10.0/hk', names)
        (source / 'public/embed.go').write_text('modified source')
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', script], cwd=source, env=env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    def test_numeric_semver_and_stale_retry(self):
        self.assertGreater(candidate.version('v2.10.0'), candidate.version('v2.9.0'))
        self.assertFalse(candidate.may_promote('v2.9.0', 'v2.10.0'))
        self.assertTrue(candidate.may_promote('v2.10.0', 'v2.9.0'))
        self.assertTrue(candidate.may_promote('v2.10.0', 'v2.10.0'))

    def test_channel_selection_is_independent(self):
        calls = []
        candidate.publish_selected('npm', self.manifest, self.root, {key: lambda *args, key=key: calls.append(key) for key in candidate.CHANNELS})
        self.assertEqual(calls, ['npm'])
        with self.assertRaises(ValueError):
            candidate.publish_selected('docs', self.manifest, self.root, {})

    def test_conflicting_existing_bytes_fail(self):
        with self.assertRaises(ValueError):
            candidate.require_matching(b'old', b'new', 'immutable')
        candidate.require_matching(b'same', b'same', 'immutable')

    def test_helm_upgrade_supplies_real_smoke_preconditions(self):
        if not shutil.which('jq'):
            self.skipTest('workflow shell execution requires jq')
        repo = Path(__file__).resolve().parents[2]
        workflow = (repo / '.github/workflows/pipeline.yml').read_text()
        step = workflow.split('      - name: Verify exact candidate against immutable supported floor\n', 1)[1]
        block = step.split('        run: |\n', 1)[1].split('\n      - ', 1)[0]
        script = '\n'.join(line.removeprefix('          ') for line in block.splitlines())
        # macOS Bash lacks case conversion; use an already lowercase repository fixture.
        script = script.replace('${GITHUB_REPOSITORY,,}', '${GITHUB_REPOSITORY}')
        digest = 'sha256:' + 'd' * 64
        fixture = {'supported_upgrade_floor': 'v2.12.0', 'fixtures': [{'previous_version': 'v2.12.0', 'platform': 'linux/arm64',
                   'previous_image': 'example.invalid/previous@' + digest, 'previous_chart': 'oci://example.invalid/chart',
                   'previous_chart_digest': digest}]}
        fixtures = self.root / 'tests/fixtures'
        fixtures.mkdir(parents=True)
        (fixtures / 'release-fixtures.json').write_text(json.dumps(fixture))
        bin_dir = self.root / 'bin'
        bin_dir.mkdir()
        for name, body in {'go': 'printf "%s\\n" "$FAKE_ROOT"', 'kind': 'exit 0',
                           'docker': 'if [[ "$*" == *"image inspect"* ]]; then echo arm64; fi',
                           'helm': 'printf "Digest: %s\\n" "$EXPECTED_DIGEST"'}.items():
            path = bin_dir / name
            path.write_text('#!/usr/bin/env bash\nset -euo pipefail\n' + body + '\n')
            path.chmod(0o755)
        scripts = self.root / 'scripts'
        scripts.mkdir()
        # Execute the actual smoke script's required-identity preflight without a cluster.
        preflight = (repo / 'scripts/helm-smoke.sh').read_text().split('namespace=', 1)[0]
        (scripts / 'helm-smoke.sh').write_text(preflight + '\ntest "$previous_chart_digest" = "$EXPECTED_DIGEST"\n')
        env = dict(os.environ, PATH=str(bin_dir) + os.pathsep + os.environ['PATH'], FAKE_ROOT=str(self.root),
                   EXPECTED_DIGEST=digest, CANDIDATE_DIGEST=self.manifest['image_digest'], VERSION='v2.10.0',
                   SURFACE='helm', GITHUB_RUN_ID='12', GITHUB_RUN_ATTEMPT='2', GITHUB_REPOSITORY='hitkeep/hitkeep')
        result = subprocess.run(['bash', '-c', script], cwd=self.root, env=env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)


if __name__ == '__main__':
    unittest.main()
