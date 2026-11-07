"""Evidence-boundary tests; executed only in GitHub Actions for this task."""
import copy
import json
import tempfile
import unittest
from pathlib import Path

from r4_probe_report import classify

ROOT = "TestR4QuotaAmountPreflightProbe"
PACKAGE = "github.com/ForceMind/MyAPI/model"
MESSAGE = "legal token debit must survive startup amount validation"


def probe_events(reproduced=("sqlite", "mysql", "postgres")):
    events = []
    versions = {"sqlite": "3.46.0", "mysql": "5.7.44", "postgres": "PostgreSQL 9.6.24"}
    cases = ("legal_token_debit", "legal_token_credit", "ordinary_reserve_settle", "invalid_negative_charge")
    for database, version in versions.items():
        for case in cases:
            test = f"{ROOT}/{database}/{case}"
            rejected = case == "invalid_negative_charge" or case == "legal_token_debit" and database in reproduced
            record = {"database": database, "case": case, "database_version": version,
                      "stage": "target_assertion",
                      "assertion": "preflight_rejects_invalid_negative_charge" if case == "invalid_negative_charge" else "preflight_accepts_legal_amount",
                      "controls_passed": True, "read_only_preserved": True,
                      "preflight_rejected": rejected, "startup_error_is_amount_guard": rejected}
            events.append({"Action": "output", "Package": PACKAGE, "Test": test,
                           "Output": "R4_CASE " + json.dumps(record) + "\n"})
            failed = case == "legal_token_debit" and database in reproduced
            if failed:
                events.append({"Action": "output", "Package": PACKAGE, "Test": test,
                               "Output": "Error Trace: fixture.go:100\nError: Received unexpected error: amount guard\nMessages: " + MESSAGE + "\n"})
            events.append({"Action": "fail" if failed else "pass", "Package": PACKAGE, "Test": test})
        events.append({"Action": "fail" if database in reproduced else "pass", "Package": PACKAGE, "Test": f"{ROOT}/{database}"})
    events.extend([{"Action": "fail" if reproduced else "pass", "Package": PACKAGE, "Test": ROOT},
                   {"Action": "fail" if reproduced else "pass", "Package": PACKAGE}])
    return events


class R4EvidenceBoundaryTest(unittest.TestCase):
    def classify_events(self, events, exit_code=1, stderr=""):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log, error, code = (root / name for name in ("go.jsonl", "go.stderr", "go.exit"))
            log.write_text("\n".join(json.dumps(event) for event in events) + "\n")
            error.write_text(stderr)
            code.write_text(str(exit_code))
            return classify(log, error, code)

    def test_complete_evidence_keeps_original_assertion_failures(self):
        result = self.classify_events(probe_events())
        self.assertTrue(result["evidence_valid"], result["errors"])
        self.assertEqual(result["status"], "reproduced")
        self.assertEqual(result["go_exit_code"], 1)
        self.assertEqual(result["reproduced_databases"], ["sqlite", "mysql", "postgres"])
        self.assertEqual(len(result["actual_failed_tests"]), 7)

    def test_no_reproduction_and_partial_results_are_reported_honestly(self):
        for reproduced, expected in [((), "not_reproduced"), (("sqlite",), "partial_reproduction")]:
            with self.subTest(reproduced=reproduced):
                result = self.classify_events(probe_events(reproduced), exit_code=1 if reproduced else 0)
                self.assertTrue(result["evidence_valid"], result["errors"])
                self.assertEqual(result["status"], expected)
                self.assertEqual(result["reproduced_databases"], list(reproduced))

    def test_missing_database_control_failure_and_changed_data_are_rejected(self):
        original = probe_events()
        missing = [event for event in original if "/postgres" not in event.get("Test", "")]
        control = copy.deepcopy(original)
        for event in control:
            if event.get("Test") == f"{ROOT}/mysql/legal_token_credit" and event["Action"] == "pass":
                event["Action"] = "fail"
        changed = copy.deepcopy(original)
        record = json.loads(changed[0]["Output"].split("R4_CASE ", 1)[1])
        record["read_only_preserved"] = False
        changed[0]["Output"] = "R4_CASE " + json.dumps(record)
        for name, events in [("missing database", missing), ("control failed", control), ("data changed", changed)]:
            with self.subTest(name=name):
                result = self.classify_events(events)
                self.assertFalse(result["evidence_valid"])
                self.assertTrue(result["errors"])

    def test_compile_failure_and_wrong_assertion_cannot_masquerade_as_bug(self):
        compile_result = self.classify_events([], exit_code=1, stderr="probe.go: undefined: fixture")
        self.assertFalse(compile_result["evidence_valid"])
        wrong = probe_events()
        wrong[1]["Output"] = "Error Trace: fixture.go:101\nError: Received unexpected error: fixture failed\nMessages: another invariant\n"
        self.assertFalse(self.classify_events(wrong)["evidence_valid"])

    def test_assertion_before_marker_or_extra_failure_is_not_target_evidence(self):
        early = probe_events()
        early[0], early[1] = early[1], early[0]
        extra = probe_events()
        extra[1]["Output"] += "Error Trace: fixture.go:102\nError: unrelated post-marker failure\n"
        parent_failure = probe_events()
        parent_failure.insert(0, {"Action": "output", "Package": PACKAGE, "Test": ROOT + "/sqlite",
                                  "Output": "Error Trace: cleanup.go:10\nError: cleanup invariant failed\n"})
        for events in (early, extra, parent_failure):
            result = self.classify_events(events)
            self.assertFalse(result["evidence_valid"])
            self.assertTrue(result["errors"])


if __name__ == "__main__":
    unittest.main()
