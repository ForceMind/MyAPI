"""Read-only evidence for a visible Linux-host maintenance window.

The parent owns Docker identity, stop, global/instance locks and manifests.
This helper does not prove absence of malicious root or future writers.
"""
import hashlib
import json
import os
import platform
import re
import stat
import sys

PROC = "/proc"
MAX_TREE_ENTRIES = 200000
MAX_PROCESSES = 65536
MAX_TASKS = 262144
MAX_FDS = 1048576
MAX_MAP_LINES = 4000000
MAX_PROC_BYTES = 32 * 1024 * 1024
LOCAL_FILESYSTEMS = frozenset(("ext4", "xfs", "btrfs", "tmpfs"))


class GuardError(Exception):
    def __init__(self, status, code):
        self.status = status
        self.code = code


def reject(code):
    raise GuardError("blocked", code)


def unknown(code):
    raise GuardError("unknown", code)


def fingerprint(value):
    encoded = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode()
    return hashlib.sha256(encoded).hexdigest()


def exact_keys(value, expected):
    if not isinstance(value, dict) or set(value) != set(expected):
        reject("INVALID_REQUEST")


def decimal(value):
    return isinstance(value, str) and re.fullmatch(r"0|[1-9][0-9]*", value) is not None


def pid_value(value):
    return type(value) is int and 0 < value <= 2147483647


def read_proc(path):
    # Production paths are constructed internally beneath fixed /proc.
    with open(path, "rb") as stream:
        data = stream.read(MAX_PROC_BYTES + 1)
    if len(data) > MAX_PROC_BYTES:
        unknown("PROC_LIMIT")
    try:
        return data.decode("utf-8")
    except UnicodeError:
        unknown("PROC_PARSE_UNKNOWN")


def unescape_mount(value):
    result = []
    index = 0
    escapes = {"040": " ", "011": "\t", "012": "\n", "134": "\\"}
    while index < len(value):
        if value[index] == "\\":
            code = value[index + 1:index + 4]
            if code not in escapes:
                unknown("MOUNT_PARSE_UNKNOWN")
            result.append(escapes[code])
            index += 4
        else:
            result.append(value[index])
            index += 1
    return "".join(result)


def parse_mountinfo(text):
    mounts = []
    for line in text.splitlines():
        halves = line.split(" - ")
        if len(halves) != 2:
            unknown("MOUNT_PARSE_UNKNOWN")
        left, right = halves[0].split(), halves[1].split()
        if len(left) < 6 or len(right) < 3 or not left[0].isdigit() or not left[1].isdigit():
            unknown("MOUNT_PARSE_UNKNOWN")
        device = left[2].split(":")
        if len(device) != 2 or not all(piece.isdigit() for piece in device):
            unknown("MOUNT_PARSE_UNKNOWN")
        mountpoint = unescape_mount(left[4])
        if not os.path.isabs(mountpoint):
            unknown("MOUNT_PARSE_UNKNOWN")
        mounts.append({
            "id": int(left[0]), "device": ":".join(str(int(piece)) for piece in device),
            "root": unescape_mount(left[3]), "point": mountpoint,
            "type": right[0], "options": set(left[5].split(",") + right[2].split(",")),
        })
    if not mounts:
        unknown("MOUNT_PARSE_UNKNOWN")
    return mounts


def within(path, root):
    return path == root or path.startswith(root.rstrip("/") + "/")


def select_mount(mounts, path):
    candidates = [mount for mount in mounts if within(path, mount["point"])]
    if not candidates:
        unknown("MOUNT_UNAVAILABLE")
    return max(candidates, key=lambda mount: len(mount["point"]))


def proc_view(mounts):
    mount = select_mount(mounts, PROC)
    if mount["point"] != PROC or mount["type"] != "proc":
        unknown("PROC_VISIBILITY_UNKNOWN")
    for option in mount["options"]:
        if option.startswith("hidepid=") and option.split("=", 1)[1] not in ("0", "off"):
            unknown("PROC_VISIBILITY_UNKNOWN")
        if option.startswith("subset=") and option != "subset=all":
            unknown("PROC_VISIBILITY_UNKNOWN")
    try:
        namespace = os.stat(PROC + "/self/ns/pid").st_ino
    except OSError:
        unknown("PROC_VISIBILITY_UNKNOWN")
    return {"mount_id": mount["id"], "pid_namespace": str(namespace)}


