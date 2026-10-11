#!/usr/bin/env python3
"""Read exact monetary values from an explicitly sealed SQLite snapshot.

This module deliberately uses only Python's native sqlite3 driver.  It does
not import the application, run migrations, or start background workers.
"""

from __future__ import annotations

import hashlib
import json
import re
import sqlite3
import stat
from contextlib import contextmanager
from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Iterator, Sequence


SHA256_RE = re.compile(r"^[0-9a-f]{64}$")
IDENTIFIER_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
AUDIT_ALLOWED_COLUMNS = {
    "quota_writer_epochs": {"id", "schema_version", "mode", "epoch", "lock_version"},
    "legacy_usage_reservations": {
        "request_id", "user_id", "token_id", "subscription_id", "reserved_quota",
        "token_reserved_quota", "actual_quota", "funding_source", "state", "reason",
    },
    "account_quota_settlement_intents": {
        "id", "event_key", "request_id", "kind", "user_id", "token_id",
        "subscription_id", "reserve_receipt_id", "writer_epoch", "actual_quota",
        "delta", "apply_token", "request_fingerprint", "state", "fact_id",
        "terminal_receipt_id",
    },
    "account_quota_settlement_facts": {
        "id", "event_key", "request_id", "kind", "user_id", "token_id",
        "subscription_id", "reserve_receipt_id", "writer_epoch", "actual_quota",
        "delta", "apply_token", "funding_applied", "token_applied",
        "terminal_receipt_id", "request_fingerprint", "state", "lock_version",
    },
    "account_quota_refund_facts": {
        "id", "event_key", "request_id", "kind", "reserve_receipt_id", "audit_key",
        "writer_epoch", "user_id", "token_id", "subscription_id", "wallet_quota",
        "subscription_quota", "token_quota", "request_fingerprint", "state",
        "terminal_receipt_id",
    },
    "account_quota_mutation_receipts": {
        "id", "writer_epoch", "request_id", "phase", "event_key", "parent_receipt_id",
        "audit_key", "reserve_fingerprint", "request_fingerprint", "billing_source",
        "user_id", "token_id", "subscription_id", "requested_quota", "applied_quota",
    },
    "account_quota_reservation_heads": {
        "request_id", "writer_epoch", "root_receipt_id", "current_receipt_id",
        "terminal_receipt_id", "reserve_fingerprint", "user_id", "token_id",
        "subscription_id", "billing_source", "applied_quota",
    },
    "account_quota_terminal_recovery_obligations": {
        "request_id", "reserve_receipt_id", "phase", "audit_key", "actual_quota",
        "writer_epoch", "request_fingerprint", "state", "terminal_receipt_id",
    },
    "usage_review_decisions": {
        "id", "request_id", "user_id", "token_id", "actual_quota",
        "evidence_digest", "statistics_applied", "log_projected",
    },
    "quota_balance_batch_drains": {
        "id", "schema_version", "generation_key", "writer_epoch",
        "payload_fingerprint", "subject_kind", "subject_id", "cache_applied", "state",
    },
    "quota_balance_batch_subjects": {"generation_id", "subject_kind", "subject_id"},
    "users": {"id", "quota", "used_quota", "quota_version"},
    "tokens": {"id", "user_id", "remain_quota", "used_quota", "quota_version"},
    "user_subscriptions": {"id", "amount_total", "amount_used"},
}
FORBIDDEN_SOURCE_MARKERS = ("://", "api_key=", "key=", "password=", "secret=", "token=")


class SnapshotError(RuntimeError):
    """Base class for snapshot validation and read failures."""


class ManifestError(SnapshotError):
    pass


class SnapshotChangedError(SnapshotError):
    pass


class UnsafeColumnError(SnapshotError):
    pass


class InexactAmountError(SnapshotError):
    pass


@dataclass(frozen=True)
class SnapshotManifest:
    path: Path
    sha256: str
    source_description: str


