"""Native SQLite transition evidence; never restores or replays application data.

The trusted parent proves service quiescence before backup/check-stopped and
supplies the previously observed database inode. No Docker state is consumed.
Run with python3 -I -B and send exactly one bounded JSON document on stdin.
"""
import contextlib
import errno
import hashlib
import importlib.util
import math
import os
import re
import shutil
import sqlite3
import stat
import sys
import tempfile
import time
from urllib.parse import quote


# Load only the adjacent installed helper, without search-path or bytecode use.
_helper_path = os.path.join(os.path.dirname(os.path.realpath(__file__)), 'backup_data.py')
try:
    _helper_spec = importlib.util.spec_from_file_location('_myapi_native_backup_data', _helper_path)
    backup_data = importlib.util.module_from_spec(_helper_spec)
    _helper_spec.loader.exec_module(backup_data)
except Exception:
    print('{"ok":false,"code":"helper_unavailable"}')
    sys.exit(2)

MAX_BYTES = 16 * 1024 ** 3
MAX_ENTRIES = 10000
KEYS = {'action', 'data', 'database', 'device', 'inode', 'backup', 'config', 'receipt'}
FUND_COLUMNS = {
    'users': ('id', 'quota', 'used_quota', 'quota_version', 'request_count', 'aff_quota', 'aff_history'),
    'tokens': ('id', 'user_id', 'remain_quota', 'used_quota', 'quota_version', 'unlimited_quota'),
}
LOG_COLUMNS = ('id', 'user_id', 'created_at', 'type', 'quota', 'prompt_tokens', 'completion_tokens', 'token_id', 'billing_event_id')
REQUIRED_COLUMNS = {
    'users': {'id', 'quota', 'used_quota'},
    'tokens': {'id', 'user_id', 'remain_quota', 'used_quota'},
    'logs': {'id', 'type', 'quota'},
}
IMMUTABLE_TABLES = {
    'quota_mutation_receipts', 'account_quota_mutation_receipts', 'user_quota_mutation_receipts',
    'token_quota_adjustments', 'admin_quota_adjustments',
}
# These durable facts have mutable delivery/lease metadata. Preserve their
# actual financial identity/payload, without treating worker progress as money.
FACT_COLUMNS = {
    'task_billing_events': ('id', 'event_id', 'event_key', 'operation_id', 'task_id', 'event_type',
                           'user_id', 'token_id', 'channel_id', 'billing_source', 'subscription_id',
                           'quota_delta', 'request_id', 'payload_version', 'payload', 'created_at'),
    'task_billing_log_outboxes': ('id', 'billing_event_id', 'payload_version', 'payload', 'created_at'),
    'account_quota_settlement_facts': ('id', 'event_key', 'request_id', 'kind', 'user_id', 'token_id',
                                      'subscription_id', 'reserve_receipt_id', 'writer_epoch', 'actual_quota',
                                      'delta', 'apply_token', 'request_fingerprint', 'created_at'),
    'account_quota_refund_facts': ('id', 'event_key', 'kind', 'request_id', 'reserve_receipt_id', 'audit_key',
                                  'writer_epoch', 'user_id', 'token_id', 'subscription_id', 'wallet_quota',
                                  'subscription_quota', 'token_quota', 'request_fingerprint', 'created_at'),
}
FACT_REQUIRED = {
    'task_billing_events': {'id', 'event_id', 'quota_delta', 'user_id', 'token_id', 'payload'},
    'task_billing_log_outboxes': {'id', 'billing_event_id', 'payload'},
    'account_quota_settlement_facts': {'id', 'event_key', 'user_id', 'token_id', 'actual_quota', 'delta'},
    'account_quota_refund_facts': {'id', 'event_key', 'user_id', 'token_id', 'wallet_quota', 'subscription_quota', 'token_quota'},
}
WORK_STATES = {
    'tasks': ('status', ('SUCCESS', 'FAILURE')),
    'task_submission_operations': ('status', ('rejected', 'succeeded', 'failed', 'canceled')),
    'task_billing_events': ('state', ('applied',)),
    'task_billing_log_outboxes': ('state', ('delivered',)),
    'prompt_learning_runs': ('state', ('succeeded', 'skipped_no_new_data', 'cancelled', 'failed', 'manually_resolved')),
    'account_quota_settlement_intents': ('state', ('applied',)),
    'account_quota_settlement_facts': ('state', ('applied',)),
    'account_quota_refund_facts': ('state', ('applied',)),
    'admin_quota_adjustments': ('status', ('succeeded',)),
}


