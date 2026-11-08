"""Registered-directory copier and isolated bundles; callers prove quiescence."""
import hashlib
import json
import os
import shutil
import sqlite3
import stat
import sys
import tempfile
import time
from urllib.parse import quote


class BundleFailure(Exception):
    pass


def bundle_check(condition, code):
    if not condition:
        raise BundleFailure(code)


def bundle_path(value, existing=False):
    bundle_check(isinstance(value, str) and len(value.encode()) <= 4096 and value != '/' and os.path.isabs(value)
                 and os.path.normpath(value) == value
                 and not any(ord(c) < 32 or 127 <= ord(c) <= 159 for c in value), 'BUNDLE_UNSAFE_PATH')
    current = '/'
    for component in value.split('/')[1:]:
        bundle_check(component not in ('', '.', '..'), 'BUNDLE_UNSAFE_PATH')
        current = os.path.join(current, component)
        try:
            info = os.lstat(current)
        except FileNotFoundError:
            bundle_check(not existing, 'BUNDLE_PATH_UNAVAILABLE')
            continue
        bundle_check(not stat.S_ISLNK(info.st_mode), 'BUNDLE_UNSAFE_PATH')
    bundle_check(os.path.realpath(value) == value, 'BUNDLE_UNSAFE_PATH')
    return value


def bundle_overlap(left, right):
    return left == right or left.startswith(right + '/') or right.startswith(left + '/')


def bundle_private(directory):
    bundle_path(directory, True)
    info = os.lstat(directory)
    bundle_check(stat.S_ISDIR(info.st_mode) and info.st_uid == os.geteuid()
                 and stat.S_IMODE(info.st_mode) == 0o700, 'BUNDLE_PRIVATE_DIRECTORY_REQUIRED')


def bundle_limits(request):
    maximum, entries = request['max_bytes'], request['max_entries']
    bundle_check(type(maximum) is int and 1 <= maximum <= 16 * 1024 ** 3
                 and type(entries) is int and 1 <= entries <= 10000, 'BUNDLE_LIMIT_INVALID')
    return maximum, entries


def bundle_canonical(value):
    return (json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(',', ':')) + '\n').encode()


def bundle_digest(value):
    return hashlib.sha256(bundle_canonical(value)).hexdigest()


def bundle_sha(value):
    return isinstance(value, str) and len(value) == 64 and all(c in '0123456789abcdef' for c in value)


def bundle_relative(value, root=False):
    return isinstance(value, str) and len(value.encode()) <= 4096 and ((root and value == '.') or
        value and not value.startswith('/') and '\\' not in value
        and all(part not in ('', '.', '..') for part in value.split('/'))
        and not any(ord(c) < 32 or 127 <= ord(c) <= 159 for c in value))


def bundle_mount_guard(source):
    # st_dev alone misses same-filesystem bind mounts. Unknown inventory is
    # unavailable, never an invented empty mount list.
    bundle_check(sys.platform.startswith('linux'), 'BUNDLE_MOUNT_INVENTORY_UNAVAILABLE')
    try:
        with open('/proc/self/mountinfo', encoding='utf-8') as reader:
            lines = reader.readlines()
        bundle_check(bool(lines), 'BUNDLE_MOUNT_INVENTORY_UNAVAILABLE')
        for line in lines:
            fields = line.split(' - ', 1)[0].split()
            bundle_check(len(fields) >= 6, 'BUNDLE_MOUNT_INVENTORY_UNAVAILABLE')
            mount = fields[4]
            for escaped, literal in [('\\040', ' '), ('\\011', '\t'), ('\\012', '\n'), ('\\134', '\\')]:
                mount = mount.replace(escaped, literal)
            bundle_check(not mount.startswith(source + '/'), 'BUNDLE_SUBMOUNT_REJECTED')
    except BundleFailure:
        raise
    except Exception:
        raise BundleFailure('BUNDLE_MOUNT_INVENTORY_UNAVAILABLE') from None


def bundle_signature(info):
    return (info.st_dev, info.st_ino, info.st_size, info.st_mtime_ns,
            info.st_ctime_ns, info.st_uid, info.st_gid, info.st_mode, info.st_nlink)


def bundle_names(fd):
    os.lseek(fd, 0, os.SEEK_SET)
    return sorted(os.listdir(fd))


