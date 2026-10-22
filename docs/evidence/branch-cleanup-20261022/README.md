# main / dev 分支整理（2026-10-22，北京时间）

维护者明确要求逐一核对全部分支，最终只保留 `main` 和 `dev`。本批范围是分支引用、历史恢复包、直接引用旧分支名的校验/手动构建配置、仓库开发约定及接手入口。应用业务代码、版本、生产数据和部署不变；没有合入 main、发布或创建标签。

## 开发入口和清理前统计

- `main` 保持远端 `92fa1182e265924312d70917a0a637965aebc617`；本地 main 原来落后，按该远端提交快进。
- `dev` 完整接续原开发线 `ea4ea6b7d11f4af99e341816eec370f57eddbd82`；业务代码基线为 `e4580454021369448fac9da09b6faa1cff2096e6`。本批整理提交由 Git 历史确认。
- 清理前远端21个分支、本地12个分支，名称去重后24个；完整33条引用快照见 [initial-branches.json](initial-branches.json)。逐一核对了 tip、祖先关系和独有提交；3条分叉线另检查提交说明、变更范围和合并冲突。没有声称对历史各版本做重新业务验收。
- 远端20个旧开发名称中，最新开发线1个、已被该线包含16个、仍有独有提交3个。3条独有历史线分别是旧0.1.4/0.1.5部署与渠道UI、历史原生数据库CI、旧预发布与跨库修订；不强行混入当前代码。
- 7个开放旧PR的原始head/base及链接见 [initial-open-prs.json](initial-open-prs.json)。7个旧PR已随head分支删除自动关闭且均未合并，审阅记录保留；未新建PR。
- [cleanup-result.json](cleanup-result.json)记录实际清理回读：远端和本地均只剩main/dev，默认分支main不变；该快照在最后证据提交前取得，最终dev HEAD以Git和远端回读为准。

## 可恢复的历史归档

[legacy-unique-commits.bundle](legacy-unique-commits.bundle) 保存3条分叉线的24个独有提交，共858362字节，SHA256：

```text
c9ffa1f68ae9fd6dc0e3e4bab28d51244db1cce4ac187bb9f78f1a1b941ed2ef
```

该包是相对于现有 main/dev 历史的增量Git包，3个前置提交都在 dev 基线历史中。[restore-verification.json](restore-verification.json)记录实际验证：在全新bare仓库中先只取main/dev基线，再验证包、取回3条历史引用，33条原始tip全部可解析。其余旧分支的对象已可从 dev 历史直接找到，不需要保留活动分支。

在含完整dev历史的仓库中可只取回需要的历史提交，不创建活动分支：

```bash
git bundle verify docs/evidence/branch-cleanup-20261022/legacy-unique-commits.bundle
git fetch docs/evidence/branch-cleanup-20261022/legacy-unique-commits.bundle refs/remotes/origin/codex/mac-durable-accounting
git show FETCH_HEAD
```

其他历史tip用清单中的完整SHA读取。生产恢复、旧库运行和发布仍受对应历史门槛约束，Git历史恢复不构成部署授权。

## 校验与剩余边界

- CI和站点校验/打包的push入口使用main/dev；原针对旧分支的回归选择改为dev。
- Docker合成验收仅接受本仓库非Draft的dev PR或显式手动运行；保留两种writer、历史handoff和独立LAN安装器。手动镜像构建选择为main/dev，原PUBLISH确认、仓库发布开关及environment约束保留。本批没有运行构建、推送镜像或部署。
- 全部工作流YAML可解析，目标分支过滤及旧分支选择器检查通过；受影响分支门禁/矩阵、镜像复用及恢复驱动定向Node检查通过。
- [workflow-verification.json](workflow-verification.json)保留旧源码与候选对照：完整installer-workflow文件有两个相同的既有失败（镜像默认版本断言，以及安全JSON报告上传被旧断言禁止）。候选6例通过、没有新增失败。本批不修改相关镜像或上传行为，不把完整文件说成全绿。未执行新的完整后端、前端、容器或真实供应商验证。
- 产品体验接受、正文日志边界、精确Key请求归因和真实供应商专项仍沿原平台计划开放，不能用分支清理替代。

## 逐分支处理清单

“独有提交”以整理前dev基线为参照，快照保留完整SHA；本地beta.1与远端mac-durable-accounting同tip。

