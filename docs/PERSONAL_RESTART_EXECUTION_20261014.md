# 新请求未知状态跨重启组合（2026-10-14）

## 当前目标与授权

当前追加阶段：第二轮真实行为测试发现 strict 人工核对丢渠道归因，按下方修复合同实施。
本片由纯证据改为实际 BUG 修复，源码候选版本准备从 beta.11 递增为 beta.12；
标签/Release/镜像未发布，历史冻结和部署数据范围待答的条件仍保持。

用户要求部署已验版本并继续下一版本开发。部署授权已经收到；历史账务冻结仍保留，
若路径涉及原库账务/schema、丢弃或迁移账号/渠道/Key，先确定该具体数据范围。
部署与本片开发分开，不以历史阻断停止独立源码开发。

部署候选固定为 `0000e8654660c01b6418313d09cae71ff933f243`，已验应用为 `d305bdc`。
PR #12 最新收口 CI `38393556593` 的十项已回读 success，Docker smoke skipped；
此前准确应用 CI `38390080664` 及修正后 13 图独立视觉复验保持其已声明范围。
PR #12 仍 Draft/open，依赖冻结 PR #11；main `92fa1182`、冻结 base `0b51bb4`、beta.10 `631b3205` 不变，beta.11 标签不存在。

已采用原生 `mode=ro`、`query_only`、单一短读事务核对当前实例的启动元数据，未调用应用初始化。
实际 writer/cache 配置与历史 execution schema 不满足候选启动门禁；私有来源路径、行数、金额、
Key 和原始报告不提交。`model/main.go` 先检查 writer、金额表示和 execution，再打开普通数据库连接及迁移。
不能通过关闭 batch、添加空 Redis、创建历史 gate/permit 或清理旧记录绕过。
并行空实例与原库升级的数据保留方式已向用户提出具体选择，答复前不执行依赖该选择的部署。

