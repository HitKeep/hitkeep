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