def bundle_no_extra_metadata(fd):
    # Manifest v1 covers bytes and POSIX ownership/mode. Refuse ACLs or other
    # xattrs rather than silently discard unrepresented security metadata.
    bundle_check(not os.listxattr(fd), 'BUNDLE_EXTENDED_METADATA_UNSUPPORTED')


def bundle_tree(source, maximum, max_entries):
    bundle_path(source, True)
    bundle_mount_guard(source)
    entries, total = [], 0
    root_fd = os.open(source, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    device = os.fstat(root_fd).st_dev

    def visit(fd, relative):
        nonlocal total
        bundle_check(bundle_relative(relative, True), 'BUNDLE_UNSAFE_PATH')
        info = os.fstat(fd)
        bundle_check(info.st_dev == device, 'BUNDLE_SUBMOUNT_REJECTED')
        bundle_no_extra_metadata(fd)
        directory = stat.S_ISDIR(info.st_mode)
        bundle_check(directory or stat.S_ISREG(info.st_mode) and info.st_nlink == 1, 'BUNDLE_SPECIAL_OR_LINK_REJECTED')
        bundle_check(len(entries) < max_entries, 'BUNDLE_ENTRY_LIMIT_EXCEEDED')
        row = {'path': relative, 'type': 'directory' if directory else 'file',
               'uid': info.st_uid, 'gid': info.st_gid, 'mode': stat.S_IMODE(info.st_mode),
               'size': None, 'sha256': None}
        entries.append(row)
        if directory:
            names = bundle_names(fd)
            for name in names:
                bundle_check(bundle_relative(name) and '/' not in name, 'BUNDLE_UNSAFE_PATH')
                child = os.stat(name, dir_fd=fd, follow_symlinks=False)
                bundle_check(not stat.S_ISLNK(child.st_mode) and
                             (stat.S_ISDIR(child.st_mode) or stat.S_ISREG(child.st_mode) and child.st_nlink == 1), 'BUNDLE_SPECIAL_OR_LINK_REJECTED')
                flags = os.O_RDONLY | os.O_NOFOLLOW | (os.O_DIRECTORY if stat.S_ISDIR(child.st_mode) else 0)
                child_fd = os.open(name, flags, dir_fd=fd)
                try:
                    bundle_check(bundle_signature(child) == bundle_signature(os.fstat(child_fd)), 'BUNDLE_SOURCE_CHANGED')
                    visit(child_fd, name if relative == '.' else relative + '/' + name)
                finally:
                    os.close(child_fd)
            bundle_check(names == bundle_names(fd), 'BUNDLE_SOURCE_CHANGED')
        else:
            total += info.st_size
            bundle_check(total <= maximum, 'BUNDLE_BYTE_LIMIT_EXCEEDED')
            checksum, count = hashlib.sha256(), 0
            while True:
                chunk = os.read(fd, 1024 * 1024)
                if not chunk:
                    break
                count += len(chunk)
                bundle_check(count <= info.st_size, 'BUNDLE_SOURCE_CHANGED')
                checksum.update(chunk)
            bundle_check(count == info.st_size, 'BUNDLE_SOURCE_CHANGED')
            row['size'], row['sha256'] = info.st_size, checksum.hexdigest()
        bundle_check(bundle_signature(info) == bundle_signature(os.fstat(fd)), 'BUNDLE_SOURCE_CHANGED')

    try:
        visit(root_fd, '.')
    finally:
        os.close(root_fd)
    bundle_mount_guard(source)
    return sorted(entries, key=lambda row: row['path'])


def bundle_copy(source, target, maximum, max_entries):
    """Exclusive fd-relative copy; failures retain the uncommitted target."""
    total, count = 0, 0
    source_fd = os.open(source, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.mkdir(target, 0o700)
        target_fd = os.open(target, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    except Exception:
        os.close(source_fd)
        raise
    device = os.fstat(source_fd).st_dev

    def metadata(fd, original):
        actual = os.fstat(fd)
        if (actual.st_uid, actual.st_gid) != (original.st_uid, original.st_gid):
            os.fchown(fd, original.st_uid, original.st_gid)
        os.fchmod(fd, stat.S_IMODE(original.st_mode))
        os.utime(fd, ns=(original.st_atime_ns, original.st_mtime_ns))
        os.fsync(fd)

    def copy_directory(reader, writer):
        nonlocal total, count
        original = os.fstat(reader)
        bundle_check(original.st_dev == device, 'BUNDLE_SUBMOUNT_REJECTED')
        bundle_no_extra_metadata(reader)
        count += 1
        bundle_check(count <= max_entries, 'BUNDLE_ENTRY_LIMIT_EXCEEDED')
        names = bundle_names(reader)
        for name in names:
            bundle_check(bundle_relative(name) and '/' not in name, 'BUNDLE_UNSAFE_PATH')
            info = os.stat(name, dir_fd=reader, follow_symlinks=False)
            bundle_check(info.st_dev == device, 'BUNDLE_SUBMOUNT_REJECTED')
            bundle_check(not stat.S_ISLNK(info.st_mode) and (stat.S_ISDIR(info.st_mode) or
                         stat.S_ISREG(info.st_mode) and info.st_nlink == 1), 'BUNDLE_SPECIAL_OR_LINK_REJECTED')
            incoming = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | (os.O_DIRECTORY if stat.S_ISDIR(info.st_mode) else 0), dir_fd=reader)
            try:
                bundle_check(bundle_signature(info) == bundle_signature(os.fstat(incoming)), 'BUNDLE_SOURCE_CHANGED')
                bundle_no_extra_metadata(incoming)
                if stat.S_ISDIR(info.st_mode):
                    os.mkdir(name, 0o700, dir_fd=writer)
                    outgoing = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=writer)
                    try:
                        copy_directory(incoming, outgoing)
                    finally:
                        os.close(outgoing)
                else:
                    count += 1
                    bundle_check(count <= max_entries, 'BUNDLE_ENTRY_LIMIT_EXCEEDED')
                    outgoing = os.open(name, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=writer)
                    try:
                        copied = 0
                        while True:
                            chunk = os.read(incoming, 1024 * 1024)
                            if not chunk:
                                break
                            copied += len(chunk)
                            total += len(chunk)
                            bundle_check(copied <= info.st_size, 'BUNDLE_SOURCE_CHANGED')
                            bundle_check(total <= maximum, 'BUNDLE_BYTE_LIMIT_EXCEEDED')
                            offset = 0
                            while offset < len(chunk):
                                written = os.write(outgoing, chunk[offset:])
                                bundle_check(written > 0, 'BUNDLE_COPY_UNAVAILABLE')
                                offset += written
                        bundle_check(copied == info.st_size, 'BUNDLE_SOURCE_CHANGED')
                        metadata(outgoing, info)
                    finally:
                        os.close(outgoing)
                bundle_check(bundle_signature(info) == bundle_signature(os.fstat(incoming)), 'BUNDLE_SOURCE_CHANGED')
            finally:
                os.close(incoming)
        bundle_check(names == bundle_names(reader) and bundle_signature(original) == bundle_signature(os.fstat(reader)), 'BUNDLE_SOURCE_CHANGED')
        metadata(writer, original)

    try:
        copy_directory(source_fd, target_fd)
    finally:
        os.close(source_fd)
        os.close(target_fd)


def bundle_sync(directory):
    fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)


