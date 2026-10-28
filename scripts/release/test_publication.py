import copy
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest
from unittest.mock import patch
import zipfile

import test_candidate

candidate = test_candidate.candidate


class PublicationTests(unittest.TestCase):
    setUp = test_candidate.CandidateTests.setUp

    def test_bad_evidence_never_reaches_a_write_command(self):
        for fault in ('missing', 'expired', 'untrusted', 'unsuccessful', 'tampered', 'changed-tag', 'wrong-attestation',
                      'wrong-signed-run', 'wrong-signed-attempt', 'wrong-signed-repository', 'unsigned-invocation'):
            with self.subTest(fault=fault):
                case = self.root / fault
                case.mkdir()
                manifest = copy.deepcopy(self.manifest)
                (self.root / 'release-candidate.json').write_text(json.dumps(manifest))
                data = io.BytesIO()
                with zipfile.ZipFile(data, 'w') as archive:
                    for name in candidate.required_files('2.10.0'):
                        archive.writestr(name, (self.root / name).read_bytes())
                    archive.writestr('release-candidate.json', json.dumps(manifest))
                raw = data.getvalue()
                (case / 'artifact.zip').write_bytes(raw + (b'tampered' if fault == 'tampered' else b''))
                run = copy.deepcopy(self.run)
                if fault == 'untrusted':
                    run['event'] = 'pull_request'
                if fault == 'unsuccessful':
                    run['conclusion'] = 'failure'
                artifact = {'id': 7, 'name': 'release-candidate-12-2', 'expired': fault == 'expired',
                            'digest': 'sha256:' + hashlib.sha256(raw).hexdigest(),
                            'workflow_run': {'id': 12, 'head_sha': 'a' * 40}}
                responses = {
                    'actions/runs/12/attempts/2': run,
                    'actions/runs/12/artifacts?per_page=100': {'artifacts': [] if fault == 'missing' else [artifact]},
                    'compare/' + 'a' * 40 + '...main': {'status': 'identical'},
                    'git/ref/tags/v2.10.0': {'object': {'type': 'commit', 'sha': ('d' if fault == 'changed-tag' else 'a') * 40}},
                    'releases/tags/v2.10.0': {'tag_name': 'v2.10.0', 'draft': True, 'prerelease': False, 'assets': []},
                }
                (case / 'responses.json').write_text(json.dumps(responses))
                gh = case / 'gh'
                gh.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
root = pathlib.Path(os.environ['FAKE_GH_ROOT'])
with (root / 'commands.jsonl').open('a') as log: log.write(json.dumps(sys.argv[1:]) + '\\n')
if sys.argv[1:3] == ['attestation', 'verify']:
    if root.name == 'wrong-attestation': sys.exit(1)
    repo = 'attacker/repo' if root.name == 'wrong-signed-repository' else 'HitKeep/hitkeep'
    run = 13 if root.name == 'wrong-signed-run' else 12
    attempt = 1 if root.name == 'wrong-signed-attempt' else 2
    uri = f'https://github.com/{repo}/actions/runs/{run}/attempts/{attempt}'
    result = {'verificationResult': {'statement': {'predicate': {'runDetails': {'metadata': {'invocationId': uri}}}}}}
    if root.name == 'unsigned-invocation':
        result = {'attestation': {'invocationId': uri}, 'verificationResult': {}}
    print(json.dumps([result]))
    sys.exit(0)
if sys.argv[1] != 'api': sys.exit(99)
route = sys.argv[2].removeprefix('repos/HitKeep/hitkeep/')
if route == 'actions/artifacts/7/zip':
    sys.stdout.buffer.write((root / 'artifact.zip').read_bytes())
else:
    print(json.dumps(json.loads((root / 'responses.json').read_text())[route]))
