import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch


SPEC = importlib.util.spec_from_file_location('delivery', Path(__file__).with_name('delivery.py'))
delivery = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(delivery)


class DeliveryTests(unittest.TestCase):
    def test_exact_identity_only(self):
        good = {'id': 42, 'repository': {'full_name': 'HitKeep/hitkeep-docs'}, 'path': '.github/workflows/sync-public-hitkeep-release.yml',
                'event': 'workflow_dispatch', 'head_branch': 'main', 'head_sha': 'a' * 40,
                'display_title': 'exact correlation', 'status': 'completed', 'conclusion': 'success'}
        delivery.validate_delivery_run(good, 'HitKeep/hitkeep-docs', 'sync-public-hitkeep-release.yml', 'a' * 40, 'exact correlation')
        for field, value in [('head_sha', 'b' * 40), ('event', 'push'), ('head_branch', 'other'), ('path', '.github/workflows/deploy.yml'),
                             ('display_title', 'same tag unrelated correlation'), ('repository', {'full_name': 'attacker/repo'})]:
            run = dict(good, **{field: value})
            with self.subTest(field=field), self.assertRaises(ValueError):
                delivery.validate_delivery_run(run, 'HitKeep/hitkeep-docs', 'sync-public-hitkeep-release.yml', 'a' * 40, 'exact correlation')

    def test_dispatch_is_pending_until_actual_success(self):
        for state, conclusion in [('queued', None), ('in_progress', None), ('completed', 'failure'), ('completed', 'cancelled')]:
            with self.subTest(state=state, conclusion=conclusion):
                self.assertNotEqual(delivery.delivery_status({'status': state, 'conclusion': conclusion}), 'verified')
        self.assertEqual(delivery.delivery_status({'status': 'completed', 'conclusion': 'success'}), 'verified')

    def test_failed_docs_never_dispatches_cloud(self):
        import json
        writes = []
        manifest = {'artifacts': [{'name': name, 'sha256': 'b' * 64} for name in
                                 ('hitkeep-configuration.json', 'hitkeep-configuration-manifest.json', 'hitkeep-cloud-linux-arm64')]}
        run = {'id': 42, 'repository': {'full_name': 'HitKeep/hitkeep-docs'},
               'path': '.github/workflows/sync-public-hitkeep-release.yml', 'event': 'workflow_dispatch',
               'head_branch': 'main', 'head_sha': 'a' * 40,
               'display_title': 'Sync public HitKeep v2.10.0 · source ' + 'c' * 40 + ' · 1.1.docs',
               'status': 'completed', 'conclusion': 'failure'}
        def command(*args):
            if args[:3] == ('gh', 'workflow', 'run'):
                writes.append(args)
                return ''
            if '/git/ref/heads/main' in args[2]:
                return json.dumps({'object': {'sha': 'a' * 40}})
            if '/actions/workflows/' in args[2]:
                return json.dumps({'workflow_runs': [run]})
            return json.dumps(run)
        with patch.object(delivery, 'public_candidate', return_value=manifest), \
                patch.object(delivery.candidate, 'command', side_effect=command), self.assertRaisesRegex(RuntimeError, 'failed'):
            delivery.dispatch('docs', 'v2.10.0', 'c' * 40, '1.1.docs')
        self.assertEqual(len(writes), 1)
        self.assertEqual(writes[0][3], 'sync-public-hitkeep-release.yml')

    def test_timeout_is_pending_with_exact_run_link(self):
        import json
        manifest = {'artifacts': [{'name': name, 'sha256': 'b' * 64} for name in
                                 ('hitkeep-configuration.json', 'hitkeep-configuration-manifest.json')]}
        with patch.object(delivery, 'public_candidate', return_value=manifest), \
                patch.object(delivery.candidate, 'command', return_value=json.dumps({'object': {'sha': 'a' * 40}})), \
                patch.object(delivery.time, 'monotonic', side_effect=[0, 1]), self.assertRaisesRegex(TimeoutError, 'pending, completion unverified'):
            delivery.dispatch('docs', 'v2.10.0', 'c' * 40, '1.1.docs', timeout=0)


if __name__ == '__main__':
    unittest.main()
