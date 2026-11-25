"""Exercise the beta.67 Docker cutover against disposable SQLite databases in Actions."""

import argparse
import contextlib
import hashlib
import json
import os
from pathlib import Path
import secrets
import sqlite3
import subprocess
import tarfile
import time
import zipfile

OLD_ARTIFACT_SHA = "f1cd3b16c03c7c30a15bfe629eadf8a1b927dcba0298e72ae0f4ba34b113527d"
OLD_TAG = "myapi:smoke-full-92e2e34a13d8f11043b1399a32e23ba0ae6bde52"
OLD_CONFIG = "sha256:5077dd01b2489d0c1dd0778e10b3b9bc534a374b4ee4c6f9ed7dcd540d3621d4"
CANDIDATE = "sha256:42cdda2e27efd404e216a39687fb601edb1a09bfc6bbada0defbb67484c8c12a"
OLD_CHARGE = "rehearsal-existing"
LATE_CHARGE = "rehearsal-new-after-online-check"
UNSTARTED = "0001-01-01T00:00:00Z"


class GateFailure(RuntimeError):
    pass


def docker(*args, env=None):
    return subprocess.check_output(
        ["docker", *args], text=True, timeout=150, env=env
    ).strip()


def docker_state(container):
    return json.loads(docker("inspect", "--format", "{{json .State}}", container))


def load_old_image(bundle, root):
    archive_path = root / "beta66-images.tar"
    with bundle.open("rb") as stream:
        assert hashlib.file_digest(stream, "sha256").hexdigest() == OLD_ARTIFACT_SHA
    with zipfile.ZipFile(bundle) as archive:
        assert sorted(archive.namelist()) == ["images.tar", "manifest.json"]
        manifest = json.loads(archive.read("manifest.json"))
        assert manifest["sourceSha"] == OLD_TAG.removeprefix("myapi:smoke-full-")
        assert manifest["edition"] == "full" and manifest["platform"] == "linux/amd64"
        [app] = [image for image in manifest["images"] if image["role"] == "app"]
        assert app["tag"] == OLD_TAG and app["id"] == OLD_CONFIG
        with archive.open("images.tar") as stream, archive_path.open("xb") as output:
            while data := stream.read(1 << 20):
                output.write(data)
    with archive_path.open("rb") as stream:
        assert hashlib.file_digest(stream, "sha256").hexdigest() == manifest["tar"]["sha256"]
    with tarfile.open(archive_path, "r:") as archive:
        with archive.extractfile("index.json") as stream:
            index = json.load(stream)
        [descriptor] = [
            desc for desc in index["manifests"]
            if desc.get("annotations", {}).get("io.containerd.image.name", "")
            .removeprefix("docker.io/library/") == OLD_TAG
        ]
        with archive.extractfile("blobs/sha256/" + descriptor["digest"].removeprefix("sha256:")) as stream:
            raw = stream.read()
        assert descriptor["digest"] == "sha256:" + hashlib.sha256(raw).hexdigest()
        assert json.loads(raw)["config"]["digest"] == OLD_CONFIG
    docker("image", "load", "--platform", "linux/amd64", "--input", str(archive_path))
    assert docker("image", "inspect", "--format", "{{.Id}}", OLD_TAG) == descriptor["digest"]
    return descriptor["digest"]


@contextlib.contextmanager
def container(image, data):
    instance = docker(
        "create", "--pull", "never", "--network", "none", "--platform", "linux/amd64",
        "--user", f"{os.getuid()}:{os.getgid()}", "--env", "SQLITE_PATH=/data/my-api.db",
        "--env", "SESSION_SECRET", "--mount", f"type=bind,src={data},dst=/data", image,
        env={**os.environ, "SESSION_SECRET": secrets.token_hex(32)},
    )
    try:
        assert docker("inspect", "--format", "{{.Image}}", instance) == image
        yield instance
    finally:
        docker("container", "rm", "--force", instance)


def start_and_read_status(instance, version):
    docker("start", instance)
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        result = subprocess.run(
            ["docker", "exec", instance, "wget", "-T", "2", "-qO-", "http://127.0.0.1:3000/api/status"],
            capture_output=True, text=True, timeout=8,
        )
        if result.returncode == 0:
            status = json.loads(result.stdout)
            assert status["success"] is True and status["data"]["version"] == version
            assert status["data"]["product_edition"] == "full"
            return
        if not docker_state(instance)["Running"]:
            break
        time.sleep(1)
    print(docker("logs", "--tail", "35", instance))
    raise AssertionError(f"{version} did not start with the fixture")


def assert_unstarted(instance):
    state = docker_state(instance)
    assert (state["Status"], state["Running"], state["StartedAt"]) == ("created", False, UNSTARTED)


