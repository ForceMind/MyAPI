"""Fixed read-only auth queries. mode=ro retains WAL visibility (never immutable)."""
import json
import os
import sqlite3
import stat
import sys
import time
from urllib.parse import quote


def main():
    request = json.load(sys.stdin)
    filename = request['sqlite_file']
    info = os.lstat(filename)
    assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and os.path.realpath(filename) == filename
    db = sqlite3.connect('file:' + quote(filename, safe='/') + '?mode=ro', uri=True, timeout=3)
    db.execute('PRAGMA query_only=ON')
    try:
        if request['action'] == 'actor':
            row = db.execute('SELECT id FROM users WHERE id = ? AND role = 100 AND status = 1 AND deleted_at IS NULL', (request['owner_id'],)).fetchone()
            return {'valid': bool(row)}
        now = int(time.time())
        if request['action'] == 'key':
            predicate, parameter = 'k.secret_hash = ?', request['secret_hash']
        else:
            assert request['action'] == 'key_id'
            predicate, parameter = 'k.id = ?', request['key_id']
        row = db.execute('SELECT k.id, k.owner_id, k.scopes_json, k.channel_ids_json, k.token_ids_json FROM management_keys k JOIN users u ON u.id = k.owner_id WHERE ' + predicate + ' AND k.revoked_at = 0 AND k.expires_at > ? AND u.role = 100 AND u.status = 1 AND u.deleted_at IS NULL', (parameter, now)).fetchone()
        if not row:
            return {'valid': False}
        scopes, channels, tokens = [json.loads(value) for value in row[2:]]
        assert isinstance(scopes, list) and all(isinstance(x, str) for x in scopes)
        assert isinstance(channels, list) and isinstance(tokens, list)
        valid = not channels and not tokens and request['scope'] in scopes
        if request.get('owner_id') is not None:
            valid = valid and row[1] == request['owner_id']
        return {'valid': valid, 'key_id': row[0], 'owner_id': row[1]}
    finally:
        db.close()


try:
    print(json.dumps(main(), separators=(',', ':')))
except Exception:
    print('{"valid":false}')
    sys.exit(1)