def bundle_read_file(filename, private=False):
    info = os.lstat(filename)
    bundle_check(stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_size <= 64 * 1024 ** 2, 'BUNDLE_METADATA_INVALID')
    if private:
        bundle_check(info.st_uid == os.geteuid() and stat.S_IMODE(info.st_mode) == 0o600, 'BUNDLE_METADATA_INVALID')
    fd = os.open(filename, os.O_RDONLY | os.O_NOFOLLOW)
    try:
        bundle_check(bundle_signature(info) == bundle_signature(os.fstat(fd)), 'BUNDLE_METADATA_CHANGED')
        with os.fdopen(fd, 'rb', closefd=False) as stream:
            content = stream.read(64 * 1024 ** 2 + 1)
        bundle_check(len(content) <= 64 * 1024 ** 2 and bundle_signature(info) == bundle_signature(os.fstat(fd)), 'BUNDLE_METADATA_CHANGED')
        return content
    finally:
        os.close(fd)


def bundle_json(raw):
    def pairs(values):
        result = {}
        for key, value in values:
            bundle_check(key not in result, 'BUNDLE_SCHEMA_INVALID')
            result[key] = value
        return result
    try:
        return json.loads(raw, object_pairs_hook=pairs,
                          parse_constant=lambda value: (_ for _ in ()).throw(BundleFailure('BUNDLE_SCHEMA_INVALID')))
    except BundleFailure:
        raise
    except Exception:
        raise BundleFailure('BUNDLE_SCHEMA_INVALID') from None


