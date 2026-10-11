#!/usr/bin/env python3
"""SQLite COPY simulation of verified legacy wallet differences, never live repair.

The CLI only creates a new private copy of a sealed source. There is no CLI to
open an existing writable database. Cache evidence stays unknown; the output
must never be installed over a production database.
"""
from __future__ import annotations

import argparse
from collections import Counter, defaultdict
import json
import os
from pathlib import Path
import shutil
import sqlite3

import audit
from readonly import SnapshotError, SnapshotReader, _sha256_file

SAFE_INTEGER = 2**53 - 1
MAX_INT64 = 2**63 - 1
FACT_COLUMNS = audit.TABLES["account_quota_settlement_facts"][0] + ("lock_version",)
IDENTITY_COLUMNS = tuple(c for c in FACT_COLUMNS if c not in (
    "id", "funding_applied", "token_applied", "state", "lock_version"))


def exact_int(value):
    if isinstance(value, dict):
        value = audit._int_amount({"value": value}, "value")
    if not isinstance(value, int) or isinstance(value, bool):
        raise SnapshotError("simulation requires exact INTEGER storage")
    return value


def historical_target(current, delta, *, used=False, original=None):
    current, delta = exact_int(current), exact_int(delta)
    original = current if original is None else exact_int(original)
    if not -audit.INT32_MAX <= delta <= audit.INT32_MAX:
        raise SnapshotError("simulation delta exceeds the original request limit")
    target = current + delta  # Python integers do not overflow.
    if any(abs(value) > SAFE_INTEGER for value in (original, current, target)):
        raise SnapshotError("historical balance exceeds simulation precision qualification")
    if audit.INT32_MIN <= original <= audit.INT32_MAX and any(not audit.INT32_MIN <= value <= audit.INT32_MAX for value in (current, target)):
        raise SnapshotError("simulation cannot create a new wide balance")
    if used and target < 0:
        raise SnapshotError("simulation used quota cannot become negative")
    return target


