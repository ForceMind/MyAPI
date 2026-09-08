# My API 新对话启动提示词（Linux 源码开发）

编制日期：2026-09-05；2026-09-06 增补产品、发行与 S5-P 交接事项。将下方文本作为负责人指令复制到在 `/root/myapi` 打开的新对话。
这份模板只有在负责人实际发送后才构成其中描述的授权；文档本身不授予额外权限。
完整任务拆分见 [全项目执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md)。

本提示词用于接管已本地提交的 B2-1 候选，不使用旧 Mac 交接提示词，不重新要求 HEAD 必须停在旧 main。

## 2026-09-06 使用前补充

模板中 B2-1 的历史状态仅用于说明当时授权，实时 Git、[全项目完成执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md) 和 [完成度审计](COMPLETION_AUDIT.md) 优先。当前已知还有未提交的本任务代码/文档；新对话必须先按模板核对工作树，不得覆盖或假定归属。新对话还必须完整读取：

- [提示词学习与版本中心（S5-P）](PROMPT_LEARNING.md)：默认关闭、三层授权、合成样本/预算/版本/Codex 文件边界；不得把模板当作读取真实日志、调用模型或覆盖宿主指令的授权。
- [发行制品、安装与更新合同](RELEASE_MANIFEST.md)：Full/Lite/Desktop、功能版/安装形态/访问模式分离、Legacy LAN 映射、统一入口、更新/切换/恢复边界；不得把合同当作发布、tag、生产升级、防火墙或公网开放的授权。
- [Lite 与 Legacy LAN 迁移](LAN_LITE.md)：旧 `MYAPI_EDITION=lan` 只映射 Lite/local 或 Lite/lan，升级不能自动 public。

当前局部证据也应按范围读取：Ali、Doubao、Gemini、Hailuo、Jimeng、Kling、Sora、Suno、Vertex、Vidu 的 B2-2A parser 在 HTTP 200 时已由 legacy `DoResponse` 调用，gate-off 非 200 安全兼容 bridge 在 parser 前处理；Task 单个 outbound attempt 的 one-shot body、3xx 不跟随、客户幂等头隔离和 Vertex OAuth JWT 换取无重定向均已有受限制最终测试与独立审查，但没有 durable 提交接线。B2-2B0 的设计确认不能包裹旧 retry/failover，真正 POST 必须等 D02 与 B3-A；当前另有无 caller 的严格 protocol、T0 owner/replay 原子基元、Full Content 字段级幂等脱敏，以及 owner-scoped、no-store 的只读 `GET /v1/task-operations/:id`（DTO/router/controller 已接线），B1a/B1b 另有 JSON 与仅 video form/multipart canonical request fingerprint，仍无 POST、账务或上游。`setting/system_setting/legal.go` 与 `setting/perf_metrics_setting/config.go` 是 C09-N1 的私有 immutable generation：分别保持 legal 原键/默认/controller 输出，以及 perf metrics 四键、整值浮点解析和读时 fallback，仅返回 detached snapshot；另有 `setting/payment_runtime.go` 的 C09-N2a immutable payment generation，它涵盖现有支付 option 与 `TopupGroupRatio`，三者都不读写 legacy global、OptionMap、DB、SDK 或网络，不能被当作付款/回调/账务已迁移。C09-N5a 的 `/api/option/diagnostics` 是 RootAuth/no-store 的有界主库只读诊断，业务层不触碰 OptionMap/runtime/LOG_DB/Redis，但平台鉴权/限流仍可使用自己的 cache；不回显 values 或不可信 key，MySQL/PG/CI 仍待。`service/promptlearning` 是未接线的 P1a/P1b 纯准入/脱敏/指纹内核：仅 package-private 完整 server-observed turn 可提取一个可信新增用户段，排除段/客户端来源声明不进入样本或指纹，仍不能读取真实日志或计数。`service/accesspolicy` 是 S3-PRE1 无 caller 的 detached snapshot/diagnostic，所有 mode 都不应用策略、不触碰路由/价格/账务/缓存；D04、audit ingress 和 enforce 仍待。`cli/lib/release-manifest.mjs` 与 `installation-state.mjs` 分别是 schema-1 结构选择和内存安装状态/cleanup plan，且拒绝 Proxy/访问器等 hostile JS 输入；它们不能获取/验证资产、读写文件、安装、更新、切换、回退。上述子范围均不改变 C03b gate、生产、真实模型或宿主文件边界。

