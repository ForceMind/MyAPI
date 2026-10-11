import concurrent.futures
import json
from pathlib import Path
import sqlite3
import tempfile
import threading
import unittest

import audit
from readonly import SnapshotError, SnapshotReader, _sha256_file
import rehearsal


class CopyRehearsalTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="accounting-simulation-")
        self.root = Path(self.temp.name)
        self.source_dir = self.root/"source"
        self.source_dir.mkdir(mode=0o700)
        self.source = self.source_dir/"sealed.db"
        self.manifest = self.source_dir/"manifest.json"
        c = sqlite3.connect(self.source)
        c.execute("CREATE TABLE quota_writer_epochs(id INTEGER,schema_version INTEGER,mode TEXT,epoch INTEGER,lock_version INTEGER)")
        c.execute("INSERT INTO quota_writer_epochs VALUES(1,1,'legacy',1,1)")
        c.execute("CREATE TABLE users(id INTEGER PRIMARY KEY,quota INTEGER,used_quota INTEGER,quota_version INTEGER)")
        c.execute("INSERT INTO users VALUES(1,2147485000,2147486000,4)")
        c.execute("CREATE TABLE tokens(id INTEGER PRIMARY KEY,user_id INTEGER,remain_quota INTEGER,used_quota INTEGER,quota_version INTEGER)")
        c.execute("INSERT INTO tokens VALUES(2,1,2147487000,2147488000,7)")
        text = {"event_key", "request_id", "kind", "request_fingerprint", "state"}
        columns = ",".join(name + (" TEXT" if name in text else " INTEGER") for name in rehearsal.FACT_COLUMNS)
        c.execute("CREATE TABLE account_quota_settlement_facts("+columns+")")
        for table in ("account_quota_settlement_intents", "account_quota_refund_facts", "quota_balance_batch_drains"):
            c.execute("CREATE TABLE "+table+"(id INTEGER)")
        c.execute("CREATE TABLE legacy_usage_reservations(request_id TEXT,user_id INTEGER,token_id INTEGER,subscription_id INTEGER,funding_source TEXT,state TEXT,reserved_quota INTEGER,token_reserved_quota INTEGER,actual_quota INTEGER)")
        c.execute("CREATE TABLE logs(request_id TEXT,user_id INTEGER,token_id INTEGER,quota INTEGER,other TEXT,type INTEGER)")
        for index, delta in enumerate((5,7,13),1):
            row = dict.fromkeys(rehearsal.FACT_COLUMNS, 0)
            row.update(id=index,event_key=f"event-{index}",request_id=f"request-{index}",kind="legacy_wallet",user_id=1,token_id=2,delta=delta,apply_token=1,state="retryable",lock_version=1)
            row["request_fingerprint"] = audit._settlement_fingerprint(row)
            c.execute("INSERT INTO account_quota_settlement_facts VALUES("+",".join("?" for _ in row)+")", tuple(row.values()))
            if index < 3:
                other=json.dumps({"admin_info":{"billing_settlement":{"event_key":row["event_key"],"actual_quota":delta,"state":"retryable"}},"private":"must-never-be-returned"})
                c.execute("INSERT INTO logs VALUES(?,1,2,?,?,2)", (row["request_id"],delta,other))
        c.commit()
        c.close()
        self.seal()

    def tearDown(self):
        self.temp.cleanup()

    def seal(self):
        self.manifest.write_text(json.dumps({"snapshot":{"path":str(self.source),"sha256":_sha256_file(self.source),"source":{"description":"synthetic copy-only simulation fixture"}}}))
        self.source.chmod(0o400)

    def plan(self):
        with SnapshotReader(self.source,self.manifest) as reader:
            return rehearsal.source_plan(reader)

    def mutable_copy(self):
        destination=self.root/"test-copy.db"
        destination.write_bytes(self.source.read_bytes())
        return destination

    def connect(self,path):
        return sqlite3.connect(path,isolation_level=None,timeout=30)

    def test_source_unchanged_exact_difference_and_frozen_fact(self):
        before=_sha256_file(self.source)
        report=rehearsal.run_simulation(self.source,self.manifest,self.root/"result")
        self.assertEqual(2,report["selected_events"])
        self.assertEqual(1,report["frozen_events"])
        self.assertEqual({"funding":2,"token":2},report["applied_legs_first_run"])
        self.assertEqual({},report["applied_legs_second_run"])
        self.assertFalse(report["production_eligible"])
        self.assertFalse(report["ledger_reconciled"])
        self.assertEqual("unknown",report["cache_evidence"])
        self.assertEqual(before,_sha256_file(self.source))
        with self.connect(self.root/"result/simulation.db") as c:
            self.assertEqual((2147484988,2147486000,6),c.execute("SELECT quota,used_quota,quota_version FROM users").fetchone())
            self.assertEqual((2147486988,2147488012,9),c.execute("SELECT remain_quota,used_quota,quota_version FROM tokens").fetchone())
            self.assertEqual((0,0,"retryable",1),c.execute("SELECT funding_applied,token_applied,state,lock_version FROM account_quota_settlement_facts WHERE id=3").fetchone())
        self.assertEqual(0o600,(self.root/"result/simulation.db").stat().st_mode & 0o777)
        self.assertEqual(0o700,(self.root/"result").stat().st_mode & 0o777)

    def test_transaction_failure_rolls_back_balance_and_flag_together(self):
        path,plan=self.mutable_copy(),self.plan()
        def fail(stage,leg):
            if stage=="before_flag":
                raise RuntimeError("synthetic interrupted transaction")
        with self.connect(path) as c:
            before=rehearsal._balances(c)
            with self.assertRaises(RuntimeError):
                rehearsal.simulate_connection(c,plan,fail)
            self.assertEqual(before,rehearsal._balances(c))
            self.assertEqual((0,0),c.execute("SELECT funding_applied,token_applied FROM account_quota_settlement_facts WHERE id=1").fetchone())

    def test_restart_after_funding_commit_only_finishes_missing_leg(self):
        path,plan=self.mutable_copy(),self.plan()
        def fail(stage,leg):
            if stage=="after_commit" and leg=="funding":
                raise RuntimeError("synthetic process interruption")
        c=self.connect(path)
        before=rehearsal._balances(c)
        with self.assertRaises(RuntimeError):
            rehearsal.simulate_connection(c,plan,fail)
        self.assertEqual((1,0),c.execute("SELECT funding_applied,token_applied FROM account_quota_settlement_facts WHERE id=1").fetchone())
        c.close()
        with self.connect(path) as reopened:
            counts=rehearsal.simulate_connection(reopened,plan)
            self.assertEqual({"funding":1,"token":2},counts)
            rehearsal.verify_conservation(before,rehearsal._balances(reopened),plan)
            self.assertEqual({},rehearsal.simulate_connection(reopened,plan))

    def test_concurrent_replay_changes_each_leg_once(self):
        path,plan=self.mutable_copy(),self.plan()
        with self.connect(path) as c:
            before=rehearsal._balances(c)
        start=threading.Barrier(2)
        def worker():
            c=self.connect(path)
            try:
                start.wait(timeout=10)
                return rehearsal.simulate_connection(c,plan)
            finally:
                c.close()
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results=list(pool.map(lambda _:worker(),range(2)))
        self.assertEqual(2,sum(r.get("funding",0) for r in results))
        self.assertEqual(2,sum(r.get("token",0) for r in results))
        with self.connect(path) as c:
            rehearsal.verify_conservation(before,rehearsal._balances(c),plan)

    def test_duplicate_log_is_frozen_and_raw_payload_never_exposed(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("INSERT INTO logs SELECT * FROM logs WHERE request_id='request-1'")
        self.seal()
        with SnapshotReader(self.source,self.manifest) as reader:
            logs=reader.settlement_log_evidence()
            self.assertNotIn("must-never-be-returned",json.dumps(logs))
            self.assertEqual([2],[r["id"] for r in rehearsal.source_plan(reader)["facts"]])

    def test_conflicting_fact_identity_is_rejected_before_mutation(self):
        path,plan=self.mutable_copy(),self.plan()
        with self.connect(path) as c:
            before=rehearsal._balances(c)
            c.execute("UPDATE account_quota_settlement_facts SET delta=6 WHERE id=1")
            with self.assertRaises(SnapshotError):
                rehearsal.simulate_connection(c,plan)
            self.assertEqual(before,rehearsal._balances(c))

    def test_same_event_in_another_request_log_freezes_candidate(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("INSERT INTO logs SELECT 'another-request',user_id,token_id,quota,other,type FROM logs WHERE request_id='request-1'")
        self.seal()
        self.assertEqual([2],[r["id"] for r in self.plan()["facts"]])

    def test_prepared_journal_with_actual_evidence_freezes_candidate(self):
        for actual in (999, 0, 1.25):
            with self.subTest(actual=actual):
                self.source.chmod(0o600)
                with sqlite3.connect(self.source) as c:
                    c.execute("DELETE FROM legacy_usage_reservations")
                    c.execute("INSERT INTO legacy_usage_reservations VALUES('request-1',1,2,0,'wallet','prepared',0,0,?)", (actual,))
                self.seal()
                self.assertEqual([2],[r["id"] for r in self.plan()["facts"]])

    def test_trigger_is_rejected_before_copy(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("CREATE TRIGGER hidden_mutation AFTER UPDATE ON users BEGIN DELETE FROM logs; END")
        self.seal()
        with self.assertRaises(SnapshotError):
            rehearsal.run_simulation(self.source,self.manifest,self.root/"result")
        self.assertFalse((self.root/"result").exists())

    def test_view_in_place_of_subject_table_is_rejected(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("ALTER TABLE users RENAME TO real_users")
            c.execute("CREATE VIEW users AS SELECT * FROM real_users")
        self.seal()
        with self.assertRaises(SnapshotError):
            self.plan()

    def test_foreign_key_cascade_is_rejected_before_copy(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("CREATE UNIQUE INDEX quota_fk_target ON users(quota)")
            c.execute("CREATE TABLE hidden_cascade(parent_quota INTEGER REFERENCES users(quota) ON UPDATE CASCADE)")
            c.execute("INSERT INTO hidden_cascade VALUES(2147485000)")
        self.seal()
        with self.assertRaises(SnapshotError):
            rehearsal.run_simulation(self.source,self.manifest,self.root/"result")
        self.assertFalse((self.root/"result").exists())

    def test_duplicate_fact_id_is_rejected(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("UPDATE account_quota_settlement_facts SET id=1 WHERE id=2")
        self.seal()
        with self.assertRaises(SnapshotError):
            self.plan()

    def test_real_amount_storage_is_rejected_before_copy(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("UPDATE account_quota_settlement_facts SET delta=5.25 WHERE id=1")
        self.seal()
        with self.assertRaises(SnapshotError):
            rehearsal.run_simulation(self.source,self.manifest,self.root/"result")
        self.assertFalse((self.root/"result").exists())

    def test_existing_destination_and_sealed_directory_are_rejected(self):
        existing=self.root/"existing"
        existing.mkdir()
        with self.assertRaises(FileExistsError):
            rehearsal.run_simulation(self.source,self.manifest,existing)
        with self.assertRaises(SnapshotError):
            rehearsal.run_simulation(self.source,self.manifest,self.source_dir/"child")
        self.assertFalse((self.source_dir/"child").exists())

    def test_writable_source_is_rejected_before_creating_output(self):
        self.source.chmod(0o600)
        with self.assertRaises(SnapshotError):
            rehearsal.run_simulation(self.source,self.manifest,self.root/"result")
        self.assertFalse((self.root/"result").exists())

    def test_simulated_deficits_are_reported_and_never_clamped(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("UPDATE users SET quota=10")
            c.execute("UPDATE tokens SET remain_quota=5")
        self.seal()
        report=rehearsal.run_simulation(self.source,self.manifest,self.root/"deficit-result")
        self.assertEqual(1,report["negative_user_quota_after_simulation"])
        self.assertEqual(1,report["negative_token_remain_after_simulation"])
        self.assertFalse(report["production_eligible"])
        with self.connect(self.root/"deficit-result/simulation.db") as c:
            self.assertEqual((-2,),c.execute("SELECT quota FROM users").fetchone())
            self.assertEqual((-7,),c.execute("SELECT remain_quota FROM tokens").fetchone())

    def test_wide_targets_are_exact_and_normal_limits_preserved(self):
        self.assertEqual(2147484995,rehearsal.historical_target(2147485000,-5))
        self.assertEqual(2147485005,rehearsal.historical_target(2147485000,5,used=True))
        for current,delta,used in ((audit.INT32_MAX,1,False),(rehearsal.SAFE_INTEGER,1,False),(100,-101,True),(100,2**31,False),(1.5,1,False)):
            with self.subTest(current=current,delta=delta),self.assertRaises(SnapshotError):
                rehearsal.historical_target(current,delta,used=used)

    def test_source_wide_qualification_survives_crossing_int32_region(self):
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("UPDATE users SET quota=?", (2**31+1,))
            c.execute("DELETE FROM logs")
            facts = c.execute("SELECT "+",".join(rehearsal.FACT_COLUMNS)+" FROM account_quota_settlement_facts").fetchall()
            for values in facts:
                row = dict(zip(rehearsal.FACT_COLUMNS, values))
                row["delta"] = audit.INT32_MAX
                c.execute("UPDATE account_quota_settlement_facts SET delta=?,request_fingerprint=? WHERE id=?", (row["delta"],audit._settlement_fingerprint(row),row["id"]))
                evidence = json.dumps({"admin_info":{"billing_settlement":{"event_key":row["event_key"],"actual_quota":row["delta"],"state":"retryable"}}})
                c.execute("INSERT INTO logs VALUES(?,1,2,?,?,2)", (row["request_id"],row["delta"],evidence))
        self.seal()
        report = rehearsal.run_simulation(self.source,self.manifest,self.root/"wide-result")
        self.assertEqual({"funding":3,"token":3}, report["applied_legs_first_run"])
        self.assertEqual({},report["applied_legs_second_run"])
        with self.connect(self.root/"wide-result/simulation.db") as c:
            self.assertEqual((2**31+1-3*audit.INT32_MAX,),c.execute("SELECT quota FROM users").fetchone())
        self.source.chmod(0o600)
        with sqlite3.connect(self.source) as c:
            c.execute("UPDATE users SET quota=?", (audit.INT32_MAX,))
        self.seal()
        with self.assertRaises(SnapshotError):
            rehearsal.run_simulation(self.source,self.manifest,self.root/"normal-result")
        self.assertFalse((self.root/"normal-result").exists())

    def test_qualification_is_field_specific_and_versions_preflighted(self):
        for mutation in ("UPDATE tokens SET used_quota=2147483640", "UPDATE users SET quota_version=9223372036854775806", "UPDATE account_quota_settlement_facts SET lock_version=9223372036854775806 WHERE id=1"):
            with self.subTest(mutation=mutation):
                path = self.root/"validation.db"
                path.write_bytes(self.source.read_bytes())
                with sqlite3.connect(path) as c:
                    c.execute(mutation)
                path.chmod(0o400)
                manifest = self.root/"validation.json"
                manifest.write_text(json.dumps({"snapshot":{"path":str(path),"sha256":_sha256_file(path),"source":{"description":"synthetic range/version preflight fixture"}}}))
                output = self.root/"invalid-result"
                with self.assertRaises(SnapshotError):
                    rehearsal.run_simulation(path,manifest,output)
                self.assertFalse(output.exists())
                path.unlink()


if __name__=="__main__":
    unittest.main()