def source_plan(reader):
    reader.validate_copy_simulation_schema()
    for table in ("users", "tokens", "account_quota_settlement_facts"):
        ids = [exact_int(row["id"]) for row in reader.select(table, ("id",))]
        if any(value <= 0 for value in ids) or len(ids) != len(set(ids)):
            raise SnapshotError("simulation requires unique positive subject and fact IDs")
    writer = reader.select("quota_writer_epochs", ("id", "schema_version", "mode", "epoch", "lock_version"))
    if len(writer) != 1 or exact_int(writer[0]["id"]) != 1 or exact_int(writer[0]["schema_version"]) != 1 or writer[0]["mode"] != "legacy" or any(exact_int(writer[0][c]) <= 0 for c in ("epoch", "lock_version")):
        raise SnapshotError("simulation requires a verified legacy writer")
    for table in ("account_quota_settlement_intents", "account_quota_refund_facts", "quota_balance_batch_drains"):
        if reader.select(table, ("id",)):
            raise SnapshotError("simulation does not support overlapping intent/refund/generation records")
    for table, columns in (("users", ("quota", "quota_version")), ("tokens", ("remain_quota", "used_quota", "quota_version"))):
        schema = reader.table_columns(table)
        if any("INT" not in schema.get(c, "").upper() for c in columns):
            raise SnapshotError("simulation requires verified SQLite integer-affinity columns")
    facts = reader.select("account_quota_settlement_facts", FACT_COLUMNS, ("actual_quota", "delta"))
    for row in facts:
        for c in ("actual_quota", "delta"):
            row[c] = exact_int(row[c])
    request_counts, event_counts = Counter(r["request_id"] for r in facts), Counter(r["event_key"] for r in facts)
    if any(v != 1 for v in (*request_counts.values(), *event_counts.values())):
        raise SnapshotError("simulation refuses duplicate settlement identities")
    logs = defaultdict(list)
    log_event_counts = Counter()
    for row in reader.settlement_log_evidence():
        logs[row["request_id"]].append(row)
        if isinstance(row["event_key"], str):
            log_event_counts[row["event_key"]] += 1
    journal = reader.select("legacy_usage_reservations", ("request_id", "user_id", "token_id", "subscription_id", "funding_source", "state", "reserved_quota", "token_reserved_quota", "actual_quota"), ("reserved_quota", "token_reserved_quota", "actual_quota"))
    journals = defaultdict(list)
    for row in journal:
        journals[row["request_id"]].append(row)
    selected = []
    for fact in facts:
        if any(type(fact[c]) is not int or fact[c] not in (0, 1) for c in ("apply_token", "funding_applied", "token_applied")):
            raise SnapshotError("simulation requires exact boolean leg markers")
        if fact["kind"] != "legacy_wallet" or fact["delta"] <= 0 or fact["apply_token"] != 1 or fact["state"] not in ("pending", "retryable") or fact["funding_applied"] != 0 or fact["token_applied"] != 0 or fact["terminal_receipt_id"] != 0:
            continue
        if audit._settlement_fingerprint(fact) != fact["request_fingerprint"]:
            raise SnapshotError("simulation settlement fingerprint conflict")
        linked = logs[fact["request_id"]]
        if len(linked) != 1 or log_event_counts[fact["event_key"]] != 1:
            continue
        log = linked[0]
        if any(type(log[c]) is not int for c in ("user_id", "token_id")) or log["quota_type"] != "integer" or log["actual_type"] != "integer" or (log["event_key"], log["user_id"], log["token_id"], log["state"]) != (fact["event_key"], fact["user_id"], fact["token_id"], fact["state"]):
            continue
        try:
            actual, logged = int(log["actual_text"]), int(log["quota_text"])
        except (ValueError, TypeError):
            continue
        if actual != logged or actual != fact["delta"]:
            continue  # This deliberately bounded rehearsal only accepts session preconsume=0.
        associated = journals[fact["request_id"]]
        if len(associated) > 1:
            raise SnapshotError("simulation reservation identity conflict")
        if associated:
            row = associated[0]
            if (row["user_id"], row["token_id"], row["subscription_id"], row["funding_source"], row["state"]) != (fact["user_id"], fact["token_id"], 0, "wallet", "prepared") or exact_int(row["reserved_quota"]) != 0 or exact_int(row["token_reserved_quota"]) != 0 or row["actual_quota"]["storage_class"] != "null":
                continue
        if exact_int(fact["lock_version"]) <= 0:
            raise SnapshotError("simulation requires a positive fact lock version")
        selected.append(fact)
    if not selected:
        raise SnapshotError("no facts meet the bounded simulation evidence policy")
    users = {row["id"]: row for row in reader.select("users", ("id", "quota", "quota_version"), ("quota", "quota_version"))}
    tokens = {row["id"]: row for row in reader.select("tokens", ("id", "user_id", "remain_quota", "used_quota", "quota_version"), ("remain_quota", "used_quota", "quota_version"))}
    # Fix the qualification to the sealed source, never to an intermediate balance.
    # All selected deltas are positive, so endpoint checks also bound each monotone step.
    totals, counts = Counter(), Counter()
    for fact in selected:
        user, token = users.get(fact["user_id"]), tokens.get(fact["token_id"])
        if user is None or token is None or token["user_id"] != fact["user_id"]:
            raise SnapshotError("simulation subject identity conflict")
        fact["source_originals"] = {
            "funding": exact_int(user["quota"]),
            "token_remain": exact_int(token["remain_quota"]),
            "token_used": exact_int(token["used_quota"]),
        }
        historical_target(0, fact["delta"])
        _increment(fact["lock_version"], 2)
        for table, subject in (("users", fact["user_id"]), ("tokens", fact["token_id"])):
            totals[table, subject] += fact["delta"]
            counts[table, subject] += 1
    for (table, subject), delta in totals.items():
        row = (users if table == "users" else tokens)[subject]
        _increment(exact_int(row["quota_version"]), counts[table, subject])
        fields = (("quota", -delta, False),) if table == "users" else (("remain_quota", -delta, False), ("used_quota", delta, True))
        for column, difference, used in fields:
            original = exact_int(row[column])
            historical_target(original, 0, used=used, original=original)
            # Validate the endpoint as a value without applying a per-request limit
            # to the aggregate; every individual delta was checked above.
            historical_target(original + difference, 0, used=used, original=original)
    return {"facts": selected, "source_event_count": len(facts)}


def _changed(cursor):
    if cursor.rowcount != 1:
        raise SnapshotError("simulation compare-and-swap conflict")


def _increment(version, count=1):
    version = exact_int(version)
    if not 0 <= version <= MAX_INT64-count:
        raise SnapshotError("simulation version cannot be incremented safely")
    return version + count


