# My API 全项目完成执行计划与交接基线

> 2026-09-08 最新：额度页首屏布局修正源码 0.1.4 已验证，真实双渠道折线可见，线上仍为 0.1.2、尚未部署。已清理约 11.77 GB Go 构建/旧 Codex 缓存；每批收尾清理要求见 AGENTS.md。详情见[渠道分析执行记录](CHANNEL_ANALYTICS_EXECUTION.md)。

> 2026-09-08 最新交付：渠道分页签与多图额度分析源码 `0.1.3` 已验收，尚未部署或推送；线上仍为 `0.1.2`。功能、验证、历史数据扫描边界及接续状态见[渠道分析执行记录](CHANNEL_ANALYTICS_EXECUTION.md)。恢复时保留两批未提交改动。

编制日期：2026-09-05；2026-09-06 纳入 Full/Lite/Desktop 定位、S5-P 提示词学习与版本中心、统一发行/安装/更新 P0 合同。适用源码目录：`/root/myapi`。

本文是负责人要求的新一轮完整计划，供新对话实施；编制本文件不等于下面所有设计已获批准或功能已经实现。已确认的 B2-0 合同继续有效；标为“建议”的商业、恢复和产品选择须在相应阶段落实为决策记录。本文不授权部署、发布、生产操作或处理历史真实账务。

## 1. 阅读路径、事实来源与最终目标

新执行者按以下顺序阅读：

1. 本目录及更具体的 `AGENTS.md`、存在时的 `/root/.codex/AGENTS.md`。
2. 本文第 2–6 节：现场、边界、依赖和决策。
3. 本文第 7–14 节：逐阶段任务与验收。
4. 本文第 15–19 节：并行、验证、同步、完成审计和启动顺序。
5. [总体产品计划](MYAPI_MASTER_PLAN.md)、[历史执行计划](DEVELOPMENT_EXECUTION_PLAN.md)、[完成证据](COMPLETION_AUDIT.md)，按任务读取本文引用的专题。
6. [提示词学习与版本中心](PROMPT_LEARNING.md)、[发行制品、安装与更新合同](RELEASE_MANIFEST.md)、[Lite 与 Legacy LAN 迁移](LAN_LITE.md)；按任务读取相应数据/权限/平台边界。
7. [新对话启动提示词](NEXT_SESSION_HANDOFF.md)。旧 [Mac 交接提示词](CODEX_HANDOFF_PROMPT.md) 和 [macOS 指南](DEVELOPMENT_ON_MACOS.md) 仅用于对应平台，不能作为当前 Linux 环境事实。

优先使用最新用户决定和当前代码/测试/远端结果；历史文档用于确定合同与寻找证据，不能证明当前状态。冲突必须明确列出并解决，不静默挑选有利于通过的表述。本文给出执行次序，不自动改写已批准业务合同。

### 1.1 必须达到的产品终态

| 领域 | 最终需要证明的结果 |
| --- | --- |
| 请求与协议 | 现有 Chat/Responses/Claude/Codex、SSE、附件及各 Provider 语义保持；现代异步 Task 的幂等、持久化、查询和恢复闭环可用 |
| 账务 | 用户钱包、Key、订阅、任务账务及派生统计具备明确事务边界；重复、并发和不确定提交不导致重复扣款、错误退款或丢失已确认变动 |
| 缓存 | 可消费余额权威源明确，Redis 丢失/陈旧/故障及 batch 退出均有可验证恢复协议；旧数据不被猜测性补扣或退款 |
| 策略 | Account Tier、Key Access Profile、模型范围、路由与计费归属真实执行；旧 Key 迁移可解释、可审计、可回退 |
| 运维与发行版 | Full、Lite、Desktop 的功能版/安装形态/访问模式分离；统一入口、服务器 Lite、个人电脑 Lite、Desktop、升级/切换/备份恢复和故障路径有完整证据 |
| 额度闭环 | 采样、数据质量、分析、展示、告警事件及选定通知通道形成闭环；上游余额、组织用量、用户钱包与 Key 限额严格区分 |
| 提示词学习与版本中心 | 默认关闭的授权采样、二层脱敏、调度/预算、版本/差异、人工 Codex 应用/备份/回滚在 Full/Lite/Desktop 可验证；不读取真实日志或宿主文件作为默认行为 |
| 独立产品体验 | 完整独立信息架构和视觉系统覆盖现有功能、三类用户、Full/Lite/Desktop/移动、七语言及各异常状态；官网同步 |
| 交付就绪 | 每项需求有实现、适用验证、独立审查、恢复说明和必要同步证据；外部验收如实完成，无隐藏阻断 |

“交付就绪”不包含执行生产部署、创建 tag、上传 GHCR/NPM、公开网站或发送真实通知。正式发布是单独批准的后续操作，不是本计划的隐含步骤。真实账号/设备/法律验收若是完成条件，缺失时仍标为待验收；不能用“暂缓”把原目标写成全部完成。

### 1.2 保留成果，避免重做

保留 S0/S1、S2-A/R1、B1、C03a 及已验收 C/D 批次、R4、Codex 本机导入、额度分析、局部日志/权限/品牌、CLI/Electron 和无发布镜像测试的有效成果。只有本次改动影响到它们时才补对应回归。历史绿灯不覆盖新 diff，局部面板和 Logo 不等于完整独立 UI。

## 2. 交接基线与当前执行状态

2026-09-08 最新核对：本地和远端 `main` 均为 `f6536ca`，PR #1 已合并，CI [34186651121](https://github.com/ForceMind/MyAPI/actions/runs/34186651121) 十项成功，开发分支已清理，`archive/r4-passkey-serveraddress-wip` 标签已保留。用户新增批准[多渠道分配管理](CHANNEL_ROUTING_EXECUTION.md)，该批源码候选 `0.1.2` 已验收，未部署或推送；本节其余旧分支、未同步和 Draft 描述仅保留历史依据，不是当前 Git 状态。

> 2026-09-08 部署更新：用户已授权部署，多渠道分配版本 `0.1.2` 已上线并通过健康、HTTPS 版本/资源及数据库检查；智能策略仍关闭。回滚备份和证据见 [多渠道执行记录](CHANNEL_ROUTING_EXECUTION.md)。源码尚未提交或推送。

2026-09-05 计划编制时的只读复核结果（历史基线，当前继续执行见第 2.3 节）：

- 源码：`/root/myapi`；远端：`https://github.com/ForceMind/MyAPI.git`。
- 当前分支：`codex/b2-durable-submissions`。
- 当前 HEAD：`513ea6d6e863883aa3415a785a4d858bcf7210b0`，仅为 B2-0 合同文档提交。
- 前一分支：`codex/r4-passkey-serveraddress`，本地 tip `83f24c5`；R4 功能提交 `c5cf662`、审计提交 `b77ae6b`。
- 本地 `main`/已缓存 `origin/main` 为 `af5ded0`；本轮未 fetch，不能声称远端此刻仍相同，更不能强制切回旧基线。
- 当前 B2 分支没有本地 tracking 信息；B2-0 此前推送遭自动审批拒绝，未取得同步成功证据。拒绝理由涉及可能外发内部业务文档，不是未登录 GitHub CLI。
- 历史 R4 CI `33900966323`、Docker smoke `33901685068` 的成功记录在审计文档中；本轮没有重新访问 GitHub 核验。
- 先前确实运行并通过了 `TestB2SubmissionSQLite` 的五个子场景；这不是完整 B2、三数据库或新修复后的证据。本轮计划编制没有运行新测试。

### 2.1 计划编制时的已知草稿（历史指纹）

以下九个文件来自本任务先前的 B2 实施，必须保留。`M` 表示已有跟踪文件修改，`??` 表示尚未跟踪的新文件。

| 状态 | 文件 | 交接时 SHA-256 |
| --- | --- | --- |
| M | `.github/workflows/ci.yml` | `303211eb5a0ca74270920f69ffe7f07128e8101d3bc5827ccf4bb052b28be504` |
| M | `common/constants.go` | `97d16a50a4b476b9c8a219d3d3dcff4bc767860ebdd2e589500c7647948191b8` |
| M | `common/init.go` | `bd4738dd7f7160ced3a4cec21b074a29c5289834878b9466b10af5e2d9a853b3` |
| M | `model/log.go` | `d7f8ba019e9f0600444b17ac3ecfd289c51aa64a44b54a7348d89243d965d702` |
| M | `model/main.go` | `bc28c75623369f301421496ccae7206909469b94d67db8136895a28363a9c280` |
| ?? | `common/task_recovery_gate_test.go` | `ac6f2ff6d450f32e5a4131dae9ca3935ea74e91767175c3953f06043252e8ce7` |
| ?? | `model/task_recovery.go` | `95e09cef72659d0b42844fa54a28141671379832e2049abd6032964224ad3f54` |
| ?? | `model/task_recovery_database_test.go` | `778fe01de72ded03b381ae5230415e4b3adceb89af31271334afe08a42694057` |
| ?? | `model/task_recovery_test.go` | `6401d71687e96b694772f34d3966fb5dad9f8c246dab3fc5f29098ede879ae58` |

计划编制轮另新增本文与 `docs/NEXT_SESSION_HANDOFF.md`，当时未改上述草稿、未提交、未推送。
负责人后续要求继续执行，以上历史指纹不再用于校验当前修复后的文件；不得为了匹配旧指纹回退文件。

新会话须按第 2.3 节最新状态和实际 Git 差异识别已知任务文件；这是明确交接例外，不是允许覆盖任意脏工作树。
发现清单外文件或来源不清的并发改动时先报告并停止写入，不 stash、reset、clean 或回退。不要为恢复“干净”而提交未经审查的源码。

### 2.2 初审发现（保留问题来源，不代表未修）

| 发现 | 位置 | 影响与必要修复 |
| --- | --- | --- |
| P1：幂等摘要依赖通用 `CryptoSecret` | `model/task_recovery.go` 的 `HashTaskSubmissionIdempotencyKey` | 默认秘密可随进程变化；需要稳定专用秘密及跨节点/重启一致性，配置变化必须可检测并拒绝不安全启用 |
| P1：`EventKey` 由调用者随意提供 | `TaskBillingEvent.BeforeCreate` | 同一业务动作换字符串可重复建事件；模型应生成规范业务键，重复同载荷返回原结果，冲突拒绝 |
| P2：允许 `AttemptNo > 1` | `TaskSubmissionAttempt` | v1 单次提交合同没有被数据层约束；需固定第一次及 operation 唯一性、创建状态校验 |
| P2：immutable 依赖 `Changed` | event/outbox `BeforeUpdate` | 不能证明 `Save` 等路径不会改写载荷；需 create-only 字段与受控状态更新，并覆盖多种 GORM 写法 |
| P2：outbox 与权威事件仅关联 ID | `TaskBillingLogOutbox.BeforeCreate` | 错误用户、额度、类型或越界值可能进入投影；从事件受控生成并验证共享字段及边界 |
| 待补验 | `task_recovery*_test.go`、CI | MySQL/PG 旧 logs 升级、ClickHouse 实际加列/重跑、完整 outcome_unknown 生命周期、崩溃与提交未知 |

这些是未启用草稿的静态审查结论，不是生产事故报告。新 gate 尚未接入请求路径；新增模型已列入迁移列表，因此 gate 关闭不免除迁移安全验收。

### 2.3 当前执行进展（2026-09-06）

负责人已要求继续，并再次强调限制本机资源。当前目标保持全项目范围；B2-1 已完成当前范围，B2-2A 的十个现代 Task Provider（Ali、Doubao、Gemini、Hailuo、Jimeng、Kling、Sora、Suno、Vertex、Vidu）纯解析子范围及 gate-off 非 200 兼容桥已完成本地验证和独立审查；Task 单个 outbound attempt 的 one-shot body、无自动 redirect、客户幂等头隔离与 Vertex OAuth JWT 换取无重定向子范围亦已完成本地验证和独立审查。B2-2B0 已新增严格协议、T0 operation+attempt 原子基元、Full Content 字段级幂等脱敏，以及 owner-scoped、只读、无账务/上游/POST caller 的 `GET /v1/task-operations/:id`；B1a 另有严格 JSON、B1b 另有严格 video form/multipart 的 canonical digest/请求 HMAC，均只作 gate-off 准备。durable POST、运行时正文捕获/接线、预扣/账务、dispatch、恢复和统一 202 均未接线。等待 C03b-0 的业务决定后进入 B3-A，不执行生产/发布操作。

| 工作包 | 当前状态 | 证据与剩余 |
| --- | --- | --- |
| R0 接管与调度 | 已完成当前只读核对 | 继续原功能分支，已知草稿按所有权分工；持续目标已建立；测试由主代理唯一调度 |
| B2-1 密钥/模型/迁移 | 已完成当前范围 | 2026-09-06 已补 GORM 动态表表达式与链式句柄标记泄漏保护、v1 dispatch savepoint panic 回滚；受限环境 common/model 定向测试及 model vet 通过，独立复审无 P1/P2/P3，PR #1 CI 全绿；十个现代 Task Provider 的 B2-2A 纯 parser 已完成本地子范围，B2-2B/C 与 B3 仍未开始 |
| B2-1 实库验证 | 已完成当前范围 | Draft PR #1 的隔离 CI 已实际通过 MySQL5.7、PG9.6、ClickHouse24.8 专用空库 fixture；本机未启动这些服务 |
| B2-2B0/B1a/B1b gate-off 基元与 owner 查询 | 已完成当前本地子范围，未接入 durable POST | B0 包含专用协议 HMAC、T0 owner/replay 原子基元、字段级 Full Content 脱敏和 owner-scoped `GET /v1/task-operations/:id`。B1a 提供无 caller 的严格 JSON 正文摘要/请求 HMAC；B1b 对 `video.create`/`video.remix` 提供严格 URL form/multipart canonical digest，Suno 非 JSON 仍 fail closed。两者均绑定 token、method、kind、route 和 body digest，拒绝不安全 MIME/正文且不回显。SQLite/合成定向 test、vet 与独立审查通过；MySQL 5.7/PostgreSQL 9.6 合同尚未在当前 SHA 实跑。无 durable POST、预扣、dispatch、账务、上游、恢复或统一 202 |
| GitHub 同步 | 已同步，Draft PR 待维护者处理 | `c045e42` 已推送至 `codex/b2-durable-submissions`，Draft PR #1 已创建并触发 CI；不包含合并、发布或生产操作 |
| C03b-0 | 未开始，待决定 | D02/D03 已提出：主库权威/Redis投影，以及历史无法证明余额的处理；答复前不实施不可逆业务选择 |
| 产品/发行/S5-P P0 合同 | P0 已完成；S5-P、S4、S5-Q 与 S3/C09 各有独立本地子范围 | 已新增 `PROMPT_LEARNING.md`、`RELEASE_MANIFEST.md` 并同步总体/执行/发行/验收文档；S5-P P1a/P1b 纯准入/脱敏/指纹内核、S5-Q P2A occurrence identity、Release Manifest schema-1/未受信 raw-bytes evidence/输入硬化、Legacy 安装画像解析与 S3-PRE1 detached diagnostic 均未接入真实数据、模型、文件、制品或发布；C09-N1 legal/perf/general/console/checkin/token/grok/discord/oidc 是保持旧键与业务语义的兼容快照化，未形成跨族原子或新限额行为。B2-2A 的十个 adapter parser 仅在 HTTP 200 时由 legacy `DoResponse` 调用，gate-off 非 200 由 parser 前的共享兼容 bridge 处理，但尚未接入 durable 提交。S4-D、完整 S5-P、S6、S7 仍未开始 |

