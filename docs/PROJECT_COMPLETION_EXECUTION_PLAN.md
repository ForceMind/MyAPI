# My API 全项目完成执行计划与交接基线

编制日期：2026-09-05。适用源码目录：`/root/myapi`。

本文是负责人要求的新一轮完整计划，供新对话实施；编制本文件不等于下面所有设计已获批准或功能已经实现。已确认的 B2-0 合同继续有效；标为“建议”的商业、恢复和产品选择须在相应阶段落实为决策记录。本文不授权部署、发布、生产操作或处理历史真实账务。

## 1. 阅读路径、事实来源与最终目标

新执行者按以下顺序阅读：

1. 本目录及更具体的 `AGENTS.md`、存在时的 `/root/.codex/AGENTS.md`。
2. 本文第 2–6 节：现场、边界、依赖和决策。
3. 本文第 7–14 节：逐阶段任务与验收。
4. 本文第 15–19 节：并行、验证、同步、完成审计和启动顺序。
5. [总体产品计划](MYAPI_MASTER_PLAN.md)、[历史执行计划](DEVELOPMENT_EXECUTION_PLAN.md)、[完成证据](COMPLETION_AUDIT.md)，按任务读取本文引用的专题。
6. [新对话启动提示词](NEXT_SESSION_HANDOFF.md)。旧 [Mac 交接提示词](CODEX_HANDOFF_PROMPT.md) 和 [macOS 指南](DEVELOPMENT_ON_MACOS.md) 仅用于对应平台，不能作为当前 Linux 环境事实。

优先使用最新用户决定和当前代码/测试/远端结果；历史文档用于确定合同与寻找证据，不能证明当前状态。冲突必须明确列出并解决，不静默挑选有利于通过的表述。本文给出执行次序，不自动改写已批准业务合同。

### 1.1 必须达到的产品终态

| 领域 | 最终需要证明的结果 |
| --- | --- |
| 请求与协议 | 现有 Chat/Responses/Claude/Codex、SSE、附件及各 Provider 语义保持；现代异步 Task 的幂等、持久化、查询和恢复闭环可用 |
| 账务 | 用户钱包、Key、订阅、任务账务及派生统计具备明确事务边界；重复、并发和不确定提交不导致重复扣款、错误退款或丢失已确认变动 |
| 缓存 | 可消费余额权威源明确，Redis 丢失/陈旧/故障及 batch 退出均有可验证恢复协议；旧数据不被猜测性补扣或退款 |
| 策略 | Account Tier、Key Access Profile、模型范围、路由与计费归属真实执行；旧 Key 迁移可解释、可审计、可回退 |
| 运维与发行版 | Full、LAN Lite、Desktop 对应的安装、运行、权限、升级、备份恢复和故障路径有完整证据 |
| 额度闭环 | 采样、数据质量、分析、展示、告警事件及选定通知通道形成闭环；上游余额、组织用量、用户钱包与 Key 限额严格区分 |
| 独立产品体验 | 完整独立信息架构和视觉系统覆盖现有功能、三类用户、Full/LAN/桌面/移动、七语言及各异常状态；官网同步 |
| 交付就绪 | 每项需求有实现、适用验证、独立审查、恢复说明和必要同步证据；外部验收如实完成，无隐藏阻断 |

“交付就绪”不包含执行生产部署、创建 tag、上传 GHCR/NPM、公开网站或发送真实通知。正式发布是单独批准的后续操作，不是本计划的隐含步骤。真实账号/设备/法律验收若是完成条件，缺失时仍标为待验收；不能用“暂缓”把原目标写成全部完成。

### 1.2 保留成果，避免重做

保留 S0/S1、S2-A/R1、B1、C03a 及已验收 C/D 批次、R4、Codex 本机导入、额度分析、局部日志/权限/品牌、CLI/Electron 和无发布镜像测试的有效成果。只有本次改动影响到它们时才补对应回归。历史绿灯不覆盖新 diff，局部面板和 Logo 不等于完整独立 UI。

