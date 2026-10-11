#!/usr/bin/env python3
"""Inspect the batch/Redis startup rule without opening the MyAPI application.

This probe reads only writer metadata from an approved sealed SQLite snapshot.
It neither verifies accounting consistency nor grants permission to start or
migrate the application. Configuration arguments are operator declarations.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from readonly import SnapshotError, SnapshotReader


WRITER_COLUMNS = ("id", "schema_version", "mode", "epoch", "lock_version")


def inspect_configuration(reader: SnapshotReader, batch: str, redis_status: str) -> dict[str, Any]:
    """Evaluate only the known batch startup rule against recorded writer state."""
    reasons = []
    writer = None
    try:
        columns = reader.table_columns("quota_writer_epochs")
        if not set(WRITER_COLUMNS).issubset(columns):
            reasons.append("writer_schema_incomplete")
        else:
            rows = reader.select("quota_writer_epochs", WRITER_COLUMNS)
            if len(rows) != 1:
                reasons.append("writer_singleton_missing_or_ambiguous")
            else:
                row = rows[0]
                exact_integers = all(type(row[key]) is int for key in ("id", "schema_version", "epoch", "lock_version"))
                if (
                    not exact_integers
                    or row["id"] != 1
                    or row["schema_version"] != 1
                    or row["mode"] not in ("legacy", "bridge", "authoritative")
                    or not 0 < row["epoch"] <= 2**63 - 1
                    or not 0 < row["lock_version"] <= 2**63 - 1
                ):
                    reasons.append("writer_state_invalid_or_unsupported")
                else:
                    writer = {
                        "mode": row["mode"],
                        "epoch": str(row["epoch"]),
                        "schema_version": str(row["schema_version"]),
                        "lock_version": str(row["lock_version"]),
                    }
    except SnapshotError:
        reasons.append("writer_schema_unavailable")

    if batch not in ("", "true", "false") or redis_status not in ("absent", "available", "unknown"):
        rule_status = "invalid_configuration"
        reasons.append("configuration_value_invalid")
    elif batch in ("", "false"):
        rule_status = "not_blocked_by_this_rule"
        reasons.append("batch_not_enabled_no_change_recommended")
    elif writer is None:
        rule_status = "unknown"
    elif writer["mode"] == "authoritative":
        rule_status = "not_blocked_by_this_rule"
        reasons.append("authoritative_sql_writer_does_not_require_redis_for_this_rule")
    elif redis_status == "absent":
        rule_status = "blocked"
        reasons.append("legacy_or_bridge_batch_requires_reviewed_redis")
    elif redis_status == "unknown":
        rule_status = "unknown"
        reasons.append("redis_availability_not_known")
    else:
        rule_status = "not_blocked_by_this_rule"
        reasons.append("redis_availability_declared_not_verified")

    return {
        "report_version": 1,
        "scope": "offline_batch_configuration_rule_only",
        "configuration_rule_status": rule_status,
        "writer_status": "recorded_in_snapshot" if writer is not None else "unknown",
        "writer": writer,
        "declared_configuration": {"batch": "unset" if batch == "" else batch, "redis_status": redis_status},
        "configuration_provenance": "operator_declared_not_independently_verified",
        "reasons": reasons,
        "application_start_approved": False,
        "migration_approved": False,
        "ledger_reconciled": False,
        "upgrade_eligible": False,
        "automatic_actions_allowed": False,
        "limitations": [
            "does not inspect balances, pending accounting work, historical cache effects, or effective runtime configuration",
            "does not change batch configuration or create a default writer state",
            "not integrated into main.go; application initialization still precedes the existing startup check",
            "a rule not blocking does not establish that application startup, migration, or deployment is safe",
        ],
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--snapshot", required=True, type=Path)
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--batch", required=True, choices=("true", "false", "unset"))
    parser.add_argument("--redis-status", required=True, choices=("absent", "available", "unknown"))
    args = parser.parse_args(argv)
    with SnapshotReader(args.snapshot, args.manifest) as reader:
        report = inspect_configuration(reader, "" if args.batch == "unset" else args.batch, args.redis_status)
    report["snapshot_verification"] = reader.verification_evidence()
    print(json.dumps(report, indent=2, sort_keys=True))
    return {"not_blocked_by_this_rule": 0, "blocked": 2, "unknown": 3, "invalid_configuration": 2}[report["configuration_rule_status"]]


if __name__ == "__main__":
    raise SystemExit(main())