def _apply_leg(connection, expected, leg, failpoint=None):
    connection.execute("BEGIN IMMEDIATE")
    try:
        row = connection.execute("SELECT " + ",".join(FACT_COLUMNS) + " FROM account_quota_settlement_facts WHERE id=?", (expected["id"],)).fetchone()
        if row is None:
            raise SnapshotError("simulation fact disappeared")
        fact = dict(zip(FACT_COLUMNS, row))
        if any(fact[c] != expected[c] for c in IDENTITY_COLUMNS) or audit._settlement_fingerprint(fact) != fact["request_fingerprint"]:
            raise SnapshotError("simulation immutable fact identity changed")
        funding, token = fact["funding_applied"], fact["token_applied"]
        if type(funding) is not int or type(token) is not int or funding not in (0, 1) or token not in (0, 1) or (token and not funding):
            raise SnapshotError("simulation invalid leg state")
        if funding and token:
            if fact["state"] != "applied":
                raise SnapshotError("simulation terminal state conflict")
            connection.commit()
            return False
        if fact["state"] not in ("pending", "retryable"):
            raise SnapshotError("simulation unknown or externally claimed fact")
        if (leg == "funding" and funding) or (leg == "token" and token):
            connection.commit()
            return False
        if leg == "funding":
            subject = connection.execute("SELECT quota,quota_version,typeof(quota),typeof(quota_version) FROM users WHERE id=?", (fact["user_id"],)).fetchone()
            if subject is None or subject[2:] != ("integer", "integer"):
                raise SnapshotError("simulation funding subject lacks exact integer values")
            quota, version = subject[:2]
            target = historical_target(quota, -fact["delta"], original=expected["source_originals"]["funding"])
            _changed(connection.execute("UPDATE users SET quota=?,quota_version=? WHERE id=? AND quota=? AND quota_version=?", (target, _increment(version), fact["user_id"], quota, version)))
        else:
            if not funding:
                raise SnapshotError("simulation token leg precedes funding")
            subject = connection.execute("SELECT remain_quota,used_quota,quota_version,typeof(remain_quota),typeof(used_quota),typeof(quota_version) FROM tokens WHERE id=? AND user_id=?", (fact["token_id"], fact["user_id"])).fetchone()
            if subject is None or subject[3:] != ("integer", "integer", "integer"):
                raise SnapshotError("simulation token subject lacks exact integer values")
            remain, used, version = subject[:3]
            target_remain = historical_target(remain, -fact["delta"], original=expected["source_originals"]["token_remain"])
            target_used = historical_target(used, fact["delta"], used=True, original=expected["source_originals"]["token_used"])
            _changed(connection.execute("UPDATE tokens SET remain_quota=?,used_quota=?,quota_version=? WHERE id=? AND user_id=? AND remain_quota=? AND used_quota=? AND quota_version=?", (target_remain, target_used, _increment(version), fact["token_id"], fact["user_id"], remain, used, version)))
        if failpoint is not None:
            failpoint("before_flag", leg)
        new_lock = _increment(fact["lock_version"])
        if leg == "funding":
            _changed(connection.execute("UPDATE account_quota_settlement_facts SET funding_applied=1,lock_version=? WHERE id=? AND lock_version=? AND funding_applied=0 AND token_applied=0", (new_lock, fact["id"], fact["lock_version"])))
        else:
            _changed(connection.execute("UPDATE account_quota_settlement_facts SET token_applied=1,state='applied',lock_version=? WHERE id=? AND lock_version=? AND funding_applied=1 AND token_applied=0", (new_lock, fact["id"], fact["lock_version"])))
        connection.commit()
    except BaseException:
        connection.rollback()
        raise
    if failpoint is not None:
        failpoint("after_commit", leg)
    return True


def simulate_connection(connection, plan, failpoint=None):
    counts = Counter()
    for fact in plan["facts"]:
        for leg in ("funding", "token"):
            if _apply_leg(connection, fact, leg, failpoint):
                counts[leg] += 1
    return dict(counts)


def _balances(connection):
    values = {
        "users": {row[0]: row[1:] for row in connection.execute("SELECT id,quota,used_quota,quota_version FROM users")},
        "tokens": {row[0]: row[1:] for row in connection.execute("SELECT id,user_id,remain_quota,used_quota,quota_version FROM tokens")},
    }
    for subjects in values.values():
        for subject, columns in subjects.items():
            exact_int(subject)
            for value in columns:
                exact_int(value)
    return values


