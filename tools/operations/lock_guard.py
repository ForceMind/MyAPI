"""Hold a stable, protected daemon lock inode until the parent's stdin closes."""
import fcntl
import json
import os
import stat
import sys


def main():
    line = sys.stdin.buffer.readline(8193)
    assert 0 < len(line) <= 8192
    request = json.loads(line)
    filename = request['lock_file']
    assert os.path.isabs(filename) and os.path.realpath(os.path.dirname(filename)) == os.path.dirname(filename)
    parent = os.stat(os.path.dirname(filename), follow_symlinks=False)
    assert stat.S_ISDIR(parent.st_mode) and parent.st_uid == os.getuid() and stat.S_IMODE(parent.st_mode) & 0o077 == 0
    fd = os.open(filename, os.O_RDWR | os.O_CREAT | os.O_NOFOLLOW, 0o600)
    try:
        info = os.fstat(fd)
        assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_uid == os.getuid() and stat.S_IMODE(info.st_mode) == 0o600
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        current = os.stat(filename, follow_symlinks=False)
        assert (current.st_dev, current.st_ino) == (info.st_dev, info.st_ino)
        print('{"locked":true}', flush=True)
        # The Node process owns the only write end. SIGKILL closes it, releasing
        # the kernel lock automatically. The stable inode is never unlinked.
        while sys.stdin.buffer.read(1024):
            pass
    finally:
        os.close(fd)


try:
    main()
except Exception:
    sys.exit(1)
