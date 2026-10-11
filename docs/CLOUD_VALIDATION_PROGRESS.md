# 历史兼容候选的云端验收

更新日期：2026-10-11。状态：`35b4c7bf` 的常规 CI 十项成功，旧 Docker handoff 就绪失败；后续 `28f4f609` 的三个 legacy Docker 场景因新增脚本错误要求 intent 而失败。`c27658c4` 的云端三文件 Node 验证 39 项通过，完整 CI 未收口；本批修正 legacy fact-only 日志合同，新的实际镜像验收仍待运行。准确当前源码及终态以 PR head、Checks 和下方运行记录为准。

## 范围与来源

继续既有修复目标：个人资料持续加载、新请求结算保护、历史金额与账务兼容。不开新功能，不合并 main，不发布版本或镜像，不部署生产。

主线基线为 `92fa1182e265924312d70917a0a637965aebc617`，修复分支为 `codex/historical-accounting-compat-20261010`。接入前的 225 个候选文件已逐文件匹配私有封存摘要；原 v10 候选、旧失败记录、原始账务副本和恢复证据继续保留。新增 CI 文件和本批交接更新属于后续验收批次，不能继续称整个提交只有 225 个文件。

缓存清理已解除上一轮磁盘不足条件，恢复时可用空间约 26 GiB。历史来源、缓存、预扣与事件证据的缺口仍存在。Goal 工具恢复时没有现存记录，已按原批准范围重建主目标；旧停止点作为历史记录保留。

## 执行计划（首次提交前准备快照）

下表保留首次提交前的准备状态，不追溯改写历史记录。当前状态以草稿 PR 中对应提交的 Checks、本页“云端执行记录”和验收说明为准；不以准备阶段状态代替实际结果。

| 阶段 | 状态 | 产物和完成条件 |
| --- | --- | --- |
| 回读基线与候选 | 已完成 | 远端 main 仍为上述基线；225 文件内容匹配，修复分支此前不存在于远端 |
| 接入云端验收 | 已完成，运行待验证 | 仅修改 `.github/workflows/ci.yml`，新增 386、审计工具合成测试、lint 和 profile 异常恢复场景 |
| 公开边界与独立审查 | 进行中 | 核验明确文件清单、私有数据排除、workflow 触发器和上传范围 |
| 提交及分支同步 | 未开始 | 只提交已审代码与文档；推送后回读远端 SHA，保持 main 和历史标签不变 |
| 准确提交的 CI | 未开始 | 建立草稿 PR；核对检查对应的 head SHA、每个作业和新增步骤结果 |
| 镜像合成验收 | 未开始 | 必要时手动运行现有 Docker smoke，构建及 runner 内加载；Full/LAN bundle 按既有规则上传 Actions artifacts 保留 7 天，不推送 GHCR 等发布仓库 |
| 历史实证与生产资格 | 未完成 | 仍依赖历史证据、专项原生数据库验收授权及真实回滚合同 |

## 本批实现与检查

CI 补充如下，复用 GitHub-hosted runner 和现有依赖：

- `GOOS=linux GOARCH=386 CGO_ENABLED=0 go build ./...`，验证本次金额表示在 32 位构建上的兼容性。
- 在 `tools/accounting-audit` 运行 `PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -v`，只使用测试生成的合成临时数据库。
- 前端执行 `bun run lint`，保留现有类型检查、测试及构建。
- 已有 Chromium 步骤执行 `tools/ui/profile-browser.mjs`，覆盖正常读取、空设置、失败重试、请求挂起、认证刷新及启动读取挂起。脚本只访问本机合成 API。

YAML 解析、补丁应用检查和独立静态复核已通过。触发器、权限、job/service 列表及 artifact 路径保持既有定义；新增浏览器结果仅进入 runner 临时目录中的 PNG/JSON 上传范围。现有 Docker smoke 手动运行还会上传构建 bundle，来源是远端 clean checkout，不包含私有生产副本。这些结果不是云端作业运行通过。本机不重复全量编译、不重新创建账务数据库副本。

## 云端执行记录