@contextlib.contextmanager
def read_database(path, timeout=10):
    with contextlib.closing(sqlite3.connect(path.as_uri() + "?mode=ro", uri=True, timeout=timeout)) as db:
        db.execute("PRAGMA query_only=ON")
        db.execute("BEGIN")
        yield db


def accounting_snapshot(path, timeout=10):
    try:
        with read_database(path, timeout) as db:
            if db.execute("PRAGMA quick_check").fetchall() != [("ok",)]:
                raise GateFailure("integrity")
            if db.execute("PRAGMA foreign_key_check").fetchone() is not None:
                raise GateFailure("foreign_keys")
            return {
                "logs": db.execute(
                    "SELECT request_id, quota FROM logs WHERE type = 2 ORDER BY id"
                ).fetchall(),
                "totals": db.execute(
                    "SELECT COUNT(*), COALESCE(SUM(quota), 0) FROM logs WHERE type = 2"
                ).fetchone(),
                "user": db.execute(
                    "SELECT quota, used_quota, request_count FROM users WHERE username = 'rehearsal'"
                ).fetchall(),
                "token": db.execute(
                    "SELECT remain_quota, used_quota FROM tokens WHERE name = 'rehearsal'"
                ).fetchall(),
                "channel": db.execute(
                    "SELECT used_quota FROM channels WHERE name = 'rehearsal'"
                ).fetchall(),
            }
    except sqlite3.DatabaseError as error:
        code = getattr(error, "sqlite_errorcode", 0) & 255
        if code in (sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED):
            raise GateFailure("database_locked") from error
        if code in (sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_NOTADB):
            raise GateFailure("integrity") from error
        raise


def seed_account(db_path):
    with contextlib.closing(sqlite3.connect(db_path)) as db, db:
        user = db.execute(
            "INSERT INTO users(username, password, quota, used_quota, request_count) "
            "VALUES ('rehearsal', 'fixture-disabled', 1000, 0, 0)"
        ).lastrowid
        token = db.execute(
            "INSERT INTO tokens(user_id, key, name, remain_quota, used_quota) "
            "VALUES (?, 'fixture-not-a-credential', 'rehearsal', 1000, 0)", (user,)
        ).lastrowid
        channel = db.execute(
            "INSERT INTO channels(key, name, status, used_quota) "
            "VALUES ('fixture-not-a-credential', 'rehearsal', 2, 0)"
        ).lastrowid
        db.execute("CREATE TABLE rehearsal_parent(id INTEGER PRIMARY KEY)")
        db.execute("CREATE TABLE rehearsal_child(parent_id REFERENCES rehearsal_parent(id))")
        db.execute("CREATE TABLE rehearsal_btree(value TEXT)")
        db.execute("INSERT INTO rehearsal_btree VALUES ('fixture')")
    return user, token, channel


def record_charge(db_path, ids, request_id, quota):
    user, token, channel = ids
    with contextlib.closing(sqlite3.connect(db_path)) as db, db:
        db.execute(
            "INSERT INTO logs(user_id, token_id, channel_id, type, quota, request_id, other) "
            "VALUES (?, ?, ?, 2, ?, ?, '{}')", (user, token, channel, quota, request_id),
        )
        db.execute(
            "UPDATE users SET quota = quota - ?, used_quota = used_quota + ?, "
            "request_count = request_count + 1 WHERE id = ?", (quota, quota, user),
        )
        db.execute(
            "UPDATE tokens SET remain_quota = remain_quota - ?, used_quota = used_quota + ? WHERE id = ?",
            (quota, quota, token),
        )
        db.execute("UPDATE channels SET used_quota = used_quota + ? WHERE id = ?", (quota, channel))


def expected_accounting(include_late_charge):
    charges = [(OLD_CHARGE, 17)]
    if include_late_charge:
        charges.append((LATE_CHARGE, 29))
    used = sum(quota for _, quota in charges)
    return {
        "logs": charges,
        "totals": (len(charges), used),
        "user": [(1000 - used, used, len(charges))],
        "token": [(1000 - used, used)],
        "channel": [(used,)],
    }


def create_backup(db_path, target):
    assert not target.exists()
    with contextlib.closing(sqlite3.connect(db_path.as_uri() + "?mode=ro", uri=True)) as source, \
         contextlib.closing(sqlite3.connect(target)) as destination:
        def stop_if_busy(status, _remaining, _total):
            if status in (sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED):
                raise GateFailure("backup_locked")

        source.backup(destination, pages=256, progress=stop_if_busy)


def pass_cutover_gate(db_path, target, expected, timeout=10):
    actual = accounting_snapshot(db_path, timeout)
    if actual != expected:
        raise GateFailure("accounting")
    create_backup(db_path, target)
    if accounting_snapshot(target) != actual:
        raise GateFailure("backup_mismatch")
    return actual


