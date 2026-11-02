"""Run the checked-in aggregate scripts against GitHub needs-result fixtures."""
import copy
import itertools
import json
import os
from pathlib import Path
import re
import subprocess
import unittest

from test_delivery import step_script


WORKFLOWS = Path(__file__).resolve().parents[2] / '.github/workflows'
CI_JOBS = ('qa-contract', 'lint', 'test', 'mcp-audit', 'lint-dashboard',
           'test-dashboard', 'e2e-dashboard', 'email-review')
RELEASE_JOBS = ('select', 'homebrew-source', 'homebrew-candidate', 'cross-cgo', 'helm-lifecycle')


def job_source(workflow, job):
    return re.split(r'\n  [a-z][a-z-]*:\n',
                    (WORKFLOWS / workflow).read_text().split(f'\n  {job}:\n', 1)[1], maxsplit=1)[0]


def expression(value, needs, event, same_repo=True, release=False):
    """Evaluate only the expression subset used by these workflow contracts."""
    values = {
        'toJSON(needs)': json.dumps(needs),
        'github.event_name': event,
        'github.repository': 'HitKeep/hitkeep',
        'github.event.pull_request.head.repo.full_name': 'HitKeep/hitkeep' if same_repo else 'contributor/hitkeep',
        'github.head_ref': 'release-please--branches--main' if release else 'fix/example',
    }
    if value in values:
        return values[value]
    value = re.sub(r'needs\.([\w-]+)\.outputs\.selected',
                   lambda m: repr(needs.get(m[1], {}).get('outputs', {}).get('selected', '')), value)
    value = re.sub(r'needs\.([\w-]+)\.result',
                   lambda m: repr(needs.get(m[1], {}).get('result', '')), value)
    for key in sorted(values, key=len, reverse=True):
        value = value.replace(key, repr(values[key]))
    value = value.replace('&&', ' and ').replace('||', ' or ')
    return eval(value, {'__builtins__': {}}, {'always': lambda: True, 'startsWith': str.startswith})


