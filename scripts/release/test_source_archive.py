"""Exercise the source archive and Homebrew consumer steps with a real Git tree."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


def step_script(workflow, name):
    step = (ROOT / '.github/workflows' / workflow).read_text().split(f'      - name: {name}\n', 1)[1]
    block = step.split('        run: |\n', 1)[1].split('\n      - ', 1)[0]
    return '\n'.join(line.removeprefix('          ') for line in block.splitlines())


class SourceArchiveTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name) / 'source'
        self.root.mkdir()
        self.runtime = Path(self.tmp.name) / 'runtime'
        self.runtime.mkdir()
        files = {'go.mod': 'module fixture\n', 'go.sum': 'fixture-sum\n', 'LICENSE': 'MIT\n',
                 'cmd/hitkeep/main.go': 'package main\n', 'public/embed.go': 'package public\n',
                 'frontend/dashboard/package.json': json.dumps({'version': '1.2.3'})}
        for name, content in files.items():
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content)
        # The workflow restores already built assets; the fixture starts with them restored.
        (self.root / 'hk').write_text('#!/bin/sh\nexit 0\n')
        (self.root / 'hk').chmod(0o755)
        shutil.copytree(ROOT / 'scripts/release', self.root / 'scripts/release', ignore=shutil.ignore_patterns('__pycache__'))
        for args in [('init', '-q'), ('config', 'user.name', 'Fixture'), ('config', 'user.email', 'fixture@example.invalid'),
                     ('add', '.'), ('commit', '-qm', 'fixture')]:
            subprocess.run(['git', *args], cwd=self.root, check=True, capture_output=True)
        sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=self.root, text=True).strip()
        for name, content in {'index.html': '<html><script src="main.js"></script></html>',
                              'main.js': 'console.log("dashboard");', 'hk.js': '/* tracker */', 'hk-vitals.js': '/* vitals */'}.items():
            (self.root / 'public' / name).write_text(content)
        self.env = dict(os.environ, VERSION='v1.2.3', SOURCE_SHA=sha, RUNNER_TEMP=str(self.runtime))
        self.archive = self.runtime / 'source-package/hitkeep-source-1.2.3.tar.gz'

    def execute(self, workflow, name, env=None):
        return subprocess.run(['bash', '-euo', 'pipefail', '-c', step_script(workflow, name)],
                              cwd=self.root, env=env or self.env, capture_output=True, text=True)

    def package(self):
        result = self.execute('pipeline.yml', 'Package exact source with existing dashboard assets')
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_archive_rejects_unbound_source_missing_scripts_and_symlinks(self):
        result = self.execute('pipeline.yml', 'Package exact source with existing dashboard assets', dict(self.env, SOURCE_SHA='a' * 40))
        self.assertNotEqual(result.returncode, 0)
        script = self.root / 'public/main.js'
        script.unlink()
        self.assertNotEqual(self.execute('pipeline.yml', 'Package exact source with existing dashboard assets').returncode, 0)
        script.symlink_to(self.root / 'go.mod')
        self.assertNotEqual(self.execute('pipeline.yml', 'Package exact source with existing dashboard assets').returncode, 0)

    def test_required_check_rejects_archive_producer_failure(self):
        validation = (ROOT / '.github/workflows/release-validation.yml').read_text()
        block = validation.split('\n  required:\n', 1)[1].split('\n  homebrew-source:\n', 1)[0]
        script = '\n'.join(line.removeprefix('          ') for line in block.split('        run: |\n', 1)[1].splitlines())
        for result in ('success', 'skipped', 'failure', 'cancelled'):
            with self.subTest(producer=result):
                run = script
                for job in ('select', 'homebrew-source', 'homebrew-candidate', 'cross-cgo', 'helm-lifecycle'):
                    run = run.replace('${{ needs.' + job + '.result }}', result if job == 'homebrew-source' else 'success')
                completed = subprocess.run(['bash', '-euo', 'pipefail', '-c', run], capture_output=True, text=True)
                self.assertEqual(completed.returncode == 0, result in ('success', 'skipped'), completed.stderr)

    def test_homebrew_formula_uses_exact_packaged_bytes_without_raw_source_download(self):
        self.package()
        tap = Path(self.tmp.name) / 'tap'
        (tap / 'Formula').mkdir(parents=True)
        formula = tap / 'Formula/hitkeep.rb'
        formula.write_text('class Hitkeep\n  url "https://example.invalid/old.tar.gz"\n  sha256 "' + '0' * 64 + '"\nend\n')
        cli = Path(self.tmp.name) / 'bin'
        cli.mkdir()
        for name, content in {'brew': '#!/bin/sh\nprintf "%s\\n" "$TAP"\n',
                              'curl': '#!/bin/sh\necho "raw source download forbidden" >&2\nexit 90\n'}.items():
            path = cli / name
            path.write_text(content)
            path.chmod(0o755)
        env = dict(self.env, PATH=str(cli) + os.pathsep + os.environ['PATH'], TAP=str(tap),
                   CANDIDATE_VERSION='1.2.3', CANDIDATE_SHA=self.env['SOURCE_SHA'])
        result = self.execute('release-validation.yml', 'Prepare candidate formula', env)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('url "' + self.archive.as_uri() + '"', formula.read_text())
        self.assertIn('sha256 "' + hashlib.sha256(self.archive.read_bytes()).hexdigest() + '"', formula.read_text())


if __name__ == '__main__':
    unittest.main()