`fea4637` 已提交的任务文件包括第 2.1 节九个草稿，以及新增 `common/task_recovery_key.go`、
`model/task_recovery_identity.go`、`model/task_recovery_identity_test.go`、`model/task_recovery_clickhouse_test.go`，
以及 `.env.example`、本计划、`NEXT_SESSION_HANDOFF.md`、`MYAPI_MASTER_PLAN.md`、
`DEVELOPMENT_EXECUTION_PLAN.md` 和 `COMPLETION_AUDIT.md` 的任务内更新。
旧九文件指纹仅作历史追溯，不能再当作忽略脏工作树的许可。源码内容以 `fea4637` 为准；随后纯文档审计提交按 Git 历史核对。
新对话若发现未提交修改，应停止并报告具体文件与来源，得到对这些实际修改的明确接管授权后才能继续。

已实测限制：Go 使用 `GOMAXPROCS=1`、`GOMEMLIMIT=768MiB`、`-p 1`；模型测试活跃单元
`CPUQuotaPerSecUSec=1s`、`MemoryMax=805306368`。两个实施角色不运行测试，独立审查只读；没有并发构建。
详细命令、结果和后续复验见[完成审计](COMPLETION_AUDIT.md#b2-1-追加安全修复2026-09-06已完成当前范围)。

## 3. 安全、资源和工具边界

### 3.1 始终保持的禁止事项

- 不访问或操作 `/root/new-api` 生产运行目录，不改 compose、数据库、日志、配置、容器和运行状态；不开展全机清理或安装计划。
- 不部署、不重启生产、不登录 GHCR、不发布 GHCR/NPM、不创建/移动/删除 tag、不强推、不合并受保护分支。
- 不运行 `git reset`、`git clean`、强制 checkout，不覆盖未提交工作，不自动导入真实凭据。
- 不读取本机 Codex/Claude/浏览器/Keychain 凭据来进行测试；已有 Full 原生导入能力只能由用户在隔离测试实例显式操作验收。
- 源码内的开发/CI/发行模板可以在明确任务范围内修改，但不能把源码路径与生产运行目录混为一谈。

### 3.2 本机约 2 CPU / 3.5GB 的预算

- 本机仅必要定向测试；Go 固定 `GOMAXPROCS=1`、`GOMEMLIMIT=768MiB`、`go test -p 1`，独立模块加 `GOWORK=off`。
- 使用可用的 cgroup/systemd 硬限制：`CPUQuota=100%`、`MemoryMax=768M`。`GOMEMLIMIT` 只是 Go 运行时软目标，不能冒称整个进程组硬上限。
- Node/Bun 单 worker，总进程组内存不超过 768MiB；V8 可用 `NODE_OPTIONS=--max-old-space-size=512` 留出堆外空间。Bun 不由 V8 参数限制总内存，仍需进程组限制。
- Go、前端、浏览器、Docker 构建互斥；两个 agent 不得分别启动测试。根模块全量、race、真实数据库矩阵、浏览器、桌面与 Docker 转交 GitHub runner。
- 若必要本机测试触发 OOM，保存证据并移交 CI，不自行取消限制或增加 swap/改变机器配置。
- 临时缓存通过独立任务目录记录；任务结束先确认无活跃进程再清理明确属于本任务的缓存，不清共享 cache、`node_modules` 或既有临时目录。

### 3.3 本机定向验证示例

以下是未来执行时的模板，不表示已运行；先核实工具链和目录，缓存目录每次创建后记录实际值。

```bash
TASK_CACHE=$(mktemp -d /tmp/myapi-completion.XXXXXX)
systemd-run --wait --pipe --quiet --working-directory=/root/myapi \
  -p MemoryMax=768M -p CPUQuota=100% \
  env GOMAXPROCS=1 GOMEMLIMIT=768MiB GOWORK=off \
  GOCACHE="$TASK_CACHE/go-build" GOMODCACHE="$TASK_CACHE/go-mod" \
  /root/.local/bin/go test -p 1 ./model \
  -run '^TestB2SubmissionSQLite$' -count=1 -timeout=180s -v
```

选取测试应随改动调整。用 `--wait --pipe` 收集最终退出码和输出；旧 `--scope` 运行没有收集到终态输出时不能计为通过。长任务持续使用原 session/job handle，观察超时不意味着任务停止，不得据此启动重复任务。

### 3.4 技术合同与技能

- `Router → Controller → Service → Model`；Provider adapter 处理协议，不负责数据库事务或写最终 HTTP 响应。
- 根模块业务 JSON 使用 `common/json.go`；`relaykit` 不依赖根模块，受影响时单独构建。
- SQLite、MySQL >=5.7.8、PostgreSQL >=9.6 同时支持；使用 GORM 和 `lockForUpdate(tx)`，不依赖 MySQL 8 / PG 新版本特性而无回退。
- 计费表达式变更前完整读取 `pkg/billingexpr/expr.md`；checked quota 数学、价格倍率校验和管理员饱和审计继续保留。
- 前端使用现有 React/TypeScript/Rsbuild/Bun。实施前读 `web/AGENTS.md`；UI、React、用户可见文字/i18n 任务按实际适用范围读取 `shadcn-ui`、`vercel-react-best-practices`、`i18n-translate` 技能。
- 依赖版本取当前 manifests/lockfile/CI，不照抄旧 Mac 环境。当前 `go.mod` 是 1.25.1，工具可执行路径已找到不代表最低版本兼容已经验证。
- 本文不重新确认第三方 API 当前存在性；进入 Provider 扩展时查阅最新官方资料并记录日期、协议版本和 fixture 来源。

## 4. 完整工作分解和依赖

| 阶段 | 当前状态 | 交付物 | 关键依赖 |
| --- | --- | --- | --- |
| R0 交接与当前任务表 | 已完成当前核对 | 已知草稿接管、证据索引、CI 入口、决策/外部验收登记 | 新会话仍先只读核对实时工作树 |
| B2-1 安全持久模型 | 已完成当前范围 | 定向、MySQL 5.7/PG 9.6/ClickHouse CI、独立审查及功能分支同步均已完成；不代表 B2-2/B3/C03b 已完成或 gate 已启用 | R0；既有 B2-0 合同 |
| C03b-0 共同账务合同 | 待决策 | 权威源、事务边界、历史不明余额、切换和回滚规则 | 可与 B2-1 修复并行设计 |
| B3-A 原子账务核心 | 未开始 | tx-scoped 余额应用、事件幂等、不可变快照、projection receipt | B2-1、C03b-0 |
| B2-2A Adapter 分层 | 部分完成（十个现代 Task Provider 的纯解析与单个 outbound attempt 子范围） | `TaskSubmitResponseParser`、一次性安全结果分类、十个 Provider 的 legacy 响应兼容、one-shot body、3xx 不跟随、客户幂等头隔离和 Vertex OAuth JWT 换取无重定向已完成；durable 接线及旧 controller retry/failover 接管仍待 | B2-1 接口边界稳定；可与 B3-A 并行 |
| B2-2B/C 提交与查询 | B2-2B0 protocol/T0、B1a 严格 JSON 指纹、B1b video form/multipart 指纹和 owner-scoped 只读 GET 已完成当前本地子范围；durable POST 接线未开始 | 幂等预检、单次 dispatch、稳定 202、查询扩展、受理事务 | B3-A、B2-2A；POST dispatch 必须等待 D02 |
| B3-B/C 恢复与投影 | 未开始 | poll/recovery、人工处置、outbox、查询/导出/统计去重 | B2-2、B3-A |
| C03b-1..4 全写入恢复 | 未开始 | Redis 投影、所有余额写入迁移、bridge/drain/epoch、联合验收 | C03b-0、B3-A；共享文件按所有权串行 |
| C09-N1..5 配置/限额收口 | N1 已完成当前只读细分；22/24 注册族已有受控快照/复制语义，新增 `gemini`、`billing_setting`、`payment_setting` 与 `global` 的不可变 generation 子范围均已完成；`performance_setting` 与 `channel_affinity_setting` 仍分别受 D14/D15 阻塞。N2a PaymentRuntime 与 N5a 历史配置只读诊断已完成各自本地子范围，接线与其余实现待开始 | PaymentRuntime、typed bulk、2 个受决策阻塞的通用热读族、成功硬限额、历史诊断/受控修复 | N2/N3 与 S3 接口共同冻结；N4 还依赖 D05、D02/C03b 与 B3 生命周期 |
| S3 账户/Key 强制策略 | S3-0 盘点与 S3-PRE1 纯预检内核已完成当前本地子范围；强制策略未开始 | 策略合同、审计模式、缓存/路由、迁移、强制执行 | PRE1 无 caller、永不强制；真正策略依赖 D04、账务和相关配置合同稳定 |
| S4 运行、发行与恢复 | 部分历史范围完成；S4-D0、schema-1 结构选择/输入硬化、raw-bytes evidence、D2A 纯安装状态和 Legacy 画像解析子范围完成 | 三库完整运行、受信 Release Manifest、安装器/更新器/形态切换、Full/Lite/Desktop、真实设备 | `legacy-installation-profile` 仅把显式旧配置 fail-closed 映射为 Full/Lite 与 local/LAN/needs_manual，永不推断 public；selector/state 内核均拒绝 hostile JS 输入，仍未接入 CLI 或文件系统；D11 前不形成信任/安装能力；最终验收用整合后的版本 |
| S5-Q 额度闭环 | 分析层与 P2A occurrence identity 已有，持久闭环未完 | 真实采样、持久告警投递、身份模型及选定扩展 | P2A 仅纯、fail-closed 的事件 identity，无 DB/outbox/worker/notifier；外发和扩展先决策 |
| S5-P 提示词学习与版本中心 | P0 合同与 P1a/P1b 纯内核子范围已完成；完整 P1 未实现 | 授权样本、脱敏、调度/预算、版本、差异、受限 Codex 应用/回滚 | P1a/P1b 不读取或保存实际数据；P1 可做合成数据，付费调用依赖 B3/C03b，文件适配依赖 S4 安装管理桥 |
| S6 完整独立 UI/官网 | 未开始 | IA、设计系统、安装/更新/提示词中心、Full/Lite/Desktop、响应式/七语言/实机 | 设计研究可先行；实施按稳定 API/权限分批进入 |
| S7 全项目交付审计 | 未开始 | 逐需求证据矩阵、候选制品、Release Manifest、恢复手册、交接 | 所有适用阶段及外部验收完成；正式发布不执行 |

关键顺序是“共享账务契约和事务核心先于完整提交集成”，不是把 C03b 所有运维工作都塞到 B2 之前。B2 决定何时提交上游，B3 决定业务事件和账务状态，C03b 负责余额应用及缓存/统计投影；三者共同使用一个主库事务内核，不能各造一套权威账本。

## 5. 决策登记：集中处理真正改变产品或历史数据的选择

状态只能是 `已确认`、`建议待决定`、`外部条件待提供`。建议值不是现有事实；未经决定不启用依赖它的新行为，但继续推进独立工作。

| ID | 需要决定的合同 | 建议与代价 | 必须在哪一步前解决 |
| --- | --- | --- | --- |
| D01 | B2 未知态、重发、幂等及日志合同 | 已确认，见第 6 节，继续沿用 | 已满足 |
| D02 | 可消费余额权威源与故障策略 | 建议新 epoch 以主库余额行加同事务 immutable mutation receipt 作唯一实时授权，`TaskBillingEvent` 保持 Task 业务事件权威；Redis/进程内存/LOG_DB/ClickHouse/统计仅可重建投影。稳定业务键+完整载荷指纹+固定锁序+余额 version 决定 exact replay/冲突；提交 unknown 用新连接按 receipt 回读，仍未知不重复 delta。主库不可用、epoch 未知或 reconciliation 中拒绝新付费预扣；进程 batch 不再承载可消费余额。代价是增加主库事务压力，需性能实测 | B3-A / C03b 实现前 |
| D03 | 旧 batch 已丢增量、混合版本、回滚 | 建议维护屏障先停新计费，所有登记节点停止旧 writer/poller、持久 drain 并报告一致版本/epoch；冻结证据、对账后才激活。只有可证明主体、稳定来源、金额、因果和已应用状态的记录可生成迁移 receipt；无法证明的旧 batch/Redis/Task/日志差异进入 `manual_review`，自动 delta 为零且禁止推测补扣/退款。接受旧 DB 基线/可信快照导入需唯一命令、操作者、原因、证据 hash、期望余额 version 和原子 receipt；旧节点或未决 drain 不得 READY | bridge 和恢复行为实现前 |
| D04 | Account Tier/Profile 权益、模型交集、disabled/fallback、计费 | 建议 A1+B1+C1+D1：enforce 范围由稳定 Tier/Profile 权威、legacy group 仅缺字段映射；Tier/Token/Profile/route-channel 能力取交集，`inherit` 不加限制、显式空 allowlist 为 deny-all；unknown/disabled 仅阻止新 dispatch，已有 operation 按原快照恢复；最多一次 fallback、仅首次 dispatch 前、不扩大权益，按最终 route group 冻结计费；off→audit→scoped enforce | S3 策略实现前 |
| D05 | 并发“成功请求硬限额”语义 | 建议成功数＋在途 reservation 不超过上限，失败释放，提交未知保留待核实；明确时间窗、租约、崩溃和 Redis 故障取舍，不偷偷把现有近似限流改成另一种产品 | C09-N4 前 |
| D06 | 额度通知与选定扩展 | 推荐先一个经批准的通知通道；收件人、凭据、真实发送另授权。明确多 Key 身份、Claude 组织用量、Antigravity public relay、TokenHub 扩展各自是否为本次最终交付必选 | S5 对应工作包前 |
| D07 | 恢复支持范围 | 确定最低旧版本、Full/Lite/Desktop × 数据库/架构/安装形态矩阵、维护窗口、备份范围、允许数据损失 RPO 与恢复时间 RTO；不能靠仅“健康检查成功”验收 | S4 旧版 fixture 与最终演练前 |
| D08 | UI 方向与完整功能清单 | 建议以角色工作流组织，沿用现有技术栈；审核 IA、关键页面及迁移映射后成批实施 | S6 页面实施前 |
| D09 | 外部账号、设备、法律、签名与发布 | 指定实际验收人/设备/隔离账号；合规结论交给负责人或合格审阅者。版本、域名、tag、发布是独立决定 | 相关外部验收前；不阻断其他代码任务 |
| D10 | Full/Lite/Desktop 兼容与发布坐标 | 建议保留 `@forcemind/myapi`/`myapi`；Legacy `myapi-lan` 先作 Lite 兼容 identity，功能版/安装形态/访问模式分离，旧 LAN 不自动公网。Lite OCI 长期坐标、字段名与弃用窗口待确认 | S4-D1 前 |
| D11 | 发行信任、原生目录和恢复目标 | 待负责人决定；当前材料推荐最小组合 T1+P1+R1：GitHub OIDC/Sigstore keyless 对原始 manifest bytes/bundle 严格绑定仓库/workflow/tag identity，Linux system/user 分离的受管目录，单节点升级前一致性备份 RPO=0、已启用外部日备目标 RPO≤24h/RTO≤4h（随数据量/机器实测）。仍须决定权威资产、信任根、签名/公证门槛、路径、最低可升级版本及数据库 downgrade 范围 | S4-D2/D3 前 |
| D12 | Desktop 与自动更新策略 | 决定 Desktop 首发 OS/arch（Linux 需 CI/制品/实机证据）、后台服务模型、检查/下载/安装默认关闭及可接受更新范围/维护窗口 | S4-D4、S6 实施前 |
| D13 | S5-P 隐私、范围与付费调用 | 决定协作/项目/Key scope、正文/审阅资料加密、历史样本导入范围、允许渠道/模型/预算和真实验收账户；建议只出草稿、人工确认后应用 | S5-P P1/P2/P4 前 |
| D14 | 性能配置热变更与全链路一致性 | 决定运行中 `DiskCachePath` 的生命周期（建议先禁止无维护窗口的热切换，选择显式 drain/重启、并存多代目录或受控迁移之一），以及八个性能字段是否要求跨 disk/monitor 消费者同一原子 runtime generation。不得只替换 source 指针后声称文件路径、统计和两类投影已经原子切换 | `performance_setting` 实现前 |
| D15 | 渠道亲和配置的 cache 生命周期 | `channel_affinity_setting` 的规则会影响渠道、重试、上游参数和账务归属，而 capacity/TTL 目前仅首次初始化 HybridCache 时生效。建议规则只以新 generation 影响后续决策，capacity/TTL 明确为重启或受控 drain/rebuild/epoch 操作；禁止普通配置写入静默清缓存、迁移 Redis 或改变 retry。需决定是否支持热重建及其维护/恢复合同 | `channel_affinity_setting` 实现前 |

不要一次询问几十个机械实现细节。先解决 D02/D03 这组近期硬依赖，再按阶段处理 D04/D05、D06、D07/D08。没有回复不等于批准；暂缓任务必须留在总表，不能从完成分母中删除。

## 6. 已确认的 B2-0 合同与补充工程原则

### 6.1 已确认，不重复改变

- `submission_unknown` 表示可能已提交但未得到可信受理结果；`outcome_unknown` 表示已受理后的执行结果不明。两者均不自动重发、不自动退款。
- 只有 provider 可验证结果或带审计的人工处置能结束未知态；不能用超时、缓存消失或状态字段单独证明应退款。
- 进入 `DISPATCHING` 后 v1 禁止可能已送达请求的重试和跨渠道 failover，包括 Provider 内重试。
- 幂等范围 `token + HTTP method + operation kind`；同 key/同指纹复用，同 key/异指纹 `409`；活动记录不自动过期，终态保留 180 天。
- 先持久化 operation，随后统一边界返回 `202` 和稳定公开 ID；每个状态可查询。adapter 不先写响应。
- 主库账务事件是 Task 账务唯一权威；分库/ClickHouse 日志为携带 `billing_event_id` 的至少一次投影，用户查询/导出/统计去重。
- 第一批仅现代 Task；Midjourney 不混入首次改造，后续整体完成审计须单列评估其恢复需求。
- C03b 完成前 gate 关闭；启用前排空并升级全部旧 writer/poller；不承诺旧新 worker 混跑。生产启用不在本计划执行范围。

### 6.2 落地时必须写成可测合同

至少冻结 operation kind 枚举、幂等 header 规范、JSON/form/multipart 指纹规则、公开查询 DTO、错误码、状态转换、事件去重公式、额度正负号、日志事件映射、lease/CAS、审计权限和保留策略。

跨进程或跨节点幂等必须在稳定密钥下成立；v1 可不支持在线密钥轮换，但必须能检测错误更换并 fail-closed，不能只加一个环境变量就算修完。主库需有可核验的协议/密钥标识绑定，保存公开标识或验证值而非明文秘密。

账务事件必须引用稳定业务主体：同一个任务在“operation 引用”和“正式 Task 引用”两种表达下仍然只有一个业务事件。payload 相同才可返回旧结果，冲突进入可诊断拒绝/人工审查。人工调整若允许多次，使用独立且稳定的审计命令 ID。

## 7. B2/B3/C03b 详细实施

### B2-1：修复持久模型与迁移基础

**所有权：** `common/constants.go`、`common/init.go`、`common/task_recovery_gate_test.go`、`model/task_recovery.go`、`model/main.go`、`model/log.go` 及模型测试由一个核心负责人修改；CI 文件另一个负责人修改，不争写。

**任务：**

1. 完成第 2.2 节五项修复，避免扩大为无关重构。
2. 把 create-only 身份/载荷与可变状态分开；状态仅通过带预期状态/版本的 CAS 修改。增加规范 create-or-load，跨数据库冲突后能安全回读，特别避免 PG 已中止事务继续读。
3. operation 的 user/token/public ID 归属、唯一 Task 关联和不可过期未知态严格校验；attempt v1 只允许一次且创建时 operation 状态合法。
4. event/outbox 校验全部有界标识、额度/符号、关联主体与共享投影字段；拒绝损坏的权威数据，不静默 clamp 成另一笔合法账目。计算阶段仍使用既有 checked quota 转换和审计。
5. 验证常规/快速主库迁移、独立 LOG_DB 和 ClickHouse 加列；新建、旧表、有历史行、重复启动均可用。保留旧空 `billing_event_id` 行，不能把所有旧日志当成一个重复事件。
6. 对门禁明确“请求创建”“恢复既有义务”“schema 兼容”三种能力；最终关闭新提交开关不得把已产生账务义务永远停住。B2-1 只提供基础，不声称启用门禁完成。

**退出条件：** 五项发现有精确回归；SQLite 定向、MySQL 5.7/PG9.6 实库及 ClickHouse 对应迁移场景通过；独立审查无阻断；默认不启用；文档、源码和必要同步一致。ClickHouse/实库未执行时维持待验证。

### C03b-0 与 B3-A：统一权威源和原子账务核心

**内核实施先完成 D02；历史切换先完成 D03。** 推荐主库为可消费钱包/Token/订阅余额的唯一授权源，Redis 为带 epoch/version 的快照投影，进程内 batch 不再承载已确认的可消费余额变化。
新事件 receipt 与 gate-off 事务内核不依赖选择如何处理已经丢失的历史增量；D03 仍阻断旧基线、epoch 激活、隔离/损失处置和真实 writer 切换，不能用内核完成替代该决定。

**拟修改范围：** 新增 model 事务应用代码，复用现有 User/Token/Subscription/TaskBillingContext；`model/quota_reserve.go`、`service/billing_session.go`、`service/funding_source.go` 的接入按所有权拆批。具体新增文件名由实现者确认，不强制按草稿堆表。

**必须明确的主库事务：**

| 事务 | 原子内容 | 失败不变量 |
| --- | --- | --- |
| T0 intent | create-or-load operation＋唯一 attempt；冲突指纹比较 | 并发只有一个所有者可以继续 |
| T1 reserve | 校验主体及价格快照、reserve event、钱包/订阅＋Token 预扣、余额版本/receipt、必要投影、operation RESERVED | 任一失败全部回滚，不留半次扣减 |
| T2 dispatch | operation 和 attempt 一起转 DISPATCHING，记录时间/版本 | 未确认提交不得发网络请求；提交结果未知先查询持久状态 |
| T3 outcome | accepted 的正式 Task＋关联＋调整事件，或 rejected 的退款事件；operation/attempt 状态和所有账务/投影同事务 | 正式 Task 不孤立，退款不重复，CAS loser 全事务回滚 |
| T4 terminal | Task 终态 CAS＋结算/退款业务事件＋余额/订阅/Token 更新＋日志/缓存/统计 outbox | 终态和财务结果不能分裂，重放只应用一次 |

网络 I/O 永远在这些事务之外。API 按显式 `tx` 工作，不能从事务内部调用使用全局 DB、异步 Redis 或进程 batch 的高层 helper。

若需要 `quota_mutations`，它是账务事件应用的幂等 receipt，不是另一套 Task 权威账本。固定锁序、行不存在处理、并发冲突、提交未知后的按键回读和余额上下界均需测试。充值/兑换等非 Task 业务使用自己的稳定业务键；禁止任意客户端字符串成为可重复授信授权。

**退出条件：** 用 SQLite 与两种最低支持数据库证明钱包/Key/订阅原子预扣、重复同载荷一次应用、异载荷拒绝、回滚、余额边界和多连接竞争；冻结供 B2/S3 调用的接口。基于合成负载记录事务延迟/吞吐基线，不编造性能提升。

2026-09-05 补充只读入口核对（不是新实现或验收）：

| 写入边界 | 当前风险与迁移归属 |
| --- | --- |
| `model/quota_reserve.go`、`user.go`、`token.go`、`utils.go`、`user_cache.go`、`token_cache.go` | Redis、DB 与进程 batch 存在分离写入/失败回退；C03b 必须统一版本 receipt，不能只修 Task 的新表 |
| `service/billing_session.go`、`funding_source.go`、`quota.go` | 资金源与 Token 顺序提交、异步退款，不能在 T1/T4 内直接复用这些全局副作用 helper |
| `relay/relay_task.go`、`service/task_billing.go`、`task_polling.go`、`model/task.go` | 当前提交/终态 CAS 与账务/日志/Task Insert 分离；按 T1/T3/T4 整合，不能只移动成功响应 |
| `model/topup.go`、`redemption.go`、`checkin.go`、`subscription.go`，管理员和 Key 编辑 | 局部已有事务，但完整缓存 receipt 和绝对余额覆盖冲突仍需按非 Task 写入包收口 |

紧随 B2-1 的最小包为 **B3-A1：gate-off Task T1 reserve＋receipt**（D02 批准后）：单写
`model/quota_mutation*.go`、相关 Task 模型与共享迁移，不接 controller/service；主库新连接按稳定业务键回查未知提交。
同键同指纹返回既有 receipt，异指纹冲突；记录 before/after/version 与完整缓存投影快照，事务内无网络/Redis/LOG_DB。
“回查仍不可读”继续 unknown，不能凭回查失败退款。此包需钱包/订阅＋Token、回滚、重放/冲突、额度界限、多连接竞争、
提交未知、旧表和重复迁移的 SQLite/MySQL5.7/PG9.6 证据，race 只在 runner 执行。

**2026-09-06 B3-A/C03b-0 可执行性盘点（只读，仍待 D02/D03）：** 现有 `TaskBillingEvent`/`TaskBillingLogOutbox` 已具备主库 event key、不可变载荷、lease/CAS 与至少一次日志投影模型，但尚无生产 writer；B2 T0 也只落 operation/attempt，不创建余额事件。旧 `BillingSession`、`WalletFunding`、`PreConsumeTokenQuota` 与 `Task` 账务会先改 Redis/进程 batch 或将 User、Token、Task、日志拆开写入，故 B3 writer 明确不得调用它们。若 D02 采用建议值，T1 必须在一个主库事务中按固定锁序锁定 User→Token→获批的 Subscription→operation/attempt→Task，以稳定 event key create-or-load reserve event，再同步完成余额边界检查/条件更新、Task 绑定、outbox 与 event applied；MySQL/PostgreSQL 使用 `lockForUpdate`，SQLite 以单写事务与有限 busy/conflict 重试保证等价保护。主库提交后 Redis 仅做可重建失效/投影，投影失败不得补偿或改变余额；提交结果未知按 event key 回读，不能再次 delta。

对 D03，旧 Task/log/batch/Redis 记录若不能同时证明 owner、来源、预扣/结算因果和余额影响，不得伪造正式 B2 event 或猜测 delta；应写入独立 reconciliation case 并保持 `manual_review`，零余额变更。旧 writer/poller 排空、冻结后才允许 proof 扫描和 epoch 激活；人工处置必须使用有操作人、原因、证据引用和不可变命令 ID 的单独审计命令。该盘点未修改模型、迁移、余额、Redis 或任何运行路径。

### B2-2A：Adapter 纯解析与一次性请求

**文件：** `relay/channel/adapter.go`、`relay/channel/api_request.go`、`relay/relay_task.go`、十个现代 Task adapter 及各自测试。

1. 先落稳纯 parser 输入/输出接口：accepted/rejected/unknown、可信 upstream ID、受控 Task 数据和 legacy 响应 DTO；parser 不接收可写 Gin writer。
2. 分组迁移 Ali/Doubao/Gemini/Hailuo/Jimeng 与 Kling/Sora/Suno/Vertex/Vidu；保留各 Provider 响应形状及 gate-off 路径。
3. 所有 header/HTTP 成功响应由 controller 最终边界发送；不能在 Task/event 落库前发成功。
4. 一次性 body、禁止 redirect、禁止凭据与客户幂等键意外透传；检查 `GetBody`、Transport/SDK 重试等隐含重发。
5. 使用 fake upstream 对每个 Provider 验证有效受理、明确拒绝、malformed、空 ID、错误正文脱敏和 body 关闭。

本步骤可以独立验收“parser 分层及兼容性”，不能因此标记完整 durable submission 已实现。

**2026-09-06 十个 Provider 纯解析子范围证据：** `TaskSubmitResponseParser` 只接收已读的不可变数据，既不接 Gin writer，
也不读写数据库、账务、网络或时钟。Ali 仅在 HTTP 200、顶层完全没有 `code` 且有严格安全 Task ID 时 accepted；
Doubao 仅在 HTTP 200、有严格顶层 `id` 且没有 `error`/`code` 冲突时 accepted；Gemini 仅在 HTTP 200、有严格
`models/.../operations/...` 名称且没有 `error` 时 accepted；Hailuo 仅在 HTTP 200、`base_resp.status_code=0` 和严格
Task ID 时 accepted；Jimeng 仅在 HTTP 200、`code=10000` 和严格 `data.task_id` 时 accepted；Kling 仅在 HTTP 200、
`code=0`、规范 `data.task_id` 存在且所有已知 Task ID 位置不冲突时 accepted。Sora 仅在 HTTP 200、唯一严格 `id`/
`task_id`、可受理状态且顶层 `error` 为 null/缺失时 accepted，并只保留已验证的本地 remix 来源公开 ID；Suno 仅在
HTTP 200、`code=success` 和严格 `data` 时 accepted，legacy `message` 使用固定安全值；Vertex 仅在 HTTP 200、严格完整
operation `name` 且没有 `error` 时 accepted；Vidu 仅在 HTTP 200、严格 `task_id`、`state=created` 且没有冲突 `error`/
`err_code` 时 accepted。所有 accepted 的 `TaskData` 都是 legacy 转换所需的最小 JSON，Provider message、prompt、媒体 URL、
上游 remix ID 和未知扩展均不保留；fetch ID 经过有界 token 校验，并在 path/query 使用前转义。

尚未得到 Provider 明确拒绝码 allowlist 的所有非成功码、非 200、空/异常 ID、读/UTF-8/JSON 不明和不安全 body
一律保守为 unknown，只保留受控 reference。gate-off 的实际 `relay/relay_task.go` 在所有非 200 时都先进入共享
兼容桥：最多读取 1 MiB + 1 字节、关闭 body、固定脱敏错误消息，并保持旧的 `fail_to_fetch_task` code/status/retry
语义；它不把非 200 分流给 parser。HTTP 200 的 Provider unknown 仍会走 legacy `TaskError`/retry 语义，尚未获得
durable unknown 的禁止重发/退款保护。

在 `CPUQuota=100%`、`MemoryMax=768M`、`MemorySwapMax=768M`、`GOMAXPROCS=1`、`GOMEMLIMIT=768MiB` 的受限服务
`myapi-b2-all-final-pass` 中，`go test -p 1 -count=1 -timeout=360s -v ./relay ./relay/channel/task/ali ./relay/channel/task/doubao ./relay/channel/task/gemini ./relay/channel/task/hailuo ./relay/channel/task/jimeng ./relay/channel/task/kling ./relay/channel/task/sora ./relay/channel/task/suno ./relay/channel/task/vertex ./relay/channel/task/vidu`
exit 0；十一个包依次为 relay 0.032s、Ali 0.026s、Doubao 0.019s、Gemini 0.020s、Hailuo 0.035s、Jimeng 0.026s、Kling
0.076s、Sora 0.020s、Suno 0.020s、Vertex 0.031s、Vidu 0.024s。两位未参与实现的独立审查者在修复 null/冲突 ID、可空嵌入字段、
错误证据、legacy 映射和测试断言后确认无 P1/P2；共享 dispatcher 已有直接测试，证明已迁移 parser 不会处理 gate-off 非 200。

**2026-09-06 one-shot 出站与凭据子范围证据：** 十个当前 Task adaptor 的 `DoRequest` 都经
`buildTaskSubmissionRequest`。该边界在 header 构造后保留正确 `ContentLength`，但清除 `GetBody`，并删除
`Idempotency-Key` / `X-Idempotency-Key` 的所有大小写映射；因此 HTTP/2 在正文送达后遇 `REFUSED_STREAM` 时不能用
`GetBody` 隐式重发。Task 请求复用既有浅复制 client 的 `http.ErrUseLastResponse` 策略，301/302/303/307/308 只返回
上游 3xx，绝不跟随 `Location`。当前十个 adaptor 不复制整组客户端 header；测试也确认 client `Authorization`、Cookie、
`X-Api-Key` 未自然进入上游，而 Provider 自建 `Authorization` 保留。普通同步 relay 的 replay 行为没有改变。

Vertex 的两条 JWT access-token exchange 路径也各自浅复制缓存 client，并仅在副本设置
`http.ErrUseLastResponse`；未改变共享 transport、proxy、timeout 或 redirect callback。受限服务
`myapi-b2-vertex-test-pass` 在显式 `/root/myapi` 工作目录、`CPUQuota=100%`、`MemoryMax=768M`、
`MemorySwapMax=768M`、`GOMAXPROCS=1`、`GOMEMLIMIT=768MiB` 下执行
`go test -p 1 -count=1 -timeout=180s -v ./relay/channel ./relay/channel/vertex`，退出状态 0；包结果为
`relay/channel 0.029s`、`relay/channel/vertex 0.021s`。合成 HTTP/2 `REFUSED_STREAM` 证明 Task 仅收到一条完整 body；
两组五类 3xx 测试证明 Task target 为 0，OAuth source 为 1/target 为 0，且 JWT form 的 POST、content type、grant 和
assertion 完整。独立审查先发现 OAuth 测试 goroutine/契约两项 P2，已改为 `atomic.Int32` 与结果 channel、补精确 form
断言后复核无新增 P1/P2。

这仍未改变成功响应的 legacy `c.Data` 写入时序、T0–T4 持久状态、幂等预检、主库账务、统一 202/查询、
poll/recovery/outbox 或 gate。尤其 `controller.RelayTask` 的旧 retry/failover 循环仍可能针对 307、429、5xx 或未知结果
创建新的 legacy attempt；只有 B2-2B/C durable dispatcher 接管后才可移除/绕开它，不能把本子范围写成完整单次 durable dispatch。

### B2-2B/C：幂等预检、单次提交和查询

**文件：** controller/relay、relay_task、相关 router、新 middleware/service/DTO；公共接口先冻结后分派。

1. 请求在身份验证和必要兼容正文转换后计算规范指纹；保留现有正文生命周期、附件验证和 Full Content 脱敏。原始 key 和规范正文不持久化、不输出日志。
2. 固定 kind 建议使用 `video.create`、`video.remix`、`suno.music`、`suno.lyrics`；最终枚举随实际路由核实。JSON 对象顺序/空白、数字词法、重复字段、表单和 multipart 文件摘要的规则写入接口文档并精确测试。
3. 有效重放不重新路由、不重复预扣或触发上游；仍检查当前凭据是否可认证和资源访问权限。撤销/删除 Key 的查询和重放行为不能绕过鉴权。
4. 完成本地验证及请求构造后进入 T0/T1/T2；durable 分支完全绕开旧 retry/failover 循环及 `defer BillingSession.Refund`。
5. 上游返回可信 accepted/rejected 才进入对应 T3；网络/读取/解析/提交结果不明进入 unknown。不能把任意 HTTP 4xx 或 2xx 一律当作可退款/成功，按 Provider 可证明语义分类，不能证明就 unknown。
6. 用有界、可脱离客户端取消的持久化 context 保存结果。提交错误必须回读；数据库始终不可用时，只返回已确认持久的 ID/状态或明确临时不可读，不能虚构已落库 outcome。
7. `POST` 使用统一 `202`，含稳定 ID、精确状态、`Location` 与 `Cache-Control: no-store`；canonical 查询建议 `/v1/task-operations/:id`，按 owner 隔离。`Task.TaskID` 复用 operation 公开 ID。
8. 旧 video/Suno fetch 先查正式 Task，缺失时查询 operation 并保持兼容 envelope；只读查询不因余额恰好耗尽而无法获得结果，但仍执行身份/权限校验。

**退出条件：** 同 key 并发只一次 network call/一次预扣；异指纹 409 无副作用；从 T0 到 HTTP write 每个崩溃点都能查到真实状态；unknown 无退款/重发；十个 adapter 及旧客户端合同通过；跨数据库及 race 实测通过。

**2026-09-06 B2-2B0 接线核验与本地 gate-off 基元（不是 durable POST 运行时实现）：** 当前 replay 在 `Distribute` 之后才可能看到选中的渠道，而 `controller.RelayTask` 已先安装 legacy refund defer 并进入 retry/failover；`RelayTaskSubmit` 还混合预扣、网络、adapter 写响应和后续调价。因此不得用一个 gate 包裹旧函数或只改 `shouldRetryTaskRelay`。冻结的最小目标切片包括严格单值 `Idempotency-Key`、JSON/form/multipart 的独立规范正文合同、T0 `operation+attempt+Owner` 单事务、owner-scoped no-store `GET /v1/task-operations/:id` DTO，以及 Full Content 对该 header 的字段级脱敏；当前已实现严格协议（1–255 可见 ASCII、专用 HMAC、成功后清除 canonical 原文）、T0、GET、字段级脱敏、B1a 严格 JSON 指纹和 B1b video form/multipart 指纹。

当前已落地的 protocol/T0/字段脱敏/B1a 基元仍无 POST caller：`CreateOrLoadTaskSubmissionIntent` 对新 operation 与唯一 v1 attempt 使用受检查的 savepoint；在 GORM `PrepareStmt` 外层事务中只为事务控制 SQL 临时解包底层事务，不改变调用方的 prepared pool。另有唯一 `GET /v1/task-operations/:id`：dashboard session 以 user scope 查询；API token 直接主库 JOIN token/user，允许 enabled/expired/exhausted、拒绝 disabled/deleted/unknown/nonpositive identity，并保留用户状态/IP allowlist；同一 SQL 总是含 `public_id + user_id`，token 请求再含 `token_id`，不存在/跨 user/同 user 跨 token 统一 404。DTO 固定 `object: task_operation`，只投影 id/kind/status/四个时间字段，坏公开状态投影返回 unavailable；200/401/403/404/500 均 no-store。B1a 只接受四个 frozen kind 的 1 MiB JSON object、可选单一 `charset=utf-8`，按 decoded UTF-8 key、原始数字词法、array 顺序和 route/origin binding 求 digest，并以专用 HMAC 绑定 token/method/kind/content family；BOM、非法 UTF-8、重复 decoded key、孤立 surrogate、尾随根值及 RFC2231/未知/重复 MIME 参数均拒绝且不回显正文。它没有 POST router/controller、预扣、dispatch、上游网络、统一 202 或账务/恢复查询。SQLite/合成定向测试与 common/model/service vet 通过，独立复审无代码 P1/P2/P3；MySQL 5.7/PostgreSQL 9.6 query contract 已挂入现有 B2 disposable CI fixture，尚待该实际 SHA 执行，race 与 CI 亦待。Full Content 不会因任意/空/别名幂等 header 隐藏整段请求或响应，也不保存原始 key 副本。

**B1b 已完成当前纯 contract 子范围（不是 durable POST 运行时实现）：** `FingerprintTaskSubmissionFormRequest` 与 `FingerprintTaskSubmissionMultipartRequest` 只接收有界 `[]byte`，只允许 `video.create`/`video.remix`；Suno 的非 JSON header/body 合同仍未冻结，因此明确拒绝。URL form 仅接受 `application/x-www-form-urlencoded`（可选单一 `charset=utf-8`），以 UTF-8、至多 1 MiB/128 个唯一字段解析并规范化 key 顺序、空值和 `+`/percent 解码；重复/空 key、错误 charset/参数、畸形转义和越界全部拒绝。multipart 仅接受单一 boundary 的 `multipart/form-data`，以至多 1 MiB/64 个唯一 part、受限 header、字段 UTF-8/文本 MIME、文件名/文件 MIME/原始 bytes digest/大小构造与随机 wire boundary 和 part 顺序无关的 material；未知 header、重复 name、空 filename 歧义、路径文件名、非 UTF-8 字段和二义性均拒绝。二者都复用专用 HMAC，并绑定 token、method、kind、route 和 canonical body digest；没有 HTTP request/`BodyStorage`、router/controller POST、预扣、dispatch、账务、上游或旧 retry/failover caller。单核/768MiB 定向 Go test、`go vet ./service ./service/promptlearning` 与未参与实现者独立审查通过；MySQL 5.7/PostgreSQL 9.6、CI/race 和运行时正文捕获/接线仍待。未来接线路由前仍须在 Kling/Jimeng 改写 body/path 前以只读 `BodyStorage` 捕获请求，不能从改写后的路径、上游 ID 或随机 multipart wire bytes 推断 operation。

不发送上游、不扣款、不启用新 POST。真正 `POST` 必须在 `Distribute` 前预检，且仅在 D02 批准、B3-A 提供 T1/T3 原子账务后分叉旧循环。C03b 前新 gate 始终关闭；已有 durable obligation 不能在关闭时回落 legacy。poller/realtime fetch/sweeper 也须在 B3-B 隔离，不能把正式 Task 的旧退款规则用于 operation。该设计和本地基元均不构成 B2-2B/C 交付。

### B3-B：恢复、轮询、未知处置

**文件：** `service/task_billing.go`、既有 Task poller/system task、model CAS/lease、新恢复服务和管理员受控接口。

- 恢复器扫描 PREPARED/RESERVED/陈旧 DISPATCHING、accepted/outcome_unknown、待应用 event/outbox；有限批量、lease、重试退避、worker 并发上限与异常队列。
- PREPARED/RESERVED 只有证明未 dispatch 且符合合同才可取消/释放；陈旧 DISPATCHING 不能自动重发或退款。
- accepted 任务使用提交时 B1 计费快照；Provider 轮询超时不等于执行失败，转 outcome_unknown 的规则明确。
- 最终状态与账务用 T4；重复轮询、租约过期接管、双 worker 和崩溃都不得重复记账。
- 人工处置要求独立权限、强认证/复核、不可变命令 ID、操作人/原因/证据引用、旧新状态与额度、预期版本；重放返回原结果，冲突拒绝。不得保存原始敏感 Provider 正文作为随意可读“证据”。
- owner 查看净化的原因/状态；管理员审计权限控制账务细节。待处置数量、最老年龄、重试、租约回收等可观测，告警不泄露 prompt/secret。

**退出条件：** submission_unknown 与 outcome_unknown 全生命周期；人工重复/冲突/越权，accepted 后异常、重启接管、停止/恢复 worker 均有可观察回归。

### B3-C：日志和统计的至少一次投影

- 日志 outbox 从已确认的权威事件产生，写失败保留重试；LOG_DB/ClickHouse 不可用不回滚已提交主账本。
- 相同 `billing_event_id` 始终对应相同不可变投影；选择确定性的去重方式，去重在分页、求和和导出之前完成。
- 无 event ID 的历史日志按旧行身份保留；多个财务事件不等于多次请求。reserve、adjustment、terminal、refund、manual 的日志类型、净消费、RPM/TPM 贡献分别定义。
- 覆盖日志列表、详情、统计 API、额度报表、CSV/JSON 导出、缓存；普通用户过滤 `admin_info`，饱和审计沿用既有字段路径。
- outbox backlog、最老延迟、失败分类和不可重试冲突具备运维入口；worker 关闭/重启不会丢投递义务。

**退出条件：** 主库、独立日志库、ClickHouse 重复投递和中断恢复后，对用户呈现的账务结果相同；最终金额和请求/用量统计有精确断言，不只检查日志“存在”。

### C03b-1..4：缓存、全写入迁移和切换演练

**建议数据职责：** 全局协议 epoch/state；各余额主体版本；稳定 mutation receipt；Redis/统计 outbox；必要的临时 legacy drain receipt。优先复用可证明等价的现有结构，不强制新增五张表。

| 批次 | 实施内容 | 验收重点 |
| --- | --- | --- |
| C03b-1 余额投影 | Redis 保存主库完整余额快照和 epoch/version，拒绝版本回退；缺 cache 仅在持久状态可证明 READY 时安全水合 | TTL/DEL/重启、stale/ahead/malformed、旧 epoch、DB commit 后 Redis 失败 |
| C03b-2 全写入口迁移 | 同步 BillingSession、Task、充值/支付、兑换、签到、订阅购买、管理员余额调整、Token 编辑及统计写入逐一接新事务/receipt | 每个授权入口的重复、回滚、提交未知、余额上限；没有剩余裸 delta 绕过 |
| C03b-3 bridge/drain | 计费准入暂停、在途请求收敛、存活 batch 持久化/一次应用、节点能力报告和 durable epoch 激活 | SIGTERM drain、SIGKILL 仅恢复持久部分、节点缺失/重名、老 worker 拒绝、drain commit unknown |
| C03b-4 联合恢复 | B2/B3、Redis、batch、配置及完整应用组合；旧任务分类迁移；开关关闭后已有账务仍清偿 | 所有组合下 no duplicate/no lost committed change；恢复手册可在隔离环境操作 |

不能按 `min(DB,Redis)` 或消费日志猜测历史余额。已经丢失且没有可靠来源的 delta 无法由代码“恢复”，按 D03 的明确业务决定处理。旧二进制不会天然遵守新 epoch，因此拒绝旧 writer 还需切换操作、进程/访问围栏和测试证据，不能只增加新表就宣称防止混跑。

**联合启用门禁：** 迁移兼容、密钥/协议一致、旧 writer/poller 排空、账务事件和余额一致、缓存协议 READY、恢复器健康、backlog 可控、unknown 处置可用、CI/独立审查/隔离演练均通过。实际生产开关保持不操作。

## 8. C09 剩余配置与硬限额

先根据现行代码收敛遗留清单，历史提到但已被 R3/R4 修复的项目不得重复实现。

| 工作包 | 内容与文件边界 | 退出条件 |
| --- | --- | --- |
| C09-N1 当前审计（已完成当前只读范围） | 已核对 24 个 `GlobalConfig.Register` 配置族：access profile、Claude、monitor、tool price、GroupRatio、Passkey/ServerAddress、legal、perf metrics、general、console、checkin、token、quota、grok、qwen、fetch、discord、oidc、gemini、`billing_setting`、`payment_setting` 与 global 共 22 个已有受控快照/复制语义；仅 `performance_setting` 与 `channel_affinity_setting` 仍待 D14/D15 决策后收口。上述已迁移族保持原键/默认/公开 getter 与宽松 generic parser 语义，以私有原子 generation、完整 partial candidate 和 detached getter 关闭活指针。`UpdateOptionsBulk` 只是数据库批量事务辅助，不能称跨 HTTP/runtime 原子合同 | 清单必须保留读写者、复合字段、未知/null 策略、风险和处理批次；不重做 R1–R4 |
| C09-N2a/b PaymentRuntime | N2a 已完成当前纯内核范围：`setting/payment_runtime.go` 提供显式 seed、不可变 generation、typed set/clear/keep、detached snapshot、候选 CAS/abort、canonical option 副本和仅含元数据的 keyring；`TopupGroupRatio` 与所有现有支付 option 同代建模。它不读/写 legacy global、OptionMap、DB、SDK 或网络。N2b 仍须让 topup、subscription、Stripe/Waffo/Pancake 请求只捕获一次代际，移除跨请求 SDK 全局状态；外部 Provider 创建与本地绑定仍是补偿/孤儿状态，不伪造单事务 | 更新竞争中单请求不混代；旧回调仅在明确窗口内验签；金额/订单语义不按回调时当前配置重算；凭据不出日志/响应 |
| C09-N3a/b typed bulk 与前端 | 单写者为已管理配置族定义有界 typed bulk、重复/未知键拒绝、族级候选、DB 事务、一次 runtime 发布和 revision/CAS；随后把可合并的前端表单改为单请求，不把外部 Provider 操作纳入虚假的 DB 原子性 | 任一字段无效、无权限、冲突或 DB 失败时 DB/runtime/OptionMap 全不变；旧 PUT 兼容；页面覆盖加载、冲突、失败与七语言 |
| C09-N2c 通用热读分批收口 | 不一次重写 generic manager；22 个安全/账务/路由优先族已转为私有不可变 generation 与 detached getter，`setting/config/config.go` 保持单一 owner 串行修改。余下 `performance_setting` 的只读盘点已确认它同时向两个 `common` 投影发布，磁盘缓存消费者还会多次读取配置；不得把单一 source generation 改造误报成全链路同代，写入前先按 D14 冻结热路径与聚合一致性合同。`channel_affinity_setting` 不纳入普通 setting-only 批次：其复杂规则/模板直接影响渠道、retry、上游参数与账务归属，且 capacity/TTL 只在 HybridCache 首建读取；先由 D15 冻结热重建/epoch 合同 | 并发读写不混 generation，不暴露可变 map/slice；每一族有调用方与迁移回归；性能设置还须验证已捕获的磁盘路径/阈值/容量不混代；亲和设置还须验证规则/template 深复制、cache/retry/账务归属不漂移 |
| C09-N4a/b 成功硬限额 | N4a 只定义可测 Reserve/Commit/Release/Recover、receipt 与假存储；D05 批准后才接 Redis Lua、memory 原子实现与 B2/B3 durable operation 生命周期 | `committed + reserved` 永不越过确认上限；失败释放、unknown 保留、崩溃恢复、重放幂等与 Redis 故障策略均有证据 |
| C09-N5a/b 历史诊断与兼容 | N5a 已完成当前本地范围：精确路径在全局限流前预先写入 no-store，路由业务链仍为 `DisableCache → RootAuth`，严格拒绝未知、重复或畸形 query；业务链只对主库 `options` 做 1024 行有界只读扫描，`key` 仅投影前 256 字符加一字符截断探针，`value` 仅投影前 65537 字符以识别 65536 字符上限。输出 raw-null/空/空白/非法 UTF-8/截断、注册表 schema、GroupRatio alias 和覆盖完整性等元数据；超长 key 一律不解析、不参与 alias 且 redacted。它不写 DB、不触碰 OptionMap、配置 runtime generation、LOG_DB 或 Redis；平台通用鉴权/限流仍可使用自身 cache，不能把该业务边界误报为整个 HTTP 请求零 Redis。未知/畸形/dynamic external key 与所有 value/parser error 均不回显；MapConfig 不调用 validator；截断 source/行覆盖不全只报告不确定。N5b 仍须另列 dry-run/apply 白名单规范化、备份、revision/hash 与幂等 | 默认零业务写入；不自动清理生产历史记录；SQLite 合成验证不代替 MySQL 5.7/PostgreSQL 9.6/CI；修复影响可预览并明确保留/拒绝/fallback |

C09-N2/N3 中会影响 S3 策略保存和读取的部分必须先于 S3 enforce；N4 的实际语义还依赖 D05、D02/C03b 的 Redis 故障/权威边界以及 B3 的 durable unknown 生命周期。其他互不影响项可按文件所有权并行推进，但 S7 前要逐项处置。

**C09-N2a 本地证据（2026-09-06）：** 在 `CPUQuota=100%`、`MemoryMax=768M`、`MemorySwapMax=768M`、`GOMAXPROCS=1`、`GOMEMLIMIT=768MiB` 的隔离进程组中，`go test -p 1 -count=1 -timeout=180s -v ./setting` 与 `go vet -p 1 ./setting` 均退出 0。未参与实现者的独立审查先发现 `TopupGroupRatio` 缺失会造成未来价格与 group ratio 混代；补入精确旧 option 名、JSON typed spec 及 set/clear/unknown/detached-copy 回归后复审无 P1/P2。该证据不代表 payment runtime 已接到旧写者、Provider、订单、webhook 或三数据库持久化路径，N2b/N3 的业务字段约束、受信明文 writer 边界和 keyring 轮换窗口仍待。

**C09-N5a 本地证据（2026-09-06）：** 在相同单核/768MiB 隔离条件下，`setting/config`、`model`、`service`、`controller`、`router` 的 N5a 定向 `go test -p 1 -count=1` 与 `go vet -p 1` 均退出 0；最后一次串行定向回归覆盖 query 拒绝、精确路径在 429/限流后端错误前的 no-store、256 字符 key 截断探针、服务脱敏和 endpoint routing。三轮未参与实现者的只读审查先阻止了 runtime validator 读取、GroupRatio 原始字节比较、值/动态 key 泄漏、截断 source/行上限误报、畸形 query 接受和未授权/限流响应缺少 no-store；最终 P3 测试隔离修复改为真实 GORM `Row` callback 并显式恢复全局限流状态，复审无 P1/P3。SQLite 内存库验证了有界 SELECT、零业务写、脱敏、RootAuth/no-store 和错误分支；MySQL 5.7/PostgreSQL 9.6 实库证据仍为 P2，race 与 CI 尚未运行，N5b 也未开始。

**C09-N1 新增快照子范围（2026-09-06）：** `general_setting`、`console_setting` 与 `checkin_setting` 均以私有 immutable generation、写端完整 candidate/单次发布和 detached getter 取代 generic loader 的原地结构体写入；保持原注册名、键、默认值和 unknown/partial/`null`/标量解析语义。`GetStatus` 对 general/console 字段各捕获一次同代 snapshot；checkin 没有新增 min/max/非负业务校验，既有负数、反向范围和极端差值风险仍待单列处置。单核/768MiB 下，console 的 `./setting/console_setting ./controller` 定向 test、checkin 所在 `./setting/operation_setting` test，以及 `./common ./model ./service ./setting/console_setting ./controller`、`./setting/operation_setting` vet 均退出 0；每项有未参与实现者独立审查，无 P1/P2/P3。该证据不代表跨族 runtime 原子性、三数据库热更新或成功硬限额已完成。

**C09 性能设置只读盘点（2026-09-06）：** `performance_setting` 的八个标量字段可形成 immutable generation，但当前启动/逐 key reload/批量保存还会分别发布 `common.DiskCacheConfig` 与 `common.PerformanceMonitorConfig`；`GlobalConfig.LoadFromDB` 和直接 generic 更新又可能只改 source 而漏同步投影。磁盘缓存的单次决策还会多次读取 enabled/threshold/max/path，`newDiskStorage` 甚至忽略已传入 path 后重新读取全局路径，因此简单改 getter 不能证明全链路同代。此轮仅盘点，未编辑、未测试、未改变运行行为；后续最小迁移也须让单次消费者捕获完整 disk snapshot，并由 D14 决定路径热切换和是否需要跨两类投影的 aggregate runtime generation。

**C09 Fetch 设置快照子范围（2026-09-06）：** `fetch_setting` 保持八个持久键、默认 SSRF protection=`true`、generic parser 的 partial/unknown/`null`/标量语义及全部既有 SSRF 策略；私有 immutable generation 在初始发布和 getter 处深复制 domain/IP/port 三组 slice，避免调用方或并发写入污染 live policy。仓内读取方只消费 detached snapshot 后构造校验或保护对象，未改路由、网络访问或默认策略；测试改为经 ConfigManager 更新且恢复完整 baseline，不能再用 getter 绕过受管发布。单核/768MiB 的 `go test -p 1 -count=1 -timeout=300s ./setting/system_setting ./service ./controller` 与相关 `go vet` 均退出 0；独立安全审查先发现视频代理测试遗留 live-pointer 引用，修复后复审无 P1/P2/P3。跨多个 option 的连续更新仍不是原子合同。

**C09 Token 设置快照子范围（2026-09-06）：** `setting/operation_setting/token_setting.go` 已以私有 immutable generation、写端完整 candidate/单次发布和 detached getter 取代对可变全局结构的原地读取；保留 `token_setting`、`max_user_tokens`、默认 `1000` 以及 generic parser 的 partial/unknown/`null`/整数语义，故 `0` 与负整数仍按既有后端合同接受。它不改 `controller/token.go`、`model/token.go` 或前端，也不修复查询计数与创建之间的 Key 数量竞态。单核/768MiB 的 `go test -p 1 -count=1 -timeout=240s ./setting/operation_setting` 与 `go vet -p 1 ./setting/operation_setting` 均退出 0；未参与实现者独立审查无 P1/P2/P3。该范围仍不代表跨族发布、服务端最小值合同或 Key 限额事务已经完成。

**C09 Quota/Qwen 设置快照子范围（2026-09-06）：** `quota_setting` 保持 `enable_free_model_pre_consume`、默认 `true` 和宽松 generic parser，只以私有 immutable generation 与 detached getter 消除活指针；既有免费模型与零预扣行为不变。`qwen` 同样保持 `sync_image_models`、默认模型列表、`null` 与空列表形状以及 `strings.Contains`（含空 pattern）的既有匹配语义，但对初始值、发布代和 getter 做深复制，避免调用方修改 slice 影响 Ali 图片协议选择。单核/768MiB 的 `go test -p 1 -count=1 -timeout=300s ./setting/operation_setting ./controller ./relay/helper`、`./setting/model_setting ./relay/channel/task/ali` 和相关 `go vet` 均退出 0；两个子范围各由未参与实现者复审，无 P1/P2/P3。它们不改变 relay 账务、Provider 协议、跨族发布或成功硬限额合同。

**C09 Gemini/Billing/Payment/Global 设置快照子范围（2026-09-06）：** `gemini` 对配置中 map/slice 与 `SafetySetting`、`SupportsImagine` 等 `ConvOptions` 回调都绑定到请求捕获的 generation；旧已捕获 options 不会在后续写入后改读新配置。`billing_setting` 将 `billing_mode` 与 `billing_expr` 两张 map 同代深复制，价格 helper、定价查询和同步 DTO 各在单次读取中使用同一 snapshot；不改 `pkg/billingexpr/expr.md` 的表达式版本、归一化、额度舍入、预扣/结算或费率。`payment_setting` 保持七个既有持久键、默认与 parser，对金额选项/折扣 map 做 detached copy，支付合规判定单次捕获 snapshot；它不接通 PaymentRuntime、SDK、订单、回调或真实付款。`global` 深复制模型黑名单及策略的 map/slice，`ConvOptions.PreserveThinkingSuffix` 绑定被捕获的 global snapshot；缓存中的旧 options 保持旧代，重试才按原有语义重建。四项均保留单 key 连续更新可显示新旧组合的兼容边界，不宣称跨 option 原子提交，后者仍属 C09-N3。单核/768MiB 的相关 `go test -p 1 -count=1` 和 `go vet -p 1` 已通过；未参与实现者独立审查在补齐 Gemini callback 与 Global partial-policy 回归后确认无 P1/P2/P3。它们不改变 Provider 协议、账务语义、PaymentRuntime 接线、路由时序或三数据库持久化合同。

**C09 渠道亲和设置只读盘点（2026-09-06）：** `channel_affinity_setting` 的 Rules 含多个 slice 和嵌套 `ParamOverrideTemplate`，直接支配渠道选择、失败 retry、请求参数与 cache key；普通 detached getter 不足以同时避免复杂图的 alias 和每请求重复深复制。更重要的是 `MaxEntries`/`DefaultTTL` 只在 HybridCache 首次初始化时读取，不能把普通配置保存误报为运行中 cache 迁移或 TTL 重写。后续至少需要 immutable 内部 snapshot、服务读取同代规则、复杂模板深复制和 cache/retry 回归；D15 未决前不改缓存、Redis namespace、规则优先级、retry、上游参数或账务归属。本轮未编辑、未测试、未改变运行行为。

**C09 Grok 设置快照子范围（2026-09-06）：** `setting/model_setting/grok.go` 已以私有 immutable generation、完整 generic candidate 和 detached getter 保持 `grok`、两项持久键、默认 `true/0.05` 以及既有有限 float 解析。`service/violation_fee.go` 原本已在单次调用内读取一次设置，故本批只消除运行中反射原地写入/活指针风险；未修改违规收费、额度转换、日志或结算。单核/768MiB 的 `go test -p 1 -count=1 -timeout=300s ./setting/model_setting ./service` 与相关 `go vet` 均退出 0；独立审查无 P1/P2/P3。负数或零金额仍按旧的“不收费”路径处理，未新增费率、限额或计费规则。

**C09 Discord/OIDC 设置快照子范围（2026-09-06）：** `setting/system_setting/discord.go` 与 `oidc.go` 已以各自私有 immutable generation 和 detached getter 保持注册名、全部持久键、generic parser、OAuth HTTP/redirect/endpoint 语义及 secret 可见性；`controller.GetStatus` 对 Discord/OIDC 各捕获一次 snapshot，仍只输出原有 enabled、client id、OIDC display name/authorization endpoint，绝不新增 secret、token、userinfo 或 well-known 输出。既有 OAuth 测试已改用受管配置更新和 cleanup，不再通过 getter 改写 live 状态。单核/768MiB 的 `go test -p 1 -count=1 -timeout=300s ./setting/system_setting ./oauth ./controller` 与相关 `go vet` 均退出 0；独立审查无 P1/P2/P3。它不实现服务器端 discovery、不形成 OIDC 与 ServerAddress 跨族原子性，也不收紧现有仅检查 ClientId 的 enable 预检。

## 9. S3 账户等级、Key 策略与迁移

输入：[账户与访问方案](ACCOUNT_ACCESS_PROFILES.md)、setting/access_profile、model user/token/access_profile、middleware/auth/distributor、路由与 B1/B3 计费快照。

### S3-0：先冻结行为表

每个请求明确 Account Tier 来源、可见模型、Key allowlist、Profile allowlist、渠道能力与路由组如何相交；空列表代表无限制还是不允许；禁用/未知/已删除策略如何处理；fallback 何时允许、最多几次、是否跨组及费用归属。不得用 fallback 扩大用户权益，也不得越过现代 Task 的 DISPATCHING 禁重发边界。

“旧 Key 不被静默拒绝”通过迁移审计/解释和受控启用实现，不能以永久旁路强制策略实现。已经存在的 durable operation 重放继续使用原路由/价格快照，新请求采用新策略。

**当前只读盘点（已完成，不代表 enforce）：** `users.account_tier_id` 与 `tokens.access_profile_id`
已有跨库兼容的增量列和只填空值迁移，但请求仍以 legacy `group` 路由；Profile 的 route/model/fallback
字段尚无业务读者，Token Redis cache 缺 Profile/schema/policy version，`AccountTierID` 也未进入请求 context。
未知显式 Profile 在兼容层会落到 `custom`/enabled，不能直接继承为 enforce 语义；订阅目前只改
`users.group`，Task/recovery 也还没有冻结 policy decision/fallback path。R1–R4 的 Option 事务/单进程发布
不等于跨节点策略 revision。以上事实要求先做 inventory、cache schema/fence、版本化 snapshot/decision
接口和 audit-only，不可把展示元数据接入 distributor、price 或 durable Task。

**D04 前可独立准备：** 对空/未知稳定 ID、group↔ID 不一致、disabled 引用、Profile 悬空 route/model
和订阅 group 做零写入诊断；让 Token cache 携带 `AccessProfileID`/schema 并使旧 hash 回库；为 Tier/Profile
变更建立独立 cache generation/fence；定义有界规范化 validator、无秘密的 `AccessPolicySnapshot`/
`PolicyDecision` envelope 与 off/audit 兼容回归。不得在这些准备中改变当前 legacy 路由、价格或访问结果。

### S3-PRE1：脱离运行时的预检 envelope（已完成当前本地子范围）

`service/accesspolicy` 现有一个无 caller 的纯内核，只接受固定、有限的 Account Tier/Profile reference、
group/model/route list presence、registry revision 与固定的 legacy group/ratio outcome。它规范排序并摘要快照，
报告空、disabled、unknown、悬空和 legacy group 差异；列表、ID、ratio、UTF-8、C0/C1/Unicode format/bidi
控制字符均 fail closed。它不接收任意 metadata/map/opaque price payload，错误不回显输入；所有 `off`、`audit`
和 `enforce` 输入都固定返回 `Applied=false`，合法 legacy outcome 逐字节保留，不会改路由、权限、价格或账务。

该包只依赖 Go 标准库，不依赖数据库、缓存、Gin、全局设置、旧路由或时钟。单核/768MiB 下
`go test -p 1 -count=1 ./service/accesspolicy` 与 `go vet -p 1 ./service/accesspolicy` 通过，最终独立复审无
P1/P2/P3；固定摘要 known-answer 锁定 schema-1 编码。它不代替 D04，也不构成 audit ingress、缓存 fence、
策略持久化、迁移或 enforce 的证据。

### 分步实施

1. **S3-1 策略快照：** 原子发布规范化注册表，版本可追溯；拒绝循环和规范化碰撞；权益/计价决策读取同一请求快照。
2. **S3-2 缓存和鉴权：** Token 缓存纳入 `AccessProfileID`、必要 schema/policy version 与失效协议；旧 cache 缺字段不能绕过强制；Key revoke/编辑后行为一致。
3. **S3-3 audit/shadow：** 先只计算新旧决策差异，记录净化原因、受影响 Key/模型/渠道数量，提供管理员解释；不改变已承诺路由。
4. **S3-4 迁移与 enforce：** 支持明确范围启用、稳定映射、未知值处理、回填/重复执行、回滚；不能直接全局打开而无差异报告。
5. **S3-5 管理与用户流程：** 用户看懂权益和选择资格，Key 创建/编辑展示模型、路由、倍率、限制、fallback；管理员可预览变更影响和审计记录。
6. **S3-6 联合回归：** 普通/管理员、旧/新 Key、所有已支持协议、缓存命中/缺失/陈旧、订阅到期、并发改策略、durable replay、同一请求价格不漂移。

**退出条件：** SQLite/两种实库迁移与策略决策、权限/缓存/race、浏览器/i18n、差异报告和关闭/回退场景通过；profile 不再只是元数据。生产迁移不执行。

## 10. S4 完整运行、发行、升级与恢复

保留 S4-01/02 的 linux/amd64、全新 SQLite、无 Redis/batch 的 Full/Legacy LAN 合成 smoke；它们不能证明以下 Full/Lite/Desktop、统一发行、形态切换或恢复范围。

### 10.1 目标矩阵

| 维度 | 计划覆盖 |
| --- | --- |
| Full 主库 | SQLite、MySQL 5.7、PostgreSQL 9.6；项目声称支持的更新版本用代表版本补兼容 |
| 日志库 | 同库、独立 SQLite/MySQL/PG、已支持 ClickHouse 组合中的关键跨库失败路径 |
| 余额/cache | DB-only；共享 Redis；历史 batch→durable；新旧 writer 切换和恢复 |
| 功能版与安装形态 | Full/Lite × server-native/server-container/personal-native/personal-container/Desktop；能力、依赖和数据兼容按独立 Release Manifest 声明 |
| 访问模式 | local/LAN/public 独立配置；公网环境检测和外部验证不把本机监听/健康误报为公网可达 |
| Lite/Desktop | Lite 服务器为正式支持场景；个人电脑 Lite/Desktop SQLite-first、单进程与回环默认。Desktop 的 macOS/Windows/Linux 声明必须与 CI、制品和实机证据一致 |
| 数据年龄 | 空库、最低受支持旧版本、部分迁移、已存在 active/unknown/terminal 任务、已有日志与历史策略 |

### 10.2 工作包

- **S4-03 三库应用合同：** 不只是模型 AutoMigrate；完整启动、登录、Key 权限、真实路由到 fake upstream、SSE/非流、Task、预扣/退款、日志/统计和数据库重连。
- **S4-04 升级路径：** 使用固定旧 commit/fixture 生成隔离旧数据，运行新程序实际迁移；重复启动、半次失败恢复、多个节点启动竞争；检查未知态与旧 Task 归属。
- **S4-05 备份恢复：** 按 D07 备份主库、必要日志库、应用配置/加密密钥与协议/幂等标识；校验和、恢复后余额/Key/任务/事件/outbox 精确一致；证明 Redis 可重建。不能漏备 HMAC/加密密钥导致历史记录不可读。
- **S4-06 失败回退：** 明确旧二进制能否读取新 schema/epoch，禁止只切镜像却保留不兼容 DB；必要时按备份恢复，并说明已接受请求的处理边界。
- **S4-07 桌面与 LAN：** CI 原生构建、安装/首次启动/升级/卸载保留数据、单实例、端口冲突、崩溃、睡眠恢复、托盘状态、回环默认、用户确认后的私网访问、系统防火墙提示、第二设备调用。
- **S4-08 健康与运维：** liveness、readiness、DB/协议/worker 状态和 drain 入口的语义明确；UI 不在后端失效时显示“可调用”。
- **S4-09 实机验收：** 复用 [实机清单](REAL_DEVICE_ACCEPTANCE.md)，手机触控/滚动/文字、macOS/Windows 实际安装和 LAN 由指定验收者记录当前 SHA、设备和结果。

### 10.3 统一发行、安装与更新工作包

- **S4-D0 产品/发行合同（已完成当前文档范围）：** [发行制品、安装与更新合同](RELEASE_MANIFEST.md) 与 [Lite/Legacy LAN 迁移](LAN_LITE.md) 定义三维产品模型、Legacy 映射、数据目录、Release Manifest、清理、切换、更新、S5-P 保留与验收。没有代码、制品或发布动作。
- **S4-D1 Release Manifest 与统一入口：** 保留 `@forcemind/myapi` / `myapi`；让 NPM bootstrap、源码清单、Full/Lite OCI、原生二进制和 Desktop 制品以同一 source SHA/版本、hash/digest、签名、兼容矩阵聚合。环境/OS/CPU 不匹配必须明确拒绝；不 fallback 到 `latest`、Full 或未知源。当前 `cli/lib/release-manifest.mjs` 只完成 schema-1 的有界结构校验和 fresh-install-only 选择，且在反射前拒绝 Proxy/revoked Proxy、自定义原型、访问器、symbol/非枚举字段、稀疏或带额外属性数组；`release-manifest-evidence.mjs` 只冻结/校验 1B–4MiB 的未受信原始 bytes evidence，拒绝伪造 typed-array、SharedArrayBuffer、Proxy、非 canonical base64 与伪造 hash，且不解析或赋予信任。`legacy-installation-profile.mjs` 仅用调用方显式提供的旧 Docker 配置做 fail-closed Full/Lite、local/LAN/needs_manual 画像映射，绝不推断 public。三者均未读取/验证受信资产或接入 CLI；升级/切换/回退明确拒绝。NPM 安装保持零副作用，正式 publish/tag 仍另授权。
- **S4-D2 安装记录、目录与受控清理：** 每个受管安装有非秘密 installation record、操作 journal、单安装锁和 `owned-files` 清单；程序、持久数据、下载暂存、回滚备份分离。健康检查提交后仅清理任务自有暂存与未选/过期的自有制品；保留 current/last-known-good、数据、配置、日志、上传、S5-P 版本/证据和 Codex 应用备份。失败不损坏旧环境或扫删未知目录。当前 D2A 的 `cli/lib/installation-state.mjs` 与 `canonical-artifact-reference.mjs` 仅验证/冻结内存 record、artifact identity 与显式 cleanup plan，在任何 descriptor/reflection 之前同样拒绝 hostile JS object/array，且拒绝路径穿越、保护目录重叠、可变 OCI/HTTPS 引用和未完成健康检查；不读写文件、不删除、不安装、不启动服务，也未接入 CLI。
- **S4-D3 更新、回退与产品形态切换：** 手动检查/下载/安装与自动开关、维护窗口分离；所有入口使用同一持久状态机、锁、预检、artifact/schema 校验、备份、排空、健康/业务验证、恢复/人工处理。Full↔Lite 先报告能力影响、活跃订单/任务/订阅与 DB 兼容；不安全时拒绝，不能删数据降级。程序回退与 DB 恢复分开，数据库引擎/跨机器迁移单列。更新不得改变 access mode、防火墙、反代、隧道、授权或 S5-P/Codex 文件状态。
- **S4-D4 形态执行者与受限管理桥：** 原生服务器/个人电脑、Docker、Desktop 各自使用适用的更新机制；应用 UI 只通过受限本机/服务器桥请求已登记安装，不能写全局 NPM、任意 Compose、宿主文件或 shell。Desktop 明确窗口关闭、退出应用、停止后台服务、开机启动、休眠/断网恢复和更新后的服务恢复；Linux Desktop 只有制品、CI、签名策略和实机验收齐备后才可列正式支持。
- **S4-D5 服务器 Lite 与公网向导：** 提供服务器原生/容器 Lite 的 SQLite-first 安装、数据/日志、备份恢复、域名/HTTPS/反代及经确认公网入口教程；个人电脑本机/LAN/公网向导另行实现。检测端口、权限、防火墙、NAT/CGNAT、IPv4/IPv6、DNS/证书和外部可达性，不能自动判断时说明人工步骤。第三方隧道、账号、费用和路由器映射均由用户选择/授权。

S5-P worker 在更新/切换中停止新的模型提交，已发请求按未知态合同恢复；升级/切换不得自动读取、重新应用或覆盖 Codex 指令文件。详见 [S5-P 专题](PROMPT_LEARNING.md)。

**退出条件：** 达到 D07/D10–D12 定义的 RPO/RTO、兼容矩阵和发行信任要求；在隔离环境重现安装/切换/更新失败、journal 恢复、程序/数据库回退与 S5-P 数据保留；必要真实设备、外部网络和签名证据齐全。没有设备时只完成可执行测试包与说明，整体对应项仍待验证。

## 11. S5 额度闭环与 Provider 扩展

输入：[额度分析](QUOTA_ANALYTICS.md)、[告警政策](QUOTA_ALERT_POLICY.md)、[Claude 组织用量](CLAUDE_USAGE_REPORT.md)、[Antigravity 公共接入门禁](ANTIGRAVITY_PUBLIC_RELAY_GATE.md)、[TokenHub 边界](TOKENHUB_INTEGRATION.md)。现有专题的 Provider 事实必须在真正实现时重新查官方来源。

### S5-Q1 采样可靠性和真实验收

保留已实现分析公式、reset-aware ETA、概览有界查询及详细渠道筛选；补长时间采样/多节点调度/渠道删除或禁用/凭据轮换/失败恢复/保留清理。使用合成时钟做确定性测试，并在用户显式提供的隔离真实账号上覆盖至少多个有效采样和窗口变化；短时成功不能冒充长期稳定。制定观测时长、覆盖率/失败率和数据规模验收目标并记录测量。

### S5-Q2 告警事件与通知交付

**P2A 本地纯合同已完成：** `EvaluateChannelQuotaAlertOccurrenceV2` 仅接受 canonical `channel:<positive-id>` 与 `snapshot:<positive-id>` 内部引用，在可信、有总量且时间一致的样本上为 threshold/recovery/reminder 生成含 source snapshot 和 reminder cycle 的 versioned digest key；未知、无总量、未授权、畸形引用和时钟/加法溢出均 fail-closed。单核/768MiB 的 `common` 定向 test/vet 与独立审查通过。它没有 DB/outbox/worker/notifier/网络/配置写入或真实渠道验证，不能称为持久事件或投递能力。

先复核已有 notifier-neutral 规则，新增持久事件与投递状态。不要直接把永久 `<subject>:<status>` 当所有未来告警唯一键，应包含可区分的状态转换或冷却周期身份，避免恢复后再次 critical 被永久吞掉。

确定一个首选通道后实现带超时、退避、次数上限、接收者权限、凭据保护和可观测失败的 notifier。重复投递不可冒充 exactly-once；通道支持幂等键则使用，无法保证时明确至少一次及重复风险。保留正常恢复、未知/无 total 不发低余额告警，失败不更新为“已送达”。Webhook 检查 SSRF/redirect，禁止通过通知配置成为任意网络代理。默认关闭真实外发，测试使用 loopback/fake transport。

**退出条件：** threshold/reminder/recovery、冷却、配置变化、重启、投递未知/重放、越权收件人、脱敏、禁用开关及管理员历史 UI 通过；真实通道验收由用户授权后进行。

### S5-Q3 多 Key 上游身份

先决定“账号”和“凭据”的身份模型：稳定、不敏感、可轮换、多 Key 是否指向同一上游账户。未经证明不得合并多个 Key 的额度或把渠道数称为账号数。实现身份和系列索引、凭据轮换关联、重复样本隔离、混合货币/计划/窗口处理，明确路由、观测与对账各自职责。

### S5-P 提示词学习与版本中心（必选）

S5-P 与 S5-Q 额度闭环并列，不替代、不缩小后者，也不把现有渠道 `SystemPrompt`、Codex 凭据导入或 Full Content 日志误写为已实现的学习中心。详细产品和数据合同见 [提示词学习与版本中心](PROMPT_LEARNING.md)。

- **S5-P0（已完成当前文档范围）：** 冻结默认关闭、三层授权、范围/水位、派生样本、二层脱敏、调度/预算、版本/差异、文件应用、安装形态和验收合同；本阶段不读取真实日志、不调用模型、不写宿主文件。
- **S5-P1a（已完成本地纯内核子范围）：** `service/promptlearning` 对包内构造的候选执行来源类别、文本边界、二层脱敏及 scope 隔离 HMAC 指纹；`hmac-sha256-v2` / `occurrence-v2` 将物理 node/epoch 和 HTTP observation 留作已验证审计 provenance，不纳入稳定 conversation/turn 的 logical occurrence 或 transport，因此跨节点/普通重启同一可信轮次可 replay、同轮不同载荷仍可 conflict。没有 caller、HTTP、DB、日志、worker、模型或文件操作。合成测试与独立审查覆盖 fail-closed 边界；真实 ingress、DB unique new/replay/conflict、用户隔离/预览/保留和 D13 仍属于 P1/P2 后续工作。
- **S5-P1b（已完成本地纯内核子范围）：** package-private `serverObservedLiveTurn` 只接受完整、server-marked 且有完整 provenance 的未来认证 ingress 观测；恰有一个 `new_user_text`，history/system/developer/assistant/tool/attachment/response/internal/automatic-analysis 段明确排除且不参与正文或任一指纹。未标记/未知/客户端声明、零或多新用户段均零泄漏 reject。独立 `observation-v1` HMAC 绑定 scope、node、epoch、conversation、turn、request observation 与规范化 eligible payload，而 occurrence/transport 继续保持逻辑 replay 语义。没有 Full Content、HTTP、DB、日志、worker、模型、文件或 caller；合成回归、两轮独立审查和受限 test/vet 通过，不替代认证 ingress/主库去重/D13。
- **S5-P1 增量样本治理：** 用户/项目/Key 隔离、可信会话/轮次识别、重试/历史去重、节点游标、低置信旧记录、预览/排除/删除和保留。原始 JSONL 不能充当持久权威样本库；三库迁移使用可移植 GORM/TEXT/CAS 合同。
- **S5-P2 调度、费用与生成：** time/count/any/all/coalesce/cooldown/补跑状态机，单 worker/lease/恢复，冻结输入/基线，正常权限/账务/限额/审计路径、模型/渠道精确选择、预算/未知态/取消。付费自动调用和生产启用依赖 B3-A、B3-B/C、C03b 与相关 S3/C09 权限；此前只做假上游/合成验证。
- **S5-P3 版本中心与 UI：** 不可变正文/审阅资料、父版本/分支、编辑/再生成、冲突、差异、导出、历史、来源/费用、七语言和全部空/错/权限/未知状态。旧 run 不能覆盖新人工编辑。
- **S5-P4 Codex 受限文件适配：** 在明确的机器/账号/路径授权下验证实际 Codex 文件优先级和加载行为；实现路径/链接防护、外部修改检测、备份、原子写后校验、应用回执和回滚。服务器、容器、Desktop、LAN 浏览器的物理权限边界分别实现；不扫描 Home、`auth.json`、Keychain 或隐藏系统指令。
- **S5-P5 联合验收：** 多节点、SQLite/MySQL/PostgreSQL、Full/Lite/Desktop、更新/切换/恢复、跨用户/撤权/注入/预算/未知费用、安全文件写入、真实设备和经授权外部模型矩阵。未有真实授权时保留待验收，不以合成 fixture 冒充。

### S5-E1 Claude 组织用量（列为待选择扩展，不假称已接入）

识别官方支持的产品形态、端点、专用授权与单位；设计独立权限/数据表/时间桶/分页/保留/导出，不把组织使用量当余额。实现前重新查官方文档，无配置不请求；测试 403/429/5xx、分页、乱序/重放、跨组织隔离和删除保留规则。真实 Admin 账号必须由负责人提供并授权使用。

### S5-E2 Antigravity 公共异步能力（独立高风险扩展）

现有 transport 不是公共 relay。若选入本次交付，必须单独完成 lifecycle/owner+Key scope/持久化幂等/budget+结算/cancel-complete 竞争/工具与网络 default-deny/保留与删除/Responses 映射/移动及 LAN 状态，复用账务内核但不套用普通 Gemini 请求生命周期。禁止读取宿主凭据或把宿主目录/环境暴露给远程工具。只有全部专题门禁与负责人明确公共接口批准后才注册路由；没有可验证额度端点时准确标记 unsupported。

### S5-E3 TokenHub 与 Midjourney 后续范围

- TokenHub：保留现有边界；按 D06 决定区域、路由权重、计费对账及管理界面是否实施。禁止复制外部项目代码/资产或绕过主路由/账务层。
- Midjourney：虽排除在 B2 首批之外，总项目必须审计 submit/notify/poll/结算/退款和重复通知。若存在同类恢复缺口，作为独立后续任务按专有协议实现，不因现代 Task 完成而宣称所有异步业务已安全。

D06 对每个扩展分别记录“本次必选/后续独立项目/能力事实不支持”。后续或不支持不能写成已实现；若用户把它列为本次必选，缺少外部条件时必须保留完成阻塞。

## 12. S6 完整独立 UI 与官网

### S6-0 功能盘点与页面合同

先把实际路由、权限、功能、旧入口和计划新增 API 映射到页面，不能只做几个漂亮页面。为每个页面写：用户角色、入口、输入/校验、查询/操作、分页筛选排序、成功返回、异常恢复与验收。

最低页面/流程清单：

| 受众/模块 | 必须完成的流程 |
| --- | --- |
| 登录与账号 | 登录/注册（按配置）、OAuth、Passkey/2FA、找回与退出、会话过期和权限变化 |
| 引导与概览 | 角色/功能版/安装形态/访问模式引导、已完成不占位、用户调用/余额摘要、管理员上游额度与运维摘要 |
| Key 与调用 | 创建/编辑/撤销、访问方案解释、模型/额度/IP/有效期、凭据只展示一次、请求示例、调用结果定位 |
| 用户与权益 | 用户管理、Account Tier/订阅权益、配额调整与审计、禁用和权限边界 |
| 渠道与 Provider | 创建/编辑/测试、显式 Codex 导入向导与 fallback、模型/路由、额度历史、凭据轮换与失败诊断 |
| 日志与任务 | 消费/完整内容日志、筛选/详情/导出、脱敏、Task 各状态查询、unknown 人工处置及审计 |
| 钱包与订阅 | 充值/支付状态、订单/订阅、失败重试、退款与账务明细，现有渠道不丢失 |
| 配置与运维 | 配置族原子保存、变更预览、通知投递、账务/恢复 backlog、健康、备份/恢复说明与只读 build 标识 |
| 提示词学习与版本中心 | 开关/范围/保留、样本预览与排除、计划/预算/费用、候选规则、编辑/再生成、差异/历史/导出、授权、应用/回滚；不展示未授权原文或凭据 |
| 安装、更新与恢复 | 当前功能版/安装形态/访问模式、运行状态、数据位置、检查/下载/安装、自动更新设置、维护窗口、切换、历史、备份/恢复与中文/英文教程入口 |
| Lite/Desktop 与公网 | 最少必要步骤、监听/权限/外网验证状态、后端不可用、端口冲突、窗口/后台/服务行为、升级提示、保留数据与关闭公网说明 |
| 官网 | 独立产品说明、Full/Lite/Desktop 及服务器/个人电脑选择、安装文档、安全边界、访问模式与发布状态；不链接不存在的正式制品 |

### S6-1 设计系统

以实际角色任务组织导航；确定字体、颜色、间距、布局、表格/表单/对话框、图表、图标、空态和交互反馈。沿用现有组件与 token 体系，不无理由更换构建框架。提供关键页面真实可交互预览，经设计方向确认后批量实施；不得用静态假数据冒充已接业务。

### S6-2..5 分批实施与回归

建议批次：身份/布局/引导 → Key/调用/用户 → 渠道/额度/日志/Task → 钱包/配置/运维 → S5-P 版本中心 → 安装/更新/Lite/Desktop/官网。每批都保留其他功能入口并覆盖真实 API 状态，组件测试与页面集成分开。

七语言 `en/zh/zh-TW/fr/ja/ru/vi` 全覆盖；动态文字和错误/校验需国际化，不能只做 key parity。覆盖加载、空、错误、无权限、过期、处理中、成功、冲突/重放、暂停和未知状态。手机、平板、桌面，至少 320/390/768/1440 宽度代表场景；键盘焦点、弹窗返回、对比度、读屏命名、触摸和长文字分别验证。

必须实际查看最终渲染与操作流程，修复遮挡、横向溢出、层级混乱和功能缺失。截图只证明画面，操作测试才能证明交互。七语言、长列表、图表和正文等代表负载记录性能基线与结果。

### S6-6 版本与全量视觉验收

统一可见 build/revision 来源。当前发布版本是受保护的 0.1.1，不为每次 UI 批次创建 tag；构建 SHA/运行时 revision 区分批次，正式版本号单独决定。实际检查 Full/Lite/Desktop、安装形态和访问模式的合理入口显示一致、无挤占布局，不声称线上已更新。

**退出条件：** 全部页面映射有实现；真实浏览器功能/视觉、七语言、响应式、无障碍、完整前端 build/test 及独立视觉/交互审查通过；真实设备按 S4 联合验收；官网一致但不发布。

## 13. S7 交付就绪与完整性审计

### S7-1 候选版本与兼容矩阵

选择一个具体候选 SHA，核对全部功能提交可达；所有后端、前端、三库/Redis/ClickHouse、root/relaykit、CLI/Desktop、Full/Lite 服务器/个人电脑安装、S5-P、安装/切换/升级和无发布 Docker 证据能覆盖该候选。候选 `RELEASE_MANIFEST.json` 必须与源码包、OCI、原生和 Desktop 工件可交叉核对。不能无限“再跑一遍”：只有代码变化、失败或未解决风险才扩展复验。

### S7-2 文档、接口和制品

更新权威产品定义、执行状态、完成审计、OpenAPI/错误码、配置示例、迁移/回滚手册、中文/英文安装指南、用户/管理员指南和 Linux/Mac/Windows 阅读路径。版本/清单/源码包/NOTICE/第三方许可证与实际代码一致，保留必要法律通知；技术命名沿用仓库既有规范，不擅自创造 namespace。人类品牌 `My API` 与历史 `MyAPI` 文档若冲突，按最新项目治理统一并回归，不全局替换协议标识。

生成可审阅候选源码包、Release Manifest、检查本地 NPM pack、桌面/镜像/原生 CI 工件与校验和，但不 publish、不建 tag、不部署。来源私有时不改变可见性，不把敏感本机交接资料、真实日志、提示词审阅资料或 Codex 备份装进对外包。

### S7-3 业务、运行、视觉和外部验收

按第 1.1 节逐要求核对；每个页面、API、任务状态、不变量、命令、制品和外部条件都需证据。必需真实账号/设备/法律/签名能力由相应负责人完成或明确仍阻塞；测试通过与外部验收不能互相替代。

### S7-4 完成判定

只有所有本次必选目标满足、无已知阻断、必要审查与同步完成时才标记“全项目交付就绪”。发布审批未申请不影响“未发布”的准确交付，但不能写“生产验证完成”。任何缩小范围必须由用户显式改变目标并保留原要求的未完成记录。

## 14. 每个工作包统一的退出条件

每个工作包进入实现前记录以下合同，完成后更新同一记录，避免新增重复管理文件：

```text
ID / 名称 / 状态：未开始、进行中、待验证、审查未通过、已完成
目标及需求来源：
输入、输出、行为与失败恢复：
依赖、关键决定及批准证据：
实现负责人、独立审查者、允许写入文件：
不变量、最小测试和必要跨模块回归：
实际验证：命令、环境、SHA、退出码、CI run/job/artifact
审查发现、修复与复验：
版本/迁移/文档/兼容影响：
本地 commit / 远端 commit / 同步状态：
未覆盖项、外部条件、下一步：
```

“已完成”不是文件已保存、单测通过、commit 成功或 job 全绿任一单一事件。没有新行为或新风险不制造无意义测试/提交；测试不能降低断言掩盖错误。

## 15. 多智能体分工与执行节奏

最多四个活跃角色（含主代理）。规划和独立审查不能冒称实施；共享目录所有改动立即可见。

| 角色 | 建议配置（可用时） | 责任与写入边界 |
| --- | --- | --- |
| 主代理/整合 | 当前主模型；不虚构已切换 | 管理依赖、文档、整合、单一测试调度、阶段同步和验收 |
| 核心实现 | Sol high/xhigh，跨模块疑难按证据提高 | 单独拥有 model/事务/账务共享接口；不会回退别人代码 |
| 独立模块实现 | Terra medium/high，按任务复杂度选择 | CI/fixture 或某一组 adapters/页面；明确文件清单 |
| 独立审查 | Sol high/xhigh；复杂规划按 AGENTS 可用 Sol ultra | 不参与对应实现，只读审查合同、风险、回归与证据 |

实施期间第四个槽可以用于无重叠的只读后续设计；进入验收时转为独立 reviewer。不是同时启动四个实现者。复杂任务规划遵守可用 AGENTS 的模型约定；实际模型/强度必须由工具成功设置，不可把文档当模型开关。

波次：

1. B2-1 核心修复＋CI/迁移测试边界设计＋C03b 合同只读规划。
2. B3-A 核心账务＋纯 parser 接口/adapter 分组；协议接口稳定后再并行。
3. B2 controller 集成＋不争写的 HTTP fixture；再进行独立审查。
4. B3 恢复/outbox 与 C03b 各 writer 分批接入；C09/S3 只读设计同时进行。
5. S3、S4 设施和 S5 独立功能按依赖推进；S6 按页面 API 稳定程度分批开始。
6. 最终停止新增功能，整合候选、独立审查、全项目验收。

每次只保留少量可独立验收的在制工作。核心接口改变立即通知下游，不在共享文件上并行争写。受限主机所有测试由主代理串行执行；CI 可按 runner 资源并行，Docker 矩阵保持既有资源限制。

## 16. 验证矩阵与性能证据

| 层级 | 本机 | GitHub Actions / 指定隔离设备 |
| --- | --- | --- |
| 静态/合同 | diff、gofmt、局部语法/有限检查 | root JSON、API、发行/Release Manifest/品牌/pack 合同完整链 |
| 业务单测 | 改动相关定向 Go，必要单 worker 前端用例 | 根模块全量、相关 race、relaykit 独立 vet/build/test |
| 持久化 | 临时 SQLite、确定性故障注入 | MySQL 5.7、PG9.6、Redis 与 ClickHouse 专项临时服务；旧 schema/并发/提交未知 |
| HTTP 协议 | 必要 loopback/fake 单场景 | 身份→路由→预扣→上游→落账→查询/导出整链，无真实供应商 |
| 前端/视觉 | 本机通常不运行重型浏览器 | Bun typecheck/test/build/i18n；真实 Chromium 操作/截图，其他浏览器按产品目标；手机/桌面实机 |
| 运行/恢复 | 不运行本机 Docker | Full/Lite push:false、服务器/个人电脑/桌面制品、三库安装/升级/切换/恢复，amd64/arm64；macOS/Windows/Linux 按声明的原生与实机 |
| 性能 | 不在生产或受限本机压测 | 固定合成负载，记录版本/数据规模/并发/配置、前后延迟吞吐内存和账务精度 |

数据库 fixture 要求显式开关、字面 loopback、专用数据库名和空库证明；在这个已隔离空库内创建旧 schema/历史行用于升级测试，不允许把非空生产 DB 当 fixture。缺 DSN 的 skip 不算实库通过。ClickHouse 不在目前 B2 job 中，需新增独立隔离验证入口。

“DB 已提交但 ACK 丢失”的测试必须实际模拟结果不明再按键回读，不能拿主动 rollback fixture 替代。no-retry 测试应从 fake Provider 实际接收计数观察，不只断言函数被调用一次。

## 17. GitHub 同步、分支与审批

- 新会话继续当前 B2 分支，不在已知脏工作树直接 switch/pull。安全收口后再决定后续功能分支与整合方式，不强制把 main 改成旧 SHA。
- 已授权阶段同步只提交本步负责文件；先检查 diff、秘密和文档一致性。PR 前读取 `.github/PULL_REQUEST_TEMPLATE.md`，核实 Git identity 和历史，不改 Git 配置，按要求如实声明 AI-assisted。
- 当前 CI 仅 main push、PR、workflow_dispatch。推荐明确采用功能分支 Draft PR 的验证入口；若尚未获 PR 写入授权，可先完成本地候选，按授权使用手动 CI。两种方式都必须核对实际运行 SHA；PR merge ref 与分支 HEAD 不同须如实记录。
- `gh` 和远程 Git 首次按审批环境执行，不在 sandbox 的网络失败后误报登录失效。绝不输出 token，不用其他工具绕过审批。
- 此前 B2-0 push 被自动审批拒绝：在安全检查无法消除阻塞时，说明准确内容/目标/原因，请求明确外发授权。不要要求用户重复“登录 CLI”。未获放行仍可推进不依赖同步的本地工作，但必需远端验证/同步完成前阶段不能验收。
- 当前发布 workflows 有额外批准和发布变量；不触发 `docker-image-branch.yml`、`docker-build.yml`、NPM/Release 等发布工作流，不修改发布门禁以跑测试。仅使用读代码 CI 和 `docker-smoke.yml` 的 push:false 路径。
- 常规阶段 commit/push/PR 与生产、merge、tag、release 不同；后者不由持续目标授权。不得因流水线失败修改无关规则或关闭必要检查。

## 18. 状态管理、阻塞和持续目标

新会话维护一张当前任务表；本文是完整工作分解，`COMPLETION_AUDIT.md` 是证据存放处，`MYAPI_MASTER_PLAN.md` 是产品合同索引，旧执行流水作历史。R0 开始时更新当前状态索引，历史 SHA 不被重写为“当前”。

持续目标建议使用完整第 1.1 节作为验收边界。若线程已有目标先读取，不创建重复目标；产品的恢复/暂停机制与完成/blocked 状态分开，不为了恢复把未完成目标标成 complete。

已确认合同内的实现、回归、审查修复与文档收口持续推进，不反复索要同一授权。真正改变历史账务、产品权益、可见性或外部写入边界时明确提出决定，并继续其他独立任务。CI/设备/账号不可用时记录具体阻塞和恢复条件，不凭“等待意图”反复轮询；只等待已证实活跃的任务 handle。

每个阶段汇报当前完成、验证、独立审查、已修复、剩余项与下一步。不以 token 消耗、工作时间、提交次数、测试数量或文档长度衡量完成度。估时以已验收工作包的实际实现/复验耗时为基础，外部设备/审批/Runner 等待单列；不能承诺新对话无条件一次完成所有外部事项。

## 19. 新会话的第一批动作

1. 只读执行 `pwd`、`git status --short --branch`、`git remote -v`、`git log --oneline --decorate -5`、`git rev-parse HEAD`，核对第 2 节已知改动和指纹。
2. 完整读取适用 AGENTS 与本文，按任务读取旧计划/审计和专题；识别哪些是已批准、建议、未实现与历史证据。
3. 确认负责人已通过交接提示词授权接管九份草稿；若有新改动停止写入报告。不要先安装、清理、切分支或 pull。
4. 建立 R0 当前任务表与完整持续目标；立即登记 D02/D03、远端外发与真实设备/账号等外部条件。
5. 在已确认 B2-0 范围修复 B2-1 五项问题；让独立角色准备三库/ClickHouse验收和 C03b-0 决策，不并行争写源码。
6. 本机串行必要定向测试→独立审查→修复复验→文档→已授权同步/CI；B2-1 未通过不叠加 durable controller 实施。
7. C03b-0 决定后进入 B3-A，按第 4 节关键路径持续完成。每个阶段保留完整终态目标，不将较容易通过的子集当作全项目完成。

本计划编制完成的标准只是“新执行者能定位现场、理解合同、知道下一步和全部退出条件”，不表示项目功能完成。
