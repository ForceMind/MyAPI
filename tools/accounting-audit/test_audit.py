import importlib.util
import hashlib
import json
import sqlite3
import stat
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("audit.py")
SPEC = importlib.util.spec_from_file_location("accounting_audit", MODULE_PATH)
audit = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(audit)


def amount(value, storage="integer"):
    return {"text": None if value is None else str(value), "storage_class": "null" if value is None else storage}


class FakeReader:
    def __init__(self, rows=None):
        self.rows = {table: [] for table in audit.TABLES}
        self.rows.update({table: [] for table in audit.SUBJECT_TABLES})
        if rows:
            self.rows.update(rows)
        self.schemas = {
            table: {column: "INTEGER" if column in amounts else "TEXT" for column in columns}
            for table, (columns, amounts) in audit.TABLES.items()
        }
        for table, columns in audit.SUBJECT_TABLES.items():
            self.schemas[table] = {"id": "INTEGER", **{column: "INTEGER" for column in columns}}

    def table_columns(self, table):
        if table not in self.schemas:
            raise LookupError(table)
        return self.schemas[table]

    def select(self, table, columns, amount_columns=()):
        return [{column: row.get(column) for column in columns} for row in self.rows[table]]


def settlement_fact(label, **changes):
    row = {
        "id": 11,
        "event_key": f"event-{label}",
        "request_id": f"request-{label}",
        "kind": "legacy_wallet",
        "user_id": 1,
        "token_id": 2,
        "subscription_id": 0,
        "reserve_receipt_id": 0,
        "writer_epoch": 0,
        "actual_quota": amount(0),
        "delta": amount(5),
        "apply_token": True,
        "funding_applied": True,
        "token_applied": True,
        "terminal_receipt_id": 0,
        "request_fingerprint": f"fingerprint-{label}",
        "state": "applied",
    }
    row.update(changes)
    row["request_fingerprint"] = audit._settlement_fingerprint(row)
    return row


def settlement_intent(fact, **changes):
    row = {key: fact[key] for key in (
        "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id",
        "reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token",
        "request_fingerprint", "state", "terminal_receipt_id",
    )}
    row.update({"id": 7, "fact_id": fact["id"]})
    row.update(changes)
    return row


def legacy_for(fact, reserved=10, actual=15):
    return {
        "request_id": fact["request_id"],
        "user_id": fact["user_id"],
        "token_id": fact["token_id"],
        "subscription_id": fact["subscription_id"],
        "funding_source": "wallet",
        "reserved_quota": amount(reserved),
        "token_reserved_quota": amount(reserved),
        "actual_quota": amount(actual),
        "state": "settled",
        "reason": "",
    }


def request_result(report):
    assert len(report["request_evidence"]) == 1
    return report["request_evidence"][0]