def verify_conservation(before, after, plan):
    users, tokens, user_count, token_count = Counter(), Counter(), Counter(), Counter()
    for fact in plan["facts"]:
        users[fact["user_id"]] += fact["delta"]
        tokens[fact["token_id"]] += fact["delta"]
        user_count[fact["user_id"]] += 1
        token_count[fact["token_id"]] += 1
    if before.keys() != after.keys() or any(before[k].keys() != after[k].keys() for k in before):
        raise SnapshotError("simulation subject set changed")
    for subject, (quota, used, version) in before["users"].items():
        if after["users"][subject] != (quota-users[subject], used, version+user_count[subject]):
            raise SnapshotError("simulation funding conservation failed")
    for subject, (uid, remain, used, version) in before["tokens"].items():
        if after["tokens"][subject] != (uid, remain-tokens[subject], used+tokens[subject], version+token_count[subject]):
            raise SnapshotError("simulation token conservation failed")


def run_simulation(snapshot, manifest, output_directory):
    output = Path(output_directory)
    if not output.is_absolute() or output.resolve() != output:
        raise SnapshotError("simulation output must be a canonical absolute new directory")
    # Plan/validate first; source trust and exact log evidence are not supplied by a caller.
    with SnapshotReader(snapshot, manifest) as reader:
        plan = source_plan(reader)
        source = reader.manifest.path
        if source.stat().st_mode & 0o222:
            raise SnapshotError("simulation source must be sealed without filesystem write bits")
        if output == source.parent or source.parent in output.parents:
            raise SnapshotError("simulation output cannot be inside the sealed source directory")
        output.mkdir(mode=0o700, exist_ok=False)
        destination = output / "simulation.db"
        with source.open("rb") as src, os.fdopen(os.open(destination, os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600), "wb") as dst:
            shutil.copyfileobj(src, dst, 1024*1024)
        if _sha256_file(destination) != reader.before_sha256:
            raise SnapshotError("simulation copy differs from sealed source")
    # SnapshotReader has now completed its final integrity/identity/hash checks.
    connection = sqlite3.connect(destination, isolation_level=None)
    try:
        connection.execute("PRAGMA foreign_keys=ON")
        before = _balances(connection)
        selected = {r["id"] for r in plan["facts"]}
        fact_query = "SELECT " + ",".join(FACT_COLUMNS) + " FROM account_quota_settlement_facts"
        untouched = {r[0]:r for r in connection.execute(fact_query) if r[0] not in selected}
        first = simulate_connection(connection, plan)
        after = _balances(connection)
        verify_conservation(before, after, plan)
        second = simulate_connection(connection, plan)
        if second or _balances(connection) != after:
            raise SnapshotError("simulation repeated run charged again")
        if untouched != {r[0]:r for r in connection.execute(fact_query) if r[0] not in selected}:
            raise SnapshotError("simulation changed frozen facts")
        if connection.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
            raise SnapshotError("simulation integrity check failed")
    finally:
        connection.close()
    with SnapshotReader(snapshot, manifest) as verification:
        pass
    report = {
        "simulation_only": True, "production_eligible": False, "cache_evidence": "unknown",
        "ledger_reconciled": False, "upgrade_eligible": False, "automatic_actions_allowed": False,
        "source_sha256": verification.before_sha256, "source_unchanged": True,
        "simulation_sha256": _sha256_file(destination),
        "selected_events": len(selected), "frozen_events": plan["source_event_count"]-len(selected),
        "applied_legs_first_run": first, "applied_legs_second_run": second,
        "selected_difference_conservation": True, "whole_historical_conservation": "not_verified",
        "negative_user_quota_after_simulation": sum(row[0] < 0 for row in after["users"].values()),
        "negative_token_remain_after_simulation": sum(row[1] < 0 for row in after["tokens"].values()),
        "frozen_facts_unchanged": True, "integrity_check": "ok",
    }
    with os.fdopen(os.open(output/"simulation-report.json", os.O_WRONLY|os.O_CREAT|os.O_EXCL, 0o600), "w") as handle:
        json.dump(report, handle, indent=2)
    return report


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshot", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--new-private-output-directory", required=True)
    args = parser.parse_args()
    try:
        print(json.dumps(run_simulation(args.snapshot, args.manifest, args.new_private_output_directory)))
    except SnapshotError as error:
        parser.exit(2, "simulation stopped: " + str(error) + "; preserve the private copy; no production action is approved\n")
    except (sqlite3.Error, OSError, ValueError):
        parser.exit(2, "simulation stopped; no production action is approved; preserve the private copy for review\n")
