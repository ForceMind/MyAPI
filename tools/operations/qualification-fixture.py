"""Fixed synthetic accounting fixture for the isolated Actions qualification."""
import hashlib
import json
import os
import sqlite3
import stat
import sys
import time


TOKEN_ID = 900001
KEY_ID = 900001
BASE_REQUEST = 'operations-synthetic-baseline'
NEW_REQUEST = 'operations-synthetic-after-start-17'


def main():
    request = json.load(sys.stdin)
    assert os.getuid() == 0 and os.environ.get('GITHUB_ACTIONS') == 'true'
    root = request['root']
    assert os.path.isabs(root) and os.path.realpath(root) == root
    info = os.stat(root, follow_symlinks=False)
    assert stat.S_ISDIR(info.st_mode) and info.st_uid == 0 and stat.S_IMODE(info.st_mode) == 0o700
    marker = os.path.join(root, '.operations-owned.json')
    assert json.load(open(marker, encoding='utf8')) == {'run_id': os.environ['GITHUB_RUN_ID']}
    case = request['case']
    assert case in ('positive', 'identity', 'anonymous', 'publish-all', 'dynamic', 'crash', 'fault')
    database = os.path.join(root, case, 'data', 'my-api.db')
    info = os.lstat(database)
    assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and os.path.realpath(database) == database
    db = sqlite3.connect(database, timeout=10)
    db.row_factory = sqlite3.Row
    try:
        assert db.execute('PRAGMA journal_mode=WAL').fetchone()[0] == 'wal'
        db.execute('PRAGMA busy_timeout=10000')
        if request['action'] == 'seed':
            root_user = db.execute('SELECT id, role, status FROM users WHERE id=1').fetchone()
            assert root_user is not None and list(root_user) == [1, 100, 1]
            assert db.execute('SELECT COUNT(*) FROM tokens').fetchone()[0] == 0
            db.execute('UPDATE users SET quota=?,used_quota=?,request_count=? WHERE id=1', (100000, 30, 2))
            columns = {row['name'] for row in db.execute('PRAGMA table_info(tokens)')}
            assert {'id', 'user_id', 'key', 'name', 'quota_version', 'remain_quota', 'used_quota'}.issubset(columns)
            db.execute('INSERT INTO tokens (id,user_id,key,status,name,created_time,accessed_time,expired_time,remain_quota,used_quota,quota_version,credential_version,unlimited_quota,model_limits_enabled,model_limits,allow_ips,"group",access_profile_id,cross_group_retry,auto_groups) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)',
                       (TOKEN_ID, 1, request['token_secret'], 1, 'operations-synthetic', int(time.time()), 0, -1, 90000, 11, 7, 1, 0, 0, '', '', 'default', 'standard', 0, '[]'))
            # Fixed beta32 must create its own real schema. A missing table is
            # a baseline failure, never something this fixture manufactures.
            management_columns = {row['name'] for row in db.execute('PRAGMA table_info(management_keys)')}
            assert {'id', 'owner_id', 'secret_hash', 'scopes_json', 'channel_ids_json', 'token_ids_json', 'expires_at', 'revoked_at'}.issubset(management_columns)
            db.execute('INSERT INTO management_keys (id,owner_id,name,prefix,secret_hash,scopes_json,channel_ids_json,token_ids_json,created_at,expires_at,revoked_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)',
                       (KEY_ID, 1, 'operations-qualification', 'myapi_mgmt_fixture', request['management_hash'], '["operations.read","operations.prepare","operations.execute"]', '[]', '[]', int(time.time()), int(time.time()) + 7200, 0))
            db.execute('INSERT INTO logs (user_id,created_at,type,content,username,token_name,model_name,quota,prompt_tokens,completion_tokens,use_time,is_stream,channel_id,token_id,"group",ip,request_id,upstream_request_id,other) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)',
                       (1, int(time.time()), 2, 'synthetic qualification fixture; no model request', 'opsroot', 'operations-synthetic', 'synthetic-no-provider', 11, 3, 4, 0, 0, 0, TOKEN_ID, 'default', '127.0.0.1', BASE_REQUEST, '', '{"synthetic":true}'))
            db.commit()
        elif request['action'] == 'charge':
            db.execute('BEGIN IMMEDIATE')
            assert db.execute('SELECT COUNT(*) FROM logs WHERE request_id=?', (NEW_REQUEST,)).fetchone()[0] == 0
            assert db.execute('UPDATE users SET quota=quota-17,used_quota=used_quota+17,request_count=request_count+1 WHERE id=1 AND quota>=17').rowcount == 1
            assert db.execute('UPDATE tokens SET remain_quota=remain_quota-17,used_quota=used_quota+17,quota_version=quota_version+1 WHERE id=? AND remain_quota>=17', (TOKEN_ID,)).rowcount == 1
            db.execute('INSERT INTO logs (user_id,created_at,type,content,username,token_name,model_name,quota,prompt_tokens,completion_tokens,use_time,is_stream,channel_id,token_id,"group",ip,request_id,upstream_request_id,other) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)',
                       (1, int(time.time()), 2, 'synthetic committed 17 units; no model request', 'opsroot', 'operations-synthetic', 'synthetic-no-provider', 17, 0, 0, 0, 0, 0, TOKEN_ID, 'default', '127.0.0.1', NEW_REQUEST, '', '{"synthetic":true,"units":17}'))
            db.commit()
        else:
            assert request['action'] == 'snapshot'
        user = db.execute('SELECT quota,used_quota,request_count FROM users WHERE id=1').fetchone()
        token = db.execute('SELECT remain_quota,used_quota,quota_version FROM tokens WHERE id=?', (TOKEN_ID,)).fetchone()
        logs = db.execute('SELECT request_id,type,quota,token_id,model_name,content,other FROM logs WHERE request_id IN (?,?) ORDER BY request_id', (BASE_REQUEST, NEW_REQUEST)).fetchall()
        options = [list(row) for row in db.execute('SELECT "key",value FROM options ORDER BY "key"')]
        print(json.dumps({'user': dict(user), 'token': dict(token), 'logs': [dict(row) for row in logs], 'options_sha256': hashlib.sha256(json.dumps(options, separators=(',', ':')).encode()).hexdigest()}, separators=(',', ':')))
    finally:
        db.close()


try:
    main()
except Exception:
    sys.exit(1)