''')
                gh.chmod(0o755)
                env = dict(os.environ, PATH=str(case) + os.pathsep + os.environ['PATH'], FAKE_GH_ROOT=str(case))
                result = subprocess.run([sys.executable, '-B', str(Path(candidate.__file__)), 'publish', '--run-id', '12', '--attempt', '2',
                                         '--directory', str(case / 'out'), '--channel', 'github'], env=env, capture_output=True)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                commands = [json.loads(line) for line in (case / 'commands.jsonl').read_text().splitlines()]
                self.assertFalse(any(args[:2] in (['release', 'upload'], ['release', 'edit']) for args in commands), commands)

    def github_fixture(self, draft=False, final_fault=None, newer=False):
        (self.root / 'release-candidate.json').write_text(json.dumps(self.manifest))
        names = list(candidate.required_files('2.10.0')) + ['release-candidate.json']
        assets = [{'name': name, 'id': index} for index, name in enumerate(names, 1)]
        state = {'tag_name': 'v2.10.0', 'draft': draft, 'prerelease': False, 'assets': assets}
        writes = []
        def api(route, *args, binary=False):
            if route.startswith('releases/assets/'):
                item = assets[int(route.rsplit('/', 1)[1]) - 1]
                return (self.root / item['name']).read_bytes()
            raise AssertionError(route)
        def json_api(route):
            if route == 'releases/tags/v2.10.0':
                result = copy.deepcopy(state)
                if final_fault and writes:
                    result.update(final_fault)
                return result
            if route == 'releases/latest':
                return {'tag_name': 'v3.0.0' if newer else 'v2.10.0', 'draft': False}
            raise AssertionError(route)
        def command(*args, **kwargs):
            if args[:3] == ('gh', 'api', '--paginate'):
                return json.dumps([[{'tag_name': 'v3.0.0' if newer else 'v2.9.0', 'draft': False, 'prerelease': False}]])
            writes.append(args)
            if args[:3] == ('gh', 'release', 'edit'):
                state['draft'] = False
            return ''
        return state, writes, patch.multiple(candidate, api=api, json_api=json_api, command=command)

    def test_github_matching_immutable_inventory_is_noop(self):
        _, writes, fake = self.github_fixture()
        with fake:
            candidate.github(self.manifest, self.root)
        self.assertEqual(writes, [])

    def test_github_conflict_fails_before_write(self):
        _, writes, fake = self.github_fixture()
        with fake, patch.object(candidate, 'api', return_value=b'conflicting bytes'), self.assertRaises(ValueError):
            candidate.github(self.manifest, self.root)
        self.assertEqual(writes, [])

    def test_github_verifies_actual_undraft_and_identity(self):
        for fault in ({'draft': True}, {'tag_name': 'v2.9.0'}):
            _, _, fake = self.github_fixture(draft=True, final_fault=fault)
            with self.subTest(fault=fault), fake, self.assertRaises(ValueError):
                candidate.github(self.manifest, self.root)

    def test_github_old_retry_cannot_become_latest(self):
        _, writes, fake = self.github_fixture(draft=True, newer=True)
        with fake:
            candidate.github(self.manifest, self.root)
        self.assertIn('--latest=false', writes[0])

    def test_matching_image_tags_are_noops(self):
        writes = []
        with patch.object(candidate, 'optional_command', return_value=self.manifest['image_digest']), \
                patch.object(candidate, 'image_version', return_value='v2.10.0'), \
                patch.object(candidate, 'command', side_effect=lambda *a: writes.append(a)), \
                patch.dict(os.environ, DOCKERHUB_REPOSITORY='hitkeep/hitkeep'):
            candidate.images(self.manifest, self.root)
        self.assertEqual(writes, [])

    def test_backport_advances_only_its_own_image_channels(self):
        writes = []
        def current(image):
            return 'v3.0.0' if image.endswith(':latest') else 'v2.9.0'
        def inspect(*args):
            return self.manifest['image_digest'] if ':2.10.0' in args[4] else 'sha256:' + 'e' * 64
        def command(*args):
            if 'create' in args:
                writes.append(args)
            return self.manifest['image_digest']
        with patch.object(candidate, 'optional_command', side_effect=inspect), \
                patch.object(candidate, 'image_version', side_effect=current), \
                patch.object(candidate, 'command', side_effect=command), \
                patch.dict(os.environ, DOCKERHUB_REPOSITORY='hitkeep/hitkeep'):
            candidate.images(self.manifest, self.root)
        self.assertFalse(any(':latest' in part for args in writes for part in args))
        self.assertTrue(any(':2.10' in part for args in writes for part in args))

    def test_registry_auth_or_outage_is_not_absence(self):
        for message in (b'unauthorized', b'503 Service Unavailable', b'connection refused'):
            result = subprocess.CompletedProcess(['registry'], 1, b'', message)
            with self.subTest(message=message), patch.object(candidate.subprocess, 'run', return_value=result), self.assertRaises(ValueError):
                candidate.optional_command('registry')

    def test_native_buildx_not_found_is_absence(self):
        result = subprocess.CompletedProcess(['docker'], 1, b'', b'ERROR: docker.io/library/busybox:absent: not found\n')
        with patch.object(candidate.subprocess, 'run', return_value=result):
            self.assertIsNone(candidate.optional_command('docker'))

    def test_native_helm_not_found_is_absence(self):
        message = b'Error: failed to perform "FetchReference" on source: ghcr.io/hitkeep/charts/hitkeep:999.0.0: not found\n'
        result = subprocess.CompletedProcess(['helm'], 1, b'', message)
        with patch.object(candidate.subprocess, 'run', return_value=result):
            self.assertIsNone(candidate.optional_command('helm'))

    def test_npm_matching_package_and_latest_are_noop(self):
        import urllib.request
        writes = []
        payload = self.root / 'verified-tracker.tgz'
        def command(*args):
            if args == ('npm', '--version'):
                return '12.2.0'
            if args == ('npm', 'view', '@hitkeep/tracker', 'dist-tags.latest'):
                return '2.10.0'
            self.assertNotEqual(args[:2], ('npm', 'view'), 'request scalar fields across supported npm versions')
            writes.append(args)
            return ''
        def metadata(*args):
            self.assertEqual(args, ('npm', 'view', '@hitkeep/tracker@2.10.0', 'dist.tarball'))
            return payload.as_uri()
        with patch.object(candidate, 'command', side_effect=command), \
                patch.object(candidate, 'optional_command', side_effect=metadata):
            candidate.npm(self.manifest, self.root)
        self.assertEqual(writes, [])

    def test_npm_new_package_never_executes_lifecycle_or_downgrades(self):
        writes = []
        def command(*args):
            if args == ('npm', '--version'):
                return '12.2.0'
            if args == ('npm', 'view', '@hitkeep/tracker', 'dist-tags.latest'):
                return '3.0.0'
            self.assertNotEqual(args[:2], ('npm', 'view'), 'request scalar fields across supported npm versions')
            writes.append(args)
            return ''
        with patch.object(candidate, 'command', side_effect=command), patch.object(candidate, 'optional_command', return_value=None):
            candidate.npm(self.manifest, self.root)
        self.assertEqual(len(writes), 1)
        self.assertIn('--ignore-scripts', writes[0])
        self.assertNotIn('latest', writes[0])


if __name__ == '__main__':
    unittest.main()