@dataclass(frozen=True)
class SnapshotSeal:
    sha256: str
    stat_identity: tuple[int, int, int, int]
    directory_entries: tuple[str, ...]


def _quote_identifier(value: str) -> str:
    if not isinstance(value, str) or not IDENTIFIER_RE.fullmatch(value):
        raise SnapshotError("unsafe SQLite identifier")
    return '"' + value.replace('"', '""') + '"'


def _assert_safe_value_column(table: str, value: str) -> None:
    _quote_identifier(table)
    _quote_identifier(value)
    if table not in AUDIT_ALLOWED_COLUMNS or value not in AUDIT_ALLOWED_COLUMNS[table]:
        raise UnsafeColumnError("table or column is outside the accounting audit allowlist")


def _sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def _directory_entries(path: Path) -> tuple[str, ...]:
    return tuple(sorted(item.name for item in path.parent.iterdir()))


def _stat_identity(path: Path) -> tuple[int, int, int, int]:
    value = path.stat()
    return value.st_dev, value.st_ino, value.st_size, value.st_mtime_ns


def _sidecar_paths(path: Path) -> tuple[Path, Path, Path]:
    return (
        Path(str(path) + "-wal"),
        Path(str(path) + "-shm"),
        Path(str(path) + "-journal"),
    )


