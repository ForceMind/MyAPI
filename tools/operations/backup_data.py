"""Copy the registered stopped instance directory; never restores any data."""
import hashlib
import json
import os
import shutil
import sqlite3
import stat
import sys
import time
from urllib.parse import quote


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


try:
    main()
except Exception:
    sys.exit(1)