首个源码提交为 `5ff2e228bdb6bb18c567e1dd97635a93a058d955`，已正常推送至上述修复分支并建立[草稿 PR #11](https://github.com/ForceMind/MyAPI/pull/11)。独立审查确认 227 个提交文件为代码、测试、CI 和文档，未包含生产数据库、凭据或原始敏感日志。

[首轮 CI 38124726661](https://github.com/ForceMind/MyAPI/actions/runs/38124726661)的 PR 测试合并提交为 `cd112d6e0ae4cf78bc842cb926f57a6ed97fc48d`，tree `c353b61066837a3ad4a66eb61b58b952769eefd0` 与源码提交一致。该轮 MySQL/PostgreSQL B2 回归均发现签到额度断言仍使用 `int(25)`，实际字段已为 `int64(25)`，数值相同但严格类型比较失败。本批将此断言改为精确 `int64`，并将充值回执的额度断言从 `EqualValues` 改为同样严格的 `int64` 比较；原金额、并发及回执数量断言保持，生产金额代码不变。

此处记录首轮发现及对应修复，不能称首轮全绿。后续准确提交、CI 每个步骤及必要 Docker smoke 的结果统一在 PR 的 Checks 和验收说明回读，失败原始日志保留为私有证据。PR Check 通过不等于业务 CI 通过。历史原生专项仍未新增或启用。

独立审查另发现同一 B2 fixture 的资金设置和数据库 schema capability 未完整恢复，本批补齐测试 cleanup，避免跨 engine 留下临时状态；恢复失败也须明确报告。主线 CI `38022171765` 的后端作业实际耗时 18分36秒，首轮新增 386 构建另用 2分28秒，两者合计21分04秒，超过原20分钟总预算；将后端总上限调整为30分钟，全部测试及各命令期限保留，不以超时调整掩盖业务失败。

### 准确候选与 Docker handoff 修复计划

PR 运行记录显示，准确源码提交 `35b4c7bf234a386bec48574fda25d4967ff58ed2` 的[常规 CI 38125528583](https://github.com/ForceMind/MyAPI/actions/runs/38125528583)十项均为 success，PR 测试合并 tree 与源码 tree 一致。[Docker smoke 38125659791](https://github.com/ForceMind/MyAPI/actions/runs/38125659791) 的 handoff 场景仍失败：固定旧源码 `71277bf6055ce68b8cd1d11f1d10f1907d7696fd` 在升级前执行了一次扣 15 额度的请求。代码路径表明该请求会产生历史 intent/fact，与当前必须先具备 execution gate 的预检相冲突；原日志未保留 clone 的具体启动错误，此原因尚待新检查直接确认。不能据此放宽生产门禁，也不能把本轮 Docker 验收写成通过。

修复保持正向迁移与拒绝/恢复两条路径互不混用：

1. 在旧镜像完成 Root、普通用户、有限 Key 和渠道配置后、发出任何请求前，保留无结算历史的旧库种子；从两个独立副本分别验证当前镜像的正向迁移和原镜像恢复。
2. 继续原运行中的固定旧镜像夹具，执行且只执行一次 15 额度请求，核对已完成 fact 的日志证据；intent 缺失在 legacy 下合法，存在时必须为正整数且恢复前后身份一致。随后保留原始升级前备份。当前容器读取这一历史快照时必须因缺少 execution gate 而拒绝，退出码固定为 1、`OOMKilled=false`，主数据库文件及 WAL 摘要保持不变。
3. 再由固定旧镜像恢复同一份原始升级前备份，精确核对用户额度、有限 Key 额度和日志身份；上游请求计数仍为 1，证明恢复及就绪探测没有重放请求。

修复不创建 gate 或 permit，不改写历史记录，不降低启动预检，也不改变结算与许可政策。先完成上述 Docker handoff 修复并形成新的准确候选，再将原生 MySQL/PostgreSQL/Redis 云验收提案绑定该最终 SHA 重新生成和复核；原生提案当前仍为 pending，不创建验收分支、不 dispatch、不启动专项 service。

2026-10-11 本批脚本已完成独立只读复审，未发现阻断问题。定向 Node 验证通过 20 项：SQLite 恢复 11 项、交接 6 项、relay 前置验证 3 项；覆盖错误退出码、OOM、错误拒绝原因、数据库或 WAL 变化，以及拒绝证据不足时不得启动旧镜像恢复。另有两份既有 workflow 测试文件执行成功，语法检查和 `git diff --check` 通过。上述均是脚本与合成单元验证，新的实际 Docker 结果仍待云端运行。

复审者执行完整 `node tools/runtime/docker-smoke.test.mjs` 时，三个既有合成上游监听用例因沙箱禁止绑定 `127.0.0.1` 而报 `listen EPERM`：非敏感摘要、Playground 流式合同、缺少 usage 或畸形 stream 拒绝。这轮完整测试未通过；原始输出保留在工具调用记录，未生成本地日志文件。不申请本机端口、不放宽断言。

后续覆盖核对发现，既有 CI 不执行这三份 runtime 单元测试，Docker smoke 只执行实际集成入口。为补齐上述受限用例及错误/OOM/摘要变化分支，本批在现有 distribution 作业加入一个 Node 步骤，仅执行 `docker-smoke.test.mjs`、`sqlite-restore-smoke.test.mjs` 和 `handoff-upgrade-smoke.test.mjs`；不新增 service、job、依赖、权限或 artifact。提交 `28f4f609914f7f64d0e7e4a206a3bee8eaf82c69` 已推送且 tree 与 PR 测试合并树一致，其 CI `38127825760` 和 Docker `38127843731` 为补验前的运行，不能据此声称新增 Node 步骤通过；新步骤结果以随后的准确提交运行回读为准。

### Legacy 结算日志合同修正

[CI 38128072273](https://github.com/ForceMind/MyAPI/actions/runs/38128072273)对应 `c27658c4279f3b715a8aa2f14d335829d9836d91`，PR 合并 tree 与源码一致；新增 Node 步骤实际通过 39 项、失败和跳过均为 0，包括本地 EPERM 的三项监听用例。`28f4f609` 的 CI 被该提交替代并取消，8 个作业成功，前后端取消，不能称全绿。

[Docker 38127843731](https://github.com/ForceMind/MyAPI/actions/runs/38127843731)对应前一 `28f4f609`，镜像 bundle 和 LAN 源码安装成功，Full fresh、LAN fresh、Full handoff 三项均在 `SMOKE_FIXTURE_CONSUME_LOG_MISMATCH` 处失败。全部失败日志与终态私有保留。两版源码独立核对确认：legacy session 不设置 `settlementIntentReady`，会直接创建并恢复 fact，审计合法输出 `state=applied` 和正 `fact_id` 而不含 `intent_id`；Root 日志路径保留该字段。新增脚本与 fake 单测此前错误要求同时存在正 intent，39 项单元通过没有识别此错误合同，不能替代实际镜像验收。

本批仅修正两处 smoke 断言和对应夹具：fact 始终必须为已完成且正整数身份，intent 缺失允许，存在的零值、null、非整数或错误身份继续拒绝。恢复前后仍比较 fact 及 intent 的存在和值，保留 15 额度请求、精确账务与隐私断言、当前镜像拒绝原因/退出码/OOM/DB及WAL摘要验证。不改 Go 结算、门禁、许可或历史数据。该修正仍须新准确提交的云端单元及 Docker 验收；任何原生专项提案只能在最终候选通过后重新绑定并独立复审，不执行旧提案。

本批无监听的 relay/proof 定向检查及 handoff 7 项通过，额外输入明确保护“只有 intent 而没有 fact 必须拒绝”和“恢复后新增 intent 也属于身份变化”。四份变更脚本语法及差异检查通过；未改的 SQLite 恢复单元沿用前轮 11 项结果。本地未启动服务或构建，完整三文件 Node 步骤和实际双镜像仍由新提交的 GitHub-hosted 运行验证。

独立复审已从固定旧版和当前 Go 真实路径确认 fact-only 合同，当前七文件差异通过复审，无阻断项；这项静态与定向结果不代替新镜像运行。

## 分工与资源

主线程负责目标、文档、workflow 步骤命名、集成、提交同步和云端结果核对。`resume_ci_plan` 使用 gpt-5.6-sol/ultra 只读规划，后仅更新本页计划；`ci_offload_implementation` 使用 gpt-5.6-sol/high，依次负责 CI、上述 B2 测试及 `tools/runtime` 的三份 smoke 脚本和三份对应测试；`candidate_publish_boundary_review` 使用 gpt-5.6-sol/xhigh，只读核验公开、运行边界及修复。禁止递归派工；共享文件由主线程串行集成。

不改 Git 用户配置。若环境没有提交身份，使用明确的 Codex 机器人标识并在 PR 声明 AI 协助；不得冒用维护者身份。草稿 PR 保留仓库模板，人工确认项如实保留未确认状态。

CI 失败只修复本范围内有证据的缺陷。候选摘要漂移、私有数据进入提交、意外发布/生产连接、账务语义或许可政策需要改变时停止对应步骤并保留证据。原生历史 MySQL/PostgreSQL 专项服务未在本批新增或启用。

## 未闭合边界

历史 beta.3 一致性来源、完整资金流、历史预扣/缓存及全部 writer、未知事件、旧终态凭证适用来源和实际回滚仍未闭合。未知项继续冻结，演练副本继续 `DO_NOT_RESTORE`，`ledger_reconciled=false`、`production_eligible=false`。

云端合成结果只证明对应代码和合成输入下的行为，不能证明用户历史数据库可安全升级。发布、实际部署及生产恢复继续另行取得明确批准。更多事实及历史验证范围见[历史账务兼容核查记录](HISTORICAL_ACCOUNTING_COMPATIBILITY.md)。