@contextlib.contextmanager
def exclusive_writer(db_path):
    with contextlib.closing(sqlite3.connect(db_path)) as db:
        assert db.execute("PRAGMA locking_mode=EXCLUSIVE").fetchone() == ("exclusive",)
        db.execute("BEGIN EXCLUSIVE")
        db.execute("UPDATE rehearsal_btree SET value = 'holding a write lock'")
        try:
            yield
        finally:
            db.rollback()


def damage_btree(db_path):
    with read_database(db_path) as db:
        page_size = db.execute("PRAGMA page_size").fetchone()[0]
        root_page = db.execute(
            "SELECT rootpage FROM sqlite_schema WHERE name = 'rehearsal_btree'"
        ).fetchone()[0]
    assert root_page > 1
    with db_path.open("r+b") as stream:
        stream.seek((root_page - 1) * page_size)
        assert stream.read(1) == b"\x0d"
        stream.seek(-1, os.SEEK_CUR)
        stream.write(b"\xff")


def violate_foreign_key(db_path):
    with contextlib.closing(sqlite3.connect(db_path)) as db, db:
        db.execute("PRAGMA foreign_keys=OFF")
        db.execute("INSERT INTO rehearsal_child VALUES (999)")


def checked_candidate_start(candidate, db_path, target, expected, timeout=10):
    baseline = pass_cutover_gate(db_path, target, expected, timeout)
    start_and_read_status(candidate, "0.2.0-beta.67")
    return baseline


def reject_cutover(root, snapshot, reason):
    data = root / reason
    data.mkdir()
    live = data / "my-api.db"
    create_backup(snapshot, live)
    target = root / f"{reason}-backup.db"
    with container(CANDIDATE, data) as candidate:
        if reason == "integrity":
            damage_btree(live)
        if reason == "foreign_keys":
            violate_foreign_key(live)
        expected = expected_accounting(reason != "accounting")
        scope = exclusive_writer(live) if reason == "database_locked" else contextlib.nullcontext()
        with scope:
            try:
                checked_candidate_start(candidate, live, target, expected, timeout=1)
            except GateFailure as error:
                assert str(error) == reason, (reason, str(error))
                print(json.dumps({"scenario": reason, "candidate_started": False, "backup_created": False}))
            else:
                raise AssertionError(f"{reason}: gate unexpectedly allowed cutover")
        assert_unstarted(candidate)
        assert not target.exists()


def rehearse(root, old_image):
    data = root / "cutover"
    data.mkdir()
    db_path = data / "my-api.db"
    target = root / "latest-backup.db"
    with container(old_image, data) as old, container(CANDIDATE, data) as candidate:
        start_and_read_status(old, "0.2.0-beta.66")
        ids = seed_account(db_path)
        record_charge(db_path, ids, OLD_CHARGE, 17)
        assert accounting_snapshot(db_path) == expected_accounting(False)
        assert_unstarted(candidate)
        assert docker_state(old)["Running"]
        record_charge(db_path, ids, LATE_CHARGE, 29)
        docker("stop", "--time", "30", old)
        old_state = docker_state(old)
        assert old_state["Status"] == "exited" and old_state["ExitCode"] == 0
        assert_unstarted(candidate)
        baseline = checked_candidate_start(candidate, db_path, target, expected_accounting(True))
        assert not docker_state(old)["Running"]
        assert accounting_snapshot(db_path) == baseline == accounting_snapshot(target)
    for reason in ("database_locked", "integrity", "foreign_keys", "accounting"):
        reject_cutover(root, target, reason)
    return baseline


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--beta66-bundle", type=Path, required=True)
    args = parser.parse_args()
    working = Path(os.environ["RUNNER_TEMP"]).resolve() / "beta67-sqlite-cutover"
    working.mkdir(mode=0o700)
    old_image = load_old_image(args.beta66_bundle, working)
    baseline = rehearse(working, old_image)
    report = {
        "scenario": "beta66-to-beta67",
        "candidate": CANDIDATE,
        "old_image": old_image,
        "online_check_consumption_rows": 1,
        "final_consumption_rows": baseline["totals"][0],
        "final_charge_total": baseline["totals"][1],
        "new_charge_occurrences_in_each_of_baseline_backup_and_active_db": 1,
        "candidate_start_permitted_only_after_database_and_backup_checks": True,
        "rejected_scenarios": ["database_locked", "integrity", "foreign_keys", "accounting"],
    }
    print(json.dumps(report, indent=2))
    with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as output:
        output.write("### beta.67 Full SQLite cutover\n\n```json\n")
        json.dump(report, output, indent=2)
        output.write("\n```\n")


if __name__ == "__main__":
    main()
