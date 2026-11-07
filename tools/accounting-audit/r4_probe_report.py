#!/usr/bin/env python3
"""Classify only the R4 synthetic Go probe's complete, exact evidence contract.

A successful evidence job never means the product is fixed. Raw Go assertion
failures remain in the artifact; compile, fixture, connection and unexpected
test failures make this checker fail.
"""

from __future__ import annotations

import argparse
import json
import os
import re
from pathlib import Path


ROOT = "TestR4QuotaAmountPreflightProbe"
DATABASES = ("sqlite", "mysql", "postgres")
CASES = (
    "legal_token_debit", "legal_token_credit", "ordinary_reserve_settle",
    "invalid_negative_charge",
)
DEBIT_MESSAGE = "legal token debit must survive startup amount validation"


def classify(log: Path, stderr: Path, exit_file: Path) -> dict:
    errors = []
    records = {}
    outcomes = {}
    markers_at = {}
    terminal_at = {}
    packages = set()
    package_outcomes = []
    events = []
    try:
        go_exit = int(exit_file.read_text().strip())
        for number, line in enumerate(log.read_text().splitlines(), 1):
            try:
                event = json.loads(line)
                if not isinstance(event, dict) or not isinstance(event.get("Action"), str):
                    raise ValueError("not a Go event")
                if any(field in event and not isinstance(event[field], str)
                       for field in ("Output", "Test", "Package")):
                    raise ValueError("invalid Go event field")
                events.append(event)
            except (ValueError, TypeError):
                errors.append(f"Go JSON 第 {number} 行无效")
        # Build/compiler diagnostics are not the desired test assertion failure.
        diagnostics = stderr.read_text().splitlines()
        if any(line.strip() and not re.fullmatch(r"go: downloading \S+ \S+", line)
               for line in diagnostics):
            errors.append("Go 标准错误非空，请检查原始日志；不能认定为目标断言复现")
    except (OSError, ValueError) as exc:
        return {"evidence_valid": False, "status": "incomplete_evidence",
                "go_exit_code": None, "databases": {},
                "errors": [f"缺少或无法读取原始取证文件：{type(exc).__name__}"]}

    for index, event in enumerate(events):
        package = event.get("Package")
        if package:
            packages.add(package)
        test = event.get("Test")
        action = event["Action"]
        output = event.get("Output", "")
        if action in ("fail", "pass", "skip"):
            if test:
                if test in outcomes:
                    errors.append(f"重复终态：{test}")
                outcomes[test] = action
                terminal_at[test] = index
            else:
                package_outcomes.append(action)
        if "R4_CASE " not in output:
            continue
        try:
            record = json.loads(output.split("R4_CASE ", 1)[1].strip())
            database, case = record["database"], record["case"]
            expected_test = f"{ROOT}/{database}/{case}"
            if database not in DATABASES or case not in CASES or test != expected_test:
                raise ValueError("unexpected marker location")
            if (database, case) in records:
                raise ValueError("duplicate marker")
            records[database, case] = record
            markers_at[expected_test] = index
        except (KeyError, ValueError, TypeError):
            errors.append("R4_CASE 标记无效、重复或不在对应子测试内")

    expected_tests = {ROOT}
    expected_failures = set()
    databases = {}
    for database in DATABASES:
        expected_tests.add(f"{ROOT}/{database}")
        case_results = {}
        versions = set()
        for case in CASES:
            prior_errors = len(errors)
            test = f"{ROOT}/{database}/{case}"
            expected_tests.add(test)
            record = records.get((database, case))
            outcome = outcomes.get(test)
            if record is None:
                errors.append(f"缺少 {database}/{case} 最终证据")
                continue
            for field in ("controls_passed", "read_only_preserved"):
                if record.get(field) is not True:
                    errors.append(f"{database}/{case} 的 {field} 未通过")
            for field in ("preflight_rejected", "startup_error_is_amount_guard"):
                if type(record.get(field)) is not bool:
                    errors.append(f"{database}/{case} 的 {field} 不是布尔值")
            assertion = ("preflight_rejects_invalid_negative_charge"
                         if case == "invalid_negative_charge"
                         else "preflight_accepts_legal_amount")
            if record.get("stage") != "target_assertion" or record.get("assertion") != assertion:
                errors.append(f"{database}/{case} 尚未到达指定目标断言")
            version = record.get("database_version")
            if not isinstance(version, str) or not version.strip():
                errors.append(f"{database}/{case} 缺少数据库实际版本")
            else:
                versions.add(version)
            if markers_at[test] >= terminal_at.get(test, -1):
                errors.append(f"{database}/{case} 缺少标记之后的测试终态")
            rejected = record.get("preflight_rejected")
            guard = record.get("startup_error_is_amount_guard")
            if case == "legal_token_debit" and rejected is True and guard is True:
                expected_failures.update((test, f"{ROOT}/{database}", ROOT))
                text = "".join(event.get("Output", "")
                               for event in events[markers_at[test] + 1:]
                               if event.get("Test") == test)
                if (outcome != "fail" or text.count(DEBIT_MESSAGE) != 1
                        or text.count("Error Trace:") != 1
                        or "Received unexpected error:" not in text):
                    errors.append(f"{database} 正常扣减没有产生唯一的指定 NoError 断言失败")
                result = "reproduced"
            else:
                expected_rejection = case == "invalid_negative_charge"
                if outcome != "pass" or rejected is not expected_rejection or guard is not expected_rejection:
                    errors.append(f"{database}/{case} 的对照或目标结果异常")
                result = "not_reproduced" if case == "legal_token_debit" else "control_passed"
            if len(errors) != prior_errors:
                result = "invalid_evidence"
            case_results[case] = {
                "test_outcome": outcome, "result": result,
                "preflight_rejected": rejected, "startup_error_is_amount_guard": guard,
                "controls_passed": record.get("controls_passed"),
                "read_only_preserved": record.get("read_only_preserved"),
                "observations": record,
            }
        if len(versions) != 1:
            errors.append(f"{database} 数据库版本缺失或前后不一致")
        databases[database] = {"version": next(iter(versions)) if len(versions) == 1 else None,
                               "cases": case_results}

    if set(outcomes) != expected_tests:
        errors.append("测试集合不完整或出现意外测试")
    actual_failures = {test for test, outcome in outcomes.items() if outcome == "fail"}
    if actual_failures != expected_failures:
        errors.append("失败集合与唯一允许的正常扣减目标断言不一致")
    target_failures = {f"{ROOT}/{database}/legal_token_debit" for database in DATABASES
                       if f"{ROOT}/{database}/legal_token_debit" in expected_failures}
    if any("Error Trace:" in event.get("Output", "")
           and event.get("Test") not in target_failures for event in events):
        errors.append("目标子测试以外存在断言失败，不能将父测试的额外失败算作复现")
    if any(outcome == "skip" for outcome in outcomes.values()):
        errors.append("专项测试出现跳过，三库证据不完整")
    expected_exit = 1 if expected_failures else 0
    if (go_exit != expected_exit or package_outcomes != ["fail" if expected_failures else "pass"]
            or len(packages) != 1 or not next(iter(packages), "").endswith("/model")):
        errors.append("Go 退出码或包结果与目标断言不一致，可能存在构建或初始化失败")
    reproduced = [database for database in DATABASES
                  if databases[database]["cases"].get("legal_token_debit", {}).get("result") == "reproduced"]
    status = ("reproduced" if len(reproduced) == 3 else
              "partial_reproduction" if reproduced else "not_reproduced")
    if errors:
        status = "incomplete_or_unexpected_evidence"
    return {"evidence_valid": not errors, "status": status, "go_exit_code": go_exit,
            "reproduced_databases": reproduced, "databases": databases,
            "actual_failed_tests": sorted(actual_failures), "errors": errors}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--log", required=True, type=Path)
    parser.add_argument("--stderr", required=True, type=Path)
    parser.add_argument("--exit-code-file", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    report = classify(args.log, args.stderr, args.exit_code_file)
    source = os.environ.get("R4_SOURCE_SHA", "")
    if (os.environ.get("GITHUB_ACTIONS") != "true"
            or os.environ.get("GITHUB_REF") != "refs/heads/dev"
            or not re.fullmatch(r"[0-9a-f]{40}", source)
            or source != os.environ.get("GITHUB_SHA")):
        report["evidence_valid"] = False
        report["status"] = "incomplete_or_unexpected_evidence"
        report["errors"].append("缺少 Actions dev 分支及精确 checkout SHA 门禁")
    report.update({"schema_version": 1, "source_sha": source,
                   "run_id": os.environ.get("GITHUB_RUN_ID"),
                   "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT"),
                   "browser": "not_run", "production_touched": False,
                   "model_requests": 0, "product_fixed": False,
                   "scope": "disposable_synthetic_databases_only"})
    encoded = json.dumps(report, ensure_ascii=False, indent=2)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(encoded + "\n")
    if not report["evidence_valid"]:
        headline = "取证未通过：证据不完整或存在意外失败，不能据此认定复现。"
    elif report["status"] == "reproduced":
        headline = "三库均复现：正常扣减后的记录在再次启动检查时被金额校验拒绝；三个对照均通过。"
    elif report["status"] == "partial_reproduction":
        headline = ("部分数据库复现：" + "、".join(report["reproduced_databases"])
                    + " 的正常扣减记录在再次启动检查时被金额校验拒绝；三库对照均通过。")
    else:
        headline = "三库均未复现：正常扣减后的记录通过再次启动检查；三个对照均通过。"
    summary = [headline, "本轮没有修产品；仅使用临时合成数据，没有请求模型、访问生产或运行浏览器。",
               "", f"来源 SHA：`{source}`；运行：`{report['run_id']}`；尝试：`{report['run_attempt']}`。",
               "", "| 数据库 | 实际版本 | 正常扣减 / 再次启动检查 | 对照 |",
               "| --- | --- | --- | --- |"]
    for database in DATABASES:
        data = report["databases"].get(database, {})
        cases = data.get("cases", {})
        debit = cases.get("legal_token_debit", {})
        result = {"reproduced": "被金额校验拒绝（复现）", "not_reproduced": "通过（未复现）"}.get(debit.get("result"), "证据缺失")
        controls_ok = all(cases.get(case, {}).get("result") == "control_passed" for case in CASES[1:])
        summary.append(f"| {database} | {data.get('version') or '未知'} | {result} | {'通过' if controls_ok else '未通过或不完整'} |")
    if report["errors"]:
        summary.extend(("", "失败原因："))
        summary.extend(f"- {error}" for error in report["errors"])
    summary.extend(("", "下一步：依据本次完整日志提出最小修复方案；产品修复仍需单独授权与验收。"))
    rendered = "\n".join(summary) + "\n"
    print(rendered)
    print("完整结构化安全报告：")
    print(encoded)
    summary_path = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary_path:
        with Path(summary_path).open("a") as output:
            output.write(rendered)
            output.write("\n<details><summary>完整结构化安全报告</summary>\n\n```json\n")
            output.write(encoded + "\n```\n</details>\n")
    return 0 if report["evidence_valid"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