def load_manifest(manifest_path: Path | str) -> SnapshotManifest:
    manifest_path = Path(manifest_path)
    try:
        raw = json.loads(manifest_path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise ManifestError("could not read snapshot manifest") from error
    if not isinstance(raw, dict):
        raise ManifestError("snapshot manifest must be a JSON object")
    snapshot = raw.get("snapshot")
    if not isinstance(snapshot, dict):
        raise ManifestError("snapshot manifest lacks snapshot object")
    path_value = snapshot.get("path")
    digest = snapshot.get("sha256")
    source = snapshot.get("source")
    if not isinstance(path_value, str) or not Path(path_value).is_absolute():
        raise ManifestError("snapshot path must be explicit and absolute")
    if not isinstance(digest, str) or not SHA256_RE.fullmatch(digest):
        raise ManifestError("snapshot sha256 must be a lowercase full digest")
    if not isinstance(source, dict):
        raise ManifestError("snapshot source description is required")
    description = source.get("description")
    if not isinstance(description, str) or not description.strip():
        raise ManifestError("snapshot source description must be non-empty")
    normalized_description = description.strip()
    if len(normalized_description) > 512 or any(
        marker in normalized_description.lower() for marker in FORBIDDEN_SOURCE_MARKERS
    ):
        raise ManifestError("snapshot source description may not contain credentials or a connection URL")
    path = Path(path_value)
    if path.resolve() != path:
        raise ManifestError("snapshot path must be canonical and contain no symbolic links")
    if path.is_symlink():
        raise ManifestError("snapshot must not be a symbolic link")
    try:
        mode = path.stat().st_mode
    except OSError as error:
        raise ManifestError("snapshot file is unavailable") from error
    if not stat.S_ISREG(mode):
        raise ManifestError("snapshot must be a regular file")
    return SnapshotManifest(
        path=path, sha256=digest, source_description=normalized_description
    )


def _seal_snapshot(manifest: SnapshotManifest) -> SnapshotSeal:
    if any(path.exists() for path in _sidecar_paths(manifest.path)):
        raise SnapshotError("snapshot has SQLite sidecars and is not sealed")
    digest = _sha256_file(manifest.path)
    if digest != manifest.sha256:
        raise SnapshotChangedError("snapshot sha256 differs from manifest")
    with manifest.path.open("rb") as handle:
        header = handle.read(100)
    if len(header) != 100 or header[:16] != b"SQLite format 3\x00":
        raise SnapshotError("snapshot lacks a valid SQLite database header")
    if header[18] != 1 or header[19] != 1:
        raise SnapshotError("snapshot is not sealed in rollback-journal format")
    return SnapshotSeal(
        sha256=digest,
        stat_identity=_stat_identity(manifest.path),
        directory_entries=_directory_entries(manifest.path),
    )


def _readonly_connection(path: Path) -> sqlite3.Connection:
    connection = sqlite3.connect(
        path.resolve().as_uri() + "?mode=ro&immutable=1", uri=True, timeout=30
    )
    connection.execute("PRAGMA query_only = ON")
    if connection.execute("PRAGMA query_only").fetchone() != (1,):
        connection.close()
        raise SnapshotError("SQLite query_only could not be enabled")
    return connection


def _verify_integrity(manifest: SnapshotManifest, seal: SnapshotSeal) -> None:
    _verify_unchanged(manifest, seal)
    connection = _readonly_connection(manifest.path)
    integrity_error = None
    try:
        result = connection.execute("PRAGMA integrity_check").fetchall()
        if result != [("ok",)]:
            integrity_error = SnapshotError("SQLite integrity_check failed")
    except Exception as error:
        integrity_error = error
    finally:
        connection.close()
    _verify_unchanged(manifest, seal)
    if integrity_error is not None:
        raise integrity_error


def _verify_unchanged(manifest: SnapshotManifest, seal: SnapshotSeal) -> str:
    if any(path.exists() for path in _sidecar_paths(manifest.path)):
        raise SnapshotChangedError("SQLite sidecar appeared while reading snapshot")
    if _stat_identity(manifest.path) != seal.stat_identity:
        raise SnapshotChangedError("snapshot file identity changed while reading")
    after_sha256 = _sha256_file(manifest.path)
    if after_sha256 != seal.sha256:
        raise SnapshotChangedError("snapshot content changed while reading")
    if _directory_entries(manifest.path) != seal.directory_entries:
        raise SnapshotChangedError("snapshot directory changed while reading")
    return after_sha256


def _deny_non_read_actions(action, _arg1, _arg2, _database, _trigger):
    allowed = {
        sqlite3.SQLITE_FUNCTION,
        sqlite3.SQLITE_PRAGMA,
        sqlite3.SQLITE_READ,
        sqlite3.SQLITE_SELECT,
        sqlite3.SQLITE_TRANSACTION,
    }
    return sqlite3.SQLITE_OK if action in allowed else sqlite3.SQLITE_DENY


class SnapshotReader:
    """One query-only transaction over a validated immutable snapshot."""

    def __init__(
        self,
        snapshot_or_manifest: Path | str,
        manifest_path: Path | str | None = None,
    ):
        requested_snapshot = None if manifest_path is None else Path(snapshot_or_manifest)
        self.manifest = load_manifest(
            snapshot_or_manifest if manifest_path is None else manifest_path
        )
        if requested_snapshot is not None:
            try:
                normalized_snapshot = requested_snapshot.resolve(strict=True)
            except OSError as error:
                raise ManifestError("requested snapshot is unavailable") from error
            if normalized_snapshot != self.manifest.path:
                raise ManifestError("requested snapshot differs from the manifest path")
        self._seal: SnapshotSeal | None = None
        self._connection: sqlite3.Connection | None = None
        self._after_sha256: str | None = None

    def __enter__(self) -> "SnapshotReader":
        self._seal = _seal_snapshot(self.manifest)
        _verify_integrity(self.manifest, self._seal)
        _verify_unchanged(self.manifest, self._seal)
        try:
            self._connection = _readonly_connection(self.manifest.path)
            self._connection.set_authorizer(_deny_non_read_actions)
            self._connection.execute("BEGIN")
            if not self._connection.in_transaction:
                raise SnapshotError("SQLite read transaction did not start")
            _verify_unchanged(self.manifest, self._seal)
        except Exception:
            if self._connection is not None:
                self._connection.close()
                self._connection = None
            _verify_unchanged(self.manifest, self._seal)
            raise
        return self

    def __exit__(self, exc_type, exc_value, traceback) -> bool:
        verification_error = None
        try:
            if self._connection is not None:
                self._connection.rollback()
                self._connection.close()
        finally:
            self._connection = None
            if self._seal is not None:
                try:
                    self._after_sha256 = _verify_unchanged(self.manifest, self._seal)
                except Exception as error:  # preserve fail-closed exit behavior
                    verification_error = error
        if verification_error is not None:
            raise verification_error
        return False

    @property
    def before_sha256(self) -> str:
        if self._seal is None:
            raise SnapshotError("snapshot reader is not open")
        return self._seal.sha256

    @property
    def after_sha256(self) -> str:
        if self._after_sha256 is None:
            raise SnapshotError("snapshot after-hash is available only after closing")
        return self._after_sha256

    def verification_evidence(self) -> dict[str, str]:
        return {
            "before_sha256": self.before_sha256,
            "after_sha256": self.after_sha256,
            "source": self.manifest.source_description,
        }

    def _require_connection(self) -> sqlite3.Connection:
        if self._connection is None:
            raise SnapshotError("snapshot reader is not open")
        return self._connection

    def table_schema(self, table: str) -> list[dict[str, object]]:
        connection = self._require_connection()
        quoted = _quote_identifier(table)
        rows = connection.execute(f"PRAGMA table_info({quoted})").fetchall()
        if not rows:
            raise SnapshotError("requested SQLite table is absent")
        return [
            {
                "name": row[1],
                "declared_type": row[2],
                "not_null": bool(row[3]),
                "primary_key_position": row[5],
            }
            for row in rows
        ]

    def table_columns(self, table: str) -> dict[str, str]:
        return {
            str(column["name"]): str(column["declared_type"])
            for column in self.table_schema(table)
        }

    def settlement_log_evidence(self) -> list[dict[str, object]]:
        """Project only settlement evidence, never return a whole log payload."""
        required = {"request_id", "user_id", "token_id", "quota", "other", "type"}
        if not required.issubset(self.table_columns("logs")):
            raise SnapshotError("settlement log evidence columns are absent")
        payload = "CASE WHEN json_valid(other) THEN other ELSE '{}' END"
        root = "$.admin_info.billing_settlement."
        sql = f"""SELECT request_id, user_id, token_id,
            CAST(quota AS TEXT), typeof(quota),
            json_extract({payload}, '{root}event_key'),
            CAST(json_extract({payload}, '{root}actual_quota') AS TEXT),
            json_type({payload}, '{root}actual_quota'),
            json_extract({payload}, '{root}state')
            FROM logs WHERE type = 2"""
        columns = ("request_id", "user_id", "token_id", "quota_text", "quota_type",
                   "event_key", "actual_text", "actual_type", "state")
        return [dict(zip(columns, row)) for row in self._require_connection().execute(sql)]

    def validate_copy_simulation_schema(self) -> None:
        """Refuse schema behavior that could hide additional copy mutations."""
        objects = self._require_connection().execute(
            "SELECT name,type FROM sqlite_master WHERE type IN ('table','view','trigger')"
        ).fetchall()
        if any(kind == "trigger" for _, kind in objects):
            raise SnapshotError("copy simulation does not support SQLite triggers")
        tables = {name for name, kind in objects if kind == "table"}
        required = {
            "users", "tokens", "quota_writer_epochs", "logs", "legacy_usage_reservations",
            "account_quota_settlement_facts", "account_quota_settlement_intents",
            "account_quota_refund_facts", "quota_balance_batch_drains",
        }
        if not required.issubset(tables):
            raise SnapshotError("copy simulation requires real accounting tables")
        for table in tables:
            if self._require_connection().execute(
                f"PRAGMA foreign_key_list({_quote_identifier(table)})"
            ).fetchone() is not None:
                raise SnapshotError("copy simulation does not support SQLite foreign keys")

    def select(
        self,
        table: str,
        columns: Sequence[str],
        amount_columns: Sequence[str] = (),
    ) -> list[dict[str, object]]:
        """Select explicit safe columns; exact amounts include text and storage class."""

        if not columns:
            raise SnapshotError("selected columns must be explicit")
        if not set(amount_columns).issubset(columns):
            raise SnapshotError("amount columns must be a subset of selected columns")
        for column in columns:
            _assert_safe_value_column(table, column)
        schema = self.table_columns(table)
        if not set(columns).issubset(schema):
            raise SnapshotError("requested column is absent from SQLite schema")
        amount_set = set(amount_columns)
        selections = []
        for column in columns:
            quoted = _quote_identifier(column)
            if column in amount_set:
                selections.extend((f"CAST({quoted} AS TEXT)", f"typeof({quoted})"))
            else:
                selections.append(quoted)
        sql = "SELECT " + ", ".join(selections) + " FROM " + _quote_identifier(table)
        rows = []
        for raw in self._require_connection().execute(sql):
            output = {}
            offset = 0
            for column in columns:
                if column not in amount_set:
                    output[column] = raw[offset]
                    offset += 1
                    continue
                text_value = raw[offset]
                storage_type = raw[offset + 1]
                offset += 2
                output[column] = {
                    "text": None if storage_type == "null" else text_value,
                    "storage_class": storage_type,
                    "declared_type": schema[column],
                }
            rows.append(output)
        return rows

    def read_exact_amounts(
        self,
        table: str,
        identity_columns: Sequence[str],
        amount_columns: Sequence[str],
    ) -> dict[str, object]:
        """Return exact amounts as decimal strings or JSON null.

        SQLite INTEGER and TEXT storage are accepted. REAL is rejected because
        its original decimal value cannot be recovered exactly.
        """

        if not identity_columns or not amount_columns:
            raise SnapshotError("identity and amount columns must be explicit")
        for column in (*identity_columns, *amount_columns):
            _assert_safe_value_column(table, column)
        schema = self.table_schema(table)
        schema_by_name = {str(item["name"]): item for item in schema}
        requested = set(identity_columns) | set(amount_columns)
        if not requested.issubset(schema_by_name):
            raise SnapshotError("requested column is absent from SQLite schema")

        selected = self.select(
            table, (*identity_columns, *amount_columns), amount_columns
        )
        rows = []
        for raw in sorted(
            selected,
            key=lambda row: tuple(row[column] for column in identity_columns),
        ):
            identities = {
                column: raw[column] for column in identity_columns
            }
            amounts = {}
            for column in amount_columns:
                storage_type = raw[column]["storage_class"]
                text_value = raw[column]["text"]
                if storage_type == "null":
                    decimal_value = None
                elif storage_type not in {"integer", "text"}:
                    raise InexactAmountError(
                        f"{table}.{column} uses inexact or unsupported SQLite storage"
                    )
                else:
                    try:
                        parsed = Decimal(text_value)
                    except (InvalidOperation, TypeError) as error:
                        raise InexactAmountError(
                            f"{table}.{column} is not an exact decimal"
                        ) from error
                    if not parsed.is_finite():
                        raise InexactAmountError(
                            f"{table}.{column} is not a finite decimal"
                        )
                    decimal_value = format(parsed, "f")
                amounts[column] = {
                    "value": decimal_value,
                    "storage_type": storage_type,
                    "declared_type": raw[column]["declared_type"],
                }
            rows.append({"identity": identities, "amounts": amounts})
        return {
            "source": self.manifest.source_description,
            "snapshot_sha256": self.before_sha256,
            "table": table,
            "schema": [schema_by_name[column] for column in (*identity_columns, *amount_columns)],
            "rows": rows,
        }

    def _test_write_is_denied(self) -> None:
        """Exercise query-only/authorizer behavior without exposing write SQL publicly."""

        self._require_connection().execute("CREATE TABLE forbidden_write(id INTEGER)")


@contextmanager
def open_snapshot(manifest_path: Path | str) -> Iterator[SnapshotReader]:
    with SnapshotReader(manifest_path) as reader:
        yield reader