class TransitionFailure(Exception):
    pass


def check(condition, code):
    if not condition:
        raise TransitionFailure(code)


def decimal_identity(value, nonzero=False):
    return (isinstance(value, str) and re.fullmatch(r'0|[1-9][0-9]{0,19}', value) is not None
            and int(value) <= 18446744073709551615 and (not nonzero or int(value) > 0))


def context(request):
    check(sys.platform == 'linux', 'linux_required')
    check(isinstance(request, dict) and set(request) == KEYS
          and request['action'] in ('preflight', 'check-work', 'backup', 'check-stopped', 'check-live'), 'invalid_request')
    check(decimal_identity(request['device']) and decimal_identity(request['inode'], True), 'invalid_identity')
    for name in ('data', 'database', 'config', 'receipt'):
        backup_data.bundle_path(request[name], True)
    root = backup_data.bundle_path(request['backup'])
    parent = os.path.dirname(root)
    backup_data.bundle_private(parent)
    check(re.fullmatch(r'[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}', os.path.basename(root)) is not None, 'unsafe_path')
    check(request['database'].startswith(request['data'] + '/'), 'unsafe_path')
    check(not backup_data.bundle_overlap(request['data'], parent), 'unsafe_path')
    for name in ('config', 'receipt'):
        check(not backup_data.bundle_overlap(request[name], parent), 'unsafe_path')
    check(request['config'] != request['receipt'], 'unsafe_path')
    check(stat.S_ISDIR(os.lstat(request['data']).st_mode), 'unsafe_path')
    source_identity(request)
    return root, parent, os.path.relpath(request['database'], request['data'])


def source_identity(request):
    info = os.lstat(request['database'])
    check(stat.S_ISREG(info.st_mode) and info.st_nlink == 1
          and str(info.st_dev) == request['device'] and str(info.st_ino) == request['inode'], 'source_changed')
    # Repeat component checks after every potentially long operation.
    backup_data.bundle_path(request['data'], True)
    backup_data.bundle_path(request['database'], True)
    for suffix in ('-wal', '-shm', '-journal'):
        sidecar = request['database'] + suffix
        try:
            extra = os.lstat(sidecar)
        except FileNotFoundError:
            continue
        check(stat.S_ISREG(extra.st_mode) and extra.st_nlink == 1, 'unsafe_path')
        backup_data.bundle_path(sidecar, True)
    return info


def private_file(filename):
    before = os.lstat(filename)
    content = backup_data.bundle_read_file(filename, True)
    check(backup_data.bundle_signature(before) == backup_data.bundle_signature(os.lstat(filename)), 'source_changed')
    return content, {'signature': list(backup_data.bundle_signature(before)),
                     'sha256': hashlib.sha256(content).hexdigest(), 'size': len(content)}


def binding(request):
    return backup_data.bundle_digest({key: request[key] for key in KEYS - {'action'}})


def payload_rows(rows):
    return [{key: value for key, value in row.items() if key not in ('dev', 'ino', 'mtime_ns', 'ctime_ns')} for row in rows]


def source_tree(request):
    source_identity(request)
    rows = backup_data.bundle_tree(request['data'], MAX_BYTES, MAX_ENTRIES, with_inodes=True)
    source_identity(request)
    return rows


