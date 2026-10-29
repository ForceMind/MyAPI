#!/usr/bin/env python3
"""Compare observations, not product acceptance. Only synthetic inputs are used."""
import hashlib
import json
import os
import sys
from pathlib import Path


def main():
    input_file, directory = map(Path, sys.argv[1:3])
    fixtures = json.loads(input_file.read_text())
    digest = hashlib.sha256(input_file.read_bytes()).hexdigest()
    reports = {}
    for engine in ("myapi", "sub2api"):
        for kind in ("request", "usage"):
            report = json.loads((directory / f"{engine}-{kind}.json").read_text())
            assert report["implementation"] == engine
            assert report["input_sha256"] == digest, "implementations did not read identical fixture bytes"
            assert report["source_sha"] == os.environ[f"CACHE_COMPARE_{engine.upper()}_SHA"]
            expected = fixtures["requests" if kind == "request" else "usage"]
            rows = {row["id"]: row for row in report["rows"]}
            assert len(rows) == len(expected) == len(report["rows"])
            assert set(rows) == {row["id"] for row in expected}
            if kind == "request":
                assert all(row["send_count"] == 1 for row in rows.values())
                if engine == "myapi":
                    assert all(row["external_dials"] == 0 for row in rows.values())
                else:
                    assert all(row["default_transport_calls"] == 0 for row in rows.values())
            reports[engine, kind] = rows

    # Go test JSON contains both package and test events. Require each intended
    # top-level test exactly once; an empty/skipped run is never a comparison.
    required = {
        "myapi": {"TestMyAPICacheIdentityComparisonProbe", "TestMyAPIUsageCompatibilityComparisonProbe"},
        "sub2api": {"TestCrossGatewayCacheCompatibilityProbe"},
    }
    for engine, tests in required.items():
        passed = []
        for line in (directory / f"{engine}-tests.jsonl").read_text().splitlines():
            event = json.loads(line)
            assert event.get("Action") not in ("fail", "skip"), "comparison test/package failed or skipped"
            if event.get("Action") == "pass" and event.get("Test") in tests:
                passed.append(event["Test"])
        assert set(passed) == tests and len(passed) == len(tests)

    request_rows = []
    for fixture in fixtures["requests"]:
        case = fixture["id"]
        a, b = reports["myapi", "request"][case], reports["sub2api", "request"][case]
        fields = ("cache_key_state", "cache_key_sha256", "session_state", "session_sha256")
        request_rows.append({"id": case, "myapi": a, "sub2api": b,
                             "different_fields": [key for key in fields if a.get(key) != b.get(key)]})

    properties = {}
    for engine in ("myapi", "sub2api"):
        rows = reports[engine, "request"]
        base, appended = rows["no_identity_first"], rows["no_identity_appended"]
        properties[engine] = {
            "generated_key_present": base["cache_key_state"] == "string",
            "generated_key_stable_across_append": (
                base.get("cache_key_sha256") is not None
                and base.get("cache_key_sha256") == appended.get("cache_key_sha256")),
            "session_present": base["session_state"] == "string",
            "session_stable_across_append": (
                base.get("session_sha256") is not None
                and base.get("session_sha256") == appended.get("session_sha256")),
            "session_isolated_across_api_keys": (
                base.get("session_sha256") is not None
                and rows["other_api_key"].get("session_sha256") is not None
                and base["session_sha256"] != rows["other_api_key"]["session_sha256"]),
            "session_isolated_across_accounts": (
                base.get("session_sha256") is not None
                and rows["other_account"].get("session_sha256") is not None
                and base["session_sha256"] != rows["other_account"]["session_sha256"]),
            "cache_key_isolated_across_api_keys": (
                base.get("cache_key_sha256") is not None
                and rows["other_api_key"].get("cache_key_sha256") is not None
                and base["cache_key_sha256"] != rows["other_api_key"]["cache_key_sha256"]),
            "cache_key_isolated_across_accounts": (
                base.get("cache_key_sha256") is not None
                and rows["other_account"].get("cache_key_sha256") is not None
                and base["cache_key_sha256"] != rows["other_account"]["cache_key_sha256"]),
        }

    usage_rows = []
    for fixture in fixtures["usage"]:
        case = fixture["id"]
        a, b = reports["myapi", "usage"][case], reports["sub2api", "usage"][case]
        def observation(row):
            return {key: row[key] for key in ("client_cache_state", "client_cache_value", "client_input", "client_output") if key in row}
        findings = {}
        for engine, row in (("myapi", a), ("sub2api", b)):
            findings[engine] = []
            if fixture["raw_cached_state"] == "positive" and row.get("client_cache_value") != fixture["raw_cached_value"]:
                findings[engine].append("positive_cache_count_not_preserved")
            if fixture["raw_cached_state"] in ("missing", "null") and row.get("client_cache_state") == "zero":
                findings[engine].append("unknown_cache_serialized_as_zero")
            if fixture["raw_cached_state"] == "zero" and row.get("client_cache_state") == "missing":
                findings[engine].append("explicit_cache_zero_omitted")
            if fixture["raw_cached_state"] == "zero" and row.get("client_cache_value", 0) > 0:
                findings[engine].append("terminal_zero_not_reflected_in_client")
        usage_rows.append({"id": case, "raw_cached_state": fixture["raw_cached_state"],
                           "myapi": a, "sub2api": b, "findings": findings,
                           "outputs_differ": observation(a) != observation(b)})

    result = {"schema_version": 1, "comparison_executed": True, "product_fixed": False,
              "real_provider_calls": False, "browser": "not_run", "input_sha256": digest,
              "source_sha": {engine: os.environ[f"CACHE_COMPARE_{engine.upper()}_SHA"] for engine in required},
              "request_properties": properties, "requests": request_rows, "usage": usage_rows,
              "scope": "request body/session signals use actual forwarding code; usage uses actual client conversion paths; authentication fixtures and transports differ; not a real-cache-hit experiment"}
    (directory / "comparison.json").write_text(json.dumps(result, indent=2) + "\n")
    lines = ["# Cache compatibility comparison", "", "Synthetic observations only; product_fixed=false. No production or real model calls.", "",
             "| Usage case | Raw cache state | MyAPI client cache | sub2api client cache |", "|---|---|---|---|"]
    for row in usage_rows:
        def label(engine):
            observed = row[engine]
            return observed["client_cache_state"] + (":" + str(observed["client_cache_value"]) if "client_cache_value" in observed else "")
        lines.append(f"| {row['id']} | {row['raw_cached_state']} | {label('myapi')} | {label('sub2api')} |")
    lines.extend(["", "## Request identity properties", "", "```json", json.dumps(properties, indent=2), "```", "",
                  "Missing/null values remain distinct in the input and findings. A successful observation run is not acceptance of unknown-to-zero or a product fix.",
                  "No billing, routing, defaults, production data, or provider configurations were changed."])
    (directory / "comparison.md").write_text("\n".join(lines) + "\n")
    print(json.dumps({"comparison_executed": True, "product_fixed": False, "request_cases": len(request_rows),
                      "usage_cases": len(usage_rows), "request_properties": properties,
                      "usage_differences": [row["id"] for row in usage_rows if row["outputs_differ"]],
                      "usage_observations": [{"id": row["id"], "raw_cached_state": row["raw_cached_state"],
                                              "myapi": observation(row["myapi"]), "sub2api": observation(row["sub2api"]),
                                              "findings": row["findings"],
                                              "myapi_sequence": row["myapi"].get("client_usage_sequence", []),
                                              "sub2api_sequence": row["sub2api"].get("client_usage_sequence", [])}
                                             for row in usage_rows]}))


if __name__ == "__main__":
    main()
