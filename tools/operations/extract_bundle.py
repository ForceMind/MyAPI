"""Extract only the two fixed artifact members, with bounded decompression."""
import json
import os
import stat
import sys
import zipfile


def main():
    request = json.load(sys.stdin)
    directory = request['directory']
    assert os.path.realpath(directory) == directory
    maximum = min(request['max_bytes'], 8 * 1024 ** 3)
    archive = os.path.join(directory, 'artifact.zip')
    info = os.lstat(archive)
    assert stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_size <= maximum
    with zipfile.ZipFile(archive, 'r') as bundle:
        entries = bundle.infolist()
        assert len(entries) == 2 and sorted(x.filename for x in entries) == ['images.tar', 'manifest.json']
        assert sum(x.file_size for x in entries) <= maximum + 16384
        for member in entries:
            mode = member.external_attr >> 16
            assert not member.is_dir() and not stat.S_ISLNK(mode)
            assert stat.S_IFMT(mode) in (0, stat.S_IFREG)
            assert not member.flag_bits & 1 and member.compress_type in (zipfile.ZIP_STORED, zipfile.ZIP_DEFLATED)
            bound = 16384 if member.filename == 'manifest.json' else maximum
            assert 0 < member.file_size <= bound
            assert member.compress_size > 0 and member.file_size <= member.compress_size * 1000
            fd = os.open(os.path.join(directory, member.filename), os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            count = 0
            with os.fdopen(fd, 'wb') as output, bundle.open(member, 'r') as source:
                while True:
                    chunk = source.read(1024 * 1024)
                    if not chunk:
                        break
                    count += len(chunk)
                    assert count <= bound and count <= member.file_size
                    output.write(chunk)
                assert count == member.file_size
                output.flush()
                os.fsync(output.fileno())
    print('{"extracted":true}')


try:
    main()
except Exception:
    sys.exit(1)