def preflight_tree(request):
    """Inspect capacity and safe entry types while a running WAL may grow."""
    backup_data.bundle_mount_guard(request['data'])
    rows, total = [], 0
    root = os.open(request['data'], os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    device = os.fstat(root).st_dev

    def visit(fd, relative):
        nonlocal total
        info = os.fstat(fd)
        check(info.st_dev == device, 'unsafe_path')
        check(os.geteuid() == 0 or (info.st_uid == os.geteuid()
              and info.st_gid in {os.getegid(), *os.getgroups()}), 'metadata_unavailable')
        backup_data.bundle_no_extra_metadata(fd)
        directory = stat.S_ISDIR(info.st_mode)
        check(directory or stat.S_ISREG(info.st_mode) and info.st_nlink == 1, 'unsafe_path')
        check(len(rows) < MAX_ENTRIES, 'bounds_exceeded')
        rows.append({'path': relative, 'type': 'directory' if directory else 'file', 'size': None if directory else info.st_size})
        if directory:
            names = backup_data.bundle_names(fd)
            for name in names:
                check(backup_data.bundle_relative(name) and '/' not in name, 'unsafe_path')
                child = os.stat(name, dir_fd=fd, follow_symlinks=False)
                check(stat.S_ISDIR(child.st_mode) or stat.S_ISREG(child.st_mode) and child.st_nlink == 1, 'unsafe_path')
                opened = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK
                                 | (os.O_DIRECTORY if stat.S_ISDIR(child.st_mode) else 0), dir_fd=fd)
                try:
                    actual = os.fstat(opened)
                    check((child.st_dev, child.st_ino) == (actual.st_dev, actual.st_ino), 'source_changed')
                    visit(opened, name if relative == '.' else relative + '/' + name)
                finally:
                    os.close(opened)
            check(names == backup_data.bundle_names(fd), 'source_changed')
        else:
            total += info.st_size
            check(total <= MAX_BYTES, 'bounds_exceeded')

    try:
        visit(root, '.')
    finally:
        os.close(root)
    backup_data.bundle_mount_guard(request['data'])
    source_identity(request)
    return rows


def space(parent, rows, external_bytes=0, copying=True):
    size = backup_data.bundle_counts(rows)['total_bytes']
    # One committed tree and one private verification copy, plus bounded metadata
    # and allocation overhead. Existing-backup checks need only the private copy.
    required = size * (2 if copying else 1) + external_bytes + 64 * 1024 ** 2 + len(rows) * 8192
    check(shutil.disk_usage(parent).free >= required, 'no_space')


def identifier(value):
    check(isinstance(value, str) and re.fullmatch(r'[A-Za-z_][A-Za-z0-9_]{0,127}', value) is not None, 'accounting_unknown')
    return '"' + value + '"'


def inventory(db):
    names = [row[0] for row in db.execute("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")]
    check(len(names) <= 1000 and set(REQUIRED_COLUMNS) <= set(names), 'accounting_unknown')
    result = []
    for name in names:
        if name not in REQUIRED_COLUMNS and name not in IMMUTABLE_TABLES and name not in FACT_COLUMNS:
            continue
        columns = db.execute('PRAGMA table_info(' + identifier(name) + ')').fetchall()
        check(columns and len(columns) <= 1000, 'accounting_unknown')
        available = [column[1] for column in columns]
        check(REQUIRED_COLUMNS.get(name, set()) <= set(available), 'accounting_unknown')
        check(FACT_REQUIRED.get(name, set()) <= set(available), 'accounting_unknown')
        primary = [column[1] for column in sorted(columns, key=lambda column: column[5]) if column[5]]
        check(primary and all(column in available for column in primary), 'accounting_unknown')
        chosen = LOG_COLUMNS if name == 'logs' else FUND_COLUMNS.get(name, FACT_COLUMNS.get(name, available))
        selected = [column for column in chosen if column in available]
        for column in primary:
            if column not in selected:
                selected.append(column)
        for column in selected:
            identifier(column)
        result.append({'name': name, 'columns': selected, 'primary': primary})
    return result