部署准备已启动现有手动
[Docker smoke 38430541538](https://github.com/ForceMind/MyAPI/actions/runs/38430541538)，
实际 head 为固定候选 `0000e865`，源码分支继续保持该 head。
复用现有 Full/LAN 构建、隔离全新安装、历史启动拒绝/旧镜像恢复及源码安装合同，
`push:false`，不创建公开 Release/GHCR 标签，不把七天测试 artifact 称为已发布生产镜像。
只有在准确源码/manifest/镜像身份与适用安装验收通过后才考虑加载可回滚候选。
该 run 的五项现均 success：Full/LAN 制品、LAN 源码新装、Full/LAN fresh SQLite、
Full handoff 拒绝/恢复演练。固定 Full 制品已下载；源码/tree/run/attempt、归档长度及 SHA256 已核对，
manifest SHA 与成功消费 job 的外部 `FULL_MANIFEST_SHA` 一致：
`289775e444adebf103afe856cad9cf003a786274cf30f7a9c8e744e3efe68a73`。
应用镜像 ID 为 `sha256:1a60ddef4c226cc57d4e6cdcf1c2b21ef7cc32d72da75a65d11a0884a3d5a3f4`。
这些是无生产数据的测试制品及合成证据；未加载或启动本地镜像，未部署，
部署方式仍待用户的数据范围选择。手动 run 的 personal mode 为 0，不另称已运行该 run 的 Redis 个人旅程。

## 本片范围与完成条件

从上述固定候选新建 `codex/personal-restart-20261014`，沿用已批准的
[个人核心卡](PERSONAL_CORE_DELIVERY_CARD.md)及[上一批下一片合同](PERSONAL_WORKBENCH_EXECUTION_20261013.md#剩余个人核心与下一片合同)。
本片首先补组合证据，缺测试不等于缺功能；已有行为通过时不改生产代码。

1. 用真实 Router → relay → 本地合成 TLS OpenAI upstream 发出已有资格的原生 Responses 严格 Token 请求。
   input_tokens 计数与生成端点分开，生成 transport 前精确绑定本次 token/request 并确认 sent。
2. completed 响应缺可信 usage 时保留 durable unknown、严格 Token 和原 writer 预留，不生成消费结算。
3. 丢弃首请求上下文/会话，关闭原测试 DB pool，以相同持久输入新建连接和 Gin engine。
   重新读取持久 pending；同 Key 新请求精确返回 typed pending，input-count 和 generation 次数均不增加。
4. Root 通过已有恢复 HTTP API 提交同一可靠 input/output 证据两次，严格预算、消费日志、决策及原
   wallet/Key writer 只结算一次；保留精确整数值、原 request 身份、余额及请求计数不变量。
   合成证据给出 input=100、output=10、内部核对额度=120，Token 用量应为 110。
   内部核对额度由现有 API 显式接收，不把它称为按官网 USD 单价推算的金额或 Token 数。
5. 真实 Router 组合首先覆盖 SQLite 重开与 legacy/authoritative 两 writer；现有
   `TestTokenBudgetConfiguredDatabases`、`TestR1UsageReviewConfiguredDatabases` 继续覆盖
   SQLite/MySQL 5.7/PostgreSQL 9.6 的底层锁、持久与幂等合同，不冒称 Router 组合已在三个引擎运行。
   生产数据库路径或 schema 若确需改变，先形成补充合同，不在本片暗加。
6. 准确新源码完成针对性两 writer 组合、现有 assigned/native race、配置三库、完整 Go/独立 relaykit
   和相关 CI；独立审查确认实际 HTTP 链、重开与单次结算证据。源码/测试/文档正常推送独立 Draft PR，
   回读 head/tree 与执行身份；不沿用旧绿灯声称新测试已执行。

不扩模型/协议资格、严格 USD、百分比分摊、failover 或商业拆除；不触碰生产和旧记录，
不恢复任何 `DO_NOT_RESTORE` 副本，不创建发布标签或推动 main。VERSION 先保留未发布 beta.11，
测试证据批次以准确提交标识，实际应用行为交付时再统一版本准备。
上句是首次仅测试阶段的决定；当前实际渠道修复依据下方追加合同统一准备 beta.12 源码版本。

## 分工、资源与失败处理

主代理负责范围、分支、CI、文档、GitHub 同步及部署决策；已创建本轮持久 Goal，无用户指定预算。
部署边界规划由 `deploy_boundary_20261014`（gpt-5.6-sol/ultra）只读审查；
组合阶段规划/实现由 `next_core_plan_20261014`（gpt-5.6-sol/high）负责。
Terra 不在当前可调用模型列表，本片使用已有 Sol 组合；独立审查另由非实现者负责。
实现者独占 `router/assigned_budget_flow_test.go` 与 `router/chat_budget_flow_test.go`，
仅为本片增加真实组合和可重开 fixture。主代理持有文档/CI，不与实现者同时写测试。
并行不超过会话四槽，不递归派工；构建/行为/浏览器由 Actions 执行，本机仅源码与静态检查。
复用现有临时数据库服务，不新增服务、权限或执行预算；不产生本机依赖、大缓存或 DB 副本。

先定位首个失败，保留失败版本及原输出；不删除断言、放宽精度/身份或修改协议来制造通过。
若需生产逻辑修复，由主代理依据实际失败证据串行分配；历史恢复、schema、付费/真实上游或
数据迁移需求独立处理。未解决项与下一步保持可追踪，生产升级资格不由合成 CI 推导。

## 状态

| 项目 | 状态 | 证据/下一步 |
| --- | --- | --- |
| 固定部署候选与远端回读 | 已完成 | `0000e865`，收口十项成功，发布镜像尚不存在 |
| 原库部署条件 | 未满足 | 只读元数据证实 writer/cache 与 execution 阻断；数据保留选择待答 |
| 固定候选 Docker 准备 | 已完成 | 手动 `38430541538` 五项 success；Full 制品身份和外部 manifest SHA 已验证，未加载/部署 |
| 新请求跨重开组合 | 待验证 | 已实现两 writer SQLite 组合，复用现有三库原语合同；不是完整进程重启或生产恢复证据 |
| 准确源码 Actions 与独立审查 | 待验证 | 首轮实际失败已定位，manual 合同修订再次静态独审通过，等待修订 head 的 Actions |
| 新开发批次提交与 Draft PR | 进行中 | `eed63b5` 已正常推送 [Draft PR #13](https://github.com/ForceMind/MyAPI/pull/13)，远端 head 已回读；当前修正测试合同 |

首轮只读独审指出：仅证明最终余额和 Token 预留，不能排除原 writer 在 unknown 期间先退款、
核对时再扣款的错误实现。已补齐 unknown 后及重开后、核对前的原 writer 持久身份/预留、
用户与 Key 中间余额，以及 authoritative fact 双腿已应用断言，未删减原断言。
复审又核清测试场景的原 writer 预扣为100，严格 Token 预留为120；
新测试显式保存、设置并还原 `constant.CountToken=false` 固定该场景。
原 writer 两次保持100，恢复明确核对金额120，仅补差20，Token 实际使用110。
非实现者 `deploy_boundary_20261014` 对首轮代码静态复审通过；随后实际 CI 暴露下面的合同错误，
静态结论不覆盖行为失败。行为、race 与包级/既有三库回归仍以准确新 head 的 Actions 终态为准。

## 首轮实际失败与合同纠正

源码 `eed63b50802ed25bddd84a29a5557e1fa3eae984`，tree `36e90b24788ee25a490678971bba6d7c808cc203`，
[CI 38433585815](https://github.com/ForceMind/MyAPI/actions/runs/38433585815) 的完整后端在
`TestAssignedAccessStrictResponsesUnknownReopenRecoveryBothWriters/authoritative` 失败：
`assigned_budget_flow_test.go:347` 期待自动 settlement fact 一条，实际零条。
失败提交、原始 GitHub 作业日志及本地私有副本保留；不是已通过的行为证据。

代码回读确认人工核对经 `ReconcileUsageReview` → `ResolveAccountQuotaUnknownUsage` →
`applyAccountQuotaTerminalReviewed`，使用原 terminal mutation 事务回执，不创建自动结算 fact。
首轮实现及静态审查误把自动结算合同用于人工核对。下一修订以真实 manual decision、
terminal receipt/obligation 的身份及终态、精确双边余额和幂等补差证明一致性，
同时确认该人工路径没有新增自动 fact；不为满足错误断言修改生产路径或重复创建债务。
首轮最终为 failure：后端上述测试失败，其他九项 success；后续 race 因完整测试提前失败未执行，
不能用另外九项或旧 race 结果代替。修订测试已明确自动 intent/fact 为零，
补齐 decision actor/user/token/channel/model、120/100/10证据与投影标记；
head、唯一 terminal receipt、已完成 lifecycle 的原 reserve/writer/fingerprint/audit 身份相连，
RequestedDeltas=AppliedDeltas、wallet=-20、Key remain=-20/used=+20、subscription=0，
并保留中间预留、最终余额及单日志/决策/回执断言。
非实现者再次逐行复审通过；格式与差异检查通过，行为仍待修订准确 head 的 CI。
下一片个人策略提取仍等待当前准确源码行为 CI 成功，修订执行身份与终态在 Draft PR #13 回读。

## 第二轮真实失败与实际渠道归因修复

修订源码 `068a012e98b92023b519a5fdea20d12c0e53ec28`、tree
`379691ff4576f7a521a7e3ac56f7e34aa56a8783`，实际合并检出 `b09884271da4debb843423e58616144bfed1af80`
已回读同一 tree。[CI 38436481872](https://github.com/ForceMind/MyAPI/actions/runs/38436481872)
后端两 writer 均在 `assigned_budget_flow_test.go:323` 失败：manual decision 期待渠道1、实际0。
原始失败作业日志保留，不把另外成功项替代这一失败或称后续 race 已执行。

两位只读审查者与主代理确认实际缺口：`PreConsumeBilling` 在自动选渠道前执行，
legacy reservation / authoritative reserve receipt 因此合法冻结渠道0；
严格 Token 预留在选定渠道后、生成派送前已把同 request/user/token 的正渠道持久化。
`GetUsageReview` 校验归属后未使用这份证据，导致后续新 manual decision/消费日志仍渠道0，
`ProjectUsageReview` 因而跳过渠道用量累计。这里的正确行为不能通过把测试期待改成0实现。

最小修复合同：

- 仅修改 `GetUsageReview` 的 strict 读模型：在已有 request/user/token 归属校验后检查正渠道，
  缺失0用本请求 strict reservation 的渠道补入内存视图；已有正值同值保持，冲突返回
  `ErrTokenBudgetConflict`。不读取当前渠道/配置/原始日志，不推断未知身份。
- 保留原始 `OriginModelName`；出站 `budget.ModelName` 可能有映射，不覆盖或直接比较。
  原状态/action资格保持，不扩协议；不修改旧 receipt、legacy row、strict reservation、
  fingerprint、金额、既有 decision 或历史统计，不增加 schema/worker。
- Router 保留0→最终正渠道的真实断言，新增消费日志归因及渠道用量只累计120一次；
  同时证明原预扣和 terminal 继承的冻结渠道仍0，金额/Token/unknown/幂等合同全部保留。
- 既有三库 `selfUseCoupledReviewDatabaseContract` 用真实 Channel fixture 验证两 writer 的
  view/new decision/日志/渠道累计与原预扣证据不变；SQLite轻量负例验证冲突和无效渠道拒绝，
  非 strict/既有 decision 保留。新商业隔离测试只是另一片已批准边界的准备，不提取生产策略。

所有权：`personal_dependency_plan_20261014`（派工Sol/ultra）独占
`model/usage_review.go`、`model/usage_review_database_test.go`、
`model/usage_review_channel_attribution_test.go`；`next_core_plan_20261014`（派工Sol/high）独占
两 Router 测试及两隔离测试草稿；非实现者独立复审；主代理持有版本/文档/CI/Git。
不递归，四槽上限，本机只静态检查；Actions执行实际行为/race/三库与必要镜像验收。

此次实际应用行为修复按版本约定准备 `0.2.0-beta.12`：统一 `VERSION`、根 `package.json`
与部署默认引用。`web/`、`electron/` 的私有构建包版本1.0.0是各自工具元数据；
应用显示版本仍取统一 VERSION。没有创建 beta.11/beta.12 标签或 Release，没有覆盖 beta.10。
已验 `0000e865` beta.11 制品保留为当时证据，它不含本修复；实际部署目标需修复准确 head
完成新的验收并明确数据范围后固定，不能沿用旧镜像声称已修复。

自行复核：变化只补已存在且归属相符的持久渠道证据，不新建账本或改变预扣身份；
冲突保持未知，金额与冻结历史保持原值。三库夹具要有真实渠道，防止0记录掩盖缺表/漏统计。
本片最可能风险是模型语义混用和回填旧决策，已明确禁止并安排对应回归。
下一片纯策略提取仍待当前实际修复准确 CI 成功；当前阶段状态为修复中、未验证、未部署。

本轮代码检查点：最小11行读模型修复、Router映射/渠道一次投影、三库真实Channel及两轻量canary均已实现；
非实现者对最终代码静态复审通过，无剩余阻断。gofmt/diffcheck通过，未运行本地Go/DB。
行为仍待第三轮准确源码Actions，当前状态为待验证。纯策略提取未开始，历史和生产保持原状。
