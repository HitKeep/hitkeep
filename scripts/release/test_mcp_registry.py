"""Keep registry metadata within the namespace GitHub OIDC can publish."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from test_delivery import step_script


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / '.github/workflows/publish-mcp.yml'


class MCPRegistryTests(unittest.TestCase):
    def test_checked_in_metadata_uses_repository_owner_namespace(self):
        metadata = json.loads((ROOT / 'server.json').read_text())
        owner = metadata['repository']['url'].removeprefix('https://github.com/').split('/')[0]
        self.assertEqual(metadata['name'], f'io.github.{owner}/hitkeep')

    def test_release_namespace_preflight(self):
        if not shutil.which('jq'):
            self.skipTest('workflow execution requires jq')
        script = step_script('validate', 'Validate GitHub OIDC namespace', WORKFLOW)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name, succeeds in (
                    ('io.github.HitKeep/hitkeep', True),
                    ('io.github.PascaleBeier/hitkeep', False),
                    ('io.github.HitKeepOther/hitkeep', False),
                    ('io.github.HitKeep/', False),
                    ('io.github.HitKeep/hitkeep/extra', False),
                    (None, False)):
                with self.subTest(name=name):
                    (root / 'server.json').write_text(json.dumps({'name': name}))
                    result = subprocess.run(['bash', '-euo', 'pipefail', '-c', script],
                                            cwd=root, env=dict(os.environ, REGISTRY_OWNER='HitKeep'),
                                            capture_output=True, text=True)
                    self.assertEqual(result.returncode == 0, succeeds, result.stderr)


if __name__ == '__main__':
    unittest.main()
