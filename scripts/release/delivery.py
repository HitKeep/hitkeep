"""Dispatch one receiver and report only its exact completed GitHub Actions run."""
import argparse
import json
import os
from pathlib import Path
import re
import tempfile
import time

import candidate


def destinations(target):
    if target not in ('docs', 'cloud'):
        raise ValueError('select one independent delivery destination')
    return [target]


def validate_delivery_run(run, repo, workflow, head, title):
    if (run.get('repository', {}).get('full_name') != repo or run.get('path') != '.github/workflows/' + workflow or
            run.get('event') != 'workflow_dispatch' or run.get('head_branch') != 'main' or
            run.get('head_sha') != head or run.get('display_title') != title):
        raise ValueError('downstream run identity mismatch')


def delivery_status(run):
    if run['status'] != 'completed':
        return 'pending'
    return 'verified' if run['conclusion'] == 'success' else 'failed'


def public_candidate(tag, source_sha, root):
    candidate.version(tag)
    if not candidate.SHA.fullmatch(source_sha):
        raise ValueError('invalid release source SHA')
    release = candidate.json_api('releases/tags/' + tag)
    if release['draft'] or release['prerelease'] or release['tag_name'] != tag:
        raise ValueError('release is not public and stable')
    assets = [a for a in release['assets'] if a['name'] == 'release-candidate.json']
    if len(assets) != 1:
        raise ValueError('public release has no unique candidate evidence')
    path = root / 'release-candidate.json'
    path.write_bytes(candidate.api(f"releases/assets/{assets[0]['id']}", '-H', 'Accept: application/octet-stream', binary=True))
    manifest = json.loads(path.read_text())
    candidate.validate_manifest(manifest)
    if manifest['tag'] != tag or manifest['source_commit'] != source_sha:
        raise ValueError('public release candidate identity mismatch')
    run_id, attempt = manifest['producer_run_id'], manifest['producer_run_attempt']
    run = candidate.json_api(f'actions/runs/{run_id}/attempts/{attempt}')
    candidate.validate_run(run, run_id, attempt)
    if run['head_sha'] != manifest['producer_workflow_sha']:
        raise ValueError('public candidate workflow revision mismatch')
    tag_object = candidate.json_api('git/ref/tags/' + tag)['object']
    if tag_object['type'] == 'tag':
        tag_object = candidate.json_api('git/tags/' + tag_object['sha'])['object']
    if tag_object['type'] != 'commit' or tag_object['sha'] != source_sha:
        raise ValueError('public release tag changed')
    candidate.command('gh', 'attestation', 'verify', str(path), '--repo', candidate.REPO,
                      '--signer-workflow', candidate.REPO + '/' + candidate.WORKFLOW,
                      '--source-ref', 'refs/heads/main', '--source-digest', manifest['producer_workflow_sha'])
    return manifest


def dispatch(target, tag, source_sha, correlation, timeout=1800):
    destinations(target)
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,99}', correlation):
        raise ValueError('invalid delivery correlation')
    destination_token = os.environ.get('GH_TOKEN')
    try:
        if os.environ.get('SOURCE_READ_TOKEN'):
            os.environ['GH_TOKEN'] = os.environ['SOURCE_READ_TOKEN']
        with tempfile.TemporaryDirectory() as temp:
            manifest = public_candidate(tag, source_sha, Path(temp))
    finally:
        if destination_token is not None:
            os.environ['GH_TOKEN'] = destination_token
        else:
            os.environ.pop('GH_TOKEN', None)
    digests = {a['name']: a['sha256'] for a in manifest['artifacts']}
    if target == 'docs':
        repo, workflow = 'HitKeep/hitkeep-docs', 'sync-public-hitkeep-release.yml'
        title = f'Sync public HitKeep {tag} · source {source_sha} · {correlation}'
        inputs = {'release_tag': tag, 'source_sha': source_sha, 'correlation_id': correlation,
                  'configuration_sha256': digests['hitkeep-configuration.json'],
                  'manifest_sha256': digests['hitkeep-configuration-manifest.json']}
    else:
        repo, workflow = 'HitKeep/hitkeep-infra', 'release-deploy.yml'
        digest = digests['hitkeep-cloud-linux-arm64']
        title = f'Release deploy {tag} {digest} both {correlation}'
        inputs = {'stack': 'both', 'release_version': tag, 'source_sha': source_sha, 'correlation_id': correlation,
                  'artifact_sha256': digest, 'artifact_url': f'https://github.com/{candidate.REPO}/releases/download/{tag}/hitkeep-cloud-linux-arm64',
                  'mode': 'deploy'}
    head = json.loads(candidate.command('gh', 'api', f'repos/{repo}/git/ref/heads/main'))['object']['sha']
    args = ['gh', 'workflow', 'run', workflow, '--repo', repo, '--ref', 'main']
    for key, value in inputs.items():
        args.extend(['-f', key + '=' + value])
    candidate.command(*args)
    deadline = time.monotonic() + timeout
    run_id = None
    while time.monotonic() < deadline:
        if run_id is None:
            listing = json.loads(candidate.command('gh', 'api', f'repos/{repo}/actions/workflows/{workflow}/runs?event=workflow_dispatch&branch=main&per_page=100'))
            matches = [r for r in listing['workflow_runs'] if r.get('display_title') == title]
            if len(matches) > 1:
                raise ValueError('ambiguous downstream correlation; inspect both runs')
            if matches:
                validate_delivery_run(matches[0], repo, workflow, head, title)
                run_id = matches[0]['id']
        if run_id is not None:
            run = json.loads(candidate.command('gh', 'api', f'repos/{repo}/actions/runs/{run_id}'))
            validate_delivery_run(run, repo, workflow, head, title)
            status = delivery_status(run)
            url = f'https://github.com/{repo}/actions/runs/{run_id}'
            if status != 'pending':
                summary = f'{target}: {status} — {url}'
                print(summary, flush=True)
                if os.environ.get('GITHUB_STEP_SUMMARY'):
                    with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as output:
                        output.write(summary + '\n')
                if status != 'verified':
                    raise RuntimeError(summary)
                return
        time.sleep(10)
    url = f'https://github.com/{repo}/actions/runs/{run_id}' if run_id else f'https://github.com/{repo}/actions/workflows/{workflow}'
    raise TimeoutError(f'{target}: pending, completion unverified — {url}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--target', choices=('docs', 'cloud'), required=True)
    parser.add_argument('--tag', required=True)
    parser.add_argument('--source-sha', required=True)
    parser.add_argument('--correlation', required=True)
    args = parser.parse_args()
    dispatch(args.target, args.tag, args.source_sha, args.correlation)