def parse_process_stat(text, expected_pid):
    closing = text.rfind(")")
    opening = text.find("(")
    if opening < 0 or closing <= opening or text[:opening].strip() != str(expected_pid):
        unknown("PROC_STAT_UNKNOWN")
    fields = text[closing + 1:].split()
    if len(fields) < 20 or len(fields[0]) != 1 or not fields[19].isdigit():
        unknown("PROC_STAT_UNKNOWN")
    return int(fields[19])


def process_start(pid, tid=None):
    base = f"{PROC}/{pid}"
    if tid is not None:
        base += f"/task/{tid}"
    return parse_process_stat(read_proc(base + "/stat"), tid if tid is not None else pid)


def generation_gone(old_starttime, current_starttime):
    return current_starttime is None or old_starttime != current_starttime


def confirm_process_gone(pid, starttime, tid=None):
    try:
        current = process_start(pid, tid)
    except FileNotFoundError:
        base = f"{PROC}/{pid}" + (f"/task/{tid}" if tid is not None else "")
        if path_absent(base):
            return True
        unknown("PROC_SCAN_RACE")
    except OSError:
        unknown("PROC_ACCESS_UNKNOWN")
    return generation_gone(starttime, current)


def parse_fdinfo(text):
    values = {}
    for line in text.splitlines():
        key, separator, value = line.partition(":")
        if separator and key in ("flags", "mnt_id", "ino"):
            if key in values:
                unknown("FDINFO_PARSE_UNKNOWN")
            values[key] = value.strip()
    if set(values) != {"flags", "mnt_id", "ino"} or re.fullmatch(r"[0-7]+", values["flags"]) is None:
        unknown("FDINFO_PARSE_UNKNOWN")
    if not values["mnt_id"].isdigit() or not values["ino"].isdigit():
        unknown("FDINFO_PARSE_UNKNOWN")
    return int(values["flags"], 8), int(values["mnt_id"]), int(values["ino"])


def parse_maps(text):
    mappings = []
    for line in text.splitlines():
        parts = line.split(None, 5)
        if len(parts) < 5 or re.fullmatch(r"[0-9a-fA-F]+-[0-9a-fA-F]+", parts[0]) is None:
            unknown("MAPS_PARSE_UNKNOWN")
        if re.fullmatch(r"[r-][w-][x-][ps]", parts[1]) is None:
            unknown("MAPS_PARSE_UNKNOWN")
        device = parts[3].split(":")
        if len(device) != 2 or any(re.fullmatch(r"[0-9a-fA-F]+", part) is None for part in device) or not parts[4].isdigit():
            unknown("MAPS_PARSE_UNKNOWN")
        mappings.append((int(device[0], 16), int(device[1], 16), int(parts[4]), parts[1]))
    return mappings


def root_identity(info):
    return {"dev": str(info.st_dev), "ino": str(info.st_ino), "uid": info.st_uid,
            "gid": info.st_gid, "mode": format(stat.S_IMODE(info.st_mode), "04o")}


def source_root(path, mounts):
    if not isinstance(path, str) or not os.path.isabs(path) or os.path.normpath(path) != path or path == "/" or any(ord(char) < 32 or ord(char) == 127 for char in path):
        reject("UNSAFE_SOURCE_ROOT")
    if os.path.realpath(path) != path:
        reject("UNSAFE_SOURCE_LINK")
    current = "/"
    previous = None
    for part in [""] + path.strip("/").split("/"):
        if part:
            current = os.path.join(current, part)
        info = os.lstat(current)
        if not stat.S_ISDIR(info.st_mode) or stat.S_ISLNK(info.st_mode) or info.st_uid != 0:
            reject("UNSAFE_SOURCE_ANCESTOR")
        if previous is not None and previous.st_mode & 0o022 and not (previous.st_mode & stat.S_ISVTX and previous.st_uid == 0 and info.st_uid == 0):
            reject("UNSAFE_SOURCE_ANCESTOR")
        if current == path and info.st_mode & 0o022:
            reject("SOURCE_ROOT_WRITABLE")
        previous = info
    mount = select_mount(mounts, path)
    if mount["type"] not in LOCAL_FILESYSTEMS:
        reject("UNSUPPORTED_FILESYSTEM")
    device = f"{os.major(info.st_dev)}:{os.minor(info.st_dev)}"
    if mount["device"] != device:
        unknown("FILESYSTEM_IDENTITY_UNKNOWN")
    if any(mount["point"] != path and within(mount["point"], path) for mount in mounts):
        reject("NESTED_SOURCE_MOUNT")
    filesystem = {"type": mount["type"], "device": device, "mount_id": mount["id"]}
    return root_identity(info), filesystem