class RequiredChecksTests(unittest.TestCase):
    def execute(self, workflow, job, step, needs, event='pull_request', same_repo=True, release=False):
        source = job_source(workflow, job)
        env = dict(os.environ, GITHUB_REPOSITORY='HitKeep/hitkeep')
        for key, value in re.findall(r'^          ([A-Z_]+): \$\{\{ (.+) \}\}$', source, re.M):
            env[key] = str(expression(value, needs, event, same_repo, release))
        script = step_script(job, step, WORKFLOWS / workflow)
        script = re.sub(r'\$\{\{ (.+?) \}\}',
                        lambda m: str(expression(m[1], needs, event, same_repo, release)), script)
        return subprocess.run(['bash', '-euo', 'pipefail', '-c', script],
                              env=env, capture_output=True, text=True)

    def ci(self, needs, event='pull_request'):
        return self.execute('ci.yml', 'required-ci', 'Enforce CI results', needs, event)

    def release(self, needs, event='pull_request', same_repo=True, release=False):
        return self.execute('release-validation.yml', 'required', 'Enforce selected validation',
                            needs, event, same_repo, release)

    def test_ci_requires_every_job_including_reduced_matrix_and_email(self):
        needs = {job: {'result': 'success'} for job in CI_JOBS}
        self.assertEqual(self.ci(needs).returncode, 0)
        for job in CI_JOBS:
            for result in ('failure', 'cancelled', 'skipped', '', None, 'neutral', True):
                with self.subTest(job=job, result=result):
                    broken = copy.deepcopy(needs)
                    broken[job]['result'] = result
                    self.assertNotEqual(self.ci(broken).returncode, 0)
            missing = copy.deepcopy(needs)
            del missing[job]
            self.assertNotEqual(self.ci(missing).returncode, 0)
        needs['email-review']['result'] = 'skipped'
        self.assertEqual(self.ci(needs, 'push').returncode, 0)
        self.assertNotEqual(self.ci(needs, 'workflow_dispatch').returncode, 0)

    def test_release_selection_truth_table_and_actual_job_predicates_agree(self):
        for event, selected, same_repo, release in itertools.product(
                ('pull_request', 'workflow_dispatch'), ('true', 'false'), (True, False), (True, False)):
            with self.subTest(event=event, selected=selected, same_repo=same_repo, release=release):
                expected = event == 'workflow_dispatch' or (selected == 'true' and same_repo)
                brew = expected and (event == 'workflow_dispatch' or release)
                needs = {job: {'result': 'skipped'} for job in RELEASE_JOBS}
                needs['select'] = {'result': 'success' if event == 'pull_request' else 'skipped',
                                   'outputs': {'selected': selected if event == 'pull_request' else ''}}
                for job, enabled in (('cross-cgo', expected), ('helm-lifecycle', expected),
                                     ('homebrew-source', brew), ('homebrew-candidate', brew)):
                    source = job_source('release-validation.yml', job)
                    predicate = re.search(r'    if: >-\n      (.+)', source)[1]
                    self.assertEqual(bool(expression(predicate, needs, event, same_repo, release)), enabled, job)
                    needs[job]['result'] = 'success' if enabled else 'skipped'
                self.assertEqual(self.release(needs, event, same_repo, release).returncode, 0)
                for job in RELEASE_JOBS:
                    for bad in ('failure', 'cancelled', '', None, True, 'neutral', 'skipped'):
                        if bad == 'skipped' and needs[job]['result'] == 'skipped':
                            continue
                        broken = copy.deepcopy(needs)
                        broken[job]['result'] = bad
                        self.assertNotEqual(self.release(broken, event, same_repo, release).returncode, 0, (job, bad))
                    broken = copy.deepcopy(needs)
                    del broken[job]
                    self.assertNotEqual(self.release(broken, event, same_repo, release).returncode, 0, job)

    def test_selector_output_must_be_a_boolean_string(self):
        for selected in ('', 'TRUE', 'null', True, False, None, [], {}):
            needs = {job: {'result': 'skipped'} for job in RELEASE_JOBS}
            needs['select'] = {'result': 'success', 'outputs': {'selected': selected}}
            self.assertNotEqual(self.release(needs).returncode, 0, selected)

    def test_aggregates_use_only_current_run_dependencies_and_all_matrix_results(self):
        for workflow, job, dependencies in (('ci.yml', 'required-ci', CI_JOBS),
                                             ('release-validation.yml', 'required', RELEASE_JOBS)):
            source = job_source(workflow, job)
            self.assertIn('    if: always()\n', source)
            declared = re.search(r'    needs:\n((?:      - .+\n)+)', source)[1]
            self.assertCountEqual(re.findall(r'      - (.+)', declared), dependencies)
            self.assertIn('RESULTS: ${{ toJSON(needs) }}', source)
            # No API lookup, checkout, historical SHA or synthetic status can replace needs.
            self.assertNotIn('uses:', source)
            self.assertNotRegex(source, r'\b(gh|curl|wget)\b|continue-on-error|head_sha|GITHUB_TOKEN')
        self.assertIn('fromJSON(needs.qa-contract.outputs.race_gates)', job_source('ci.yml', 'test'))
        self.assertIn('fail-fast: false', job_source('ci.yml', 'test'))
        self.assertIn('fail-fast: false', job_source('release-validation.yml', 'homebrew-candidate'))

    def test_all_required_contexts_are_eligible_on_every_main_pr(self):
        for workflow, job, name in (('ci.yml', 'required-ci', 'Required CI'),
                                    ('release-validation.yml', 'required', 'Required release validation'),
                                    ('zizmor.yml', 'zizmor', 'Run zizmor'),
                                    ('govulncheck.yml', 'govulncheck', 'Run govulncheck')):
            source = (WORKFLOWS / workflow).read_text()
            trigger = re.search(r'^  pull_request:([^\n]*(?:\n    .*)*)', source, re.M)
            self.assertIsNotNone(trigger, workflow)
            self.assertNotRegex(trigger[0], r'paths|branches-ignore|types:')
            self.assertIn(f'name: {name}', job_source(workflow, job))
            self.assertNotIn('pull_request_target:', source)
        self.assertIn("if: github.event_name == 'pull_request'", job_source('ci.yml', 'email-review'))


if __name__ == '__main__':
    unittest.main()
