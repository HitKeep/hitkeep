#!/usr/bin/env bash
set -euo pipefail
[[ "$VERSION" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ && "$SOURCE_SHA" =~ ^[a-f0-9]{40}$ ]]
test "$(git rev-parse HEAD)" = "$SOURCE_SHA"
test "$(jq -r .version frontend/dashboard/package.json)" = "${VERSION#v}"
git diff --exit-code -- public/embed.go
stage="$(mktemp -d "$RUNNER_TEMP/source-stage.XXXXXX")"
export stage
git archive --format=tar "$SOURCE_SHA" | tar -xf - -C "$stage"
cp -R public/. "$stage/public/"
for file in go.mod go.sum LICENSE cmd/hitkeep/main.go public/embed.go public/index.html public/hk.js public/hk-vitals.js; do
  test -s "$stage/$file"
done
python3 - <<'PY'
import gzip, os, tarfile
from pathlib import Path, PurePosixPath
from html.parser import HTMLParser
stage = Path(os.environ['stage'])
class Assets(HTMLParser):
    def handle_starttag(self, tag, attrs):
        if tag != 'script':
            return
        src = dict(attrs).get('src')
        if src and (PurePosixPath(src).is_absolute() or '..' in PurePosixPath(src).parts or not src.endswith('.js') or not (stage / 'public' / src).is_file()):
            raise ValueError('dashboard script is missing or not local')
index = (stage / 'public/index.html').read_text()
if '<script' not in index:
    raise ValueError('dashboard contains no application scripts')
Assets().feed(index)
prefix = 'hitkeep-' + os.environ['VERSION'][1:]
output = Path(os.environ['RUNNER_TEMP']) / 'source-package'
output.mkdir(exist_ok=True)
with (output / ('hitkeep-source-' + os.environ['VERSION'][1:] + '.tar.gz')).open('wb') as raw:
    with gzip.GzipFile(filename='', fileobj=raw, mode='wb', mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w', format=tarfile.PAX_FORMAT) as archive:
            for path in sorted(stage.rglob('*')):
                if path.is_symlink() or not (path.is_file() or path.is_dir()):
                    raise ValueError('source archive contains an unsupported member')
                info = archive.gettarinfo(str(path), prefix + '/' + path.relative_to(stage).as_posix())
                info.uid = info.gid = info.mtime = 0
                info.uname = info.gname = ''
                if path.is_file():
                    with path.open('rb') as content:
                        archive.addfile(info, content)
                else:
                    archive.addfile(info)
PY