def snapshot_tree(path, expected):
    entries = []
    inodes = set()
    root_fd = os.open(path, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)
    try:
        if root_identity(os.fstat(root_fd)) != expected:
            unknown("SOURCE_ROOT_CHANGED")
        root_info = os.fstat(root_fd)
        entries.append(["", expected["dev"], expected["ino"], root_info.st_uid, root_info.st_gid,
                        stat.S_IMODE(root_info.st_mode), root_info.st_size, root_info.st_mtime_ns, root_info.st_ctime_ns])

        def visit(directory, relative):
            before = os.fstat(directory)
            names = sorted(entry.name for entry in os.scandir(directory))
            for name in names:
                if len(entries) >= MAX_TREE_ENTRIES:
                    unknown("TREE_LIMIT")
                info = os.stat(name, dir_fd=directory, follow_symlinks=False)
                if info.st_dev != int(expected["dev"]):
                    reject("NESTED_SOURCE_FILESYSTEM")
                is_directory = stat.S_ISDIR(info.st_mode)
                if not is_directory and (not stat.S_ISREG(info.st_mode) or info.st_nlink != 1):
                    reject("UNSAFE_SOURCE_ENTRY")
                if info.st_uid != 0 or info.st_mode & 0o022:
                    reject("UNSAFE_SOURCE_OWNER_OR_MODE")
                child = (relative + "/" if relative else "") + name
                entries.append([child, str(info.st_dev), str(info.st_ino), info.st_uid, info.st_gid,
                                stat.S_IMODE(info.st_mode), info.st_size, info.st_mtime_ns, info.st_ctime_ns])
                inodes.add((os.major(info.st_dev), os.minor(info.st_dev), info.st_ino))
                if is_directory:
                    fd = os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=directory)
                    try:
                        opened = os.fstat(fd)
                        if (opened.st_dev, opened.st_ino) != (info.st_dev, info.st_ino):
                            unknown("SOURCE_TREE_CHANGED")
                        visit(fd, child)
                    finally:
                        os.close(fd)
            after = os.fstat(directory)
            if (before.st_dev, before.st_ino, before.st_mtime_ns, before.st_ctime_ns) != (after.st_dev, after.st_ino, after.st_mtime_ns, after.st_ctime_ns):
                unknown("SOURCE_TREE_CHANGED")

        visit(root_fd, "")
        final = os.lstat(path)
        if root_identity(final) != expected:
            unknown("SOURCE_ROOT_CHANGED")
    finally:
        os.close(root_fd)
    return {"entries": len(entries), "metadata_sha256": fingerprint(entries)}, inodes


def visible_ids(path):
    result = []
    with os.scandir(path) as iterator:
        for entry in iterator:
            if entry.name.isdigit() and entry.is_dir(follow_symlinks=False):
                result.append(int(entry.name))
    return sorted(result)


def path_absent(path):
    try:
        os.lstat(path)
    except FileNotFoundError:
        return True
    except OSError:
        unknown("PROC_ACCESS_UNKNOWN")
    return False


def descriptor_gone(path):
    try:
        os.stat(path)
    except FileNotFoundError:
        return True
    except OSError:
        unknown("PROC_ACCESS_UNKNOWN")
    return False


def scan_task(pid, tid, starttime, source_inodes, counts):
    base = f"{PROC}/{pid}/task/{tid}"
    try:
        with os.scandir(base + "/fd") as iterator:
            descriptors = sorted(int(entry.name) for entry in iterator if entry.name.isdigit())
    except FileNotFoundError:
        if confirm_process_gone(pid, starttime, tid):
            counts["disappeared_tasks"] += 1
            return
        unknown("PROC_SCAN_RACE")
    for descriptor in descriptors:
        counts["fds"] += 1
        if counts["fds"] > MAX_FDS:
            unknown("PROC_LIMIT")
        target = base + f"/fd/{descriptor}"
        info_path = base + f"/fdinfo/{descriptor}"
        try:
            info = os.stat(target)
            # A socket, pipe, anonymous descriptor or directory cannot be one
            # of the regular payload files. Do not classify by link-name text.
            if not stat.S_ISREG(info.st_mode):
                counts["non_regular_fds"] += 1
                continue
            counts["regular_fds"] += 1
            identity = (os.major(info.st_dev), os.minor(info.st_dev), info.st_ino)
            if identity not in source_inodes:
                continue
            counts["source_regular_fds"] += 1
            first = parse_fdinfo(read_proc(info_path))
            after = os.stat(target)
            second = parse_fdinfo(read_proc(info_path))
            if first != second or first[2] != info.st_ino or (after.st_dev, after.st_ino) != (info.st_dev, info.st_ino):
                unknown("PROC_SCAN_RACE")
            if (first[0] & os.O_ACCMODE) in (os.O_WRONLY, os.O_RDWR):
                counts["fd_writers"] += 1
                reject("WRITABLE_FD_PRESENT")
        except FileNotFoundError:
            if descriptor_gone(target) or confirm_process_gone(pid, starttime, tid):
                counts["disappeared_fds"] += 1
                continue
            unknown("PROC_SCAN_RACE")
    try:
        mappings = parse_maps(read_proc(base + "/maps"))
    except FileNotFoundError:
        if confirm_process_gone(pid, starttime, tid):
            counts["disappeared_tasks"] += 1
            return
        unknown("PROC_SCAN_RACE")
    counts["mapping_regions"] += len(mappings)
    if counts["mapping_regions"] > MAX_MAP_LINES:
        unknown("PROC_LIMIT")
    for major, minor, inode, permissions in mappings:
        if (major, minor, inode) in source_inodes and permissions[1] == "w" and permissions[3] == "s":
            counts["shared_writable_mappings"] += 1
            reject("WRITABLE_MMAP_PRESENT")
    if confirm_process_gone(pid, starttime, tid):
        counts["disappeared_tasks"] += 1