| 范围 | 原名称 | 原tip | 独有提交 | 处理 |
| --- | --- | --- | ---: | --- |
| 本地 | `codex/cloud-platform-flow-20261022` | `ea4ea6b7d11f4af99e341816eec370f57eddbd82` | 0 | 接续为 dev 后移除旧名称 |
| 本地 | `codex/deploy-0.1.4-actions` | `7525e539863b3daab595f82fce9c17af50240640` | 16 | 独有提交已进入 bundle，移除旧名称 |
| 本地 | `codex/deploy-0.2.0-beta.1` | `e8ad900b10abba1844dcdbaf7127916b16b905b0` | 7 | 独有提交已进入 bundle，移除旧名称 |
| 本地 | `codex/deploy-0.2.0-beta.3` | `2f2717bffe580baf23820480a7d40d54e676199e` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `codex/deploy-0.2.0-beta.7` | `7492ec36edf68716953cc81c304f4c902d392751` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `codex/historical-accounting-compat-20261010` | `0b51bb4e176326c37daa129caaf1bd542623a492` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `codex/historical-accounting-native-20261011` | `c8e7bbc04676e46f2ad0c806a9ac78562923a4ef` | 1 | 独有提交已进入 bundle，移除旧名称 |
| 本地 | `codex/personal-core-20261014` | `c1beb07520f43c106b4406ba8a0b1227bc1cf67e` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `codex/personal-restart-20261014` | `963fabe521e9905c450b2e45968d0d7402488594` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `codex/subscription-key-share-20261014` | `6e7aadeb16dc06a192298ec9b13aac5ff4c6e28c` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `codex/ui-stability-20261012` | `0000e8654660c01b6418313d09cae71ff933f243` | 0 | 提交已在 dev 中，移除旧名称 |
| 本地 | `main` | `f6536ca96126415f74d239165fd688589bd54af9` | 0 | 本地 main 快进到远端 main |
| 远端 | `codex/beta11-release-candidate-20261010` | `28bc16cd4efee628ca689058c3c91ffbf3d15016` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/beta8-full-ui-20261007` | `5c995e9062283c35ad459775b6b06f626f6f6c64` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/beta9-chat-api-20261009` | `ba2a4181440b4a8aa6f5fdb351add4c65518ab72` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/cloud-cancel-errors-20261021` | `002bb033924488d7fd7333409ca1d101c43fae35` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/cloud-platform-flow-20261022` | `ea4ea6b7d11f4af99e341816eec370f57eddbd82` | 0 | 接续为 dev 后移除旧名称 |
| 远端 | `codex/deploy-0.1.4-actions` | `7525e539863b3daab595f82fce9c17af50240640` | 16 | 独有提交已进入 bundle，移除旧名称 |
| 远端 | `codex/docker-smoke-image-reuse-20261010` | `8aece4e622512dfe83f31d6e70042e6d2652c13d` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/historical-accounting-compat-20261010` | `0b51bb4e176326c37daa129caaf1bd542623a492` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/historical-accounting-native-20261011` | `c8e7bbc04676e46f2ad0c806a9ac78562923a4ef` | 1 | 独有提交已进入 bundle，移除旧名称 |
| 远端 | `codex/mac-durable-accounting` | `e8ad900b10abba1844dcdbaf7127916b16b905b0` | 7 | 独有提交已进入 bundle，移除旧名称 |
| 远端 | `codex/personal-app-journey-20261009` | `b96744f52c4178ae1eed362e8ee6630cc5a77190` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/personal-core-20261014` | `c1beb07520f43c106b4406ba8a0b1227bc1cf67e` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/personal-core-clarity-20261009` | `a17a8e11b7600b8c3091f80047eef6cbcfa87f11` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/personal-restart-20261014` | `963fabe521e9905c450b2e45968d0d7402488594` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/r1-handoff-20261002` | `71277bf6055ce68b8cd1d11f1d10f1907d7696fd` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/r1-usage-review-20261002` | `4f9699bca18ae6d7de564baf87ea7f103f55d84e` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/settlement-review-status-20261009` | `09e587d46493f68569434e8506e3893c1a6fb804` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/subscription-admission-errors-20261009` | `026905e1cc9a3197c51bc6e80f81b07bdeac170d` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/subscription-key-share-20261014` | `6e7aadeb16dc06a192298ec9b13aac5ff4c6e28c` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `codex/ui-stability-20261012` | `0000e8654660c01b6418313d09cae71ff933f243` | 0 | 提交已在 dev 中，移除旧名称 |
| 远端 | `main` | `92fa1182e265924312d70917a0a637965aebc617` | 0 | 保留 stable main |