def check_work(db):
    # Missing optional modules are not invented as empty work queues. Present
    # modules must expose their actual state column and every row must be final.
    present = dict(db.execute("SELECT name,type FROM sqlite_master WHERE type IN ('table','view')"))
    for name, (column, settled) in WORK_STATES.items():
        if name not in present:
            continue
        check(present[name] == 'table', 'work_unknown')
        columns = {row[1] for row in db.execute('PRAGMA table_info(' + identifier(name) + ')')}
        check(column in columns, 'work_unknown')
        query = ('SELECT 1 FROM ' + identifier(name) + ' WHERE ' + identifier(column)
                 + ' IS NULL OR ' + identifier(column) + ' NOT IN (' + ','.join('?' for _ in settled) + ') LIMIT 1')
        check(db.execute(query, settled).fetchone() is None, 'work_pending')


def typed_row(row):
    # SQLite types are significant: TEXT "10" is not integer 10, and numeric
    # equality must not hide a floating-point rewrite of an integer balance.
    check(all(value is None or type(value) in (str, bytes, int, float) for value in row), 'accounting_unknown')
    check(all(not isinstance(value, float) or math.isfinite(value) for value in row), 'accounting_unknown')
    return tuple((type(value), value) for value in row)


def historical_rows(baseline, tables, live=None):
    for table in tables:
        name, columns, primary = table['name'], table['columns'], table['primary']
        selection = ','.join(identifier(column) for column in columns)
        query = 'SELECT ' + selection + ' FROM ' + identifier(name)
        lookup = query + ' WHERE ' + ' AND '.join(identifier(column) + ' IS ?' for column in primary)
        positions = [columns.index(column) for column in primary]
        for row in baseline.execute(query):
            expected = typed_row(row)
            numeric = set(columns) if name in FUND_COLUMNS else {'id', 'type', 'quota'} if name == 'logs' else set()
            check(all(type(row[index]) is int for index, column in enumerate(columns) if column in numeric), 'accounting_unknown')
            key = tuple(row[index] for index in positions)
            check(all(value is not None for value in key), 'accounting_unknown')
            if live is not None:
                cursor = live.execute(lookup, key)
                actual = cursor.fetchone()
                check(actual is not None and typed_row(actual) == expected and cursor.fetchone() is None, 'accounting_changed')


@contextlib.contextmanager
def read_database(filename):
    # mode=ro keeps WAL visibility; immutable would silently ignore saved WAL.
    db = sqlite3.connect('file:' + quote(filename, safe='/') + '?mode=ro', uri=True, timeout=3)
    try:
        db.execute('PRAGMA query_only=ON')
        db.execute('PRAGMA trusted_schema=OFF')
        if hasattr(db, 'setlimit'):
            db.setlimit(sqlite3.SQLITE_LIMIT_LENGTH, 16 * 1024 ** 2)
        deadline = time.monotonic() + 120
        db.set_progress_handler(lambda: int(time.monotonic() >= deadline), 1000)
        db.execute('BEGIN')
        check(db.execute('PRAGMA integrity_check').fetchall() == [('ok',)], 'sqlite_integrity')
        yield db
        db.rollback()
    finally:
        db.close()


@contextlib.contextmanager
def service_identity(info):
    uid, gid, groups = os.geteuid(), os.getegid(), os.getgroups()
    check(uid in (0, info.st_uid), 'live_identity_unknown')
    changed = uid == 0 and info.st_uid != 0
    try:
        if changed:
            os.setgroups([])
            os.setegid(info.st_gid)
            os.seteuid(info.st_uid)
        yield
    finally:
        if changed:
            os.seteuid(uid)
            os.setegid(gid)
            os.setgroups(groups)


