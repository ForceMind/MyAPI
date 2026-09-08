#!/usr/bin/env python3
"""Inspect or remove known regenerable development caches; never session data."""
import argparse
import json
import os
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
        exe = (process / 'exe').resolve(strict=True)
        if exe.is_relative_to(release_root):
            protected.add(release_root / exe.relative_to(release_root).parts[0])
        if (process / 'comm').read_text().strip() in {'go', 'compile', 'link'}:
            go_running = True
    except (OSError, RuntimeError):
        continue
releases = [p for p in release_root.iterdir() if p.is_dir() and not p.is_symlink() and re.fullmatch(r'\d+\.\d+\.\d+-x86_64-unknown-linux-musl', p.name)] if release_root.exists() else []
releases.sort(key=lambda p: tuple(map(int, p.name.split('-')[0].split('.'))), reverse=True)
protected.update(releases[:2])  # Keep the current release and a recent rollback copy.
targets = [p for p in releases if p not in protected]
if not go_running:
    targets += [Path('/tmp/myapi-go-cache'), Path('/root/.cache/go-build')]
# Download archives are no longer needed once browser prerequisites are installed.
targets += [Path('/tmp/myapi-routing-browser/dnf-cache'), Path('/tmp/myapi-routing-browser/rpms')]
report = {'apply': args.apply, 'go_cache_skipped_active_build': go_running, 'preserved_codex_releases': sorted(str(p) for p in protected), 'targets': []}
for target in targets:
    if not target.exists() or target.is_symlink():
        continue
    size = int(subprocess.check_output(['du', '-sb', str(target)], text=True).split()[0])
    report['targets'].append({'path': str(target), 'bytes': size})
    if args.apply:
        shutil.rmtree(target)
report['total_bytes'] = sum(item['bytes'] for item in report['targets'])
print(json.dumps(report, ensure_ascii=False, indent=2))
