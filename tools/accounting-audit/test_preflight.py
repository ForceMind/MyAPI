import hashlib
import json
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from preflight import inspect_configuration
from readonly import SnapshotReader


class ConfigurationProbeTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="accounting-preflight-")
        self.root = Path(self.temporary.name)
        self.database = self.root / "sealed.db"
        self.manifest = self.root / "manifest.json"

    def tearDown(self):
        self.temporary.cleanup()

    def seal(self, rows, columns="id, schema_version, mode, epoch, lock_version"):
        if self.database.exists():
            self.database.unlink()
        with sqlite3.connect(self.database) as connection:
            connection.execute("CREATE TABLE unrelated(id INTEGER)")
            if columns is not None:
                connection.execute(f"CREATE TABLE quota_writer_epochs({columns})")
                if rows:
                    placeholders = ",".join("?" for _ in rows[0])
                    connection.executemany(f"INSERT INTO quota_writer_epochs VALUES ({placeholders})", rows)
        self.digest = hashlib.sha256(self.database.read_bytes()).hexdigest()
        self.manifest.write_text(json.dumps({"snapshot": {"path": str(self.database), "sha256": self.digest,
            "source": {"description": "synthetic startup configuration fixture"}}}), encoding="utf-8")

    def inspect(self, batch="true", redis_status="absent"):
        before_entries = set(self.root.iterdir())
        with SnapshotReader(self.database, self.manifest) as reader:
            report = inspect_configuration(reader, batch, redis_status)
        self.assertEqual(self.digest, reader.after_sha256)
        self.assertEqual(before_entries, set(self.root.iterdir()))
        for flag in ("application_start_approved", "migration_approved", "ledger_reconciled", "upgrade_eligible", "automatic_actions_allowed"):
            self.assertIs(report[flag], False)
        return report

    def test_legacy_and_bridge_without_redis_are_blocked(self):
        for mode in ("legacy", "bridge"):
            with self.subTest(mode=mode):
                self.seal([(1, 1, mode, 1, 1)])
                self.assertEqual("blocked", self.inspect()["configuration_rule_status"])

    def test_authoritative_without_redis_is_not_misblocked(self):
        self.seal([(1, 1, "authoritative", 9007199254740993, 1)])
        report = self.inspect()
        self.assertEqual("not_blocked_by_this_rule", report["configuration_rule_status"])
        self.assertEqual("9007199254740993", report["writer"]["epoch"])

    def test_declared_redis_is_not_verified_or_permission_to_start(self):
        self.seal([(1, 1, "legacy", 1, 1)])
        report = self.inspect(redis_status="available")
        self.assertEqual("not_blocked_by_this_rule", report["configuration_rule_status"])
        self.assertEqual("operator_declared_not_independently_verified", report["configuration_provenance"])
        self.assertEqual("unknown", self.inspect(redis_status="unknown")["configuration_rule_status"])

    def test_missing_schema_is_unknown_and_does_not_create_writer(self):
        for columns in (None, "id INTEGER"):
            with self.subTest(columns=columns):
                self.seal([], columns)
                self.assertEqual("unknown", self.inspect()["configuration_rule_status"])

    def test_ambiguous_or_invalid_writer_is_unknown(self):
        cases = [[], [(1, 1, "legacy", 1, 1), (1, 1, "authoritative", 2, 1)],
            [(2, 1, "legacy", 1, 1)], [(1, 2, "legacy", 1, 1)], [(1, 1, "other", 1, 1)],
            [(1, 1, "legacy", 0, 1)], [(1, 1, "legacy", 1, 0)], [(1, 1, "legacy", 1.5, 1)],
            [(1, 1, "legacy", "1", 1)], [(1, 1, "legacy", None, 1)]]
        for rows in cases:
            with self.subTest(rows=rows):
                self.seal(rows)
                self.assertEqual("unknown", self.inspect()["configuration_rule_status"])

    def test_unset_or_false_is_not_a_recommendation_to_disable_batch(self):
        self.seal([], None)
        for batch in ("", "false"):
            with self.subTest(batch=batch):
                report = self.inspect(batch=batch)
                self.assertEqual("not_blocked_by_this_rule", report["configuration_rule_status"])
                self.assertEqual("unknown", report["writer_status"])
                self.assertIn("batch_not_enabled_no_change_recommended", report["reasons"])

    def test_invalid_configuration_is_rejected(self):
        self.seal([(1, 1, "legacy", 1, 1)])
        for batch, redis_status in (("TRUE", "absent"), ("true ", "absent"), ("true", "maybe")):
            with self.subTest(batch=batch, redis_status=redis_status):
                self.assertEqual("invalid_configuration", self.inspect(batch, redis_status)["configuration_rule_status"])

    def test_cli_outputs_matching_snapshot_evidence_and_rule_exit_status(self):
        self.seal([(1, 1, "legacy", 1, 1)])
        result = subprocess.run([sys.executable, str(Path(__file__).with_name("preflight.py")),
            "--snapshot", str(self.database), "--manifest", str(self.manifest),
            "--batch", "true", "--redis-status", "absent"], capture_output=True, text=True)
        self.assertEqual(2, result.returncode, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual("blocked", report["configuration_rule_status"])
        self.assertEqual(self.digest, report["snapshot_verification"]["before_sha256"])
        self.assertEqual(self.digest, report["snapshot_verification"]["after_sha256"])
        self.assertEqual(self.digest, hashlib.sha256(self.database.read_bytes()).hexdigest())
        self.assertFalse(report["application_start_approved"])


if __name__ == "__main__":
    unittest.main()
