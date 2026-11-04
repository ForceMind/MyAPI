"""Copy the registered stopped instance directory; never restores any data."""
import hashlib
import json
import os
import shutil
import stat
import sys


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


def main():
    request = json.load(sys.stdin)
    source, database = request['data_dir'], request['sqlite_file']
    assert os.path.realpath(source) == source and database.startswith(source + '/')
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