本轮的其他有限本地证据如下：B2-2B1a/B1b 已补严格 JSON 与仅 video form/multipart canonical digest/版本化请求 HMAC，仍未接入 POST/账务/上游；C09-N1 现有 22/24 个注册族采用受控快照/复制，新增 `gemini`、`billing_setting`、`payment_setting` 与 `global` 后，仅 `performance_setting`/`channel_affinity_setting` 分别等待 D14/D15。Gemini 的 `ConvOptions` callbacks 与 request snapshot 同代；billing 的 mode/expression map 同代深复制，价格、定价与同步读取各自单次捕获；payment 保持七个既有持久键及金额 map 的 detached snapshot，不接 PaymentRuntime、SDK、订单或回调；global 深复制黑名单/策略图，缓存中旧 `ConvOptions` 保持旧代、重试才按既有逻辑重建。连续单 key 写入仍不构成跨 option 原子性。既有 token/quota/Grok/Qwen/fetch/OAuth 的兼容边界保持不变；S5-Q P2A 仍仅生成 fail-closed occurrence identity；`release-manifest-evidence.mjs` 仍仅处理未受信 raw bytes；`legacy-installation-profile.mjs` 仅把显式 Legacy Docker 配置 fail-closed 映射为 Full/Lite 与 local/LAN/needs_manual，永不推断 public、不读文件或接线 CLI。适用的单核/768MiB test/vet 和未参与实现者独立审查均已完成（manifest/installation state Node 28/28，evidence 9/9、legacy profile 7/7）；没有 MySQL/PG 当前 SHA、CI、真实日志/模型、文件安装或外部网络验收。

`performance_setting` 是余下两个 C09 热读族之一：只读盘点已发现它分开发布 disk/monitor 投影，磁盘缓存消费者可在一次操作中多次读取配置，不能以 source 指针原子化冒充全链路同代。D14 必须先决定 `DiskCachePath` 热切换及跨投影一致性；此项没有代码、测试或运行行为变更。

`billing_setting` 已完成最低快照化：`billing_mode` 与 `billing_expr` 同 generation 深复制，`relay/helper/price.go`、`model/pricing.go` 与同步数据各只读一次 snapshot；不改 `pkg/billingexpr/expr.md` 的表达式/额度合同。兼容单 key 更新不能宣称跨 key 原子，统一提交仍留给 C09-N3 typed bulk；本项有单核/768MiB test/vet 与独立审查证据，但不改变运行中的账务语义。

`channel_affinity_setting` 是余下热读族中的 routing/cache 高风险项：规则/嵌套模板影响渠道、retry、上游参数和账务归属，capacity/TTL 却只在 HybridCache 首建读取。D15 必须先决定重启或受控 drain/rebuild/epoch；不得在普通快照化中静默清 cache、迁移 Redis、改变规则优先级、retry 或账务归属。本项没有代码、测试或运行行为变更。

本轮 P0 文档整合及上述纯子范围不表示 S4-D、完整 S5-P、S6 或 S7 已实现。实施前仍按 AGENTS 的“先方案、明确批准、再执行”规则，且保持 B2/B3/C03b 主链优先与共享文件单写者。