## 2. 交接基线与当前执行状态

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

### 2.3 当前执行进展（2026-09-05）

负责人已要求继续，并再次强调限制本机资源。当前目标保持全项目范围，优先完成 B2-1，不执行生产/发布操作。

| 工作包 | 当前状态 | 证据与剩余 |
| --- | --- | --- |
| R0 接管与调度 | 已完成当前只读核对 | 继续原功能分支，已知草稿按所有权分工；持续目标已建立；测试由主代理唯一调度 |
| B2-1 密钥/模型/迁移 | 已完成当前范围 | 2026-09-06 已补 GORM 动态表表达式与链式句柄标记泄漏保护、v1 dispatch savepoint panic 回滚；受限环境 common/model 定向测试及 model vet 通过，独立复审无 P1/P2/P3，PR #1 CI 全绿；B2-2/B3 仍未开始 |
| B2-1 实库验证 | 已完成当前范围 | Draft PR #1 的隔离 CI 已实际通过 MySQL5.7、PG9.6、ClickHouse24.8 专用空库 fixture；本机未启动这些服务 |
| GitHub 同步 | 已同步，Draft PR 待维护者处理 | `c045e42` 已推送至 `codex/b2-durable-submissions`，Draft PR #1 已创建并触发 CI；不包含合并、发布或生产操作 |
| C03b-0 | 未开始，待决定 | D02/D03 已提出：主库权威/Redis投影，以及历史无法证明余额的处理；答复前不实施不可逆业务选择 |

`fea4637` 已提交的任务文件包括第 2.1 节九个草稿，以及新增 `common/task_recovery_key.go`、
`model/task_recovery_identity.go`、`model/task_recovery_identity_test.go`、`model/task_recovery_clickhouse_test.go`，
以及 `.env.example`、本计划、`NEXT_SESSION_HANDOFF.md`、`MYAPI_MASTER_PLAN.md`、
`DEVELOPMENT_EXECUTION_PLAN.md` 和 `COMPLETION_AUDIT.md` 的任务内更新。
旧九文件指纹仅作历史追溯，不能再当作忽略脏工作树的许可。源码内容以 `fea4637` 为准；随后纯文档审计提交按 Git 历史核对。
新对话若发现未提交修改，应停止并报告具体文件与来源，得到对这些实际修改的明确接管授权后才能继续。