def bundle_context(request):
    maximum, entries = bundle_limits(request)
    source = bundle_path(request['data_dir'])
    database = bundle_path(request['sqlite_file'])
    bundle_check(database.startswith(source + '/'), 'BUNDLE_UNSAFE_PATH')
    relative = database[len(source) + 1:]
    bundle_check(bundle_relative(relative), 'BUNDLE_UNSAFE_PATH')
    if request['action'] == 'stopped_scan':
        return source, relative, maximum, entries, None
    backups, state = bundle_path(request['backups_dir'], True), bundle_path(request['state_dir'], True)
    bundle_private(backups)
    bundle_check(stat.S_ISDIR(os.lstat(state).st_mode), 'BUNDLE_UNSAFE_PATH')
    bundle_check(not any(bundle_overlap(a, b) for a, b in [(source, backups), (source, state), (backups, state)]), 'BUNDLE_PATH_OVERLAP')
    identifier = request['bundle_id']
    bundle_check(isinstance(identifier, str) and 1 <= len(identifier) <= 64 and identifier[0] in 'abcdefghijklmnopqrstuvwxyz0123456789'
                 and all(c in 'abcdefghijklmnopqrstuvwxyz0123456789-' for c in identifier), 'BUNDLE_ID_INVALID')
    bundle_check(bundle_sha(request['identity_sha256']), 'BUNDLE_IDENTITY_INVALID')
    return source, relative, maximum, entries, os.path.join(backups, identifier)


def bundle_counts(tree):
    return {'file_count': sum(row['type'] == 'file' for row in tree),
            'directory_count': sum(row['type'] == 'directory' for row in tree),
            'total_bytes': sum(row['size'] for row in tree if row['type'] == 'file')}