def scan_writers(source_inodes):
    counts = {key: 0 for key in ("processes", "tasks", "fds", "regular_fds", "non_regular_fds", "source_regular_fds", "fd_writers", "shared_writable_mappings",
                                "mapping_regions", "disappeared_processes", "disappeared_tasks", "disappeared_fds")}
    pids = visible_ids(PROC)
    if not pids or os.getpid() not in pids or len(pids) > MAX_PROCESSES:
        unknown("PROC_VISIBILITY_UNKNOWN")
    for pid in pids:
        try:
            starttime = process_start(pid)
        except FileNotFoundError:
            if path_absent(f"{PROC}/{pid}"):
                counts["disappeared_processes"] += 1
                continue
            unknown("PROC_SCAN_RACE")
        counts["processes"] += 1
        try:
            tids = visible_ids(f"{PROC}/{pid}/task")
        except FileNotFoundError:
            if confirm_process_gone(pid, starttime):
                counts["disappeared_processes"] += 1
                continue
            unknown("PROC_SCAN_RACE")
        if not tids:
            if confirm_process_gone(pid, starttime):
                counts["disappeared_processes"] += 1
                continue
            unknown("PROC_SCAN_RACE")
        for tid in tids:
            counts["tasks"] += 1
            if counts["tasks"] > MAX_TASKS:
                unknown("PROC_LIMIT")
            try:
                task_start = process_start(pid, tid)
            except FileNotFoundError:
                if path_absent(f"{PROC}/{pid}/task/{tid}"):
                    counts["disappeared_tasks"] += 1
                    continue
                unknown("PROC_SCAN_RACE")
            scan_task(pid, tid, task_start, source_inodes, counts)
        if confirm_process_gone(pid, starttime):
            counts["disappeared_processes"] += 1
    return counts


def validate_binding(binding, path):
    # This checksum detects inconsistent data, not a malicious root caller.
    # The parent binds the returned observation to its protected journal and
    # freshly inspected Docker/configuration identity.
    exact_keys(binding, ("schema_version", "path_sha256", "data_root", "source_process", "proc_view", "filesystem", "fingerprint"))
    payload = {key: value for key, value in binding.items() if key != "fingerprint"}
    if type(binding["schema_version"]) is not int or binding["schema_version"] != 1 or binding["fingerprint"] != fingerprint(payload) or binding["path_sha256"] != hashlib.sha256(os.fsencode(path)).hexdigest():
        reject("INVALID_BINDING")
    exact_keys(binding["data_root"], ("dev", "ino", "uid", "gid", "mode"))
    exact_keys(binding["source_process"], ("pid", "starttime"))
    exact_keys(binding["proc_view"], ("mount_id", "pid_namespace"))
    exact_keys(binding["filesystem"], ("type", "device", "mount_id"))
    root = binding["data_root"]
    if not decimal(root["dev"]) or not decimal(root["ino"]) or type(root["uid"]) is not int or root["uid"] != 0 or type(root["gid"]) is not int or root["gid"] < 0 or not isinstance(root["mode"], str) or re.fullmatch(r"[0-7]{4}", root["mode"]) is None:
        reject("INVALID_BINDING")
    if not pid_value(binding["source_process"]["pid"]) or not decimal(binding["source_process"]["starttime"]):
        reject("INVALID_BINDING")
    if not decimal(binding["proc_view"]["pid_namespace"]) or type(binding["proc_view"]["mount_id"]) is not int or type(binding["filesystem"]["mount_id"]) is not int:
        reject("INVALID_BINDING")
    return binding


