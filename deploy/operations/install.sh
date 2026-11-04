#!/bin/sh
# Offline installation only. Does not start/enable the service or alter MyAPI.
set -eu
[ "$(id -u)" = 0 ] || { echo 'root required' >&2; exit 1; }
[ "$(uname -s)" = Linux ] && [ "$(uname -m)" = x86_64 ] || { echo 'Linux amd64 required' >&2; exit 1; }
command -v node >/dev/null && command -v python3 >/dev/null || { echo 'Install Node 22 and Python 3 separately' >&2; exit 1; }
node -e 'if(Number(process.versions.node.split(".")[0])<22)process.exit(1)'
source_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
for destination in /opt/myapi-operations /etc/myapi-operations /var/lib/myapi-operations /etc/systemd/system/myapi-operations.service; do
  [ ! -e "$destination" ] && [ ! -L "$destination" ] || { echo 'Installation target already exists; inspect and preserve it before installing' >&2; exit 1; }
done
install -d -m 0700 /opt/myapi-operations/tools/operations /opt/myapi-operations/tools/runtime /etc/myapi-operations /var/lib/myapi-operations/state /var/lib/myapi-operations/backups
for source in "$source_root"/tools/operations/*.mjs "$source_root"/tools/operations/*.py; do
  case "$source" in *.test.mjs|*/docker-qualification.mjs|*/qualification-fixture.py) continue ;; esac
  install -m 0600 "$source" /opt/myapi-operations/tools/operations/
done
install -m 0600 "$source_root/tools/runtime/smoke-image-bundle.mjs" /opt/myapi-operations/tools/runtime/
install -m 0600 "$source_root/deploy/operations/config.example.json" /etc/myapi-operations/config.example.json
install -m 0644 "$source_root/deploy/operations/myapi-operations.service" /etc/systemd/system/myapi-operations.service
echo 'Installed offline templates only. Configure config.json, TLS certificate/key, bridge.token and read-only github.token (root:root 0600), register exact paths, then inspect the unit before starting manually.'
