#!/usr/bin/env python3
"""Conservative, offline accounting evidence classifier for sealed SQLite snapshots.

This module deliberately contains no MyAPI imports and never mutates a database.  The
snapshot trust boundary (manifest/hash/integrity checks and read-only transaction) is
implemented by ``readonly.SnapshotReader``.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
from collections import defaultdict
from pathlib import Path
from typing import Any, Iterable, Mapping


INT32_MIN = -(2**31)
INT32_MAX = 2**31 - 1

TABLES: dict[str, tuple[tuple[str, ...], tuple[str, ...]]] = {
    "legacy_usage_reservations": (
        ("request_id", "user_id", "token_id", "subscription_id", "funding_source", "reserved_quota", "token_reserved_quota", "actual_quota", "state", "reason"),
        ("reserved_quota", "token_reserved_quota", "actual_quota"),
    ),
    "account_quota_settlement_intents": (
        ("id", "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id", "reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token", "request_fingerprint", "state", "fact_id", "terminal_receipt_id"),
        ("actual_quota", "delta"),
    ),
    "account_quota_settlement_facts": (
        ("id", "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id", "reserve_receipt_id", "writer_epoch", "actual_quota", "delta", "apply_token", "funding_applied", "token_applied", "terminal_receipt_id", "request_fingerprint", "state"),
        ("actual_quota", "delta"),
    ),
    "account_quota_refund_facts": (
        ("id", "event_key", "request_id", "kind", "reserve_receipt_id", "audit_key", "writer_epoch", "user_id", "token_id", "subscription_id", "wallet_quota", "subscription_quota", "token_quota", "request_fingerprint", "state", "terminal_receipt_id"),
        ("wallet_quota", "subscription_quota", "token_quota"),
    ),
    "account_quota_mutation_receipts": (
        ("id", "writer_epoch", "request_id", "phase", "event_key", "parent_receipt_id", "audit_key", "reserve_fingerprint", "request_fingerprint", "billing_source", "user_id", "token_id", "subscription_id", "requested_quota", "applied_quota"),
        ("requested_quota", "applied_quota"),
    ),
    "account_quota_reservation_heads": (
        ("request_id", "writer_epoch", "root_receipt_id", "current_receipt_id", "terminal_receipt_id", "reserve_fingerprint", "user_id", "token_id", "subscription_id", "billing_source", "applied_quota"),
        ("applied_quota",),
    ),
    "account_quota_terminal_recovery_obligations": (
        ("request_id", "reserve_receipt_id", "phase", "audit_key", "actual_quota", "writer_epoch", "request_fingerprint", "state", "terminal_receipt_id"),
        ("actual_quota",),
    ),
    "usage_review_decisions": (
        ("id", "request_id", "user_id", "token_id", "actual_quota", "evidence_digest", "statistics_applied", "log_projected"),
        ("actual_quota",),
    ),
    "quota_balance_batch_drains": (
        ("id", "schema_version", "generation_key", "writer_epoch", "payload_fingerprint", "subject_kind", "subject_id", "cache_applied", "state"),
        (),
    ),
    "quota_balance_batch_subjects": (
        ("generation_id", "subject_kind", "subject_id"),
        (),
    ),
}

SUBJECT_TABLES: dict[str, tuple[str, ...]] = {
    "users": ("quota", "used_quota"),
    "tokens": ("remain_quota", "used_quota"),
    "user_subscriptions": ("amount_total", "amount_used"),
}


def _anon(kind: str, value: Any) -> str:
    return hashlib.sha256(f"accounting-audit-v1\0{kind}\0{value}".encode()).hexdigest()


def _columns(reader: Any, table: str) -> dict[str, str]:
    value = reader.table_columns(table)
    if isinstance(value, Mapping):
        return {str(k): str(v) for k, v in value.items()}
    return {str(column): "" for column in value}


def _select(reader: Any, table: str, columns: Iterable[str], amount_columns: Iterable[str] = ()) -> list[dict[str, Any]]:
    columns, amount_columns = tuple(columns), tuple(amount_columns)
    try:
        rows = reader.select(table, columns, amount_columns=amount_columns)
    except TypeError:
        rows = reader.select(table, columns, amount_columns)
    return [dict(row) for row in rows]


def _amount(value: Any) -> dict[str, Any]:
    if isinstance(value, Mapping):
        text, storage = value.get("text"), value.get("storage_class")
    elif value is None:
        text, storage = None, "null"
    else:  # Test/readers may expose a native value, but bool is never money.
        text = str(value) if not isinstance(value, bool) else None
        storage = "integer" if isinstance(value, int) and not isinstance(value, bool) else "real" if isinstance(value, float) else "invalid"
    if storage == "null" or text is None:
        return {"value": None, "storage_class": "null", "exact": False, "status": "unknown"}
    text = str(text)
    if storage != "integer":
        return {"value": text, "storage_class": str(storage), "exact": False, "status": "inexact_storage"}
    try:
        parsed = int(text, 10)
    except ValueError:
        return {"value": text, "storage_class": "integer", "exact": False, "status": "invalid_integer"}
    if str(parsed) != text and not (text.startswith("+") and str(parsed) == text[1:]):
        return {"value": text, "storage_class": "integer", "exact": False, "status": "invalid_integer"}
    return {"value": str(parsed), "storage_class": "integer", "exact": True, "status": "exact"}


def _int_amount(row: Mapping[str, Any], column: str) -> int | None:
    amount = _amount(row.get(column))
    return int(amount["value"]) if amount["exact"] else None


def _sqlite_bool(value: Any) -> bool | None:
    if isinstance(value, bool):
        return value
    if isinstance(value, int) and value in (0, 1):
        return bool(value)
    return None


def _identity(row: Mapping[str, Any]) -> tuple[Any, ...]:
    return tuple(row.get(key) for key in (
        "event_key", "request_id", "kind", "user_id", "token_id", "subscription_id",
        "reserve_receipt_id", "writer_epoch", "apply_token", "request_fingerprint",
    )) + tuple(_amount(row.get(key))["value"] for key in ("actual_quota", "delta"))


def _go_json_digest(payload: Mapping[str, Any]) -> str:
    encoded = json.dumps(payload, ensure_ascii=False, separators=(",", ":"))
    encoded = (encoded.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
               .replace("\u2028", "\\u2028").replace("\u2029", "\\u2029"))
    return hashlib.sha256(encoded.encode()).hexdigest()


def _valid_normalized_text(value: Any, maximum_bytes: int, required: bool = True) -> bool:
    if not isinstance(value, str) or value != value.strip() or len(value.encode()) > maximum_bytes:
        return False
    return bool(value) or not required


def _plain_int(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool)


def _settlement_fingerprint(row: Mapping[str, Any]) -> str | None:
    actual, delta = _int_amount(row, "actual_quota"), _int_amount(row, "delta")
    apply_token = _sqlite_bool(row.get("apply_token"))
    if actual is None or delta is None or apply_token is None:
        return None
    if not _valid_normalized_text(row.get("event_key"), 128) or not _valid_normalized_text(row.get("request_id"), 64):
        return None
    if not _plain_int(row.get("user_id")) or row.get("user_id") <= 0:
        return None
    if not _plain_int(row.get("token_id")) or row.get("token_id") < 0:
        return None
    if apply_token and row.get("token_id") <= 0:
        return None
    if not _plain_int(row.get("subscription_id")) or row.get("subscription_id") < 0:
        return None
    if not _plain_int(row.get("reserve_receipt_id")) or not _plain_int(row.get("writer_epoch")):
        return None
    if row.get("kind") == "authoritative":
        if not 0 <= actual <= INT32_MAX or delta != 0 or apply_token or row.get("reserve_receipt_id", 0) <= 0 or row.get("writer_epoch", 0) <= 0:
            return None
        payload = {
            "version": 2, "event_key": row.get("event_key"), "request_id": row.get("request_id"),
            "kind": row.get("kind"), "user_id": row.get("user_id"), "token_id": row.get("token_id"),
            "subscription_id": row.get("subscription_id"), "reserve_receipt_id": row.get("reserve_receipt_id"),
            "writer_epoch": row.get("writer_epoch"), "actual_quota": actual,
        }
    else:
        kind = row.get("kind")
        if kind not in ("legacy_wallet", "legacy_self_use", "legacy_subscription"):
            return None
        if actual != 0 or delta == 0 or not -INT32_MAX <= delta <= INT32_MAX or row.get("reserve_receipt_id") not in (0, None) or row.get("writer_epoch") not in (0, None):
            return None
        if (kind == "legacy_subscription") != (row.get("subscription_id", 0) > 0):
            return None
        payload = {
            "version": 1, "event_key": row.get("event_key"), "request_id": row.get("request_id"),
            "kind": row.get("kind"), "user_id": row.get("user_id"), "token_id": row.get("token_id"),
            "subscription_id": row.get("subscription_id"), "delta": delta, "apply_token": apply_token,
        }
    return _go_json_digest(payload)


def _refund_fingerprint(row: Mapping[str, Any]) -> str | None:
    wallet = _int_amount(row, "wallet_quota")
    subscription = _int_amount(row, "subscription_quota")
    token = _int_amount(row, "token_quota")
    if wallet is None or subscription is None or token is None:
        return None
    if not _valid_normalized_text(row.get("event_key"), 128) or not _valid_normalized_text(row.get("request_id"), 64):
        return None
    if not _valid_normalized_text(row.get("audit_key"), 96, required=False):
        return None
    if not _plain_int(row.get("user_id")) or row.get("user_id") <= 0:
        return None
    if not _plain_int(row.get("token_id")) or row.get("token_id") <= 0:
        return None
    if not _plain_int(row.get("subscription_id")) or row.get("subscription_id") < 0:
        return None
    if not _plain_int(row.get("reserve_receipt_id")) or not _plain_int(row.get("writer_epoch")):
        return None
    if not all(0 <= value <= INT32_MAX for value in (wallet, subscription, token)):
        return None
    kind = row.get("kind")
    if kind == "authoritative":
        valid = row.get("reserve_receipt_id", 0) > 0 and row.get("writer_epoch", 0) > 0 and bool(row.get("audit_key")) and wallet == subscription == token == 0 and row.get("subscription_id", 0) == 0
    elif kind == "legacy_wallet":
        valid = row.get("subscription_id", 0) == 0 and subscription == 0 and (wallet > 0 or token > 0)
    elif kind == "legacy_self_use":
        valid = row.get("subscription_id", 0) == 0 and wallet == subscription == 0 and token > 0
    elif kind == "legacy_subscription":
        valid = row.get("subscription_id", 0) > 0 and wallet == 0 and subscription > 0
    else:
        valid = False
    if not valid:
        return None
    return _go_json_digest({
        "version": 2, "event_key": row.get("event_key"), "kind": row.get("kind"),
        "request_id": row.get("request_id"), "reserve_receipt_id": row.get("reserve_receipt_id"),
        "audit_key": row.get("audit_key"), "writer_epoch": row.get("writer_epoch"),
        "user_id": row.get("user_id"), "token_id": row.get("token_id"),
        "subscription_id": row.get("subscription_id"), "wallet_quota": wallet,
        "subscription_quota": subscription, "token_quota": token,
    })


def _fact_application(fact: Mapping[str, Any]) -> str:
    funding = _sqlite_bool(fact.get("funding_applied"))
    token_required = _sqlite_bool(fact.get("apply_token"))
    token = _sqlite_bool(fact.get("token_applied"))
    state = fact.get("state")
    if state == "manual":
        return "manual_review_required"
    if state not in ("pending", "materialized", "claimed", "retryable", "applied"):
        return "identity_conflict"
    if funding is None or token_required is None or token is None:
        return "evidence_incomplete"
    if state == "applied" and (not funding or not token):
        return "identity_conflict"
    if funding and token:
        return "applied" if state == "applied" else "applied_pending_finalization"
    if not token_required and funding:
        return "identity_conflict"
    if funding or (token_required and token):
        return "partial_application"
    return "pending"


def _authoritative_closure(fact: Mapping[str, Any], receipts: Mapping[Any, Mapping[str, Any]], heads: Mapping[str, Mapping[str, Any]], obligations: Mapping[str, Mapping[str, Any]]) -> str:
    request_id = str(fact.get("request_id", ""))
    reserve = receipts.get(fact.get("reserve_receipt_id"))
    terminal = receipts.get(fact.get("terminal_receipt_id"))
    head, obligation = heads.get(request_id), obligations.get(request_id)
    if not all((reserve, terminal, head, obligation)):
        return "authoritative_terminal_missing"
    same_subject = all(
        item.get("request_id") == request_id
        and item.get("writer_epoch") == fact.get("writer_epoch")
        and item.get("user_id", fact.get("user_id")) == fact.get("user_id")
        and item.get("token_id", fact.get("token_id")) == fact.get("token_id")
        and item.get("subscription_id", fact.get("subscription_id")) == fact.get("subscription_id")
        for item in (reserve, terminal, head)
    )
    head_current = receipts.get(head.get("current_receipt_id"))
    root_receipt = receipts.get(head.get("root_receipt_id"))
    current = head_current
    chain_ids: set[Any] = set()
    chain_cycle = False
    while current:
        if current.get("id") in chain_ids:
            chain_cycle = True
            break
        chain_ids.add(current.get("id"))
        current = receipts.get(current.get("parent_receipt_id"))
    fact_actual = _amount(fact.get("actual_quota"))
    terminal_actual = _amount(terminal.get("requested_quota"))
    terminal_applied = _amount(terminal.get("applied_quota"))
    current_applied = _amount(head_current.get("applied_quota")) if head_current else _amount(None)
    head_applied = _amount(head.get("applied_quota"))
    obligation_actual = _amount(obligation.get("actual_quota"))
    quota_amounts_valid = all(
        value["exact"] and 0 <= int(value["value"]) <= INT32_MAX
        for value in (fact_actual, terminal_actual, terminal_applied, current_applied, head_applied, obligation_actual)
    )
    closed = (
        quota_amounts_valid
        and not chain_cycle
        and reserve.get("phase") in ("reserve", "reserve_adjust")
        and reserve.get("id") in chain_ids
        and root_receipt is not None
        and root_receipt.get("id") in chain_ids
        and root_receipt.get("phase") == "reserve"
        and root_receipt.get("parent_receipt_id") in (0, None)
        and root_receipt.get("request_id") == request_id
        and root_receipt.get("writer_epoch") == fact.get("writer_epoch")
        and root_receipt.get("user_id") == fact.get("user_id")
        and root_receipt.get("token_id") == fact.get("token_id")
        and root_receipt.get("subscription_id") == fact.get("subscription_id")
        and root_receipt.get("billing_source") == head.get("billing_source")
        and root_receipt.get("reserve_fingerprint") == head.get("reserve_fingerprint")
        and terminal.get("phase") == "settle"
        and terminal.get("id") == fact.get("terminal_receipt_id") == head.get("terminal_receipt_id")
        and terminal.get("parent_receipt_id") == head.get("current_receipt_id")
        and terminal_actual["value"] == fact_actual["value"]
        and terminal_applied["value"] == fact_actual["value"]
        and reserve.get("reserve_fingerprint") == head.get("reserve_fingerprint")
        and terminal.get("reserve_fingerprint") == head.get("reserve_fingerprint")
        and reserve.get("billing_source") == head.get("billing_source")
        and terminal.get("billing_source") == head.get("billing_source")
        and head_current is not None
        and head_current.get("request_id") == request_id
        and head_current.get("writer_epoch") == fact.get("writer_epoch")
        and head_current.get("user_id") == fact.get("user_id")
        and head_current.get("token_id") == fact.get("token_id")
        and head_current.get("subscription_id") == fact.get("subscription_id")
        and head_current.get("billing_source") == head.get("billing_source")
        and head_current.get("reserve_fingerprint") == head.get("reserve_fingerprint")
        and current_applied["value"] == head_applied["value"]
    )
    obligation_consistent = (
        obligation.get("state") == "applied"
        and obligation.get("request_id") == request_id
        and obligation.get("phase") == "settle"
        and obligation.get("writer_epoch") == fact.get("writer_epoch")
        and obligation.get("reserve_receipt_id") == head.get("current_receipt_id")
        and obligation.get("terminal_receipt_id") == terminal.get("id")
        and obligation.get("audit_key") == terminal.get("audit_key")
        and obligation.get("request_fingerprint") == head.get("reserve_fingerprint")
        and obligation_actual["value"] == fact_actual["value"]
    )
    return "authoritative_full_verification_required" if same_subject and closed and obligation_consistent else "identity_conflict"


def audit_snapshot(reader: Any) -> dict[str, Any]:
    """Read whitelisted evidence and return a non-actionable audit report."""
    schema_gaps: list[dict[str, Any]] = []
    rows: dict[str, list[dict[str, Any]]] = {}
    schemas: dict[str, dict[str, str]] = {}
    for table, (wanted, amounts) in TABLES.items():
        try:
            present = _columns(reader, table)
        except Exception as exc:  # A missing table is evidence, not an empty ledger.
            schema_gaps.append({"table": table, "missing_table": True, "detail": type(exc).__name__})
            rows[table] = []
            continue
        schemas[table] = present
        missing = [column for column in wanted if column not in present]
        if missing:
            schema_gaps.append({"table": table, "missing_columns": missing})
            rows[table] = []
            continue
        rows[table] = _select(reader, table, wanted, amounts)

    private_subjects: list[dict[str, Any]] = []
    for table, amount_columns in SUBJECT_TABLES.items():
        try:
            present = _columns(reader, table)
        except Exception as exc:
            schema_gaps.append({"table": table, "missing_table": True, "detail": type(exc).__name__})
            continue
        missing = [column for column in ("id", *amount_columns) if column not in present]
        entry: dict[str, Any] = {"table": table, "columns": present, "amount_anomalies": []}
        private_subjects.append(entry)
        if missing:
            schema_gaps.append({"table": table, "missing_columns": missing})
            continue
        for row in _select(reader, table, ("id", *amount_columns), amount_columns):
            subject_hash = _anon(f"{table}:subject", row.get("id"))
            for column in amount_columns:
                amount = _amount(row.get(column))
                outside = amount["exact"] and not INT32_MIN <= int(amount["value"]) <= INT32_MAX
                if not amount["exact"] or outside:
                    entry["amount_anomalies"].append({
                        "subject_hash": subject_hash,
                        "column": column,
                        "original_value": amount["value"],
                        "storage_class": amount["storage_class"],
                        "classification": "out_of_int32_range" if outside else amount["status"],
                    })

    duplicate_request_identity: set[str] = set()
    receipt_groups: dict[Any, list[dict[str, Any]]] = defaultdict(list)
    for row in rows["account_quota_mutation_receipts"]:
        receipt_groups[row.get("id")].append(row)
    for members in receipt_groups.values():
        if len(members) > 1:
            duplicate_request_identity.update(str(row.get("request_id")) for row in members)
    receipts = {receipt_id: members[0] for receipt_id, members in receipt_groups.items()}
    request_identity_maps: dict[str, dict[str, dict[str, Any]]] = {}
    for table in (
        "account_quota_reservation_heads", "account_quota_terminal_recovery_obligations",
        "legacy_usage_reservations", "usage_review_decisions",
    ):
        grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for row in rows[table]:
            grouped[str(row.get("request_id"))].append(row)
        duplicate_request_identity.update(request_id for request_id, members in grouped.items() if len(members) > 1)
        request_identity_maps[table] = {request_id: members[0] for request_id, members in grouped.items()}
    heads = request_identity_maps["account_quota_reservation_heads"]
    obligations = request_identity_maps["account_quota_terminal_recovery_obligations"]
    legacy = request_identity_maps["legacy_usage_reservations"]
    decisions = request_identity_maps["usage_review_decisions"]
    by_request: dict[str, dict[str, list[dict[str, Any]]]] = defaultdict(lambda: defaultdict(list))
    for table in (
        "account_quota_settlement_intents", "account_quota_settlement_facts", "account_quota_refund_facts",
        "account_quota_terminal_recovery_obligations", "usage_review_decisions", "legacy_usage_reservations",
    ):
        for row in rows[table]:
            by_request[str(row.get("request_id"))][table].append(row)

    settlement_event_rows: dict[str, list[tuple[str, dict[str, Any]]]] = defaultdict(list)
    for table in ("account_quota_settlement_intents", "account_quota_settlement_facts"):
        for row in rows[table]:
            settlement_event_rows[str(row.get("event_key"))].append((table, row))
    global_event_conflicts: set[str] = set()
    event_owner: dict[str, str] = {}
    for event_key, members in settlement_event_rows.items():
        requests = {str(row.get("request_id")) for _, row in members}
        event_owner[event_key] = min(requests)
        per_table = [table for table, _ in members]
        if len(requests) != 1 or any(per_table.count(table) > 1 for table in set(per_table)):
            global_event_conflicts.add(event_key)
    settlement_events_by_request: dict[str, set[str]] = defaultdict(set)
    for event_key, members in settlement_event_rows.items():
        for _, row in members:
            settlement_events_by_request[str(row.get("request_id"))].add(event_key)
    for event_keys in settlement_events_by_request.values():
        if len(event_keys) > 1:
            global_event_conflicts.update(event_keys)
    facts_by_id: dict[Any, list[dict[str, Any]]] = defaultdict(list)
    facts_by_terminal: dict[Any, list[dict[str, Any]]] = defaultdict(list)
    for fact in rows["account_quota_settlement_facts"]:
        facts_by_id[fact.get("id")].append(fact)
        if fact.get("terminal_receipt_id") not in (0, None):
            facts_by_terminal[fact.get("terminal_receipt_id")].append(fact)
    for intent in rows["account_quota_settlement_intents"]:
        fact_id = intent.get("fact_id")
        if fact_id not in (0, None):
            matches = facts_by_id.get(fact_id, [])
            if len(matches) != 1 or matches[0].get("event_key") != intent.get("event_key") or matches[0].get("request_id") != intent.get("request_id"):
                global_event_conflicts.add(str(intent.get("event_key")))
                global_event_conflicts.update(str(row.get("event_key")) for row in matches)
    for members in facts_by_terminal.values():
        if len({(row.get("event_key"), row.get("request_id")) for row in members}) > 1:
            global_event_conflicts.update(str(row.get("event_key")) for row in members)

    refund_event_rows: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for refund in rows["account_quota_refund_facts"]:
        refund_event_rows[str(refund.get("event_key"))].append(refund)
    global_refund_conflicts = {
        event_key for event_key, members in refund_event_rows.items()
        if len(members) > 1 or len({str(row.get("request_id")) for row in members}) > 1
    }

    request_evidence: list[dict[str, Any]] = []
    for request_id in sorted(by_request):
        group = by_request[request_id]
        classifications: set[str] = set()
        events: list[dict[str, Any]] = []
        intent_groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
        fact_groups: dict[str, list[dict[str, Any]]] = defaultdict(list)
        for row in group["account_quota_settlement_intents"]:
            intent_groups[str(row.get("event_key"))].append(row)
        for row in group["account_quota_settlement_facts"]:
            fact_groups[str(row.get("event_key"))].append(row)
        for event_key in sorted(set(intent_groups) | set(fact_groups)):
            intent_rows, fact_rows = intent_groups[event_key], fact_groups[event_key]
            intent = intent_rows[0] if intent_rows else None
            fact = fact_rows[0] if fact_rows else None
            event_status: list[str] = []
            duplicate_identity = len(intent_rows) > 1 or len(fact_rows) > 1
            canonical_fingerprints = [_settlement_fingerprint(row) for row in (*intent_rows, *fact_rows)]
            fingerprint_invalid = any(
                fingerprint is None or fingerprint != row.get("request_fingerprint")
                for row, fingerprint in zip((*intent_rows, *fact_rows), canonical_fingerprints)
            )
            paired_identity_conflict = bool(
                intent and fact and (
                    _identity(intent) != _identity(fact)
                    or intent.get("fact_id") not in (0, None, fact.get("id"))
                    or intent.get("terminal_receipt_id") not in (0, None, fact.get("terminal_receipt_id"))
                )
            )
            known_states = {"pending", "materialized", "claimed", "retryable", "applied", "manual"}
            state_invalid = any(row.get("state") not in known_states for row in (*intent_rows, *fact_rows))
            manual_state = any(row.get("state") == "manual" for row in (*intent_rows, *fact_rows))
            if duplicate_identity or paired_identity_conflict or fingerprint_invalid or state_invalid or event_key in global_event_conflicts or request_id in duplicate_request_identity:
                event_status.append("identity_conflict")
            elif manual_state:
                event_status.append("manual_review_required")
            elif fact:
                event_status.append(_fact_application(fact))
                if fact.get("kind") == "authoritative":
                    event_status.append(_authoritative_closure(fact, receipts, heads, obligations))
                    event_status.append("authoritative_full_verification_required")
                else:
                    journal = legacy.get(request_id)
                    if journal is None:
                        event_status.append("preupgrade_journal_absent")
                        event_status.append("actual_unknown")
                    else:
                        expected_source = {
                            "legacy_wallet": "wallet",
                            "legacy_self_use": "self_use",
                            "legacy_subscription": "subscription",
                        }.get(fact.get("kind"))
                        journal_identity_matches = (
                            journal.get("user_id") == fact.get("user_id")
                            and journal.get("token_id") == fact.get("token_id")
                            and journal.get("subscription_id") == fact.get("subscription_id")
                            and journal.get("funding_source") == expected_source
                        )
                        actual = _int_amount(journal, "actual_quota")
                        reserved = _int_amount(journal, "reserved_quota")
                        token_reserved = _int_amount(journal, "token_reserved_quota")
                        delta = _int_amount(fact, "delta")
                        if not journal_identity_matches:
                            event_status.append("identity_conflict")
                        elif reserved is None or token_reserved is None or not 0 <= reserved <= INT32_MAX or not 0 <= token_reserved <= INT32_MAX or actual is not None and not 0 <= actual <= INT32_MAX:
                            event_status.append("identity_conflict")
                        elif journal.get("state") not in ("review_pending", "settled"):
                            event_status.extend(("actual_unknown", "legacy_state_unresolved"))
                        elif actual is None:
                            event_status.append("actual_unknown")
                        elif delta is None:
                            event_status.append("inexact_amount")
                        elif delta == actual - reserved:
                            event_status.append("legacy_recorded_delta_consistent")
                        else:
                            event_status.append("identity_conflict")
            else:
                event_status.append("intent_pending")
            amount_evidence = {
                "delta": _amount(fact.get("delta") if fact else intent.get("delta") if intent else None),
                "fact_actual": _amount(fact.get("actual_quota") if fact else intent.get("actual_quota") if intent else None),
                "reservation_recorded": _amount(legacy.get(request_id, {}).get("reserved_quota")),
                "token_reservation_recorded": _amount(legacy.get(request_id, {}).get("token_reserved_quota")),
                "journal_actual": _amount(legacy.get(request_id, {}).get("actual_quota")),
            }
            if any(value["status"] != "exact" for key, value in amount_evidence.items() if key in ("delta", "fact_actual")):
                event_status.append("evidence_incomplete")
            if "actual_unknown" in event_status or "inexact_amount" in event_status:
                event_status.append("evidence_incomplete")
            if schema_gaps and "settled" in event_status:
                event_status.remove("settled")
                event_status.append("evidence_incomplete")
            classifications.update(event_status)
            funding_marker = _sqlite_bool(fact.get("funding_applied")) if fact else None
            apply_token_marker = _sqlite_bool(fact.get("apply_token")) if fact else None
            token_marker = _sqlite_bool(fact.get("token_applied")) if fact else None
            reservation = amount_evidence["reservation_recorded"]
            events.append({
                "event_hash": _anon("event", event_key),
                "classifications": sorted(set(event_status)),
                "intent_state": (
                    intent.get("state") if intent and intent.get("state") in {"pending", "materialized", "claimed", "retryable", "applied", "manual"}
                    else "unrecognized" if intent else None
                ),
                "fact_state": (
                    fact.get("state") if fact and fact.get("state") in {"pending", "materialized", "claimed", "retryable", "applied", "manual"}
                    else "unrecognized" if fact else None
                ),
                "funding_classification": "applied" if funding_marker is True else "pending" if funding_marker is False else "unknown",
                "token_classification": (
                    "not_applicable" if apply_token_marker is False
                    else "applied" if apply_token_marker is True and token_marker is True
                    else "pending" if apply_token_marker is True and token_marker is False
                    else "unknown"
                ),
                "counted_delta_records": 0 if {"identity_conflict", "manual_review_required"}.intersection(event_status) else 1,
                "counted_identity_records": 1 if (
                    event_owner.get(event_key) == request_id
                    and event_key == min(settlement_events_by_request.get(request_id, {event_key}))
                ) else 0,
                "amount_evidence": amount_evidence,
                "reservation_record_classification": (
                    "recorded_zero" if reservation["exact"] and reservation["value"] == "0"
                    else "recorded_nonzero" if reservation["exact"]
                    else "unknown"
                ),
                "preconsume_effect": "unknown_without_ledger_cache_evidence",
                "application_markers_scope": "database_recorded_markers_only",
            })

        refunds = group["account_quota_refund_facts"]
        if refunds:
            refund_events = [str(row.get("event_key")) for row in refunds]
            if len(refund_events) != len(set(refund_events)) or any(event in global_refund_conflicts for event in refund_events):
                classifications.add("identity_conflict")
            refund_states = {str(row.get("state")) for row in refunds}
            if any(_refund_fingerprint(row) != row.get("request_fingerprint") for row in refunds):
                classifications.add("identity_conflict")
            classifications.add("refund_fact_marked_applied" if refund_states == {"applied"} else "refund_outcome_unknown")
            classifications.add("refund_full_verification_required")
            if fact_groups:
                classifications.add("settlement_refund_mutual_exclusion_conflict")
                classifications.add("manual_review_required")
        if request_id in decisions:
            classifications.add("manual_review_evidence_present")
            subject = heads.get(request_id) or legacy.get(request_id)
            decision = decisions[request_id]
            if subject is None or decision.get("user_id") != subject.get("user_id") or decision.get("token_id") != subject.get("token_id"):
                classifications.add("identity_conflict" if subject is not None else "evidence_incomplete")
        if request_id in duplicate_request_identity:
            classifications.add("identity_conflict")
        if request_id in legacy and not fact_groups and not intent_groups:
            classifications.add("legacy_journal_only")
        if "identity_conflict" in classifications:
            classifications.add("manual_review_required")
        if schema_gaps:
            classifications.discard("settled")
            classifications.add("evidence_incomplete")
        request_evidence.append({
            "request_hash": _anon("request", request_id),
            "classifications": sorted(classifications),
            "events": events,
            "refund_event_hashes": [_anon("event", row.get("event_key")) for row in refunds],
            "automatic_action": None,
        })

    subjects_by_generation: dict[Any, list[dict[str, Any]]] = defaultdict(list)
    for row in rows["quota_balance_batch_subjects"]:
        subjects_by_generation[row.get("generation_id")].append(row)
    generations: list[dict[str, Any]] = []
    for row in rows["quota_balance_batch_drains"]:
        attached = subjects_by_generation.get(row.get("id"), [])
        classifications = ["payload_fingerprint_unverified"]
        cache_applied = _sqlite_bool(row.get("cache_applied"))
        if cache_applied is None:
            classifications.append("evidence_incomplete")
        if not attached:
            classifications.append("subject_unknown")
        elif row.get("subject_kind") and row.get("subject_id") and not any(
            subject.get("subject_kind") == row.get("subject_kind")
            and subject.get("subject_id") == row.get("subject_id")
            for subject in attached
        ):
            classifications.append("identity_conflict")
        if row.get("state") != "applied" or cache_applied is not True:
            classifications.append("generation_unfinalized")
        generations.append({
            "generation_hash": _anon("generation", row.get("generation_key")),
            "payload_fingerprint_hash": _anon("payload_fingerprint", row.get("payload_fingerprint")),
            "writer_epoch": str(row.get("writer_epoch")),
            "schema_version": str(row.get("schema_version")),
            "state": row.get("state"),
            "cache_applied": cache_applied,
            "subject_count": len(attached),
            "request_allocation": "not_available",
            "classifications": classifications,
        })

    request_review = any(any(label in {
        "identity_conflict", "evidence_incomplete", "actual_unknown", "inexact_amount",
        "refund_outcome_unknown", "authoritative_terminal_missing", "authoritative_full_verification_required",
        "refund_full_verification_required", "intent_pending", "partial_application", "pending",
        "applied_pending_finalization", "manual_review_required", "preupgrade_journal_absent",
        "legacy_state_unresolved", "settlement_refund_mutual_exclusion_conflict",
    } for label in request["classifications"]) for request in request_evidence)
    subject_review = any(entry["amount_anomalies"] for entry in private_subjects)
    generation_review = any(generation["classifications"] for generation in generations)
    return {
        "report_version": 1,
        "audit_status": "evidence_incomplete" if schema_gaps or request_review or subject_review or generation_review else "evidence_classified",
        "ledger_reconciled": False,
        "upgrade_eligible": False,
        "automatic_actions_allowed": False,
        "limitations": [
            "no balance conservation conclusion without opening balances and complete pre-consume evidence",
            "legacy reservation values are recorded identity evidence, not proof that a balance debit occurred",
            "application flags are database markers and do not prove whole-account ledger conservation",
            "writer mode, writer epoch configuration, and historical version validity are not fully verified",
            "external cache effects and a complete funding flow are outside this classifier",
            "batch generations are independent evidence and are never allocated to requests",
            "unknown or inexact amounts do not imply zero and do not create a debit",
        ],
        "schema_gaps": schema_gaps,
        "private_subject_physical_schema": private_subjects,
        "request_evidence": request_evidence,
        "batch_generation_summary": generations,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshot", required=True, type=Path)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args(argv)
    output_absolute = args.output.absolute()
    if os.path.lexists(output_absolute):
        parser.error("--output must be a new file; existing paths and symbolic links are refused")
    output_resolved = output_absolute.resolve(strict=False)
    for protected, label in ((args.snapshot, "--snapshot"), (args.manifest, "--manifest")):
        try:
            protected_resolved = protected.resolve(strict=True)
        except OSError as exc:
            parser.error(f"{label} is unavailable: {exc}")
        if output_resolved == protected_resolved:
            parser.error(f"--output must differ from {label}")
    try:
        from readonly import SnapshotReader
    except ImportError as exc:  # pragma: no cover - packaging failure, not accounting logic
        parser.error(f"read-only snapshot support is unavailable: {exc}")
    with SnapshotReader(args.snapshot, args.manifest) as reader:
        report = audit_snapshot(reader)
    report["snapshot_verification"] = reader.verification_evidence()
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    descriptor = os.open(output_absolute, flags, 0o600)
    with os.fdopen(descriptor, "w", encoding="utf-8") as output:
        json.dump(report, output, indent=2, sort_keys=True)
        output.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
