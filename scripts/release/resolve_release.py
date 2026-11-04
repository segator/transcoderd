"""Resolve an existing stable release to immutable source before any build."""
import base64
import json
import os
import re
import subprocess
from pathlib import Path


def github_api(path):
    result = subprocess.run(
        ['gh', 'api', path], check=True, capture_output=True, text=True
    )
    return json.loads(result.stdout)


def resolve(tag, event, ref, sha, debug_run, repository, api=github_api):
    if not tag:
        return {'source_ref': '', 'source_sha': sha, 'release_tag': ''}
    if event != 'workflow_dispatch' or ref != 'refs/heads/main' or debug_run:
        raise ValueError('Release completion requires a main dispatch without debug fixtures')
    if not re.fullmatch(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', tag):
        raise ValueError('Expected a stable vMAJOR.MINOR.PATCH tag')
    prefix = f'repos/{repository}'
    release = api(f'{prefix}/releases/tags/{tag}')
    if release['draft'] or release['prerelease'] or release['tag_name'] != tag:
        raise ValueError('Expected an existing published stable release')
    if any(a['name'] == 'windows-update.json.sig' for a in release['assets']):
        raise ValueError('Release already has signed Windows assets; refuse replacement')
    obj = api(f'{prefix}/git/ref/tags/{tag}')['object']
    visited = set()
    for _ in range(6):
        if not re.fullmatch(r'[0-9a-f]{40}', obj['sha']) or obj['sha'] in visited:
            raise ValueError('Invalid or cyclic tag target')
        visited.add(obj['sha'])
        if obj['type'] == 'commit':
            break
        if obj['type'] != 'tag':
            raise ValueError('Tag must resolve to a commit')
        obj = api(f"{prefix}/git/tags/{obj['sha']}")['object']
    else:
        raise ValueError('Too many annotated tag indirections')
    source = obj['sha']
    comparison = api(f'{prefix}/compare/{source}...main')
    if comparison['status'] not in ('ahead', 'identical'):
        raise ValueError('Release commit is not an ancestor of main')
    content = api(f'{prefix}/contents/version.txt?ref={source}')
    if content['encoding'] != 'base64':
        raise ValueError('Unexpected version file encoding')
    version = base64.b64decode(content['content']).decode('utf-8').strip()
    if version != tag[1:]:
        raise ValueError('Tagged source version does not match release')
    return {'source_ref': source, 'source_sha': source, 'release_tag': tag}


if __name__ == '__main__':
    try:
        values = resolve(
            os.environ.get('RELEASE_TAG', ''), os.environ['GITHUB_EVENT_NAME'],
            os.environ['GITHUB_REF'], os.environ['GITHUB_SHA'],
            os.environ.get('WINDOWS_DEBUG_RUN', ''), os.environ['GITHUB_REPOSITORY']
        )
        with Path(os.environ['GITHUB_OUTPUT']).open('a', encoding='utf-8') as output:
            for key, value in values.items():
                output.write(f'{key}={value}\n')
        print('Release source validated' if values['release_tag'] else 'Normal workflow source')
    except (ValueError, KeyError, subprocess.CalledProcessError) as error:
        raise SystemExit(f'Release source validation failed: {error}') from None
