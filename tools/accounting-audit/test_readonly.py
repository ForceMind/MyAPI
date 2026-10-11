#!/usr/bin/env python3

import hashlib
import json
import sqlite3
import tempfile
import unittest
from pathlib import Path

from readonly import (
    InexactAmountError,
    SnapshotChangedError,
    SnapshotError,
    SnapshotReader,
    UnsafeColumnError,
    open_snapshot,
)


def sha256_file(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


class SnapshotReaderTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="accounting-reader-")
        self.root = Path(self.temporary.name)
        self.database = self.root / "sealed.db"
        connection = sqlite3.connect(self.database)
        connection.execute(
            "CREATE TABLE account_quota_settlement_facts ("
            "id INTEGER PRIMARY KEY, event_key TEXT, apply_token INTEGER, "
            "actual_quota NUMERIC, delta NUMERIC, "
            "password TEXT)"
        )
        connection.execute(
            "CREATE TABLE legacy_usage_reservations ("
            "request_id TEXT PRIMARY KEY, funding_source TEXT, password TEXT)"
        )
        connection.executemany(
            "INSERT INTO account_quota_settlement_facts"
            "(id, actual_quota, delta, password) VALUES (?, ?, ?, ?)",
            [
                (1, 9007199254740993, None, "never-read"),
                (2, 0, 0, "never-read"),
            ],
        )
        connection.commit()
        connection.execute(
            "INSERT INTO legacy_usage_reservations"
            "(request_id, funding_source, password) VALUES (?, ?, ?)",
            ("synthetic-request", "wallet", "never-read"),
        )
        connection.commit()
        connection.close()
        self.manifest = self.root / "manifest.json"
        self.write_manifest(sha256_file(self.database))

    def tearDown(self):
        self.temporary.cleanup()

    def write_manifest(self, digest: str):
        self.manifest.write_text(
            json.dumps(
                {
                    "snapshot": {
                        "path": str(self.database),
                        "sha256": digest,
                        "source": {
                            "description": "synthetic accounting fixture; no production values"
                        },
                    }
                }
            ),
            encoding="utf-8",
        )

    def test_exact_large_integer_null_zero_schema_and_no_side_effect(self):
        before_hash = sha256_file(self.database)
        before_entries = sorted(item.name for item in self.root.iterdir())
        with open_snapshot(self.manifest) as reader:
            result = reader.read_exact_amounts(
                "account_quota_settlement_facts", ("id",), ("actual_quota", "delta")
            )
            self.assertEqual(before_hash, reader.before_sha256)
            self.assertEqual(
                "NUMERIC",
                reader.table_columns("account_quota_settlement_facts")["actual_quota"],
            )
            compatible_rows = reader.select(
                "account_quota_settlement_facts",
                ("id", "event_key", "apply_token", "actual_quota", "delta"),
                ("actual_quota", "delta"),
            )
            self.assertEqual("9007199254740993", compatible_rows[0]["actual_quota"]["text"])
            self.assertEqual("integer", compatible_rows[0]["actual_quota"]["storage_class"])
            with self.assertRaises(SnapshotError):
                _ = reader.after_sha256
        self.assertEqual(before_hash, reader.after_sha256)
        self.assertEqual(
            {"before_sha256", "after_sha256", "source"},
            set(reader.verification_evidence()),
        )
        self.assertEqual("9007199254740993", result["rows"][0]["amounts"]["actual_quota"]["value"])
        self.assertIsNone(result["rows"][0]["amounts"]["delta"]["value"])
        self.assertEqual("0", result["rows"][1]["amounts"]["actual_quota"]["value"])
        self.assertEqual("0", result["rows"][1]["amounts"]["delta"]["value"])
        self.assertEqual("integer", result["rows"][0]["amounts"]["actual_quota"]["storage_type"])
        self.assertEqual("NUMERIC", result["rows"][0]["amounts"]["actual_quota"]["declared_type"])
        self.assertEqual(before_hash, sha256_file(self.database))
        self.assertEqual(before_entries, sorted(item.name for item in self.root.iterdir()))
        self.assertFalse(Path(str(self.database) + "-wal").exists())
        self.assertFalse(Path(str(self.database) + "-shm").exists())

    def test_real_storage_is_rejected(self):
        connection = sqlite3.connect(self.database)
        connection.execute(
            "UPDATE account_quota_settlement_facts SET actual_quota = ? WHERE id = 1",
            (0.5,),
        )
        connection.commit()
        connection.close()
        self.write_manifest(sha256_file(self.database))
        with open_snapshot(self.manifest) as reader:
            selected = reader.select(
                "account_quota_settlement_facts",
                ("id", "actual_quota"),
                ("actual_quota",),
            )
            self.assertEqual("real", selected[0]["actual_quota"]["storage_class"])
            self.assertEqual("0.5", selected[0]["actual_quota"]["text"])
            with self.assertRaises(InexactAmountError):
                reader.read_exact_amounts(
                    "account_quota_settlement_facts", ("id",), ("actual_quota",)
                )

    def test_bad_hash_is_rejected(self):
        self.write_manifest("0" * 64)
        with self.assertRaises(SnapshotChangedError):
            with open_snapshot(self.manifest):
                pass

    def test_write_is_denied(self):
        before = sha256_file(self.database)
        with open_snapshot(self.manifest) as reader:
            with self.assertRaises(sqlite3.DatabaseError):
                reader._test_write_is_denied()
        self.assertEqual(before, sha256_file(self.database))

    def test_wal_or_shm_sidecar_is_rejected(self):
        for suffix in ("-wal", "-shm", "-journal"):
            with self.subTest(suffix=suffix):
                sidecar = Path(str(self.database) + suffix)
                sidecar.write_bytes(b"not sealed")
                try:
                    with self.assertRaises(SnapshotError):
                        with open_snapshot(self.manifest):
                            pass
                finally:
                    sidecar.unlink()

    def test_wal_header_without_sidecar_is_rejected_without_creating_files(self):
        contents = bytearray(self.database.read_bytes())
        contents[18] = 2
        contents[19] = 2
        self.database.write_bytes(contents)
        self.write_manifest(sha256_file(self.database))
        before_entries = sorted(item.name for item in self.root.iterdir())
        with self.assertRaises(SnapshotError):
            with open_snapshot(self.manifest):
                pass
        self.assertEqual(before_entries, sorted(item.name for item in self.root.iterdir()))
        self.assertFalse(Path(str(self.database) + "-wal").exists())
        self.assertFalse(Path(str(self.database) + "-shm").exists())

    def test_source_change_during_transaction_is_rejected(self):
        with self.assertRaises(SnapshotChangedError):
            with open_snapshot(self.manifest) as reader:
                reader.read_exact_amounts(
                    "account_quota_settlement_facts", ("id",), ("actual_quota",)
                )
                with self.database.open("ab") as handle:
                    handle.write(b"changed")

    def test_secret_column_is_rejected_without_reading_it(self):
        with open_snapshot(self.manifest) as reader:
            rows = reader.select(
                "legacy_usage_reservations", ("request_id", "funding_source")
            )
            self.assertEqual("wallet", rows[0]["funding_source"])
            with self.assertRaises(UnsafeColumnError):
                reader.read_exact_amounts(
                    "account_quota_settlement_facts", ("id",), ("password",)
                )

    def test_manifest_requires_safe_source_description(self):
        raw = json.loads(self.manifest.read_text(encoding="utf-8"))
        raw["snapshot"]["source"]["description"] = "mysql://user:key@example.invalid/db"
        self.manifest.write_text(json.dumps(raw), encoding="utf-8")
        with self.assertRaises(SnapshotError):
            with open_snapshot(self.manifest):
                pass

    def test_explicit_snapshot_must_match_manifest(self):
        different = self.root / "different.db"
        different.write_bytes(self.database.read_bytes())
        with self.assertRaises(SnapshotError):
            with SnapshotReader(different, self.manifest):
                pass


if __name__ == "__main__":
    unittest.main()