def observe(request):
    if sys.platform != "linux" or platform.machine().lower() not in ("x86_64", "amd64") or os.geteuid() != 0:
        reject("ROOT_LINUX_AMD64_REQUIRED")
    action = request.get("action") if isinstance(request, dict) else None
    if action == "bind_running":
        exact_keys(request, ("action", "data_dir", "host_pid"))
        if not pid_value(request["host_pid"]):
            reject("INVALID_REQUEST")
    elif action == "stopped_scan":
        exact_keys(request, ("action", "data_dir", "binding"))
    else:
        reject("INVALID_REQUEST")
    path = request["data_dir"]
    mounts = parse_mountinfo(read_proc(PROC + "/self/mountinfo"))
    view = proc_view(mounts)
    root, filesystem = source_root(path, mounts)
    tree_before, inodes = snapshot_tree(path, root)
    if action == "bind_running":
        pid = request["host_pid"]
        first = process_start(pid)
        mapped = os.stat(f"{PROC}/{pid}/root/data")
        second = process_start(pid)
        if first != second:
            unknown("SOURCE_PROCESS_CHANGED")
        if (str(mapped.st_dev), str(mapped.st_ino)) != (root["dev"], root["ino"]) or not stat.S_ISDIR(mapped.st_mode):
            reject("HOST_DATA_BINDING_MISMATCH")
        binding = {"schema_version": 1, "path_sha256": hashlib.sha256(os.fsencode(path)).hexdigest(),
                   "data_root": root, "source_process": {"pid": pid, "starttime": str(first)},
                   "proc_view": view, "filesystem": filesystem}
        binding["fingerprint"] = fingerprint(binding)
        return {"binding": binding, "evidence": {"tree_entries": tree_before["entries"], "tree_metadata_sha256": tree_before["metadata_sha256"]}}
    binding = validate_binding(request["binding"], path)
    if binding["data_root"] != root or binding["filesystem"] != filesystem or binding["proc_view"] != view:
        unknown("SOURCE_BOUNDARY_CHANGED")
    old = binding["source_process"]
    if not confirm_process_gone(old["pid"], int(old["starttime"])):
        reject("SOURCE_GENERATION_ALIVE")
    counts = scan_writers(inodes)
    tree_after, _ = snapshot_tree(path, root)
    if tree_before != tree_after:
        unknown("SOURCE_TREE_CHANGED")
    after_mounts = parse_mountinfo(read_proc(PROC + "/self/mountinfo"))
    after_root, after_filesystem = source_root(path, after_mounts)
    if after_root != root or after_filesystem != filesystem or proc_view(after_mounts) != view:
        unknown("SOURCE_BOUNDARY_CHANGED")
    if not confirm_process_gone(old["pid"], int(old["starttime"])):
        unknown("SOURCE_PROCESS_CHANGED")
    evidence = {**counts, "old_generation_gone": True, "tree_entries": tree_after["entries"],
                "tree_metadata_sha256": tree_after["metadata_sha256"]}
    evidence["fingerprint"] = fingerprint(evidence)
    return {"binding": binding, "evidence": evidence}


def main():
    action = "unknown"
    try:
        raw = sys.stdin.buffer.read(16385)
        if len(raw) > 16384:
            reject("INVALID_REQUEST")
        request = json.loads(raw)
        if isinstance(request, dict) and request.get("action") in ("bind_running", "stopped_scan"):
            action = request["action"]
        result = observe(request)
        reply = {"schema_version": 1, "action": action, "status": "observed", "code": None, **result}
        exit_code = 0
    except GuardError as error:
        reply = {"schema_version": 1, "action": action, "status": error.status, "code": error.code}
        exit_code = 2
    except (PermissionError, FileNotFoundError, OSError, UnicodeError):
        reply = {"schema_version": 1, "action": action, "status": "unknown", "code": "KERNEL_OBSERVATION_UNAVAILABLE"}
        exit_code = 2
    except Exception:
        reply = {"schema_version": 1, "action": action, "status": "unknown", "code": "OBSERVATION_UNCONFIRMED"}
        exit_code = 2
    print(json.dumps(reply, separators=(",", ":")))
    return exit_code


if __name__ == "__main__":
    sys.exit(main())