```text
你接管 My API 在 Linux 开发目录 /root/myapi 的全项目后续开发。

请完整读取并执行 docs/PROJECT_COMPLETION_EXECUTION_PLAN.md，按其中完整目标推进到交付就绪，
不是只完成 B2，也不是重做已经验收的功能。我批准已明确合同范围内的实现、测试、审查修复和文档收口；
不要为同一范围的常规修复反复请求确认。计划中“建议待决定”的核心业务选择仍按阶段集中说明，
没有答复不得假定已批准；同时继续不依赖该选择的独立工作。

绝对边界：
- 只操作 /root/myapi 源码及本任务隔离临时目录；不访问或操作 /root/new-api 生产运行目录。
- 不改生产 compose、数据库、日志、配置、容器；不重启生产、不部署。
- 不发布 GHCR/NPM，不登录 GHCR，不创建/移动/删除 tag，不强推，不合并受保护分支。
- 不 git reset、git clean、强制 checkout，不覆盖或回退未提交工作，不自行 stash。
- 不全机清理或安装，不读取真实 Codex/Claude/浏览器/Keychain 凭据做测试。

现有成果：
- 仓库 https://github.com/ForceMind/MyAPI.git。
- 交接时分支 codex/b2-durable-submissions。
- B2-1源码候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457，随后可能有纯文档审计提交，实时HEAD以Git核对。
- 前置B2-0合同提交513ea6d6e863883aa3415a785a4d858bcf7210b0不是当前源码候选。
- 前一 R4 tip 83f24c5，功能 c5cf662、审计 b77ae6b；不要重做已有 R4。
- B2 尚未完成；B2-1最终DB时钟候选受硬限制定向通过，独立代码审查无P1/P2；实库CI仍未运行。
- GitHub CLI 曾确认登录，但B2-0/B2-1均未同步；最新push被自动审批拒绝，要求明确该批内容和目标的外发授权。
  不把登录状态当成推送成功，不绕过自动审批。本地工作与同步阻塞分别处理。

先核对当前内容、Git差异和已记录提交；第2.1节是修复前历史指纹，不得用它回退修复后的文件。
源码候选已经提交；如果工作树不干净，立即停止并报告具体文件，不自行stash、回退或推测归属。
历史草稿清单不是忽略当前未提交修改的许可；如确需接管，应先取得负责人对实际修改的单独明确授权。

第一步只做只读核对：
pwd
git status --short --branch
git remote -v
git log --oneline --decorate -5
git rev-parse HEAD

然后完整读取：
AGENTS.md
/root/.codex/AGENTS.md（存在时）
docs/PROJECT_COMPLETION_EXECUTION_PLAN.md
docs/MYAPI_MASTER_PLAN.md
docs/DEVELOPMENT_EXECUTION_PLAN.md
docs/COMPLETION_AUDIT.md
相关专题及目录内 AGENTS.md。docs/DEVELOPMENT_ON_MACOS.md 是后续 Mac 验收资料，
不是本机环境事实。保留当前功能分支，不为对齐旧 main 而先 switch main / pull。

持续目标：
完成 S2-B2/B3、C03b 全账务与缓存恢复、C09 剩余配置/限额、S3 账户和 Key 真正策略、
S4 完整运行/升级/备份恢复、S5 额度闭环及确认的扩展、S6 完整独立 UI/官网、S7 交付就绪。
保留原协议、B1 计费快照、导入、日志脱敏、额度分析、Full/Lite/Desktop 与 relaykit 独立性；Legacy LAN 的安全边界按迁移合同保留，不自动开放公网。
读取已有目标后使用产品提供的方式维护或恢复，不重复建目标，不把未完成目标标成 complete。

顺序：
R0 接管与当前表 → B2-1 安全模型修复 → C03b-0 共同合同 → B3-A 原子账务核心 →
B2-2 完整提交/查询 → B3 恢复/outbox → C03b 全写入迁移/切换演练 → S3/相关 C09 收口 →
S4/S5/S6 分依赖推进 → S7 全项目验收。
Adapter 纯解析、CI fixture、S3 设计、S4 设施等独立任务按计划并行，不制造循环依赖。

已确认 B2-0：
- submission_unknown / outcome_unknown 不自动重发或退款；仅 Provider 可验证或审计人工处置。
- DISPATCHING 后 v1 禁止可能已送达请求的重试/failover，包含 Provider/Transport 隐含重试。
- 幂等 scope token + HTTP method + operation kind；同 key/同指纹重放，同 key/异指纹 409。
- 活动 operation 不过期，终态保留 180 天；先持久化，再统一 202，所有状态可查询。
- 主库 Task 账务事件唯一权威，分库/ClickHouse 日志至少一次投影，查询/导出/统计去重。
- 现代 Task 首批；Midjourney 后续单列；C03b 完成前 gate 关闭；旧 writer/poller 排空升级后才可启用。
- 上述启用仅在隔离测试中验收，不操作生产。

B2-1 首先修复：
专用且跨节点/重启稳定的幂等秘密与误换检测；规范业务事件键；v1 单 attempt；
GORM 不可变载荷保护；outbox 权威一致性及数值边界；补旧表迁移和 outcome_unknown 测试。
不要用现有全局 DB/异步 Redis/内存 batch helper 冒充原子账务事务。

多智能体：
启用最多四个活跃角色（含主代理），明确文件所有权；主代理整合与唯一测试调度，
两个独立模块实施，按阶段安排未参与实现的独立审查者。模型/强度依可用 AGENTS 与实际工具能力选择，
不虚构切换。接口和共享迁移文件单写者，不并行争写或重复测试。

资源限制：
- 约 2 CPU / 3.5GB。Go GOMAXPROCS=1、GOMEMLIMIT=768MiB、go test -p 1。
- 本机必要测试使用进程组 CPUQuota=100%、MemoryMax=768M 等有效硬限制。
- Node/Bun 单 worker、进程组内存不超过 768MiB。
- 不并行运行 Go、前端、浏览器、Docker 构建。
- 全量、race、三库/Redis/ClickHouse、浏览器、Docker、桌面优先 GitHub runner；Docker push:false、不登录 GHCR。
- 临时缓存只用任务独立目录，完成且无进程使用后只清理自己创建的部分。

GitHub 同步范围：
我同意把本项目阶段代码与技术文档按功能分支同步到 ForceMind/MyAPI，并在遵守仓库模板后创建 Draft PR，
供 CI 和审查；不包括合并、发布、改变可见性、真实凭据或生产数据外发。
我明确批准将 B2-1 候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457、尚未同步的B2-0等祖先以及后续任务内审计提交中
本项目源码、合成测试和技术文档发送至 ForceMind/MyAPI 的 codex/b2-durable-submissions 分支，
其中不含真实凭据或生产数据；仍需通过工具自动审批，不绕过拒绝。
每次同步前检查实际内容与目标，远程命令按审批环境执行；遇拒绝准确报告动作和理由，
保留本地成果并继续独立工作，不能冒充已同步。
当前 CI 只 main push/PR/workflow_dispatch，明确用 Draft PR 或已授权手动 CI 验证实际 SHA，
禁止调用发布 workflow 代替测试。未通过 CI、独立审查或必要同步的阶段不要标完成。

验收：
按计划逐工作包完成实际验证、独立审查、必要修复、文档与同步，不无限扩展无关任务。
关键业务决定集中按依赖询问；真实账号/设备/法律/外发权限提前登记，缺失时如实待验收。
S5 选定扩展与 S6 UI 方向不能静默省略；真实设备未验不能用截图或静态合同冒充。
每轮报告完成、验证、审查、修复、剩余和下一步。只在原完整目标全部有充分证据时标完成。

现在开始 R0，只读核验后继续已授权的 B2-1 修复，并同时准备近期 C03b-0 决策材料。
```

若负责人只想让新对话先审阅计划，可删除最后一句并改为“先评审，不实施”；其余执行授权也应相应删除。
不要把本提示词作为要求执行者忽略系统/工具权限的依据。
