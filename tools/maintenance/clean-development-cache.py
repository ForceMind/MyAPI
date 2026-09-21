#!/usr/bin/env python3
"""Inspect or remove known regenerable development caches; never session data."""
import argparse
import json
from pathlib import Path
import re
import shutil
import subprocess

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--apply', action='store_true', help='Remove the displayed cache targets')
args = parser.parse_args()
release_root = Path('/root/.codex/packages/standalone/releases')
current = Path('/root/.codex/packages/standalone/current').resolve()
protected = {current}
go_running = False
for process in Path('/proc').glob('[0-9]*'):
    try:
        executable = (process / 'exe').resolve(strict=True)
        if executable.is_relative_to(release_root):
            protected.add(release_root / executable.relative_to(release_root).parts[0])
        if (process / 'comm').read_text().strip() in {'go', 'compile', 'link'}:
            go_running = True
    except (OSError, RuntimeError):
        continue
releases = [path for path in release_root.iterdir() if path.is_dir() and not path.is_symlink() and re.fullmatch(r'\d+\.\d+\.\d+-x86_64-unknown-linux-musl', path.name)] if release_root.exists() else []
releases.sort(key=lambda path: tuple(map(int, path.name.split('-')[0].split('.'))), reverse=True)
protected.update(releases[:2])
targets = [path for path in releases if path not in protected]
if not go_running:
    targets += [Path('/tmp/myapi-go-cache'), Path('/root/.cache/go-build')]
targets += [Path('/tmp/myapi-routing-browser/dnf-cache'), Path('/tmp/myapi-routing-browser/rpms')]
report = {
    'apply': args.apply,
    'go_cache_skipped_active_build': go_running,
    'preserved_codex_releases': sorted(str(path) for path in protected),
    'targets': [],
}
for target in targets:
    if not target.exists() or target.is_symlink():
        continue
    size = int(subprocess.check_output(['du', '-sb', str(target)], text=True).split()[0])
    report['targets'].append({'path': str(target), 'bytes': size})
    if args.apply:
        shutil.rmtree(target)
report['total_bytes'] = sum(item['bytes'] for item in report['targets'])
print(json.dumps(report, ensure_ascii=False, indent=2))