def bundle_integrity(data, relative, maximum, max_entries):
    """SQLite only touches a disposable DB/WAL/SHM copy, never bundle/source."""
    parent = os.path.dirname(data)
    bundle_private(parent)
    workspace = tempfile.mkdtemp(prefix='.myapi-bundle-integrity-', dir=parent)
    database = None
    try:
        filename = os.path.join(workspace, 'check.db')
        total = 0
        for suffix in ('', '-wal', '-shm'):
            source = os.path.join(data, relative + suffix)
            try:
                before = os.lstat(source)
            except FileNotFoundError:
                bundle_check(suffix != '', 'BUNDLE_DATABASE_MISSING')
                continue
            bundle_check(stat.S_ISREG(before.st_mode) and before.st_nlink == 1
                         and os.path.realpath(source) == source, 'BUNDLE_SPECIAL_OR_LINK_REJECTED')
            incoming = os.open(source, os.O_RDONLY | os.O_NOFOLLOW)
            try:
                bundle_check(bundle_signature(before) == bundle_signature(os.fstat(incoming)), 'BUNDLE_SOURCE_CHANGED')
                outgoing = os.open(filename + suffix, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
                with os.fdopen(outgoing, 'wb') as writer:
                    copied = 0
                    while True:
                        chunk = os.read(incoming, 1024 * 1024)
                        if not chunk:
                            break
                        copied += len(chunk)
                        total += len(chunk)
                        bundle_check(copied <= before.st_size and total <= maximum, 'BUNDLE_SOURCE_CHANGED')
                        writer.write(chunk)
                    bundle_check(copied == before.st_size, 'BUNDLE_SOURCE_CHANGED')
                    writer.flush()
                    os.fsync(writer.fileno())
                bundle_check(bundle_signature(before) == bundle_signature(os.fstat(incoming)), 'BUNDLE_SOURCE_CHANGED')
            finally:
                os.close(incoming)
        deadline = time.monotonic() + 120
        database = sqlite3.connect('file:' + quote(filename, safe='/') + '?mode=ro', uri=True, timeout=3)
        database.execute('PRAGMA query_only=ON')
        database.set_progress_handler(lambda: int(time.monotonic() >= deadline), 1000)
        database.execute('BEGIN')
        bundle_check(database.execute('PRAGMA integrity_check').fetchall() == [('ok',)], 'BUNDLE_SQLITE_INTEGRITY_FAILED')
        database.rollback()
    except BundleFailure:
        raise
    except sqlite3.DatabaseError as error:
        code = getattr(error, 'sqlite_errorcode', None)
        if type(code) is int and code & 0xff in (11, 26):
            raise BundleFailure('BUNDLE_SQLITE_INTEGRITY_FAILED') from None
        raise BundleFailure('BUNDLE_SQLITE_INTEGRITY_UNAVAILABLE') from None
    except Exception:
        raise BundleFailure('BUNDLE_SQLITE_INTEGRITY_UNAVAILABLE') from None
    finally:
        failed = sys.exc_info()[0] is not None
        cleanup_failed = False
        try:
            if database is not None:
                database.close()
        except Exception:
            cleanup_failed = True
        try:
            shutil.rmtree(workspace)
        except Exception:
            cleanup_failed = True
        if cleanup_failed and not failed:
            raise BundleFailure('BUNDLE_VALIDATION_CLEANUP_UNAVAILABLE')


def bundle_validate_manifest(value, request, relative):
    keys = {'schema_version', 'kind', 'bundle_id', 'identity_sha256', 'source_path_sha256', 'sqlite_relative', 'created_unix_seconds', 'tree_sha256', 'entries'}
    bundle_check(isinstance(value, dict) and set(value) == keys and value['schema_version'] == 1
                 and type(value['schema_version']) is int and value['kind'] == 'myapi-sqlite-directory-backup', 'BUNDLE_SCHEMA_INVALID')
    bundle_check(value['bundle_id'] == request['bundle_id'] and value['identity_sha256'] == request['identity_sha256']
                 and value['source_path_sha256'] == hashlib.sha256(request['data_dir'].encode()).hexdigest()
                 and value['sqlite_relative'] == relative, 'BUNDLE_IDENTITY_MISMATCH')
    bundle_check(type(value['created_unix_seconds']) is int and 0 < value['created_unix_seconds'] <= 9007199254740991
                 and bundle_sha(value['tree_sha256']), 'BUNDLE_SCHEMA_INVALID')
    rows = value['entries']
    bundle_check(isinstance(rows, list) and 1 <= len(rows) <= request['max_entries'], 'BUNDLE_SCHEMA_INVALID')
    seen, total = set(), 0
    for row in rows:
        bundle_check(isinstance(row, dict) and set(row) == {'path', 'type', 'uid', 'gid', 'mode', 'size', 'sha256'}
                     and bundle_relative(row['path'], True) and row['path'] not in seen, 'BUNDLE_SCHEMA_INVALID')
        seen.add(row['path'])
        bundle_check(all(type(row[key]) is int and 0 <= row[key] <= 4294967295 for key in ('uid', 'gid'))
                     and type(row['mode']) is int and 0 <= row['mode'] <= 0o7777, 'BUNDLE_SCHEMA_INVALID')
        if row['type'] == 'directory':
            bundle_check(row['size'] is None and row['sha256'] is None, 'BUNDLE_SCHEMA_INVALID')
        else:
            bundle_check(row['type'] == 'file' and type(row['size']) is int and row['size'] >= 0
                         and bundle_sha(row['sha256']), 'BUNDLE_SCHEMA_INVALID')
            total += row['size']
    bundle_check(total <= request['max_bytes'] and rows == sorted(rows, key=lambda row: row['path'])
                 and rows[0]['path'] == '.' and rows[0]['type'] == 'directory'
                 and any(row['path'] == relative and row['type'] == 'file' for row in rows)
                 and bundle_digest(rows) == value['tree_sha256'], 'BUNDLE_SCHEMA_INVALID')
    return rows


def bundle_load(request, committed):
    source, relative, maximum, max_entries, root = bundle_context(request)
    bundle_private(root)
    bundle_mount_guard(root)
    expected_names = ['COMMITTED', 'data', 'manifest.json'] if committed else ['data', 'manifest.json']
    bundle_check(sorted(os.listdir(root)) == expected_names, 'BUNDLE_UNCOMMITTED_OR_EXTRA_FILES')
    bundle_check(bundle_sha(request['expected_manifest_sha256']), 'BUNDLE_DIGEST_REQUIRED')
    raw = bundle_read_file(os.path.join(root, 'manifest.json'), True)
    digest = hashlib.sha256(raw).hexdigest()
    bundle_check(digest == request['expected_manifest_sha256'], 'BUNDLE_MANIFEST_DIGEST_MISMATCH')
    value = bundle_json(raw)
    tree = bundle_validate_manifest(value, request, relative)
    if committed:
        marker = bundle_json(bundle_read_file(os.path.join(root, 'COMMITTED'), True))
        expected = {'schema_version': 1, 'bundle_id': request['bundle_id'],
                    'manifest_sha256': digest, 'identity_sha256': request['identity_sha256'],
                    'tree_sha256': value['tree_sha256']}
        bundle_check(isinstance(marker, dict) and set(marker) == set(expected) and type(marker['schema_version']) is int
                     and marker == expected, 'BUNDLE_COMMIT_INVALID')
    actual = bundle_tree(os.path.join(root, 'data'), maximum, max_entries)
    bundle_check(actual == tree, 'BUNDLE_PAYLOAD_CHANGED')
    return root, value, digest, source, relative, maximum, max_entries


def bundle_result(request, value, digest, **flags):
    return {'schema_version': 1, 'bundle_id': request['bundle_id'], 'manifest_sha256': digest,
            'identity_sha256': value['identity_sha256'], 'tree_sha256': value['tree_sha256'],
            **bundle_counts(value['entries']), 'source_quiescence_verified': False, **flags}


def bundle_write(filename, content):
    fd = os.open(filename, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
    with os.fdopen(fd, 'wb') as writer:
        writer.write(content)
        writer.flush()
        os.fchmod(writer.fileno(), 0o600)
        os.fsync(writer.fileno())


def bundle_request(request):
    action = request['action']
    keys = {'action', 'data_dir', 'sqlite_file', 'max_bytes', 'max_entries'}
    if action != 'stopped_scan':
        keys |= {'backups_dir', 'state_dir', 'bundle_id', 'identity_sha256'}
    if action in ('bundle_commit', 'bundle_verify', 'bundle_restore'):
        keys.add('expected_manifest_sha256')
    if action == 'bundle_commit':
        keys.add('expected_tree_sha256')
    if action == 'bundle_restore':
        keys.add('destination')
    bundle_check(set(request) == keys, 'BUNDLE_REQUEST_INVALID')
    source, relative, maximum, max_entries, root = bundle_context(request)
    if action in ('stopped_scan', 'bundle_prepare'):
        before = bundle_tree(source, maximum, max_entries)
        bundle_check(any(row['path'] == relative and row['type'] == 'file' for row in before), 'BUNDLE_DATABASE_MISSING')
        if action == 'stopped_scan':
            return {'schema_version': 1, 'tree_sha256': bundle_digest(before),
                    **bundle_counts(before), 'source_quiescence_verified': False}
        os.mkdir(root, 0o700)
        bundle_private(root)
        bundle_mount_guard(root)
        payload = os.path.join(root, 'data')
        bundle_copy(source, payload, maximum, max_entries)
        bundle_check(bundle_tree(source, maximum, max_entries) == before == bundle_tree(payload, maximum, max_entries), 'BUNDLE_SOURCE_CHANGED')
        bundle_integrity(payload, relative, maximum, max_entries)
        bundle_check(bundle_tree(source, maximum, max_entries) == before == bundle_tree(payload, maximum, max_entries), 'BUNDLE_SOURCE_CHANGED')
        value = {'schema_version': 1, 'kind': 'myapi-sqlite-directory-backup', 'bundle_id': request['bundle_id'],
                 'identity_sha256': request['identity_sha256'], 'source_path_sha256': hashlib.sha256(source.encode()).hexdigest(),
                 'sqlite_relative': relative, 'created_unix_seconds': int(time.time()),
                 'tree_sha256': bundle_digest(before), 'entries': before}
        bundle_validate_manifest(value, request, relative)
        raw = bundle_canonical(value)
        bundle_check(len(raw) <= 64 * 1024 ** 2, 'BUNDLE_METADATA_LIMIT_EXCEEDED')
        bundle_write(os.path.join(root, 'manifest.json'), raw)
        bundle_sync(root)
        bundle_sync(request['backups_dir'])
        return bundle_result(request, value, hashlib.sha256(raw).hexdigest(), prepared=True, committed=False, integrity_verified=True)
    root, value, digest, source, relative, maximum, max_entries = bundle_load(request, action != 'bundle_commit')
    payload = os.path.join(root, 'data')
    bundle_integrity(payload, relative, maximum, max_entries)
    bundle_check(bundle_tree(payload, maximum, max_entries) == value['entries'], 'BUNDLE_PAYLOAD_CHANGED')
    if action == 'bundle_commit':
        bundle_check(bundle_sha(request['expected_tree_sha256']) and request['expected_tree_sha256'] == value['tree_sha256']
                     and bundle_tree(source, maximum, max_entries) == value['entries'], 'BUNDLE_SOURCE_CHANGED')
        marker = bundle_canonical({'schema_version': 1, 'bundle_id': request['bundle_id'],
                                   'manifest_sha256': digest, 'identity_sha256': request['identity_sha256'], 'tree_sha256': value['tree_sha256']})
        temporary = os.path.join(root, '.commit-' + os.urandom(12).hex())
        bundle_write(temporary, marker)
        os.link(temporary, os.path.join(root, 'COMMITTED'), follow_symlinks=False)
        os.unlink(temporary)
        bundle_sync(root)
        return bundle_result(request, value, digest, committed=True, verified=True, integrity_verified=True)
    if action == 'bundle_restore':
        target = bundle_path(request['destination'])
        bundle_private(os.path.dirname(target))
        bundle_check(not any(bundle_overlap(target, directory) for directory in
                             (source, request['backups_dir'], request['state_dir'])), 'BUNDLE_PATH_OVERLAP')
        bundle_check(not os.path.lexists(target), 'BUNDLE_TARGET_EXISTS')
        bundle_copy(payload, target, maximum, max_entries)
        bundle_check(bundle_tree(target, maximum, max_entries) == value['entries'] == bundle_tree(payload, maximum, max_entries), 'BUNDLE_RESTORE_CHANGED')
        bundle_integrity(target, relative, maximum, max_entries)
        bundle_check(bundle_tree(target, maximum, max_entries) == value['entries'], 'BUNDLE_RESTORE_CHANGED')
        bundle_sync(os.path.dirname(target))
        return bundle_result(request, value, digest, committed=True, verified=True, integrity_verified=True, restored=True)
    return bundle_result(request, value, digest, committed=True, verified=True, integrity_verified=True)


def summary(filename):
    values = {}
    for suffix in ('', '-wal', '-shm'):
        target = filename + suffix
        try:
            info = os.lstat(target)
        except FileNotFoundError:
            values[suffix or 'db'] = None
            continue
        assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and os.path.realpath(target) == target
        checksum = hashlib.sha256()
        fd = os.open(target, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, 'rb') as source:
            while True:
                chunk = source.read(1024 * 1024)
                if not chunk:
                    break
                checksum.update(chunk)
        after = os.lstat(target)
        assert (info.st_ino, info.st_size, info.st_mtime_ns, info.st_ctime_ns) == (after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns)
        values[suffix or 'db'] = {'bytes': info.st_size, 'sha256': checksum.hexdigest()}
    assert values['db'] is not None
    return values


def live_summary(source, database):
    """Observe one read snapshot; a changing WAL is normal after startup."""
    guard = None
    db = None
    try:
        directory = os.stat(source, follow_symlinks=False)
        assert stat.S_ISDIR(directory.st_mode) and os.path.realpath(source) == source
        before = os.lstat(database)
        assert stat.S_ISREG(before.st_mode) and before.st_nlink == 1 and os.path.realpath(database) == database
        guard = os.open(database, os.O_RDONLY | os.O_NOFOLLOW)
        opened = os.fstat(guard)
        assert (before.st_dev, before.st_ino) == (opened.st_dev, opened.st_ino)
        for suffix in ('-wal', '-shm'):
            try:
                info = os.lstat(database + suffix)
            except FileNotFoundError:
                continue
            assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and os.path.realpath(database + suffix) == database + suffix
        deadline = time.monotonic() + 5
        db = sqlite3.connect('file:' + quote(database, safe='/') + '?mode=ro', uri=True, timeout=3)
        db.execute('PRAGMA query_only=ON')
        db.set_progress_handler(lambda: int(time.monotonic() >= deadline), 1000)
        db.execute('BEGIN')
        health = db.execute('PRAGMA quick_check(1)').fetchall()
        if health != [('ok',)]:
            return {'state': 'corrupt', 'code': 'SQLITE_QUICK_CHECK_FAILED'}
        counts = {}
        for table in ('users', 'tokens', 'logs'):
            counts[table] = db.execute('SELECT COUNT(*) FROM ' + table).fetchone()[0]
        db.rollback()
        after = os.lstat(database)
        current_directory = os.stat(source, follow_symlinks=False)
        assert os.path.realpath(source) == source and os.path.realpath(database) == database
        assert (directory.st_dev, directory.st_ino) == (current_directory.st_dev, current_directory.st_ino)
        assert stat.S_ISREG(after.st_mode) and after.st_nlink == 1
        assert (opened.st_dev, opened.st_ino) == (after.st_dev, after.st_ino)
        # No size/mtime/ctime or WAL hash comparison: valid writers may commit
        # during this transaction, while these three counts remain one snapshot.
        return {'state': 'observed', 'counts': counts}
    except sqlite3.DatabaseError as error:
        # Stable SQLite result codes: SQLITE_CORRUPT=11, SQLITE_NOTADB=26.
        if (getattr(error, 'sqlite_errorcode', 0) & 0xff) in (11, 26):
            return {'state': 'corrupt', 'code': 'SQLITE_CORRUPT'}
        return {'state': 'unknown', 'code': 'SQLITE_OBSERVATION_UNAVAILABLE'}
    except Exception:
        return {'state': 'unknown', 'code': 'SQLITE_OBSERVATION_UNAVAILABLE'}
    finally:
        if db is not None:
            db.close()
        if guard is not None:
            os.close(guard)


def main():
    request = json.load(sys.stdin)
    if request.get('action') in ('stopped_scan', 'bundle_prepare', 'bundle_commit', 'bundle_verify', 'bundle_restore'):
        try:
            print(json.dumps(bundle_request(request), separators=(',', ':')))
        except Exception as error:
            code = str(error) if isinstance(error, BundleFailure) else 'BUNDLE_OPERATION_UNAVAILABLE'
            print(json.dumps({'schema_version': 1, 'verified': False, 'error': code}, separators=(',', ':')))
            sys.exit(1)
        return
    source, database = request['data_dir'], request['sqlite_file']
    assert os.path.realpath(source) == source and database.startswith(source + '/')
    if request['action'] == 'live_summary':
        print(json.dumps(live_summary(source, database), separators=(',', ':')))
        return
    before = summary(database)
    if request['action'] == 'summary':
        print(json.dumps(before, separators=(',', ':')))
        return
    assert request['action'] == 'backup'
    target = request['destination']
    assert os.path.realpath(os.path.dirname(target)) == os.path.dirname(target)
    assert target != source and not target.startswith(source + '/')
    # Validate the complete tree before making a copy; no links or special files.
    total = 0
    maximum = min(request['max_bytes'], 16 * 1024 ** 3)
    for root, directories, files in os.walk(source, followlinks=False):
        for name in directories + files:
            info = os.lstat(os.path.join(root, name))
            assert not stat.S_ISLNK(info.st_mode)
            assert stat.S_ISDIR(info.st_mode) or stat.S_ISREG(info.st_mode) and info.st_nlink == 1
            if stat.S_ISREG(info.st_mode):
                total += info.st_size
                assert total <= maximum
    os.mkdir(target, 0o700)
    for root, directories, files in os.walk(source, followlinks=False):
        destination = target + root[len(source):]
        info = os.lstat(root)
        assert stat.S_ISDIR(info.st_mode) and os.path.realpath(root) == root
        for name in directories:
            os.mkdir(os.path.join(destination, name), 0o700)
        for name in files:
            src, dst = os.path.join(root, name), os.path.join(destination, name)
            metadata = os.lstat(src)
            assert stat.S_ISREG(metadata.st_mode) and metadata.st_nlink == 1
            fd = os.open(src, os.O_RDONLY | os.O_NOFOLLOW)
            out = os.open(dst, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, stat.S_IMODE(metadata.st_mode))
            with os.fdopen(fd, 'rb') as reader, os.fdopen(out, 'wb') as writer:
                shutil.copyfileobj(reader, writer, 1024 * 1024)
                writer.flush()
                os.fchown(writer.fileno(), metadata.st_uid, metadata.st_gid)
                os.fchmod(writer.fileno(), stat.S_IMODE(metadata.st_mode))
                os.fsync(writer.fileno())
            shutil.copystat(src, dst, follow_symlinks=False)
            after = os.lstat(src)
            assert (metadata.st_ino, metadata.st_size, metadata.st_mtime_ns, metadata.st_ctime_ns) == (after.st_ino, after.st_size, after.st_mtime_ns, after.st_ctime_ns)
        os.chown(destination, info.st_uid, info.st_gid, follow_symlinks=False)
        shutil.copystat(root, destination, follow_symlinks=False)
    copied = summary(target + database[len(source):])
    after = summary(database)
    assert before == copied == after
    for root, _, _ in os.walk(target, topdown=False):
        fd = os.open(root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
        os.fsync(fd)
        os.close(fd)
    print(json.dumps({'before': before, 'after': after, 'copied': copied}, separators=(',', ':')))


if __name__ == '__main__':
    try:
        main()
    except Exception:
        sys.exit(1)
