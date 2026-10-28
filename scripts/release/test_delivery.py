"""Execute the actual fixed receiver steps against a fake GitHub CLI."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


WORKFLOW = Path(__file__).resolve().parents[2] / '.github/workflows/release-publish.yml'
DESTINATIONS = {
    'docs': ('hitkeep-docs', 'sync-public-hitkeep-release.yml'),
    'cloud': ('hitkeep-infra', 'release-deploy.yml'),
    'assistant': ('hitkeep-assistant-plugins', 'sync-hitkeep-release.yml'),
    'homebrew': ('homebrew-hitkeep', 'update-formula.yml'),
}


def step_script(job, name):
    source = re.split(r'\n  [a-z][a-z-]*:\n', WORKFLOW.read_text().split(f'\n  {job}:\n', 1)[1], maxsplit=1)[0]
    step = source.split(f'      - name: {name}\n', 1)[1]
    block = step.split('        run: |\n', 1)[1].split('\n      - ', 1)[0]
    return '\n'.join(line.removeprefix('          ') for line in block.splitlines())


class DeliveryTests(unittest.TestCase):
    def setUp(self):
        if not shutil.which('jq'):
            self.skipTest('workflow execution requires jq')
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        cli = self.root / 'gh'
        cli.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
with open('calls.jsonl', 'a') as log:
    log.write(json.dumps(args) + '\\n')
fault = os.environ.get('FAULT', '')
repo = 'HitKeep/' + os.environ['RECEIVER_REPO']
workflow = os.environ['RECEIVER_WORKFLOW']
if args[:2] == ['run', 'watch']:
    if os.environ['RECEIVER_REPO'] in ('hitkeep-docs', 'hitkeep-infra'):
        print('PRIVATE_WATCH_STDOUT')
        print('PRIVATE_WATCH_STDERR', file=sys.stderr)
    sys.exit(1 if fault == 'watch' else 0)
endpoint = next((arg for arg in args if arg.startswith('repos/')), '')
if endpoint.endswith('/git/ref/heads/main'):
    print('a' * 40)
elif endpoint.endswith('/dispatches'):
    assert 'return_run_details=true' in args
    assert 'ref=main' in args
    if fault == 'lost': sys.exit(1)
    print('' if fault == 'empty' else json.dumps({'workflow_run_id': 'bad' if fault == 'id' else 42}))
elif endpoint.endswith('/actions/runs/42'):
    run = {'id': 42, 'repository': {'full_name': repo}, 'head_repository': {'full_name': repo},
           'path': '.github/workflows/' + workflow, 'event': 'workflow_dispatch',
           'head_branch': 'main', 'head_sha': 'a' * 40, 'status': 'completed', 'conclusion': 'success'}
    if fault in ('head_sha', 'event', 'head_branch', 'path'): run[fault] = 'wrong'
    if fault in ('repository', 'head_repository'): run[fault] = {'full_name': 'attacker/repo'}
    if fault == 'run_id': run['id'] = 43
    if fault == 'receiver': run['conclusion'] = 'failure'
    if fault == 'pending': run['status'] = 'in_progress'
    if fault == 'readback' and Path('read-once').exists(): run['head_sha'] = 'b' * 40
    Path('read-once').touch()
    print(json.dumps(run))
else:
    raise SystemExit('unexpected API or discovery: ' + endpoint)
''')
        cli.chmod(0o755)

    def execute(self, target, fault=''):
        repo, workflow = DESTINATIONS[target]
        env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ['PATH'],
                   RECEIVER_REPO=repo, RECEIVER_WORKFLOW=workflow, FAULT=fault,
                   TAG='v2.10.0', SOURCE_SHA='c' * 40, CONFIGURATION_SHA256='d' * 64,
                   MANIFEST_SHA256='e' * 64, ARTIFACT_SHA256='f' * 64,
                   PACKAGE_SHA256='1' * 64, ARCHIVE_SHA256='2' * 64,
                   GITHUB_OUTPUT=str(self.root / 'output'), RUNNER_TEMP=str(self.root))
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', step_script(target, 'Dispatch and verify exact receiver run')],
                                cwd=self.root, env=env, capture_output=True, text=True)
        calls = [json.loads(line) for line in (self.root / 'calls.jsonl').read_text().splitlines()]
        return result, calls

    def test_all_receivers_watch_only_the_returned_id_and_read_back(self):
        for target, (repo, workflow) in DESTINATIONS.items():
            with self.subTest(target=target):
                (self.root / 'calls.jsonl').write_text('')
                result, calls = self.execute(target)
                self.assertEqual(result.returncode, 0, result.stderr)
                dispatch = [call for call in calls if any(arg.endswith('/dispatches') for arg in call)]
                self.assertEqual(len(dispatch), 1)
                self.assertIn(f'repos/HitKeep/{repo}/actions/workflows/{workflow}/dispatches', dispatch[0])
                self.assertFalse(any('correlation' in arg for arg in dispatch[0]))
                self.assertIn(['run', 'watch', '42', '--repo', f'HitKeep/{repo}', '--exit-status', '--compact'], calls)
                self.assertEqual(sum(f'repos/HitKeep/{repo}/actions/runs/42' in call for call in calls), 2)
                self.assertIn('verified=true', (self.root / 'output').read_text())

    def test_private_watch_never_exposes_progress_or_annotations(self):
        for target in ('docs', 'cloud'):
            for fault in ('', 'watch'):
                with self.subTest(target=target, fault=fault):
                    result, _ = self.execute(target, fault)
                    self.assertEqual(result.returncode == 0, fault == '')
                    self.assertNotIn('PRIVATE_WATCH_STDOUT', result.stdout + result.stderr)
                    self.assertNotIn('PRIVATE_WATCH_STDERR', result.stdout + result.stderr)

    def test_unknown_dispatch_and_wrong_identity_fail_without_another_destination(self):
        for fault in ('empty', 'lost', 'id', 'run_id', 'repository', 'head_repository', 'path', 'event', 'head_branch',
                      'head_sha', 'watch', 'receiver', 'pending', 'readback'):
            with self.subTest(fault=fault):
                for name in ('calls.jsonl', 'output', 'read-once'):
                    (self.root / name).unlink(missing_ok=True)
                result, calls = self.execute('docs', fault)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertEqual(sum(any(arg.endswith('/dispatches') for arg in call) for call in calls), 1)
                self.assertFalse(any('hitkeep-infra' in arg for call in calls for arg in call))
                self.assertNotIn('verified=true', (self.root / 'output').read_text() if (self.root / 'output').exists() else '')

    def test_leaf_retries_only_reach_summary_and_core_channel_stays_isolated(self):
        source = WORKFLOW.read_text()
        for target in DESTINATIONS:
            job = re.split(r'\n  [a-z][a-z-]*:\n', source.split(f'\n  {target}:\n', 1)[1], maxsplit=1)[0]
            self.assertIn("inputs.channel == 'all'", job)
            self.assertNotIn("inputs.channel == 'github'", job)
            for other in DESTINATIONS:
                if other != target:
                    needs = re.search(r'^    needs: (.+)$', job, re.M).group(1)
                    self.assertNotIn(other, needs)
        self.assertFalse((WORKFLOW.parents[2] / 'scripts/release/delivery.py').exists())

    def test_release_caller_allows_every_nested_pipeline_permission(self):
        release = WORKFLOW.with_name('release.yml').read_text()
        caller = release.split('\n  build-release:\n', 1)[1].split('\n  candidate:\n', 1)[0]
        granted = dict(re.findall(r'^      ([a-z-]+): (none|read|write)(?:\s+#.*)?$', caller, re.M))
        pipeline = WORKFLOW.with_name('pipeline.yml').read_text()
        requested = re.findall(r'^      ([a-z-]+): (none|read|write)(?:\s+#.*)?$', pipeline, re.M)
        self.assertTrue(requested)
        levels = {'none': 0, 'read': 1, 'write': 2}
        for permission, level in requested:
            with self.subTest(permission=permission, requested=level):
                self.assertGreaterEqual(levels[granted.get(permission, 'none')], levels[level])

    def test_release_groups_keep_all_native_pending_runs(self):
        for path in (WORKFLOW, WORKFLOW.with_name('release.yml')):
            block = path.read_text().split('\nconcurrency:\n', 1)[1].split('\n\n', 1)[0]
            fields = dict(line.strip().split(': ', 1) for line in block.splitlines())
            with self.subTest(workflow=path.name):
                self.assertEqual(fields.get('queue'), 'max')
                self.assertEqual(fields.get('cancel-in-progress'), 'false')

    def test_activation_conditions_preserve_source_channels_and_partial_success(self):
        from types import SimpleNamespace as NS
        source = WORKFLOW.read_text()
        for flag in ('RELEASE_PUBLICATION_ENABLED', 'RELEASE_DOCS_DELIVERY_ENABLED', 'RELEASE_CLOUD_DELIVERY_ENABLED'):
            self.assertNotIn(flag, source)
        github = NS(ref='refs/heads/main', event_name='workflow_dispatch', event=NS(workflow_run=NS(conclusion='success')))
        inputs = NS(channel='all')
        needs = NS(verify=NS(outputs=NS(ready='true')), **{name: NS(result='success') for name in ('github', 'npm', 'images', 'helm')})
        def enabled(job):
            block = re.split(r'\n  [a-z][a-z-]*:\n', source.split(f'\n  {job}:\n', 1)[1], maxsplit=1)[0]
            expression = re.search(r'^    if: \$\{\{ (.+) \}\}$', block, re.M).group(1)
            expression = expression.replace('&&', 'and').replace('||', 'or')
            return eval(expression, {'__builtins__': {}}, {'github': github, 'inputs': inputs, 'needs': needs})
        self.assertTrue(enabled('verify'))
        github.ref = 'refs/heads/untrusted'
        self.assertFalse(enabled('verify'))
        github.ref = 'refs/heads/main'
        github.event_name = 'workflow_run'
        self.assertTrue(enabled('verify'))
        github.event.workflow_run.conclusion = 'failure'
        self.assertFalse(enabled('verify'))
        github.event_name = 'workflow_dispatch'
        github.event.workflow_run.conclusion = 'success'
        for channel in ('github', 'npm', 'images', 'helm'):
            inputs.channel = channel
            for target in ('github', 'npm', 'images', 'helm'):
                self.assertEqual(enabled(target), target == channel)
            for target in DESTINATIONS:
                self.assertFalse(enabled(target))
        inputs.channel = 'all'
        needs.verify.outputs.ready = 'false'
        for target in ('github', 'npm', 'images', 'helm'):
            self.assertFalse(enabled(target))
        needs.verify.outputs.ready = 'true'
        needs.npm.result = 'failure'
        self.assertFalse(enabled('cloud'))
        for target in ('docs', 'assistant', 'homebrew'):
            self.assertTrue(enabled(target))
        needs.github.result = 'failure'
        for target in DESTINATIONS:
            self.assertFalse(enabled(target))

    def test_summary_requires_receiver_readback(self):
        script = step_script('summary', 'Report independent destination results')
        summary = self.root / 'summary'
        results = {'docs': {'result': 'success', 'outputs': {'verified': 'true', 'run_url': 'https://github.com/HitKeep/hitkeep-docs/actions/runs/42'}},
                   'cloud': {'result': 'success', 'outputs': {'verified': 'false'}},
                   'assistant': {'result': 'failure', 'outputs': {'run_url': 'https://github.com/HitKeep/assistant/actions/runs/43'}},
                   'homebrew': {'result': 'skipped', 'outputs': {}}}
        result = subprocess.run(['bash', '-euo', 'pipefail', '-c', script], cwd=self.root,
                                env=dict(os.environ, RESULTS=json.dumps(results), GITHUB_STEP_SUMMARY=str(summary)), capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        text = summary.read_text()
        self.assertIn('docs: delivery verified', text)
        self.assertIn('cloud: completion unverified', text)
        self.assertIn('assistant: failure', text)
        self.assertIn('homebrew: not attempted', text)


if __name__ == '__main__':
    unittest.main()