@contextlib.contextmanager
def live_database(request):
    """Open as the database owner, pinning DB and existing sidecar inodes.

    SQLite can create/coordinate SHM even in mode=ro. Doing so as root inside
    the service directory would break ownership and is never allowed here.
    """
    info = source_identity(request)
    guards = {}
    try:
        for suffix in ('', '-wal', '-shm', '-journal'):
            filename = request['database'] + suffix
            try:
                observed = os.lstat(filename)
            except FileNotFoundError:
                continue
            check(stat.S_ISREG(observed.st_mode) and observed.st_nlink == 1 and observed.st_uid == info.st_uid, 'live_identity_unknown')
            fd = os.open(filename, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
            guards[suffix] = fd
            opened = os.fstat(fd)
            check((opened.st_dev, opened.st_ino) == (observed.st_dev, observed.st_ino), 'source_changed')
        with service_identity(info):
            with read_database(request['database']) as db:
                yield db
                source_identity(request)
        for suffix in ('', '-wal', '-shm', '-journal'):
            filename = request['database'] + suffix
            try:
                observed = os.lstat(filename)
            except FileNotFoundError:
                check(suffix not in guards, 'source_changed')
                continue
            check(stat.S_ISREG(observed.st_mode) and observed.st_nlink == 1 and observed.st_uid == info.st_uid, 'live_identity_unknown')
            backup_data.bundle_path(filename, True)
            if suffix in guards:
                opened = os.fstat(guards[suffix])
                check((opened.st_dev, opened.st_ino) == (observed.st_dev, observed.st_ino), 'source_changed')
    finally:
        for fd in guards.values():
            os.close(fd)


@contextlib.contextmanager
def saved_database(root, relative, expected):
    backup_data.bundle_private(root)
    with tempfile.TemporaryDirectory(prefix='.native-verify-', dir=root) as workspace:
        data = os.path.join(root, 'data')
        private_data = os.path.join(workspace, 'data')
        backup_data.bundle_copy(data, private_data, MAX_BYTES, MAX_ENTRIES)
        check(backup_data.bundle_tree(data, MAX_BYTES, MAX_ENTRIES) == expected
              == backup_data.bundle_tree(private_data, MAX_BYTES, MAX_ENTRIES), 'backup_unverified')
        with read_database(os.path.join(private_data, relative)) as db:
            yield db
        # Any SHM/checkpoint coordination happened only inside the private copy.
        check(backup_data.bundle_tree(data, MAX_BYTES, MAX_ENTRIES) == expected, 'backup_unverified')


def load_backup(request, root):
    backup_data.bundle_private(root)
    backup_data.bundle_mount_guard(root)
    check(sorted(os.listdir(root)) == ['COMMITTED', 'config', 'data', 'manifest.json', 'receipt'], 'backup_unverified')
    raw = backup_data.bundle_read_file(os.path.join(root, 'manifest.json'), True)
    manifest = backup_data.bundle_json(raw)
    marker = backup_data.bundle_json(backup_data.bundle_read_file(os.path.join(root, 'COMMITTED'), True))
    check(marker == {'sha256': hashlib.sha256(raw).hexdigest()}, 'backup_unverified')
    check(isinstance(manifest, dict) and set(manifest) == {'schema', 'binding', 'source', 'config', 'receipt', 'accounting'}
          and type(manifest['schema']) is int and manifest['schema'] == 1
          and manifest['binding'] == binding(request), 'backup_unverified')
    check(isinstance(manifest['source'], list) and 1 <= len(manifest['source']) <= MAX_ENTRIES, 'backup_unverified')
    expected = payload_rows(manifest['source'])
    check(backup_data.bundle_tree(os.path.join(root, 'data'), MAX_BYTES, MAX_ENTRIES) == expected, 'backup_unverified')
    for name in ('config', 'receipt'):
        content = backup_data.bundle_read_file(os.path.join(root, name), True)
        check(hashlib.sha256(content).hexdigest() == manifest[name]['sha256'] and len(content) == manifest[name]['size'], 'backup_unverified')
    return manifest, expected


def original_files(request, manifest):
    for name in ('config', 'receipt'):
        _, actual = private_file(request[name])
        check(actual == manifest[name], 'source_changed')


def main():
    raw = sys.stdin.buffer.read(65537)
    check(len(raw) <= 65536, 'invalid_request')
    request = backup_data.bundle_json(raw)
    root, parent, relative = context(request)
    if request['action'] == 'check-work':
        with live_database(request) as db:
            check_work(db)
        return {'ok': True, 'workSettled': True}
    if request['action'] in ('preflight', 'backup'):
        check(not os.path.lexists(root), 'backup_exists')
        before = preflight_tree(request) if request['action'] == 'preflight' else source_tree(request)
        files = {name: private_file(request[name]) for name in ('config', 'receipt')}
        space(parent, before, sum(len(value[0]) for value in files.values()))
        if request['action'] == 'preflight':
            return {'ok': True, 'preflight': True}
        os.mkdir(root, 0o700)
        backup_data.bundle_private(root)
        backup_data.bundle_copy(request['data'], os.path.join(root, 'data'), MAX_BYTES, MAX_ENTRIES)
        expected = payload_rows(before)
        check(backup_data.bundle_tree(os.path.join(root, 'data'), MAX_BYTES, MAX_ENTRIES) == expected, 'source_changed')
        for name, (content, _) in files.items():
            backup_data.bundle_write(os.path.join(root, name), content)
        with saved_database(root, relative, expected) as db:
            check_work(db)
            tables = inventory(db)
            historical_rows(db, tables)
        manifest = {'schema': 1, 'binding': binding(request), 'source': before,
                    'config': files['config'][1], 'receipt': files['receipt'][1], 'accounting': tables}
        original_files(request, manifest)
        check(source_tree(request) == before, 'source_changed')
        document = backup_data.bundle_canonical(manifest)
        check(len(document) <= 64 * 1024 ** 2, 'bounds_exceeded')
        backup_data.bundle_write(os.path.join(root, 'manifest.json'), document)
        backup_data.bundle_sync(root)
        try:
            backup_data.bundle_write(os.path.join(root, 'COMMITTED'), backup_data.bundle_canonical({'sha256': hashlib.sha256(document).hexdigest()}))
            backup_data.bundle_sync(root)
            backup_data.bundle_sync(parent)
            load_backup(request, root)
            original_files(request, manifest)
            check(source_tree(request) == before, 'source_changed')
        except Exception:
            # The helper created this new directory and marker itself. Keep the
            # failed payload for diagnosis, but never leave it committed.
            if os.path.lexists(os.path.join(root, 'COMMITTED')):
                os.unlink(os.path.join(root, 'COMMITTED'))
            backup_data.bundle_sync(root)
            raise
    else:
        manifest, expected = load_backup(request, root)
        space(parent, expected, copying=False)
        original_files(request, manifest)
        source_root = next(row for row in manifest['source'] if row['path'] == '.')
        observed_root = os.lstat(request['data'])
        check((str(observed_root.st_dev), str(observed_root.st_ino)) == (source_root['dev'], source_root['ino']), 'source_changed')
        if request['action'] == 'check-stopped':
            check(source_tree(request) == manifest['source'], 'source_changed')
        with saved_database(root, relative, expected) as baseline:
            tables = inventory(baseline)
            check(tables == manifest['accounting'], 'backup_unverified')
            if request['action'] == 'check-live':
                with live_database(request) as live:
                    check(inventory(live) == tables, 'accounting_unknown')
                    historical_rows(baseline, tables, live)
            else:
                historical_rows(baseline, tables)
        original_files(request, manifest)
        if request['action'] == 'check-stopped':
            check(source_tree(request) == manifest['source'], 'source_changed')
        else:
            source_identity(request)
        load_backup(request, root)
    result = {'ok': True, 'backup': root, 'backupVerified': True, 'accountingVerified': True}
    if request['action'] == 'check-stopped':
        result['sourceUnchanged'] = True
    if request['action'] == 'check-live':
        result['historicalRowsVerified'] = True
    return result


if __name__ == '__main__':
    try:
        reply = main()
    except TransitionFailure as error:
        reply = {'ok': False, 'code': str(error)}
    except backup_data.BundleFailure as error:
        message = str(error)
        code = ('unsafe_path' if any(word in message for word in ('PATH', 'LINK', 'MOUNT', 'METADATA', 'PRIVATE'))
                else 'source_changed' if 'CHANGED' in message else 'backup_unverified')
        reply = {'ok': False, 'code': code}
    except OSError as error:
        reply = {'ok': False, 'code': 'no_space' if error.errno == errno.ENOSPC else 'unavailable'}
    except Exception:
        reply = {'ok': False, 'code': 'accounting_unknown'}
    print(backup_data.bundle_canonical(reply).decode(), end='')
    if not reply['ok']:
        sys.exit(2)