已实测限制：Go 使用 `GOMAXPROCS=1`、`GOMEMLIMIT=768MiB`、`-p 1`；模型测试活跃单元
`CPUQuotaPerSecUSec=1s`、`MemoryMax=805306368`。两个实施角色不运行测试，独立审查只读；没有并发构建。
详细命令、结果和后续复验见[完成审计](COMPLETION_AUDIT.md#s2-b2-1-安全持久模型2026-09-05待验证)。

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
| B2-1 安全持久模型 | 待验证 | 最终定向及独立代码审查通过，实库验证/同步仍待完成 | R0；既有 B2-0 合同 |
| C03b-0 共同账务合同 | 待决策 | 权威源、事务边界、历史不明余额、切换和回滚规则 | 可与 B2-1 修复并行设计 |
| B3-A 原子账务核心 | 未开始 | tx-scoped 余额应用、事件幂等、不可变快照、projection receipt | B2-1、C03b-0 |
| B2-2A Adapter 分层 | 未开始 | 纯解析接口、一次性请求构建；legacy 响应兼容 | B2-1 接口边界稳定；可与 B3-A 并行 |
| B2-2B/C 提交与查询 | 未开始 | 幂等预检、单次 dispatch、稳定 202/查询、受理事务 | B3-A、B2-2A |
| B3-B/C 恢复与投影 | 未开始 | poll/recovery、人工处置、outbox、查询/导出/统计去重 | B2-2、B3-A |
| C03b-1..4 全写入恢复 | 未开始 | Redis 投影、所有余额写入迁移、bridge/drain/epoch、联合验收 | C03b-0、B3-A；共享文件按所有权串行 |
| C09-N1..5 配置/限额收口 | 待细分 | 配置读写一致性、支付快照、bulk、硬限额、历史诊断 | R0 可先扫描；与 S3 接口共同冻结 |
| S3 账户/Key 强制策略 | 未开始 | 策略合同、审计模式、缓存/路由、迁移、强制执行 | 设计可提前；执行依赖账务和相关配置合同稳定 |
| S4 运行与恢复 | 部分历史范围完成 | 三库完整运行、升级/备份恢复、桌面/LAN、真实设备 | 测试设施可先建；最终验收用整合后的版本 |
| S5 额度闭环 | 分析层已有，闭环未完 | 真实采样、告警投递、身份模型及选定扩展 | 核心采样保留；外发和扩展先决策 |
| S6 完整独立 UI/官网 | 未开始 | IA、设计系统、全功能页面、响应式/七语言/实机 | 设计研究可先行；实施按稳定 API/权限分批进入 |
| S7 全项目交付审计 | 未开始 | 逐需求证据矩阵、候选制品、恢复手册、交接 | 所有适用阶段及外部验收完成；正式发布不执行 |

关键顺序是“共享账务契约和事务核心先于完整提交集成”，不是把 C03b 所有运维工作都塞到 B2 之前。B2 决定何时提交上游，B3 决定业务事件和账务状态，C03b 负责余额应用及缓存/统计投影；三者共同使用一个主库事务内核，不能各造一套权威账本。

## 5. 决策登记：集中处理真正改变产品或历史数据的选择

状态只能是 `已确认`、`建议待决定`、`外部条件待提供`。建议值不是现有事实；未经决定不启用依赖它的新行为，但继续推进独立工作。

| ID | 需要决定的合同 | 建议与代价 | 必须在哪一步前解决 |
| --- | --- | --- | --- |
| D01 | B2 未知态、重发、幂等及日志合同 | 已确认，见第 6 节，继续沿用 | 已满足 |
| D02 | 可消费余额权威源与故障策略 | 建议主库事务唯一授权；Redis 仅可重建投影；READY 后 Redis 故障可回主库，旧协议不明状态拒绝新预扣。代价是增加主库事务压力，需性能实测 | B3-A / C03b 实现前 |
| D03 | 旧 batch 已丢增量、混合版本、回滚 | 建议维护窗口先停新计费、持久 drain、对账、激活新 epoch；无法证明余额的主体隔离人工审查，禁止推测补扣/退款。接受旧 DB 基线须明确认可业务损失 | bridge 和恢复行为实现前 |
| D04 | Account Tier/Profile 权益、模型交集、disabled/fallback、计费 | 建议白名单取交集，未知/禁用新请求拒绝；fallback 不扩大权益，现代 Task 只允许 dispatch 前选择；计费快照随请求冻结 | S3 策略实现前 |
| D05 | 并发“成功请求硬限额”语义 | 建议成功数＋在途 reservation 不超过上限，失败释放，提交未知保留待核实；明确时间窗、租约、崩溃和 Redis 故障取舍，不偷偷把现有近似限流改成另一种产品 | C09-N4 前 |
| D06 | 额度通知与选定扩展 | 推荐先一个经批准的通知通道；收件人、凭据、真实发送另授权。明确多 Key 身份、Claude 组织用量、Antigravity public relay、TokenHub 扩展各自是否为本次最终交付必选 | S5 对应工作包前 |
| D07 | 恢复支持范围 | 确定最低旧版本、Full/LAN/Desktop×DB/架构矩阵、维护窗口、备份范围、允许数据损失 RPO 与恢复时间 RTO；不能靠仅“健康检查成功”验收 | S4 旧版 fixture 与最终演练前 |
| D08 | UI 方向与完整功能清单 | 建议以角色工作流组织，沿用现有技术栈；审核 IA、关键页面及迁移映射后成批实施 | S6 页面实施前 |
| D09 | 外部账号、设备、法律、签名与发布 | 指定实际验收人/设备/隔离账号；合规结论交给负责人或合格审阅者。版本、域名、tag、发布是独立决定 | 相关外部验收前；不阻断其他代码任务 |

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

### B2-2A：Adapter 纯解析与一次性请求

**文件：** `relay/channel/adapter.go`、`relay/channel/api_request.go`、`relay/relay_task.go`、十个现代 Task adapter 及各自测试。

1. 先落稳纯 parser 输入/输出接口：accepted/rejected/unknown、可信 upstream ID、受控 Task 数据和 legacy 响应 DTO；parser 不接收可写 Gin writer。
2. 分组迁移 Ali/Doubao/Gemini/Hailuo/Jimeng 与 Kling/Sora/Suno/Vertex/Vidu；保留各 Provider 响应形状及 gate-off 路径。
3. 所有 header/HTTP 成功响应由 controller 最终边界发送；不能在 Task/event 落库前发成功。
4. 一次性 body、禁止 redirect、禁止凭据与客户幂等键意外透传；检查 `GetBody`、Transport/SDK 重试等隐含重发。
5. 使用 fake upstream 对每个 Provider 验证有效受理、明确拒绝、malformed、空 ID、错误正文脱敏和 body 关闭。

本步骤可以独立验收“parser 分层及兼容性”，不能因此标记完整 durable submission 已实现。

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
| C09-N1 当前审计 | 扫描 setting/config、option、实际热读入口，分类已经快照化/仍活指针/仅历史脏值；产出有限清单 | 每项有调用者、风险、处理批次，无“全部配置以后再看” |
| C09-N2 配置读取/支付快照 | 按相关配置族收敛 runtime 读写，支付请求/验签使用一致候选和密钥版本；保留 R4 完整地址/Passkey 快照 | 并发发布、失败保留、请求内一致、必要密钥轮换窗口和 webhook 重送回归 |
| C09-N3 前端 bulk 与 API 原子性 | 设置页面跨多个 HTTP 保存改为适用的后端事务批次，验证失败不半次发布；定义哪些族相关、哪些不承诺全局事务 | 真实 handler 与浏览器：部分非法、DB 失败、并发写、旧客户端兼容、七语言 |
| C09-N4 成功硬限额 | D05 确认后实现 reservation/commit/release、幂等 receipt、lease/窗口/故障规则，覆盖内存与 Redis | 并发不会突破已定义上限；失败释放、进程崩溃、未知状态、动态配置均有测试 |
| C09-N5 历史诊断与兼容 | 只读列出 raw null、未知 key、alias 冲突及修复建议；测试副本上的规范化/重复应用 | 不自动清理生产历史记录；修复影响可预览，明确保留/拒绝/fallback |

C09-N2/N3 中会影响 S3 策略保存和读取的部分必须先于 S3 enforce；其他互不影响项并行推进，但 S7 前要逐项处置。

## 9. S3 账户等级、Key 策略与迁移

输入：[账户与访问方案](ACCOUNT_ACCESS_PROFILES.md)、setting/access_profile、model user/token/access_profile、middleware/auth/distributor、路由与 B1/B3 计费快照。

### S3-0：先冻结行为表

每个请求明确 Account Tier 来源、可见模型、Key allowlist、Profile allowlist、渠道能力与路由组如何相交；空列表代表无限制还是不允许；禁用/未知/已删除策略如何处理；fallback 何时允许、最多几次、是否跨组及费用归属。不得用 fallback 扩大用户权益，也不得越过现代 Task 的 DISPATCHING 禁重发边界。

“旧 Key 不被静默拒绝”通过迁移审计/解释和受控启用实现，不能以永久旁路强制策略实现。已经存在的 durable operation 重放继续使用原路由/价格快照，新请求采用新策略。

### 分步实施

1. **S3-1 策略快照：** 原子发布规范化注册表，版本可追溯；拒绝循环和规范化碰撞；权益/计价决策读取同一请求快照。
2. **S3-2 缓存和鉴权：** Token 缓存纳入 `AccessProfileID`、必要 schema/policy version 与失效协议；旧 cache 缺字段不能绕过强制；Key revoke/编辑后行为一致。
3. **S3-3 audit/shadow：** 先只计算新旧决策差异，记录净化原因、受影响 Key/模型/渠道数量，提供管理员解释；不改变已承诺路由。
4. **S3-4 迁移与 enforce：** 支持明确范围启用、稳定映射、未知值处理、回填/重复执行、回滚；不能直接全局打开而无差异报告。
5. **S3-5 管理与用户流程：** 用户看懂权益和选择资格，Key 创建/编辑展示模型、路由、倍率、限制、fallback；管理员可预览变更影响和审计记录。
6. **S3-6 联合回归：** 普通/管理员、旧/新 Key、所有已支持协议、缓存命中/缺失/陈旧、订阅到期、并发改策略、durable replay、同一请求价格不漂移。

**退出条件：** SQLite/两种实库迁移与策略决策、权限/缓存/race、浏览器/i18n、差异报告和关闭/回退场景通过；profile 不再只是元数据。生产迁移不执行。

## 10. S4 完整运行、升级与恢复

保留 S4-01/02 的 linux/amd64、全新 SQLite、无 Redis/batch 的 Full/LAN 合成 smoke；它们不能证明以下新增范围。

### 10.1 目标矩阵

| 维度 | 计划覆盖 |
| --- | --- |
| Full 主库 | SQLite、MySQL 5.7、PostgreSQL 9.6；项目声称支持的更新版本用代表版本补兼容 |
| 日志库 | 同库、独立 SQLite/MySQL/PG、已支持 ClickHouse 组合中的关键跨库失败路径 |
| 余额/cache | DB-only；共享 Redis；历史 batch→durable；新旧 writer 切换和恢复 |
| 运行架构 | Linux amd64 与 arm64 隔离构建/启动；Full/LAN 对应能力与禁用项 |
| LAN/Desktop | SQLite-first、单进程；macOS/Windows 安装和第二设备 LAN。若已有合同声明更多 DB，不得未经决定删除支持 |
| 数据年龄 | 空库、最低受支持旧版本、部分迁移、已存在 active/unknown/terminal 任务、已有日志与历史策略 |

### 10.2 工作包

- **S4-03 三库应用合同：** 不只是模型 AutoMigrate；完整启动、登录、Key 权限、真实路由到 fake upstream、SSE/非流、Task、预扣/退款、日志/统计和数据库重连。
- **S4-04 升级路径：** 使用固定旧 commit/fixture 生成隔离旧数据，运行新程序实际迁移；重复启动、半次失败恢复、多个节点启动竞争；检查未知态与旧 Task 归属。
- **S4-05 备份恢复：** 按 D07 备份主库、必要日志库、应用配置/加密密钥与协议/幂等标识；校验和、恢复后余额/Key/任务/事件/outbox 精确一致；证明 Redis 可重建。不能漏备 HMAC/加密密钥导致历史记录不可读。
- **S4-06 失败回退：** 明确旧二进制能否读取新 schema/epoch，禁止只切镜像却保留不兼容 DB；必要时按备份恢复，并说明已接受请求的处理边界。
- **S4-07 桌面与 LAN：** CI 原生构建、安装/首次启动/升级/卸载保留数据、单实例、端口冲突、崩溃、睡眠恢复、托盘状态、回环默认、用户确认后的私网访问、系统防火墙提示、第二设备调用。
- **S4-08 健康与运维：** liveness、readiness、DB/协议/worker 状态和 drain 入口的语义明确；UI 不在后端失效时显示“可调用”。
- **S4-09 实机验收：** 复用 [实机清单](REAL_DEVICE_ACCEPTANCE.md)，手机触控/滚动/文字、macOS/Windows 实际安装和 LAN 由指定验收者记录当前 SHA、设备和结果。

**退出条件：** 达到 D07 定义的 RPO/RTO，在隔离环境重现升级失败与恢复成功；必要真实设备证据齐全。没有设备时只完成可执行测试包与说明，整体对应项仍待验证。

## 11. S5 额度闭环与 Provider 扩展

输入：[额度分析](QUOTA_ANALYTICS.md)、[告警政策](QUOTA_ALERT_POLICY.md)、[Claude 组织用量](CLAUDE_USAGE_REPORT.md)、[Antigravity 公共接入门禁](ANTIGRAVITY_PUBLIC_RELAY_GATE.md)、[TokenHub 边界](TOKENHUB_INTEGRATION.md)。现有专题的 Provider 事实必须在真正实现时重新查官方来源。

### S5-Q1 采样可靠性和真实验收

保留已实现分析公式、reset-aware ETA、概览有界查询及详细渠道筛选；补长时间采样/多节点调度/渠道删除或禁用/凭据轮换/失败恢复/保留清理。使用合成时钟做确定性测试，并在用户显式提供的隔离真实账号上覆盖至少多个有效采样和窗口变化；短时成功不能冒充长期稳定。制定观测时长、覆盖率/失败率和数据规模验收目标并记录测量。

### S5-Q2 告警事件与通知交付

先复核已有 notifier-neutral 规则，新增持久事件与投递状态。不要直接把永久 `<subject>:<status>` 当所有未来告警唯一键，应包含可区分的状态转换或冷却周期身份，避免恢复后再次 critical 被永久吞掉。

确定一个首选通道后实现带超时、退避、次数上限、接收者权限、凭据保护和可观测失败的 notifier。重复投递不可冒充 exactly-once；通道支持幂等键则使用，无法保证时明确至少一次及重复风险。保留正常恢复、未知/无 total 不发低余额告警，失败不更新为“已送达”。Webhook 检查 SSRF/redirect，禁止通过通知配置成为任意网络代理。默认关闭真实外发，测试使用 loopback/fake transport。

**退出条件：** threshold/reminder/recovery、冷却、配置变化、重启、投递未知/重放、越权收件人、脱敏、禁用开关及管理员历史 UI 通过；真实通道验收由用户授权后进行。

### S5-Q3 多 Key 上游身份

先决定“账号”和“凭据”的身份模型：稳定、不敏感、可轮换、多 Key 是否指向同一上游账户。未经证明不得合并多个 Key 的额度或把渠道数称为账号数。实现身份和系列索引、凭据轮换关联、重复样本隔离、混合货币/计划/窗口处理，明确路由、观测与对账各自职责。

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
| 引导与概览 | 角色/发行版引导、已完成不占位、用户调用/余额摘要、管理员上游额度与运维摘要 |
| Key 与调用 | 创建/编辑/撤销、访问方案解释、模型/额度/IP/有效期、凭据只展示一次、请求示例、调用结果定位 |
| 用户与权益 | 用户管理、Account Tier/订阅权益、配额调整与审计、禁用和权限边界 |
| 渠道与 Provider | 创建/编辑/测试、显式 Codex 导入向导与 fallback、模型/路由、额度历史、凭据轮换与失败诊断 |
| 日志与任务 | 消费/完整内容日志、筛选/详情/导出、脱敏、Task 各状态查询、unknown 人工处置及审计 |
| 钱包与订阅 | 充值/支付状态、订单/订阅、失败重试、退款与账务明细，现有渠道不丢失 |
| 配置与运维 | 配置族原子保存、变更预览、通知投递、账务/恢复 backlog、健康、备份/恢复说明与只读 build 标识 |
| LAN/Desktop | 最少必要步骤、监听地址/权限状态、后端不可用、端口冲突、升级提示与保留数据说明 |
| 官网 | 独立产品说明、Full/LAN/Desktop 选择、安装文档、安全边界和发布状态；不链接不存在的正式制品 |

### S6-1 设计系统

以实际角色任务组织导航；确定字体、颜色、间距、布局、表格/表单/对话框、图表、图标、空态和交互反馈。沿用现有组件与 token 体系，不无理由更换构建框架。提供关键页面真实可交互预览，经设计方向确认后批量实施；不得用静态假数据冒充已接业务。

### S6-2..5 分批实施与回归

建议批次：身份/布局/引导 → Key/调用/用户 → 渠道/额度/日志/Task → 钱包/配置/运维 → LAN/Desktop/官网。每批都保留其他功能入口并覆盖真实 API 状态，组件测试与页面集成分开。

七语言 `en/zh/zh-TW/fr/ja/ru/vi` 全覆盖；动态文字和错误/校验需国际化，不能只做 key parity。覆盖加载、空、错误、无权限、过期、处理中、成功、冲突/重放、暂停和未知状态。手机、平板、桌面，至少 320/390/768/1440 宽度代表场景；键盘焦点、弹窗返回、对比度、读屏命名、触摸和长文字分别验证。

必须实际查看最终渲染与操作流程，修复遮挡、横向溢出、层级混乱和功能缺失。截图只证明画面，操作测试才能证明交互。七语言、长列表、图表和正文等代表负载记录性能基线与结果。

### S6-6 版本与全量视觉验收

统一可见 build/revision 来源。当前发布版本是受保护的 0.1.1，不为每次 UI 批次创建 tag；构建 SHA/运行时 revision 区分批次，正式版本号单独决定。实际检查 Full/LAN/Desktop 合理入口显示一致、无挤占布局，不声称线上已更新。

**退出条件：** 全部页面映射有实现；真实浏览器功能/视觉、七语言、响应式、无障碍、完整前端 build/test 及独立视觉/交互审查通过；真实设备按 S4 联合验收；官网一致但不发布。

## 13. S7 交付就绪与完整性审计

### S7-1 候选版本与兼容矩阵

选择一个具体候选 SHA，核对全部功能提交可达；所有后端、前端、三库/Redis/ClickHouse、root/relaykit、CLI/Desktop、安装/升级和无发布 Docker 证据能覆盖该候选。不能无限“再跑一遍”：只有代码变化、失败或未解决风险才扩展复验。

### S7-2 文档、接口和制品

更新权威产品定义、执行状态、完成审计、OpenAPI/错误码、配置示例、迁移/回滚手册、用户/管理员指南和 Linux/Mac/Windows 阅读路径。版本/清单/源码包/NOTICE/第三方许可证与实际代码一致，保留必要法律通知；技术命名沿用仓库既有规范，不擅自创造 namespace。人类品牌 `My API` 与历史 `MyAPI` 文档若冲突，按最新项目治理统一并回归，不全局替换协议标识。

生成可审阅候选源码包、检查本地 NPM pack、桌面/镜像 CI 工件与校验和，但不 publish、不建 tag、不部署。来源私有时不改变可见性，不把敏感本机交接资料装进对外包。

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
| 静态/合同 | diff、gofmt、局部语法/有限检查 | root JSON、API、发行/品牌/pack 合同完整链 |
| 业务单测 | 改动相关定向 Go，必要单 worker 前端用例 | 根模块全量、相关 race、relaykit 独立 vet/build/test |
| 持久化 | 临时 SQLite、确定性故障注入 | MySQL 5.7、PG9.6、Redis 与 ClickHouse 专项临时服务；旧 schema/并发/提交未知 |
| HTTP 协议 | 必要 loopback/fake 单场景 | 身份→路由→预扣→上游→落账→查询/导出整链，无真实供应商 |
| 前端/视觉 | 本机通常不运行重型浏览器 | Bun typecheck/test/build/i18n；真实 Chromium 操作/截图，其他浏览器按产品目标；手机/桌面实机 |
| 运行/恢复 | 不运行本机 Docker | Full/LAN push:false，三库运行/升级/恢复，amd64/arm64；macOS/Windows 原生与实机 |
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