class AccountingAuditTests(unittest.TestCase):
    def test_intent_and_fact_are_one_delta_record(self):
        fact = settlement_fact("dedup")
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_intents": [settlement_intent(fact)],
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [legacy_for(fact)],
        }))
        result = request_result(report)
        self.assertEqual(1, len(result["events"]))
        self.assertEqual(1, result["events"][0]["counted_delta_records"])
        self.assertIn("legacy_recorded_delta_consistent", result["events"][0]["classifications"])
        self.assertNotIn("identity_conflict", result["classifications"])

    def test_legacy_without_journal_keeps_actual_unknown(self):
        result = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [settlement_fact("preupgrade")],
        })))
        classes = result["events"][0]["classifications"]
        self.assertIn("preupgrade_journal_absent", classes)
        self.assertIn("actual_unknown", classes)
        self.assertNotIn("legacy_recorded_delta_consistent", classes)

    def test_null_actual_is_not_zero(self):
        unknown_fact = settlement_fact("null", delta=amount(-5))
        zero_fact = settlement_fact("zero", delta=amount(-5))
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [unknown_fact, zero_fact],
            "legacy_usage_reservations": [legacy_for(unknown_fact, 5, None), legacy_for(zero_fact, 5, 0)],
        }))
        by_hash = {row["request_hash"]: row for row in report["request_evidence"]}
        null_classes = by_hash[audit._anon("request", "request-null")]["events"][0]["classifications"]
        zero_classes = by_hash[audit._anon("request", "request-zero")]["events"][0]["classifications"]
        self.assertIn("actual_unknown", null_classes)
        self.assertIn("legacy_recorded_delta_consistent", zero_classes)
        self.assertNotIn("actual_unknown", zero_classes)

    def test_exact_delta_with_zero_and_nonzero_reservations(self):
        zero = settlement_fact("zero-reserve", delta=amount(5))
        nonzero = settlement_fact("nonzero-reserve", delta=amount(5))
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [zero, nonzero],
            "legacy_usage_reservations": [legacy_for(zero, 0, 5), legacy_for(nonzero, 10, 15)],
        }))
        for result in report["request_evidence"]:
            self.assertIn("legacy_recorded_delta_consistent", result["events"][0]["classifications"])

    def test_each_partial_side_is_reported(self):
        funding_only = settlement_fact("funding-only", token_applied=False, state="claimed")
        token_only = settlement_fact("token-only", funding_applied=False, token_applied=True, state="claimed")
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [funding_only, token_only],
            "legacy_usage_reservations": [legacy_for(funding_only), legacy_for(token_only)],
        }))
        events = {row["request_hash"]: row["events"][0] for row in report["request_evidence"]}
        funding = events[audit._anon("request", "request-funding-only")]
        token = events[audit._anon("request", "request-token-only")]
        self.assertEqual(("applied", "pending"), (funding["funding_classification"], funding["token_classification"]))
        self.assertEqual(("pending", "applied"), (token["funding_classification"], token["token_classification"]))
        self.assertIn("partial_application", funding["classifications"])
        self.assertIn("partial_application", token["classifications"])

    def test_apply_token_false_is_not_partial(self):
        fact = settlement_fact("no-token", apply_token=False, token_applied=True)
        event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [legacy_for(fact)],
        })))["events"][0]
        self.assertEqual("not_applicable", event["token_classification"])
        self.assertIn("applied", event["classifications"])
        self.assertNotIn("partial_application", event["classifications"])

    def test_all_applied_before_state_finalization_is_not_a_new_obligation(self):
        fact = settlement_fact("finalize", state="claimed")
        event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [legacy_for(fact)],
        })))["events"][0]
        self.assertIn("applied_pending_finalization", event["classifications"])
        self.assertEqual(1, event["counted_delta_records"])

    def test_manual_and_unknown_fact_states_fail_closed(self):
        manual = settlement_fact("manual-state", state="manual", funding_applied=False, token_applied=False)
        manual_report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [manual],
            "legacy_usage_reservations": [legacy_for(manual)],
        }))
        manual_event = request_result(manual_report)["events"][0]
        self.assertEqual("manual", manual_event["fact_state"])
        self.assertIn("manual_review_required", manual_event["classifications"])
        self.assertEqual(0, manual_event["counted_delta_records"])
        self.assertEqual("evidence_incomplete", manual_report["audit_status"])

        unknown = settlement_fact("unknown-state", state="invented", funding_applied=False, token_applied=False)
        unknown_event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [unknown],
            "legacy_usage_reservations": [legacy_for(unknown)],
        })))["events"][0]
        self.assertEqual("unrecognized", unknown_event["fact_state"])
        self.assertIn("identity_conflict", unknown_event["classifications"])
        self.assertEqual(0, unknown_event["counted_delta_records"])

    def test_paired_identity_conflict_is_not_two_charges(self):
        fact = settlement_fact("conflict")
        intent = settlement_intent(fact, request_fingerprint="different")
        result = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_intents": [intent],
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [legacy_for(fact)],
        })))
        self.assertIn("identity_conflict", result["events"][0]["classifications"])
        self.assertEqual(0, result["events"][0]["counted_delta_records"])
        self.assertIn("manual_review_required", result["classifications"])

    def test_pending_refund_is_not_subtracted_from_settlement(self):
        fact = settlement_fact("refund")
        refund = {
            "id": 40, "event_key": "refund-event", "request_id": fact["request_id"], "kind": "legacy_wallet",
            "reserve_receipt_id": 0, "audit_key": "", "writer_epoch": 0, "user_id": 1, "token_id": 2,
            "subscription_id": 0, "wallet_quota": amount(5), "subscription_quota": amount(0),
            "token_quota": amount(5), "request_fingerprint": "refund-fp", "state": "pending", "terminal_receipt_id": 0,
        }
        refund["request_fingerprint"] = audit._refund_fingerprint(refund)
        result = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "account_quota_refund_facts": [refund],
            "legacy_usage_reservations": [legacy_for(fact)],
        })))
        self.assertIn("refund_outcome_unknown", result["classifications"])
        self.assertIn("settlement_refund_mutual_exclusion_conflict", result["classifications"])
        self.assertIsNone(result["automatic_action"])
        self.assertNotIn("net_delta", result)

    def test_generation_without_request_identity_is_never_allocated(self):
        generation = {
            "id": 50, "schema_version": 3, "generation_key": "gen", "writer_epoch": 4,
            "payload_fingerprint": "payload", "subject_kind": 1, "subject_id": 10,
            "cache_applied": True, "state": "applied",
        }
        report = audit.audit_snapshot(FakeReader({"quota_balance_batch_drains": [generation]}))
        self.assertEqual([], report["request_evidence"])
        self.assertEqual("not_available", report["batch_generation_summary"][0]["request_allocation"])
        self.assertIn("subject_unknown", report["batch_generation_summary"][0]["classifications"])

    def test_authoritative_settlement_requires_terminal_closure(self):
        fact = settlement_fact(
            "authoritative", kind="authoritative", reserve_receipt_id=100, writer_epoch=8,
            actual_quota=amount(15), delta=amount(0), apply_token=False,
            terminal_receipt_id=101,
        )
        result = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
        })))
        self.assertIn("authoritative_terminal_missing", result["events"][0]["classifications"])
        self.assertNotIn("settled", result["events"][0]["classifications"])

    def test_large_integer_subject_is_exact_and_real_is_inexact(self):
        report = audit.audit_snapshot(FakeReader({
            "users": [{"id": 1, "quota": amount(2**53 + 1), "used_quota": amount("1.5", "real")}],
        }))
        users = next(row for row in report["private_subject_physical_schema"] if row["table"] == "users")
        findings = {row["column"]: row for row in users["amount_anomalies"]}
        self.assertEqual(str(2**53 + 1), findings["quota"]["original_value"])
        self.assertEqual("out_of_int32_range", findings["quota"]["classification"])
        self.assertEqual("1.5", findings["used_quota"]["original_value"])
        self.assertEqual("inexact_storage", findings["used_quota"]["classification"])
        self.assertFalse(report["automatic_actions_allowed"])

    def test_missing_schema_is_not_treated_as_empty(self):
        reader = FakeReader()
        del reader.schemas["account_quota_settlement_facts"]
        report = audit.audit_snapshot(reader)
        self.assertEqual("evidence_incomplete", report["audit_status"])
        self.assertTrue(any(gap["table"] == "account_quota_settlement_facts" for gap in report["schema_gaps"]))

    def test_cli_reads_sealed_sqlite_and_protects_inputs_and_output(self):
        with tempfile.TemporaryDirectory(prefix="accounting-audit-cli-", dir="/tmp") as temporary:
            root = Path(temporary)
            snapshot = root / "sealed.db"
            manifest = root / "manifest.json"
            output = root / "report.json"
            connection = sqlite3.connect(snapshot)
            connection.execute("PRAGMA journal_mode = DELETE")
            amount_columns = {column for _, amounts in audit.TABLES.values() for column in amounts}
            for table, (columns, _) in audit.TABLES.items():
                definitions = []
                for column in columns:
                    declared = "INTEGER" if column in amount_columns or column in {
                        "id", "user_id", "token_id", "subscription_id", "reserve_receipt_id", "writer_epoch",
                        "fact_id", "terminal_receipt_id", "parent_receipt_id", "root_receipt_id", "current_receipt_id",
                        "schema_version", "subject_kind", "subject_id", "generation_id", "apply_token",
                        "funding_applied", "token_applied", "cache_applied", "statistics_applied", "log_projected",
                    } else "TEXT"
                    definitions.append(f'"{column}" {declared}')
                connection.execute(f'CREATE TABLE "{table}" ({", ".join(definitions)})')
            for table, columns in audit.SUBJECT_TABLES.items():
                connection.execute(
                    f'CREATE TABLE "{table}" ("id" INTEGER, '
                    + ", ".join(f'"{column}" INTEGER' for column in columns)
                    + ")"
                )
            fact = settlement_fact("cli")
            legacy = legacy_for(fact)
            fact_columns = audit.TABLES["account_quota_settlement_facts"][0]
            legacy_columns = audit.TABLES["legacy_usage_reservations"][0]
            connection.execute(
                f'INSERT INTO account_quota_settlement_facts ({", ".join(fact_columns)}) VALUES ({", ".join("?" for _ in fact_columns)})',
                [fact[column]["text"] if isinstance(fact[column], dict) else fact[column] for column in fact_columns],
            )
            connection.execute(
                f'INSERT INTO legacy_usage_reservations ({", ".join(legacy_columns)}) VALUES ({", ".join("?" for _ in legacy_columns)})',
                [legacy[column]["text"] if isinstance(legacy[column], dict) else legacy[column] for column in legacy_columns],
            )
            connection.execute("INSERT INTO users (id, quota, used_quota) VALUES (?, ?, ?)", (1, 2**53 + 1, 0))
            connection.commit()
            connection.close()
            before_hash = hashlib.sha256(snapshot.read_bytes()).hexdigest()
            manifest.write_text(json.dumps({
                "snapshot": {
                    "path": str(snapshot.resolve()),
                    "sha256": before_hash,
                    "source": {"description": "synthetic CLI accounting fixture; no production values"},
                }
            }), encoding="utf-8")

            command = [sys.executable, str(MODULE_PATH), "--snapshot", str(snapshot), "--manifest", str(manifest), "--output"]
            completed = subprocess.run([*command, str(output)], text=True, capture_output=True, check=False)
            self.assertEqual(0, completed.returncode, completed.stderr)
            report = json.loads(output.read_text(encoding="utf-8"))
            self.assertEqual(before_hash, report["snapshot_verification"]["before_sha256"])
            self.assertEqual(before_hash, report["snapshot_verification"]["after_sha256"])
            self.assertEqual("synthetic CLI accounting fixture; no production values", report["snapshot_verification"]["source"])
            event = report["request_evidence"][0]["events"][0]
            self.assertEqual("10", event["amount_evidence"]["reservation_recorded"]["value"])
            self.assertEqual("unknown_without_ledger_cache_evidence", event["preconsume_effect"])
            users = next(item for item in report["private_subject_physical_schema"] if item["table"] == "users")
            self.assertEqual(str(2**53 + 1), users["amount_anomalies"][0]["original_value"])
            self.assertEqual(before_hash, hashlib.sha256(snapshot.read_bytes()).hexdigest())
            self.assertEqual(0o600, stat.S_IMODE(output.stat().st_mode))

            for refused in (snapshot, manifest, output):
                failed = subprocess.run([*command, str(refused)], text=True, capture_output=True, check=False)
                self.assertNotEqual(0, failed.returncode)
            self.assertEqual(before_hash, hashlib.sha256(snapshot.read_bytes()).hexdigest())

    def test_cross_request_event_and_fact_id_conflict_is_counted_once_as_unknown(self):
        fact_a = settlement_fact("cross-a")
        intent = settlement_intent(fact_a, fact_id=77)
        fact_b = settlement_fact("cross-b", id=77, event_key=fact_a["event_key"])
        fact_b["request_fingerprint"] = audit._settlement_fingerprint(fact_b)
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_intents": [intent],
            "account_quota_settlement_facts": [fact_b],
        }))
        events = [request["events"][0] for request in report["request_evidence"]]
        self.assertEqual(2, len(events))
        self.assertTrue(all("identity_conflict" in event["classifications"] for event in events))
        self.assertEqual(0, sum(event["counted_delta_records"] for event in events))
        self.assertEqual(1, sum(event["counted_identity_records"] for event in events))

    def test_multiple_settlement_events_for_one_request_are_one_unknown_identity(self):
        first = settlement_fact("multi-event")
        second = settlement_fact("multi-event", id=12, event_key="event-multi-event-second")
        second["request_fingerprint"] = audit._settlement_fingerprint(second)
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [first, second],
            "legacy_usage_reservations": [legacy_for(first)],
        }))
        result = request_result(report)
        self.assertEqual(2, len(result["events"]))
        self.assertTrue(all("identity_conflict" in event["classifications"] for event in result["events"]))
        self.assertEqual(0, sum(event["counted_delta_records"] for event in result["events"]))
        self.assertEqual(1, sum(event["counted_identity_records"] for event in result["events"]))
        self.assertEqual("evidence_incomplete", report["audit_status"])

    def test_authoritative_links_require_subscription_and_billing_identity(self):
        fact = settlement_fact(
            "closed-authoritative", kind="authoritative", reserve_receipt_id=100, writer_epoch=8,
            actual_quota=amount(15), delta=amount(0), apply_token=False, token_applied=True,
            terminal_receipt_id=101,
        )
        reserve = {
            "id": 100, "request_id": fact["request_id"], "writer_epoch": 8, "phase": "reserve",
            "parent_receipt_id": 0, "reserve_fingerprint": "reserve-fp", "request_fingerprint": "reserve-request-fp",
            "billing_source": "wallet", "user_id": 1, "token_id": 2, "subscription_id": 0,
            "requested_quota": amount(10), "applied_quota": amount(10), "event_key": "reserve-event", "audit_key": "",
        }
        terminal = {
            **reserve, "id": 101, "phase": "settle", "parent_receipt_id": 100,
            "requested_quota": amount(15), "applied_quota": amount(15), "event_key": "settle-event",
        }
        head = {
            "request_id": fact["request_id"], "writer_epoch": 8, "root_receipt_id": 100, "current_receipt_id": 100,
            "terminal_receipt_id": 101, "reserve_fingerprint": "reserve-fp", "user_id": 1, "token_id": 2,
            "subscription_id": 0, "billing_source": "wallet", "applied_quota": amount(10),
        }
        obligation = {
            "request_id": fact["request_id"], "reserve_receipt_id": 100, "phase": "settle", "audit_key": "",
            "actual_quota": amount(15), "writer_epoch": 8, "request_fingerprint": "reserve-fp",
            "state": "applied", "terminal_receipt_id": 101,
        }
        receipts = {100: reserve, 101: terminal}
        self.assertEqual(
            "authoritative_full_verification_required",
            audit._authoritative_closure(fact, receipts, {fact["request_id"]: head}, {fact["request_id"]: obligation}),
        )
        terminal["subscription_id"] = 999
        terminal["billing_source"] = "subscription"
        self.assertEqual(
            "identity_conflict",
            audit._authoritative_closure(fact, receipts, {fact["request_id"]: head}, {fact["request_id"]: obligation}),
        )
        terminal["subscription_id"], terminal["billing_source"] = 0, "wallet"
        for target, key, bad_value in (
            (head, "root_receipt_id", 999),
            (obligation, "reserve_receipt_id", 999),
            (obligation, "audit_key", "wrong-audit"),
        ):
            original = target[key]
            target[key] = bad_value
            self.assertEqual(
                "identity_conflict",
                audit._authoritative_closure(fact, receipts, {fact["request_id"]: head}, {fact["request_id"]: obligation}),
            )
            target[key] = original

    def test_legacy_subject_and_source_mismatch_blocks_delta_consistency(self):
        fact = settlement_fact("journal-identity")
        journal = legacy_for(fact)
        journal["funding_source"] = "subscription"
        event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [journal],
        })))["events"][0]
        self.assertIn("identity_conflict", event["classifications"])
        self.assertNotIn("legacy_recorded_delta_consistent", event["classifications"])

    def test_cross_request_refund_event_is_identity_conflict(self):
        def refund(request_id):
            row = {
                "id": 90 if request_id == "refund-a" else 91, "event_key": "shared-refund", "request_id": request_id,
                "kind": "legacy_wallet", "reserve_receipt_id": 0, "audit_key": "", "writer_epoch": 0,
                "user_id": 1, "token_id": 2, "subscription_id": 0, "wallet_quota": amount(5),
                "subscription_quota": amount(0), "token_quota": amount(5), "state": "pending", "terminal_receipt_id": 0,
            }
            row["request_fingerprint"] = audit._refund_fingerprint(row)
            return row

        report = audit.audit_snapshot(FakeReader({"account_quota_refund_facts": [refund("refund-a"), refund("refund-b")]}))
        self.assertEqual(2, len(report["request_evidence"]))
        self.assertTrue(all("identity_conflict" in row["classifications"] for row in report["request_evidence"]))

    def test_invalid_sqlite_boolean_is_unknown(self):
        fact = settlement_fact("bad-bool", funding_applied=2)
        event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [legacy_for(fact)],
        })))["events"][0]
        self.assertIn("evidence_incomplete", event["classifications"])
        self.assertEqual("unknown", event["funding_classification"])

    def test_refund_fingerprint_rejects_untrimmed_or_invalid_subject_identity(self):
        refund = {
            "event_key": "refund-normalized", "kind": "legacy_wallet", "request_id": "refund-request",
            "reserve_receipt_id": 0, "audit_key": "", "writer_epoch": 0, "user_id": 1, "token_id": 2,
            "subscription_id": 0, "wallet_quota": amount(5), "subscription_quota": amount(0), "token_quota": amount(5),
        }
        self.assertIsNotNone(audit._refund_fingerprint(refund))
        for changes in (
            {"event_key": " refund-normalized"}, {"request_id": ""}, {"audit_key": " audit"},
            {"user_id": 0}, {"token_id": 0}, {"subscription_id": -1},
        ):
            candidate = {**refund, **changes}
            self.assertIsNone(audit._refund_fingerprint(candidate))

    def test_legacy_delta_uses_symmetric_max_quota_bound(self):
        valid = settlement_fact("delta-bound-valid", delta=amount(-audit.INT32_MAX))
        valid_event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [valid],
            "legacy_usage_reservations": [legacy_for(valid, audit.INT32_MAX, 0)],
        })))["events"][0]
        self.assertIn("legacy_recorded_delta_consistent", valid_event["classifications"])
        self.assertNotIn("identity_conflict", valid_event["classifications"])

        invalid = settlement_fact("delta-bound-invalid", delta=amount(-(2**31)))
        invalid["request_fingerprint"] = audit._go_json_digest({
            "version": 1, "event_key": invalid["event_key"], "request_id": invalid["request_id"],
            "kind": invalid["kind"], "user_id": invalid["user_id"], "token_id": invalid["token_id"],
            "subscription_id": invalid["subscription_id"], "delta": -(2**31), "apply_token": True,
        })
        invalid_event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [invalid],
            "legacy_usage_reservations": [legacy_for(invalid, audit.INT32_MAX, 0)],
        })))["events"][0]
        self.assertIn("identity_conflict", invalid_event["classifications"])
        self.assertEqual(0, invalid_event["counted_delta_records"])

    def test_invalid_normalized_settlement_identity_cannot_supply_its_own_fingerprint(self):
        invalid = settlement_fact("invalid-subject", user_id=0, token_id=0, apply_token=True)
        invalid["request_fingerprint"] = audit._go_json_digest({
            "version": 1, "event_key": invalid["event_key"], "request_id": invalid["request_id"],
            "kind": invalid["kind"], "user_id": 0, "token_id": 0, "subscription_id": 0,
            "delta": 5, "apply_token": True,
        })
        event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [invalid],
            "legacy_usage_reservations": [legacy_for(invalid)],
        })))["events"][0]
        self.assertIn("identity_conflict", event["classifications"])
        self.assertEqual(0, event["counted_delta_records"])
        for key, value in (
            ("event_key", " event"), ("event_key", ""), ("event_key", "é" * 65),
            ("request_id", " request"), ("request_id", ""), ("request_id", "é" * 33),
        ):
            candidate = settlement_fact(f"invalid-{key}")
            candidate[key] = value
            self.assertIsNone(audit._settlement_fingerprint(candidate))

    def test_duplicate_request_journals_fail_closed(self):
        fact = settlement_fact("duplicate-journal")
        conflicting = legacy_for(fact, 10, 100)
        matching = legacy_for(fact, 10, 15)
        report = audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [conflicting, matching],
        }))
        event = request_result(report)["events"][0]
        self.assertIn("identity_conflict", event["classifications"])
        self.assertNotIn("legacy_recorded_delta_consistent", event["classifications"])
        self.assertEqual(0, event["counted_delta_records"])
        self.assertEqual("evidence_incomplete", report["audit_status"])

    def test_duplicate_receipt_id_marks_related_request_conflict(self):
        fact = settlement_fact("duplicate-receipt")
        receipt = {
            "id": 55, "writer_epoch": 1, "request_id": fact["request_id"], "phase": "reserve",
            "event_key": "reserve-duplicate", "parent_receipt_id": 0, "audit_key": "",
            "reserve_fingerprint": "reserve-fp", "request_fingerprint": "request-fp", "billing_source": "wallet",
            "user_id": 1, "token_id": 2, "subscription_id": 0,
            "requested_quota": amount(10), "applied_quota": amount(10),
        }
        event = request_result(audit.audit_snapshot(FakeReader({
            "account_quota_settlement_facts": [fact],
            "legacy_usage_reservations": [legacy_for(fact)],
            "account_quota_mutation_receipts": [receipt, dict(receipt)],
        })))["events"][0]
        self.assertIn("identity_conflict", event["classifications"])
        self.assertEqual(0, event["counted_delta_records"])
if __name__ == "__main__":
    unittest.main()
