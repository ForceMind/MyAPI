# My API 完整首个预发布执行计划

计划版本：2026-09-15-full-beta-v1。状态：执行中；2026-09-15 已进入 A0，不执行未经单独授权的外部发布或生产部署。

> 最新交接：用户已要求移交 Claude，Codex 实现代理已中断；当前仍在 A1，详见 [Claude 接手记录](CLAUDE_HANDOFF.md)。WP3-B2 最后一轮 capability CAS 与前端第22批已于 2026-09-15 完成验收（见 A1 进度）；最新 Goal 查询为 null，以下 Goal 及测试描述保留为历史进度，不代表当前全部变更通过。

## 1. 唯一目标与优先级

用户已明确：达到最早完整产品计划的全部必选目标后，才允许交付首个预发布版本；精简不必要的模拟测试与重复流程；后续由 Sol 和 Terra 开发。

本计划取代此前对话中提出的小范围 beta 草案，并作为唯一首版执行计划。原 [总体产品计划](MYAPI_MASTER_PLAN.md)、[完整需求和契约](PROJECT_COMPLETION_EXECUTION_PLAN.md) 继续作为需求来源；其中历史进度不是当前事实。后续执行以最新用户决定、本计划和当前代码/证据为准。

最终交付：完整 Full/Lite/Desktop 产品、独立 UI/官网、账户与 Key 强制策略、调用与持久账务恢复、额度与通知闭环、提示词学习与版本中心、统一安装/更新/切换/恢复，以及匹配版本的制品和中文/英文操作文档。

建议版本仍为 `0.2.0-beta.1`，版本号待最终封版确定。必须功能完整并通过适用验收；beta 表示首次预发布，不用来放宽账务、权限、数据恢复或交互要求。

## 2. 接手基线与已有成果

2026-09-15 本会话核对基线：`codex/mac-durable-accounting@504d4beb2f5ca72e5ebdb7462dc7416da5cd92ea`，远端分支同 SHA；110 个跟踪文件修改、37 个新增业务/测试文件，另有本会话计划文档。执行时只刷新差异和变化项，不重新做一次全项目盘点。

- S0/S1、已验收支付/协议/数值安全修复、Codex 导入、现有额度分析、CLI/Electron 等成果保留。旧结果在输入和环境适用时复用。
- M1–M4 与 WP3-A 已有实现和本机验证记录；外部数据库、完整集成和启用条件未全部验收。
- WP3-B、渠道路由 R1、资金模式 C0、Codex 窗口改动保留并接续，禁止 reset/stash/删除功能或为清洁工作树盲目提交。
- 已复现：`router/relay_router_test.go:108` 的非主节点空库 fixture 缺 `logs`/`billing_log_projection_identities`，沙箱外单测退出 1。先修测试初始化并检查真实启动，不把这个 fixture 失败直接当作生产事故。
- 生产 writer 注册仍有 11 项 `Migrated:false`；须按真实调用链迁移，不直接改标志冒充完成。
- 当前开发分支 GitHub Actions 查询无运行记录。现有 CI 入口为 main push、PR 和 manual；后续应使用明确的实际触发方式。
- 当前产品版本 `0.1.1`；历史 tag 不覆盖。当前会话 Goal 为空，执行主代理启动时重新读取。

## 3. 完整交付清单

| ID / 原计划 | 必须完成的用户结果 | 完成证据 |
| --- | --- | --- |
| F1 / S2 B2/B3/C03b | 普通请求和异步 Task 从预扣、提交、结果、结算/退款到日志形成可恢复闭环；所有余额写入使用统一主库权威；Redis 为投影；未知结果可查询和人工处置 | 接通真实应用路径；重复/并发/提交未知/重启恢复不多扣、不错退、不丢确认数据；writer 迁移和隔离环境切换通过 |
| F2 / S2 C09 | 支付运行配置、typed bulk、剩余热读设置、成功请求硬限额、历史配置诊断及受控修复可用 | 保存成功后持久状态与运行配置一致；失败不半生效；并发硬限额不会超卖；恢复操作可预览和审计 |
| F3 / S3 | 账户等级、Key Profile、模型/路由权益真正生效，旧 Key 可审计迁移 | 请求使用同一策略快照；先 audit 再范围启用；无权益扩大；禁用、撤销、订阅变化、回放和回退正确 |
| F4 / S5-Q | 渠道额度当前/历史准确、采样稳定、告警持久并可送达，账号与凭据身份明确 | 套餐和窗口按最新上游数据显示；消失的5小时窗口不伪装当前值；历史保留；凭据轮换、重置、失败、恢复、提醒/通知重试一致 |
| F5 / S5-P | 授权样本→脱敏→调度/预算→生成→不可变版本→差异/导出→人工应用/回滚完整可用 | 默认关闭；用户/项目/Key 隔离；撤权生效；后台调用走统一权限和账务；旧生成不覆盖人工编辑；授权目标文件可备份、校验、恢复 |
| F6 / S4 | Full、服务器/个人电脑 Lite、Desktop 可安装运行，统一入口支持安装、更新、切换和恢复 | Release Manifest 验证、单安装锁、持久 journal、备份、排空、失败恢复实际可执行；保留账号/Key/余额/日志/学习数据；local/LAN/public 状态准确 |
| F7 / S6 | 完整独立管理界面、用户界面、Lite/Desktop 体验和官网覆盖全部功能 | 全部旧路由/新功能有页面映射；七语言、手机/平板/桌面、键盘、加载/空/错/权限/处理中/未知状态可操作；官网入口对应真实制品 |
| F8 / S7 | 可安装且可追溯的首个完整预发布候选 | 必选需求清零；独立审查无阻断；版本/源码/镜像/原生/Desktop/NPM/清单一致；安装恢复及文档完成；发布后回读另记 |

F1 包含所有现存余额写入者：普通 BillingSession、User/Token、订阅、充值、兑换、签到、邀请奖励、admin、Task 及统计/日志投影。Midjourney 必须审计其专有 submit/notify/poll/结算；有同类缺口则修复，不用现代 Task 的通过代替。

F1 还须提供实际受控切换操作面，并在隔离环境证明 `legacy → bridge → authoritative`：所有登记 writer/poller 排空、在途归零、基线核对、backfill、投影积压和 Redis epoch 校验。当前 planner/缺失条件提示不等于可执行 apply。新提交关闭时已有恢复义务仍须可完成；权威模式不可盲目降回旧 writer。

F4 的多 Key 身份和系列隔离须完成；不能把多个凭据默认合并为一个上游账号。Claude 组织用量、Antigravity 公共 relay、TokenHub 扩展在原计划属于待选择项，必须在 A0 逐项登记为首版必选/后续独立项/上游能力不支持。不得把待选择变成默默遗漏；选为必选后缺少外部条件会阻断完成。

2026-09-15 A0 选择：首版实现站内告警记录和管理员配置的 HTTPS webhook 通知；Claude 组织用量 E1、Antigravity 公共 relay E2、TokenHub 扩展 E3 登记为首版后的独立项目，不计入 F1–F8 必选完成分母。理由是三项在原计划中均为待选择扩展，分别需要组织 Admin 凭据、高风险公共工具/网络策略或外部产品协议；它们的现有安全边界和文档继续保留。现有 Midjourney 属于当前产品能力，仍须按 F1 审计并修复。

## 4. Sol / Terra 工作方式

- **主代理：** 用 Sol 主持执行，管理一个 Goal、接口、集成和阶段验收。用户当前用其他主模型时由实际主代理协调；文件配置不能代表模型已经切换。
- **Sol 实现角色：** high 起步；复杂事务、恢复、权限与安装状态机用 xhigh/ultra。负责 `model/`、核心 `service/`、相关 controller/middleware 与共享接口。
- **Terra 实现角色：** medium/high；负责已冻结 API 的页面、i18n、CLI/发行脚本、文档和明确独立模块。
- **独立审查角色：** 未参与对应实现的 Sol high；在核心集成批次和最终候选安排。Terra 可以验证 Sol 的实现，但不将实现者自查记成独立审查。
- 当前并发上限 4 席：主代理 + Sol 核心 + Terra 产品 + 按需审查/验证。审查与实现交替占位，不为保持席位满载制造任务；不递归派工。
- 每次派工只记录目标、输入、依赖、具体文件、允许操作、验收和返回结果。同一文件只允许一个 writer；`model/main.go`、`controller/option.go`、`service/billing_session.go` 和共享 DTO 由核心 owner 串行集成。
- Terra 在等待 API 时做真实设计系统、路由迁移表、文档和安装适配器；不搭建一次性假业务应用。页面接真实项目接口后再验收。

## 5. 执行阶段与依赖

| 阶段 | Sol 主线 | Terra 并行线 | 退出条件 |
| --- | --- | --- | --- |
| A0 基线与决定 | 修复已复现启动/fixture 阻断；核对 WIP 所有权；冻结第6节会影响近期实现的合同 | 原路由→新页面映射；平台/制品矩阵；复用 CI 和测试入口；整理当前 Codex 显示剩余项 | 可运行基线、接口 owner、待选项和外部验收责任明确；不要求重跑全部历史测试 |
| A1 核心调用与账务 | 完成 F1 和 F2：WP3-B/C、所有 writer、Task dispatch/recovery、日志去重、人工处置、配置和硬限额 | 完成渠道路由与额度窗口现有 WIP；制作真实可交互 UI 基础和安装清单工具 | 新路径在隔离实例实际启用并验收；旧路径兼容和升级检查通过；核心独立审查无阻断 |
| A2 权益与额度闭环 | 完成 F3、F4 后端：策略快照、缓存失效、audit/enforce、样本身份、持久通知、选定扩展 | Key/用户/渠道/额度/通知页面接线；七语言；安装器下载校验和制品适配 | 真实 API→权限→账务/观测→页面完整；通知投递状态准确；旧 Key 迁移可解释 |
| A3 安装更新与学习中心 | 完成 F5、F6 核心：样本持久化/调度预算/版本/文件应用；安装锁/journal/更新恢复/受限管理桥 | Lite/Desktop 运行交互、安装更新页面、学习中心、中文/英文操作指南 | Full/Lite/Desktop 完整流程可运行；应用/回滚和更新恢复实测；授权和预算得到执行 |
| A4 完整独立 UI 与联合验收 | 修复跨模块集成问题；完成 F7/F8 后端与兼容检查；独立安全/账务/恢复审查 | 补齐全部页面及官网；真实浏览器和声明平台安装验收；制品与文档收口 | F1–F7 逐项通过；所有必需外部验收有证据；无已知阻断或严重交互问题 |
| A5 预发布封版 | 核对最终 SHA、迁移/恢复说明和审查结论 | 统一版本/清单/制品；运行最终候选门禁；准备 Release/NPM beta/镜像发布清单 | F8 满足；外部发布经批准后执行并回读，随后验收安装结果 |

依赖规则：A0 后 UI 设计与发行基础可以持续并行；账户强制策略依赖 A1 的账务/配置接口；学习付费调用依赖账务与权限，文件应用依赖安装管理桥。A3 的安装与学习由不同 owner 并行，先冻结接口。A4 是最后补齐和联合验收，不表示此前不做 UI。任何批次修改公共接口，通知所有消费者并补对应验证。

当前状态：A0 已完成当前范围；A1 进行中；A2–A5 未开始。已创建一个完整预发布主 Goal。A1 由 Sol/high 负责普通 BillingSession authoritative 闭环和 writer 真实性登记，Terra/high 负责渠道路由预览前端收口；主代理复核 WIP、决定和集成验证。计划编制和历史 WIP不作为阶段完成证据。

2026-09-15 A0 当前证据：

- Sol/high 只修改 `router/relay_router_test.go`，先在隔离 SQLite 中引导日志与投影 schema，再执行真实非主节点 `InitDB`；生产 fail-closed 行为未改。修复前定向测试退出 1，修复后定向测试和完整 `./router` 在允许本地监听的环境均退出 0。
- Terra/high 完成 Codex 当前/历史窗口 UI：当前窗口只使用最新上游响应；历史系列保留记录时套餐和时长并明确说明不会因升级而改写；七语言由受管脚本更新。本轮主代理复验专属 Vitest 9/9、typecheck、涉及文件 lint，退出 0。
- 核心 WIP 定向验证：`./model -run 'Test(AccountQuota|UserFunding|QuotaBatchDrain)'` 首次仅因沙箱禁止 miniredis 监听失败，在允许监听环境退出 0；`./service` 的 authoritative BillingSession/恢复相关定向测试退出 0；`./controller` 资金模式相关定向测试退出 0。
- A0 页面/权限盘点确认：登录身份流程可复用；Key、用户权益、渠道、日志、钱包、系统运维均为部分可复用；Task unknown/账务恢复、提示词学习、安装更新恢复、Full/Lite/Desktop 状态页面完全缺失；告警页缺持久记录与投递历史。当前 `service/authz` 仅覆盖渠道资源，不能替代 F3 策略。各缺口已映射至 F1–F7 和 A1–A4 owner。
- A0 退出条件已满足：当前基线可运行、关键合同与可选扩展已登记、共享文件 owner 已确定、页面缺口已映射。外部账号/设备/签名条件保持提前登记，不阻止 A1 软件实现。

2026-09-15 A1 前端/发行进度：

- Codex 两个生产入口在刷新失败时都清除旧 current payload；当前窗口只按最新成功上游响应显示，历史页面仍保留记录时套餐和窗口。路由预览成功后刷新失败会隐藏旧策略，页面按 `channel.read` 权限挂载，36 个路由预览字面量键已进入七语言。
- 资金模式页面只有服务端明确 `ready=true` 且对应 mutation capability 为 true 时开放充值、兑换、转奖励或购买；未就绪/retirement/disabled 保留只读历史入口。主代理进一步修复了 partial capability 默认开启问题。
- 前端整改专属测试由主代理复验：额度/路由/权限/资金相关用例均退出 0，typecheck 和涉及文件 lint 退出 0；独立 Sol 复审无剩余 P0–P3。真实浏览器、真实账号及后端部署联调仍待对应阶段。
- 全量前端门禁：Node 26 默认实验性 webstorage 使 72 文件中的 4 文件共 11 项失败；按已知项目兼容方式设置 `NODE_OPTIONS=--no-experimental-webstorage` 后 72 文件/404 项全部通过，生产 build 通过。全仓 lint 仍有既有/WIP 错误，未涉及上述整改文件；作为 A4 候选阻断保留，不在本批扩大成全仓样式清理。
- 预发布通道本地合同已完成：GHCR 仅显式受保护发布、构建身份绑定已验证 tag SHA；GitHub Release 单一 finalize；NPM beta 与稳定通道隔离；严格 SemVer；三通道 stable latest 串行、单调和失败关闭；GHCR Full/LAN 在全部 digest/OCI/provenance/SBOM 预检后才统一 promotion，并支持同候选幂等补齐。主代理复验 `npm run release:workflow:check`，最终为 10 项 Node 测试及 32/32 合同通过；独立 Sol 复审无剩余 P0–P3。真实 registry、Buildx raw、Cosign 权限和中断重跑仍待发布前在线验收。
- WP3-B1 已完成当前代码范围：BillingSession 在结算前持久化独立 intent，intent 失败可使用 settlement fact 应急，双失败写入 reserve lifecycle manual evidence；同会话异值冲突不污染证据；扫描任务无业务 payload 且入队有界；Task/普通日志区分 pending、applied readback、manual；启动期 schema capability fail-closed。主代理复验最后冲突/manual identity 定向测试退出 0，Sol 执行 service/model 全包、相关 race、vet 均退出 0；独立 Sol 最终复审无 P0–P3，`billing_session_callers` 已据实设为 migrated。MySQL/PostgreSQL、真实 Redis、多节点及真实进程中断仍待 A1 集成验收，整体 gate 保持关闭。剩余 10 类 writer 继续迁移。
- 剩余 writer 只读盘点已完成：`delta_update_user_quota` 及六个 `*WithContext` 入口目前无生产 caller，不能仅凭 helper 存在标记迁移；实际生产写入分三批串行处理。第一批为 topup/redemption/checkin 及 invite/affiliate credit；第二批为 token+legacy relay、Task 与 Midjourney；第三批为 subscription wallet/overflow 与 admin add/subtract/override。管理员补单按持久订单归 credit，直接改余额归 admin；affiliate transfer 必须在同一事务减少 `aff_quota` 并创建 wallet receipt。

2026-09-15 Claude 接手后首批验收：

- WP3-B2 capability CAS 最终验收通过。`model/user_quota_mutation.go` 统一发布 helper（捕获 pool/binding generation/旧 snapshot，CAS 发布，发布前后复核，post-CAS 失效时只向当前 binding 修复为 unknown 或置 nil，不重绑旧 pool）；`EnsureQuotaWriterEpochStateWithDB` 同样经 capture/publish 路径。复审确认：旧 refresh/Ensure 在 pre-Store 屏障交错切库时返回 `ErrUserQuotaBusinessSchemaUnavailable` 且不覆盖新快照（hook 注入测试覆盖）；同 pool 两个 expectation 共享 previousSnapshot 时后到者 CAS 失败、状态不倒退；`system_tasks/system_task_locks/task_recovery_identities` 仍属合法 legacy，只有 receipt/projection/epoch/cursor/drain 组合缺失才失败关闭。本机复验（GOMAXPROCS=1、GOMEMLIMIT=768MiB、GOWORK=off、-p 1）：`go test ./model -run TestBusinessCredit -race` 通过、`./model` 全包通过、`go vet ./model` 干净、topup/redemption/checkin/quota writer/task recovery/payment 定向 race 测试通过。MySQL/PostgreSQL 受控入口仍无本机 DSN，未实际运行，不能称三库通过。`credit_recharge_redemption_checkin` 标志维持 false，待 invite/affiliate 同批评审后统一登记。
- 前端第22批验收通过。四个文件（tag-batch-edit-dialog、model-mapping-editor、upstream-update-dialog、model-mapping-editor.test）差异核对完整：useCallback 稳定化与 effect 依赖补全保持映射同步/行 id/顺序行为，spread/curly/fragment/self-closing 为等价改写；新增 model-mapping-editor 回归测试保护解析契约。复验：4 文件 scoped oxlint（项目配置）0 错误、相关 Vitest 4 文件 7/7 通过（NODE_OPTIONS=--no-experimental-webstorage）、typecheck 通过。全仓 lint 既有项未在本批扩大清理。
- credit 边缘批（第一批 writer 收尾）已实现并通过：新用户初始额度在创建事务内判型，authoritative 以 Quota=0 创建并同事务写 `user_init:<id>` receipt（提交后按确定性 key 反查投影，覆盖注册/管理员/OAuth 三路径），legacy/bridge 保持直接赋值，判型失败回滚注册；invitee 奖励 post-commit 判型，authoritative 走 `invite:invitee:<id>` 幂等 MutateUserQuota，失败 SysError 不静默；inviter aff 奖励经新表 `invite_reward_grants`（inviter+invitee 唯一）同事务 grant+增量，重复 finishInsert 不加第二次；affiliate→wallet 经新表 `aff_quota_transfers`（user+request_id 唯一）持久操作身份，单事务 funding gate→重放零写/异值冲突→建单→锁用户→扣 aff→authoritative receipt 或 legacy 直写→标记 succeeded，前端每次转移尝试生成一次 request id 并重试复用。复核 credit_edge.go 关键路径与 user.go 注册改动无问题；新增 10 测试主代理独立复跑 10/10 通过，model 全包、vet、controller build、前端 typecheck/oxlint/wallet vitest 均通过。MySQL/PostgreSQL 仍未本机验证。
- 第二批（token 与 legacy relay、Task/Midjourney）已实现并通过：postConsumeQuotaWithResult 拆三层模式分发（billing 回退 `billing-settlement:{requestID}`、realtime WSS 按连接内序号 `realtime:{requestID}:{seq}`、violation fee `violation-fee:{requestID}`）；token 回滚键固定为 `billing-token-rollback:{requestID}`（去 nano，修复恢复重复退回缺陷）；preConsume 非 legacy fail-closed。Task legacy 结算/退款经 AccountQuotaSettlementFact 事实化（`task-settle:{taskID}`/`task-refund:{taskID}`，含重放指纹稳定字段校验与 legacy token used 宽容预检）；Midjourney settle 迁到 postConsumeQuotaWithEvent（`mj-billing:{mjId}`），refund 三模式分发+持久事实（`mj-refund:{mjId}`，并发 4 路收敛一次）。service/model 全包、vet、build 通过；新增 22+ 测试。已知边界：事实应用与统计列非同一事务的崩溃窗口残留 used_quota 偏高（fail-closed 不双退）；authoritative 下无 durable operation 的孤儿 legacy task 与订阅资金源直写维持既有限制。
- 第三批（订阅余额/overflow、admin writer、注册核对）已实现并通过：PurchaseSubscriptionWithBalance 模式分发（authoritative `subscription-wallet:{tradeNo}` receipt 与订单同事务；bridge fail-closed）；overflow 逐路径核实无需新写入点（relay 经 BillingSession 分流、task 经 durable 管道、订阅侧已有 PreConsumeRecord/结算事实幂等）；admin add/subtract/override 经新表 `admin_quota_adjustments`（target_user+request_id 唯一、锁内二次查重、并发 8 路收敛一次、override 锁内算 delta 走同一内核键 `admin-adjust:{user}:{requestId}`，OperatorUserID 记录管理员），前端 user-quota-dialog 按参数组生成 request id 重试复用。注册表逐 caller 核对后更新：increase/decrease/try_reserve user/token、credit_recharge_redemption_checkin、subscription_wallet_overflow、admin_quota_mutations 据实置 true；delta_update_user_quota 无生产 caller 保持 false（AllWritersMigrated 仍 false）。model/service 全包通过；controller 可用子集通过，全量 18 项既有失败源于工作区日志投影 fixture（"missing table logs"），与本批无关，待后续统一处理。前端 typecheck 通过；oxlint 报 2 项 HEAD 既有问题未动。已知语义差异（有意）：authoritative 下 admin 减法拒绝透支、余额购要求用户 enabled；余额购与 admin 调整 bridge fail-closed（与批一 bridge=legacy 不同），切换窗口内这两个入口在 bridge 不可用。
- KIMI 渠道批（用户要求提前）：官方核实 kimi-k2.5/moonshot-v1（2026-08-31）与 kimi-k2 系列（2026-05-25）已全部下线，旧 ModelList 五个模型全部失效；已更新为 kimi-k3、kimi-k2.7-code、kimi-k2.7-code-highspeed、kimi-k2.6（[Kimi 开放平台模型列表](https://platform.kimi.com/docs/models)）。前端渠道类型 25 显示名改为 "Kimi (Moonshot)"（wire id 与后端名称不变），channel-utils 标签同步；渠道抽屉 type 25 新增 base URL 预设（api.moonshot.cn 默认 / api.moonshot.ai / kimi-coding-plan / 自定义，仿 VolcEngine 模式，kimi-coding-plan 走既有 ChannelSpecialBases）；七语言 i18n 各补 5 键并 i18n:sync 通过（sync 顺带填充了既有 WIP 的其他新键，属脚本正常职责）。验证：moonshot 适配器 3/3、`go build ./...`、前端 typecheck/oxlint/channels vitest 20 文件 112 测试通过；controller 1 项失败为既有日志投影 fixture 问题（同第三批记录的 18 项）。未覆盖：usage-logs model-badge 的硬编码 'Moonshot' label、抽屉新交互无自动化测试、未在真实浏览器验收。
- 受控切换操作面（A1 收尾）后端已实现：`quota_writer_mode_transitions` 不可变证据表（applying/succeeded/failed + pre/post audit 快照）；`ApplyQuotaWriterModeTransition`（事务内 epoch 行锁+LockVersion CAS、合法性复用 Plan 规则、legacy→bridge 需集群排空确认（Option 表 epoch 绑定，切换后自然失效）、bridge→authoritative 重跑全部硬条件、失败落 failed 证据、提交后发布 Redis epoch 与 post-audit、authoritative 拒绝降级、并发双 apply 收敛一次）；审计的 cluster_drain_ack/inflight_zero 从硬编码缺失改为真实读取（Option 确认记录 + BillingSession 进程级在途计数，sync.Once 对称释放）；`DriveQuotaWriterDrains` 有界同步排空（batch 队列/projection 积压，复用既有 worker 不复制逻辑）；5 个 admin 端点 `/api/quota-writer/{status,plan,apply,transitions,drain}`。验证：model/service/router 全包通过、vet/build 干净；controller 18 项既有 fixture 失败不变。边界：多进程在途靠操作员 ack 背书、balance drain 应用 legacy-only、projection 重放 gate off 只报告、authoritative 下常驻审计显示 cluster_drain_ack 缺失属设计、MySQL/PostgreSQL/真实 Redis 未验证。
- 切换操作面前端已实现：系统设置 Billing 分组新 section「Quota Writer Mode」（/system-settings/billing/quota-writer）：模式/epoch/在途总览、9 项审计清单、Plan 预览、Apply 表单（expected_epoch 自动填、ack_note 必填、不可逆二次确认、409 missing 内联展示）、Drain 恢复卡片、transitions 分页历史（状态徽标+失败原因展开）；无权限/加载/错误态齐全。i18n 105 新键七语言真实翻译并 sync 通过；typecheck/oxlint/build 通过；新增 6 个组件测试+system-settings vitest 全过。未真实浏览器联调验收。
- C09-N2b 支付运行配置接线完成：46 个支付键接入既有 PaymentRuntime 内核；下单/webhook 17 条路径全部入口单次捕获 detached 快照（`setting.CapturePaymentConfig()`），`stripe.Key` SDK 全局态清零改 per-request client；单键与支付 bulk 发布桥接为候选→CAS 成代，任一键失败整组中止、runtime 停留旧代、OptionMap 保持逐键回滚；DB 提交后发布失败记 SysError 以旧代为准。19 个新测试通过；setting/model/service 全包通过。controller 发现既有 panic `TestPaymentWebhookLogPrivacy/stripe/valid`（stripe-go v81 nil 解引用，回退代码复现确认非本批引入，建议后续给 GetObjectValue 加 nil 防护）与既有 quota capability fixture 失败家族，均未扩大。
- C09-N4 成功请求硬限额完成（D05 合同落地）：模型成功数从 check→execute→record 改为 reserve→commit/release 原子预留式，Redis 三 Lua（reserve/commit/release，LIST 已提交+ZSET 预留双键，committed+reserved 永不上限）与内存等价实现（单 mutex）；失败释放、未知保留至窗口过期；Redis 故障 fail-close 与现状一致；维度键/窗口/429 格式/配置语义全兼容，滚动升级可读旧版 LIST。并发 50 上限 10 恒 ≤10（Redis+内存全链路屏障测试、-race 通过）；middleware/common/setting 全包通过。边界：Redis Cluster 需同 slot；请求超窗口时 commit 不补记（保守不超卖）。
- C09-N3a typed bulk 单写者完成：`PUT /api/option/typed-bulk` + `GET .../revision`（RootAuth）；封闭 typed union（string/number/boolean/string_list，JSON 词素严格匹配）、重复/未知键拒绝（21 族白名单，排除 payment/user_funding/performance/channel_affinity 专属或决策族）、全量校验零写入→单事务+revision CAS（options 表持久单调计数行）→提交后按族一次发布、失败整组中止旧代保留；409 冲突零写入；并发双 bulk 串行化恰好一成一败；旧 PUT 与支付 bulk 兼容回归通过。契约冻结于 docs/OPTION_TYPED_BULK_API.md。15 个新测试通过。
- C09-N5b 配置受控修复完成：三个白名单动作（group_ratio.canonical_sync、value.trim_space、json.bom_trim_repair）闭式枚举，keep/repair/reject/fallback 判定语义，schema_indeterminate 族只 fallback 不猜修，不自动清理任何历史；dry-run 严格零写入（三处断言+无登记行）；apply 逐行 value-hash CAS（MySQL BINARY 守卫），同事务备份（旧值全文隔离表，无 API 出口）+登记（只存 hash）；幂等（already_applied/already_at_target_unregistered）；发布失败登记 publish_failed；值不出 model 层、审计不记值；三端点 RootAuth+no-store。测试全绿；唯一失败为 HEAD 即可复现的既有日志投影 fixture 项。
- C09-N3b 前端表单单请求改造完成：盘点 system-settings 全部多键表单，7 个键全落在 typed bulk 白名单族的表单完成迁移（checkin、ssrf、models 区 global/claude/gemini/grok、passkey）；混合型（oauth、ratio、pricing 等含扁平旧键）与排除族/外部副作用型保持现状并记录理由。新增 use-typed-bulk-options 共享 hook（类型映射、revision 加载、单请求提交、409 重载 action、400 校验错误展示）。11 个 hook 测试+system-settings vitest 22/22、typecheck/oxlint/build 通过；七语言 sync 通过（脚本顺带补齐既有 WIP 翻译）。未真实浏览器联调。
- D14/D15 热读族收口完成：performance_setting 改不可变 generation+detached 快照，磁盘缓存单次决策读同一生效代（修复单次决策多次读混代与 newDiskStorage 忽略传入 path 重读全局两个既有缺陷）；放置字段（path/max_size）普通保存只更新配置代，经新维护端点 `POST /api/option/disk_cache/rebuild`（排空语义、失败保留旧代）或重启生效；channel_affinity_setting 同代化，MaxEntries/TTL 经 `POST /api/option/channel_affinity_cache/rebuild` 原子换入生效；统计端点并列配置代/生效代+rebuild_required 可核对。两族不进 typed bulk。setting/common/service 12 包测试通过、relaykit 独立构建通过。边界：Redis 模式亲和缓存重建只换内存回退层；多节点需逐节点重建/重启。
- C09 全部关闭（N2b/N3a/N3b/N4/N5b/D14/D15），F2 后端与前端主体完成；残留：无诊断 schema 族逐族补齐、混合型表单待后端键空间扩展、真实浏览器/实库验收。
- controller 测试债务统一批完成：38 项失败+1 panic 全部归零（`go test ./controller` 310 顶层测试全 PASS）。修复点为共享 fixture 三处（model_list_test 日志投影 schema 引导、payment_webhook_transaction capability 刷新、cleanup 恢复、relay_task_durable epoch 引导）；生产最小改动三处：stripe webhook 缺 data 对象 nil 防护（400 替代 panic，补回归测试）、Waffo/WaffoPancake webhook 补回 WIP 丢失的解析失败告警日志（ACK 语义不变）；TestCopyChannel 错误文案对齐 WIP 新文案"invalid cloned channel settings"（经主代理确认保留 WIP 文案）。router 测试无回退。
- A1 完成度更新：F1 持久账务（writer 迁移三批、切换操作面、持久事实层）与 F2 配置/硬限额（C09 全部子批）软件实现与本机验证完成；A1 退出条件中"新路径在隔离实例实际启用并验收"与三库/真实 Redis/真实浏览器/真实账号验收仍未做，A1 不宣布关闭。
- A2-F3-B1 权益策略 ingress 完成（2026-09-16）：新增 `setting/access_policy_mode.go`（`access_policy_mode_setting` 族，mode=off/audit/enforce 默认 off，`enforce_groups` 白名单作用域，immutable generation+拒绝失败保留旧代）；`service/accesspolicy/ingress.go` 是 S3-PRE1 detached 内核的唯一生产入口（`BuildRequestSnapshot`/`EvaluateRequest`/`BlockingFindings`，从已加载 token/userCache/profile 注册表组装快照，零 I/O）；`middleware/distributor.go` 在 legacy 渠道选择成功后、首个 dispatch 前接入 `middleware/access_policy.go` 钩子：off 零开销、audit 经 `logger.LogWarn` 记录 findings（digest/group/codes/request_id/token_id）不拒绝、enforce 仅对 `enforce_groups` 内 group 且存在阻断性 finding（legacy_group_difference/disabled_reference/unknown_reference）才 403 拒绝，评估错误 fail-open。附带修复两个既有 presence 丢失缺陷：`setting/access_profile.go` clone 与 `model/access_profile.go` ResolveAccessProfile 此前把显式空列表塌缩成 nil（D04"显式空名单拒绝"语义依赖该区分）。新增 context key `access_profile_id`/`account_tier_id` 由 auth/WriteContext 写入。i18n 新增 `distributor.policy_denied`（en/zh-CN/zh-TW）。验证：`go test ./setting ./service/accesspolicy ./middleware ./model ./controller ./router ./service` 全绿、vet 干净、`go build ./...` 与 relaykit 独立构建通过。边界：enforce 评估失败仍 fail-open（作用域内 outage 防护）；渠道能力层交集与旧 Key 可审计迁移留后续批次；前端无 UI 改动。收尾核对：`model/option.go` 无需为新族加 key 钩子——通用 `token_setting.*` 分层路径经 `config.ValidateConfigFromMap`（`ValidatingMapConfig`）+ 全局注册已覆盖校验/更新/启动加载，typed-bulk 挂载与运维 UI 留待运维面批次；`gofmt -l` 与 `git diff --check` 干净（gofmt 报告项均为既有未提交 WIP）。
- A2-F3-B2 模型权益交集完成（2026-09-16）：D04「模型/路由权益真正生效」按已实现 profile 模型面落地——`LegacyOutcome` 新增 `UsingModel`（校验拒绝非法模型文本），`Evaluate` 对 `AllowedModels` 产生 `legacy_model_difference` finding（absent=不限制、explicit_empty=拒绝全部模型、values=白名单比对）；`BlockingFindings` 纳入该码，enforce 作用域内 profile 显式模型名单外的模型请求被拒。`AllowedRoutes` 当前承载路由组身份（与 AllowedGroups 同源），组比对已由 group finding 覆盖，未做重复 route 比对；渠道能力层（type-58 route 能力等）仍留后续批次。`SnapshotSource.UsingModel`、`applyAccessPolicyDecision(c, group, model)` 接线，distributor 传 `modelRequest.Model`。新增 5 个定向测试（名单外模型 flag、名单内无 flag、absent 不限制、显式空拒绝、非法模型文本校验），`go test ./service/accesspolicy ./middleware ./setting ./service` 全绿、vet/gofmt 干净、`go build ./...` 与 relaykit 独立构建通过。旧 Key 可审计迁移与渠道能力交集留后续批次。
- A2-F4-B1 渠道额度身份隔离完成（2026-09-16）：`ChannelQuotaSnapshot` 新增不可逆 `AccountRef`，由 `ChannelQuotaAccountRef(provider, accountID)` 生成 SHA-256 引用，不持久化上游账户原文；未知身份保留空引用，后续告警不能猜测合并。该引用纳入快照 series identity、内容/去重 digest、单条写入查询，并使批次缺失窗口检测精确按 `AccountRef` 读取前序成功样本；因此同频道的不同凭据/账户不会互相创建窗口消失标记，令牌刷新但账户不变可延续系列，账户变化自动切断系列。Codex 后台采样在解析或刷新 OAuth 凭据后以确认的 `account_id` 写入引用；失败及身份无法确认的样本维持未知身份。新增账户引用稳定性、跨账户快照不合并、不同账户不产生对方 absence marker 的回归测试。验证：`go test -p 1 -count=1 ./model ./controller -run 'Test(ChannelQuota|Codex)'` 与 `go build ./model ./controller` 通过。边界：本批只完成身份边界，不产生告警事件或 webhook；事务 outbox 留 F4-B2。
- A2-F4-B2 事务告警状态与 outbox 完成（2026-09-16）：新增 `ChannelQuotaAlertState`（`channel + AccountRef + metric/window/source/plan/unit/currency` 摘要系列）和不可变 `ChannelQuotaAlertEvent`（OccurrenceV2 `EventKey` 唯一、pending outbox 状态）；两表加入主库 `AutoMigrate`。`RecordChannelQuotaSnapshotBatchWithContext` 在同一事务中写入每条成功、可信账户且有 provider total 的快照后，计算 status（未知/无总量/非法值不伪装 healthy）、创建/更新状态并以 `EvaluateChannelQuotaAlertOccurrenceV2` 写入 threshold/recovery 事件；状态和事件任一步失败都会回滚快照批次。首次低额度会产生 threshold；尚无成功送达记录时相同状态不产生虚假 reminder；迟到样本不倒退状态。旧 `NULL account_ref` 按未知身份兼容读取。验证新增阈值→恢复、未送达去重、跨账户系列隔离测试；`go test -p 1 -count=1 ./model`、Codex/controller 定向回归、`go vet ./model ./common ./controller`、受影响包构建均通过。边界：事件仅为 pending，领取、HTTPS webhook 投递、重试、实际送达时间与 reminder 恢复留 F4-B3。

恢复规则：实现失败保留差异并局部修正；隔离数据实验失败保留故障证据，从实验前备份重建测试副本；schema 或权威模式不可逆时采用已验证恢复/前滚方案，不直接回退二进制继续写旧账。最终候选出现阻断发现则返回对应 owner 修复，仅补受影响回归后重新封版。

## 6. 集中冻结关键决定

已确认且直接沿用：D02 主库唯一可消费权威；D03 历史不明差异人工核查，不能推测性补扣/退款；完整首版目标；Sol/Terra 分工；精简重复验证。用户已确认按本计划持续开发，因此 D04、D05、D08、D13、D14、D15 采用下表推荐合同进入实现；外部通知、扩展、设备、签名、域名和真实账号仍按实际可用条件登记。

下列建议由执行时采用的方案明确确认；未确认的业务选择保持待决定，先做不依赖它的工作。一次只处理当前依赖，不逐个询问机械细节。

| 关联决定 | 推荐合同 | 最迟决定点 |
| --- | --- | --- |
| D04 权益 | Tier/Key/Profile/渠道能力取交集；inherit 不增加限制、显式空名单拒绝；最多一次且仅首次 dispatch 前 fallback；off→audit→范围 enforce | A2 前 |
| D05 硬限额 | 成功数+在途预留不超过上限；失败释放、结果未知保留待恢复 | A1 对应实现前 |
| D14/D15 热配置 | 新请求使用新代规则；磁盘目录及缓存容量/TTL 变更经维护排空/重建或重启，避免普通保存偷偷迁移 | A1 对应实现前 |
| D06 通知与扩展 | 首个外发通道建议管理员配置的 HTTPS webhook，持久重试、SSRF/重定向约束；站内投递记录同步提供；三个选定扩展逐项列入矩阵 | A0 登记，A2 实现前确定 |
| D07/D10–D12 平台与信任 | 保留已有包/镜像命名；Full/Lite 分离于安装形态；受信 manifest 固定来源/签名身份；自动检查/下载/安装默认关闭；支持的旧版和平台按下表实测 | A0 登记，A3 前冻结路径/信任根/兼容版本 |
| D08 UI | 按用户任务组织导航：概览、Key/调用、渠道/额度、日志/任务、用户/权益、钱包、学习中心、系统/安装；沿用 React/Rsbuild/Bun 和现有组件基础，独立设计视觉 | A1 提供真实可交互关键页面后批量实施 |
| D13 学习 | 默认关闭、显式 scope 授权、派生样本最小保存、独立加密/保留策略；仅生成草稿，人工确认后应用；模型/渠道/预算显式配置 | A3 前 |
| D09 外部验收 | 指定测试账号、通知接收者、设备、签名/公证和域名；付费/发送/文件应用只对指定测试目标授权 | A0 登记，提前安排，不等封版时才申请 |

平台建议矩阵：Linux amd64/arm64 的 Full/Lite 原生和容器；macOS arm64/x64 与 Windows x64 的个人 Lite/Desktop；local/LAN/public 三种访问模式按明确配置实现。Linux Desktop、Windows arm64 等额外组合在 A0 按原承诺和可用设备定案，不擅自宣称支持。每个声明支持的制品至少真实安装/启动/调用一次。

数据矩阵：SQLite、MySQL 5.7.8+、PostgreSQL 9.6+ 共享业务都验证；保留的 ClickHouse 日志路径验证迁移与去重；Redis 验证丢失/陈旧/恢复。采用覆盖风险的代表组合，不盲目运行平台×数据库×语言的全笛卡尔积。

备份恢复目标：建议升级前一致备份 RPO=0，即不丢失已确认数据；外部日备/RTO 目标沿用 D11 建议并以数据规模实测后冻结。没有真实测量不能承诺恢复耗时。

## 7. 精简验证：以完成风险覆盖为准

### 省略或合并

- 不重新证明未受本批改动影响的历史成果；不在每个小提交重复全量 Go/race、全量前端构建和多平台镜像。
- 不为了覆盖率添加只验证执行、私有常量、函数调用顺序或源码文本布局的测试；不新增随机循环、sleep、无基准性能测试。
- 同一缺陷已有精确回归时复用并补缺口；不同时写几套等价 mock、浏览器脚本和测试报告。
- 不建立并行维护的假业务服务器或另一个 UI 原型项目；直接在项目运行界面开发，用真实本地 API/临时数据库验证流程。
- 文案/间距等低风险可逆修改用类型、lint、翻译检查和实际页面检查；不为每次微调追加机械单测。
- 每批更新一次本计划状态和相关使用文档；审查有新发现才复审，不做无新证据的多轮全盘审计。

### 必须保留

| 风险 | 最小有效验证 | 运行时机 |
| --- | --- | --- |
| 金额、事务、幂等、未知结果 | 精确余额/回执/日志断言；重复/并发、关键写失败或提交未知、重启恢复；无法安全在真实上游制造的故障使用一次性定向注入 | 对应核心批次；改动后定向回归 |
| 权限、撤销、隐私、文件写入 | 越权拒绝、撤权生效、敏感内容不外泄、路径/链接/并发外改拒绝；备份和原子应用可恢复 | 对应批次 |
| 数据库与缓存 | 受影响三库迁移/事务、Redis 真实服务验证；独立日志库和 ClickHouse 的相关恢复检查 | 涉及该契约的集成批次，用 CI 临时服务 |
| API/Provider | 保留协议回归；正常流程用真实应用路由；少量 fixture 只覆盖外部不可控响应和关键失败 | 适配器变化及核心整合 |
| UI | 接真实应用 API 的浏览器操作；关键角色、加载/失败/权限/冲突、窄屏和长文案；七语言完整性，重点实看中文/英文及长文本语言 | 功能批次完成时；最终完整路径一次 |
| 安装与恢复 | 声明平台实际制品新装；旧版本副本升级；更新中断读取 journal 恢复；备份还原后核对业务数据 | 安装链完成后及最终候选 |
| 项目集成 | 根模块构建/test/vet、相关 race、`GOWORK=off` relaykit 独立构建、前端 typecheck/lint/test/build、发行合同 | 公共内核集成节点和最终候选，普通小改不全跑 |

测试失败先区分代码、fixture、权限和外部依赖；端口/缓存权限问题按工具审批流程在允许环境复验，不重复在同一受限条件下运行。针对性模拟是关键故障证据，不能冒充真实账号、通知送达、支付、设备或生产验收。

最终候选的必需门禁覆盖实际发布 SHA。候选后仅改文档时按依赖复用代码证据；代码/配置/构建输入变化时补受影响检查并记录对应版本。检查通过后直接交付下一阶段，不无故扩大测试。

## 8. Goal、状态、同步与外部操作

执行启动时 Goal 查询为空，已创建完整首版主 Goal并进入 A0。后续沿用该 Goal；阶段状态只写本文件，不另建多套计划。用户未指定 token/时间预算，不自动填造预算或工期。

每阶段维护：状态、owner、当前提交/未提交范围、已通过验证、剩余阻断、下一项。已批准执行范围内连续实现、修正、验证和文档更新，阶段结束直接进入依赖已满足的下一阶段。

当前仍保留“不要提交、推送、部署”的旧边界。采用计划时可单独明确 GitHub 同步授权，目标为已有 `ForceMind/MyAPI`、`codex/mac-durable-accounting` 或维护者指定分支。未获得该授权前保留本地成果和“未同步”，不创建/推送 PR；获得后按批次精准提交、推送并用 PR/manual 触发 CI。远端返回与实际 checkout SHA 回读，不能把 push 成功当测试完成。

候选准备包含版本、清单、源码/NPM pack、原生/镜像/Desktop 工件、校验和及完整说明。外部 Release/tag/GHCR/NPM/官网发布和生产部署分别明确授权。GitHub prerelease、NPM beta、Docker 固定预发布 tag 不更新稳定 latest；沿用已有技术命名，发布后核对可下载制品及安装结果。

## 9. 完成标准与启动交接

完成必须同时满足：F1–F8 必选项全部实现；完整 UI 接线；三库/缓存/恢复及声明平台通过；选定真实账号/通知/设备验证完成；独立审查无阻断；同一候选版本的制品和文档齐备；已授权同步完成。仅生成代码、关闭 gate、纯内核、mock 成功或本机启动都不能替代完整功能验收。

新功能可以因安全和产品策略默认关闭，但必须能在授权的隔离实例里实际启用、使用、恢复和关闭。真实生产迁移不作为软件实现的隐含动作；历史未决差异继续人工处理。

不能因缺少设备/凭据就删去必选项。提前登记外部条件，继续独立开发；最终缺少必需证据时明确待验证。范围、重要业务规则或外部副作用改变才需要补充决定。

给 Sol 执行会话的启动指令：

> 执行 docs/FULL_PRERELEASE_EXECUTION_PLAN.md。目标是最早完整计划全部必选功能达成后交付首个预发布候选。Sol 主开发与协调，Terra 辅助页面、工具、文档和验证；按文件 owner 并行，重型验证串行。先读取 Goal、当前差异与本计划，保留 WP3-B/R1/C0/Codex WIP，从 A0 已知失败与近期决定开始。复用已验收成果，不重做全量盘点。按第7节精简验证，保留关键账务/权限/恢复断言与实际 UI/安装验收。每阶段更新本计划并连续推进。遵守当前 GitHub/发布/生产授权；缺外部条件提前登记，不能冒称完成或静默缩小首版范围。

## 10. 本轮计划自查

- 原 S0/S1 成果复用；S2→F1/F2，S3→F3，S4→F6，S5-Q→F4，S5-P→F5，S6→F7，S7→F8，必选范围完整映射。
- 核心接口先于消费者，UI/发行基础提前并行；高风险文件单 owner；不把所有工作串行等到后端全部完成。
- 省略重复/形式化测试，不降低金额、权限、数据、恢复和实际交互验收。
- 本轮没有重跑业务测试，也没有新增实现；旧测试结果只作为定位信息。计划自查与后续独立代码审查分别记录。
- Sol/ultra 已返回只读规划建议，已纳入完整范围、隔离环境权威模式切换、关键故障验证和前置决策；这是规划复核，不是新实现的独立代码验收。

## 11. 2026-09-20 开发进度复核与后续收口计划

本节更新当前状态，优先于第 5 节早期的“A2–A5 未开始”概括及交接文件旧快照。范围是部署准备情况检查和持续收口；未提交、推送、发布、部署或切换运行中的账务模式。

### 11.1 当前基线与部署结论

- 分支 `codex/mac-durable-accounting`，HEAD `504d4beb2f5ca72e5ebdb7462dc7416da5cd92ea`；检查时 283 个已跟踪文件修改、121 个未跟踪文件，共 404 项。当前可运行代码包含大量 HEAD 以外的工作，不能用该 SHA 代表完整候选。
- 产品版本仍为 `0.1.1`；现有 Compose 默认镜像为 `ghcr.io/forcemind/myapi:v0.1.1`，直接使用该默认镜像不会包含本地新增功能。
- 当前代码通过本轮构建和下列本机回归，可进入隔离试运行准备；尚未验证实际安装启动、浏览器操作或生产可用性。完整首个预发布仍未就绪，不能以关闭新功能代替 F1–F8 验收。
- A1 已有大量实现与本机回归，但启用门禁和真实环境验收未关闭；A2 已进入实现，权益 ingress/模型白名单、账户级额度系列及告警 outbox 已落盘；A3/A4 仍有产品流程缺口；A5 未形成可追溯候选。

### 11.2 本轮实际验证

| 检查 | 结果及边界 |
| --- | --- |
| `go test -p 1 ./model ./service ./controller ./router ./middleware ./service/accesspolicy` | 六包通过；使用 `GOMAXPROCS=1 GOMEMLIMIT=768MiB GOWORK=off`。首次沙箱缓存权限失败，随后获准在沙箱外复验通过；不是三库/真实 Redis 验收 |
| 根服务 `go build -p 1 -o /tmp/myapi-readiness-server .` | 通过，产物仅在临时目录，未启动或部署 |
| `relaykit` 内 `GOWORK=off go build -p 1 ./...` | 独立构建通过 |
| `web/` 内 `bun run typecheck`、`bun run build` | 均通过 |
| 前端全量 Vitest | 95 个文件、451 项测试通过；以 `NODE_OPTIONS=--localstorage-file=/private/tmp/myapi-vitest-localstorage` 分四片执行，组件/jsdom 测试不等于真实浏览器验收 |
| `bun run lint`、`bun run format:check` | lint 无 error，仅保留既有 footer 的 `react/no-danger` warning；format 通过 |
| 发行脚本测试及工作流合同 | 10/10 测试、32/32 静态合同通过；没有触发远端 CI 或发布制品 |
| 升级、Desktop、官网静态检查 | 升级合同 18/18、Desktop 合同 32/32、官网静态检查通过；不证明真实升级恢复或设备安装 |
| `git diff --check` | 通过 |

验证日志暂存 `/tmp/myapi-readiness-*.log`，本节记录摘要；临时日志不属于可长期依赖的发行证据。本轮未执行全仓 Go test/race/vet，未回读远端 CI；未验证 MySQL/PostgreSQL/ClickHouse、真实 Redis、支付、webhook 送达、真实浏览器及跨平台安装。

### 11.3 已确认的关键缺口

1. **权威账务 writer 注册阻断已收口，运行验收仍未完成。** 全仓调用图确认 `DeltaUpdateUserQuota` 只有定义，且只委托已登记的 increase/decrease writer；生产 writer 清单现已移除这个无生产调用者的 convenience helper，没有把其 `Migrated` 标志直接改真。生产登记项已全部迁移，审计仍会对注入的真实未迁移 writer 失败关闭。下一步是在隔离实例演练 legacy→bridge→authoritative、在途排空及重启恢复，并完成三库与真实 Redis 验收。
2. **权益强制契约主体已收口，fallback 与外部验收仍未完成。** 在已配置的 enforce group 内，策略求值失败返回 policy denied；范围外与 audit 继续只记录。Tier 与 Profile 的 route/model 名单已按 nil=inherit、空表=deny-all 取交集，unknown/disabled 会收紧为 deny-all；同一请求使用一个注册表快照，批量保存与数据库 reload 均同代发布。初选、自动重试、锁定任务和 remix 的最终 group 均在上游调用前检查；策略拒绝保留 403。令牌 Redis 投影现保存 `AccessProfileID`，缓存 schema 已升为 2，旧 schema 命中会删除并回源，避免显式 profile 在缓存命中时退回 legacy group；显式 `AccountTierID` 的变化也会推进认证版本并撤销旧会话，纳入现有版本栅栏。配置的首次 dispatch fallback、渠道能力更细的交集、旧 Key 可审计迁移和实际浏览器流程仍须收口。
3. **持久告警投递闭环已完成本机实现。** 现有 outbox 已接通 master worker、root-only 状态/配置/历史/手工执行 API 和系统设置页面；URL+签名密钥通过一个隐藏配置同代发布，投递强制 HTTPS、公开网络目标、DNS/私网复验、无代理/无重定向、5 秒超时、HMAC 签名、指数退避和八次隔离。worker 每条事件在发送前即时领取，避免批量租约过期重投。真实外部 webhook 仍需隔离目标和管理员凭据验收，pending 不能写成已送达。
4. **学习与版本中心已有持久化、版本、应用状态机和首个用户页面，仍缺完整产品链。** Policy/sample、instruction version 和 application 已加入主库迁移：策略默认关闭、按用户和 opaque scope 隔离、撤销 generation、仅保存二次脱敏文本/HMAC 指纹，并以 occurrence 唯一性裁定重放或冲突；人工或未来生成的 instruction 以 command 幂等键追加不可变版本，保留父版本、内容 hash 和 scope 边界；应用请求以授权代、过期时间、目标/备份摘要和 CAS 状态机记录 apply/rollback。`/prompt-learning` 已提供仅 dashboard session 可修改的 self scope 策略和手工版本历史页面。真实受信 ingress、预算调度、模型生成、受信文件执行、完整导航/应用 UI 和实际文件回滚仍未完成；不能把模型表或手工页面认定为学习功能完整可用。
5. **安装恢复、UI 与发行验收未关闭。** 已有 CLI/Electron/官网及静态合同应保留复用；`installation-journal-store` 已提供受管理根内的持久 journal 和 owner lock，但 `installation-state` 的受信 manifest 获取/验签、制品暂存、服务切换、备份恢复和统一安装更新尚未闭环，不能仅补一轮测试就视为完成。学习中心、安装恢复和 Full/Lite/Desktop 状态页面仍有缺口。版本与实际制品亦未封版。

### 11.4 接续执行顺序与验收

沿用 F1–F8 和现有技术栈，不另建平行项目或缩水 beta。以下为本轮提交审阅的接续计划，不表示本轮已经开始实施。

| 批次 | 实施范围 | 退出条件 |
| --- | --- | --- |
| R0 集成基线 | writer 注册语义已与生产调用图对齐；frontend lint/format/typecheck/build 与全量组件测试已通过，继续核对 WIP 归属和版本一致性 | 保留现有成果；本地检查通过；所有变更可追踪 |
| R1 账务启用验收 | `model/`、账务 service/controller、隔离启动 fixture 与既有 DB CI；覆盖排空、切换、投影、unknown/重启恢复 | 隔离实例完成模式切换；三库及真实 Redis/相关 ClickHouse 路径通过，余额/回执/日志不重扣不漏记；不操作生产模式 |
| R2 权限与通知闭环 | `service/accesspolicy`、middleware、token/user 缓存、告警 model/worker/API 和对应页面 | Tier/Key/Profile/渠道能力交集、撤销及旧 Key 迁移可验证；enforce 异常行为符合冻结合同；通知领取/重试/送达/历史可操作 |
| R3 学习与版本中心 | 在现有 promptlearning 内核上接存储、scope 授权、预算调度、版本 API 和 UI | 默认关闭；授权/撤权与隔离有效；生成走统一权限账务；人工应用、外改冲突、备份回滚完整可用 |
| R4 安装恢复与产品界面 | CLI/安装器/Electron、相关页面、官网、七语言；复用现有工具 | 持久 journal 与单安装锁生效；Full/Lite/Desktop 的声明平台完成新装、升级中断、恢复及真实浏览器操作；官网对应真实制品 |
| R5 候选封版 | 版本、发行 manifest、校验和、操作文档、独立审查及候选 CI | F1–F8 无阻断；测试与制品绑定同一候选版本；另行取得同步/发布/部署授权后才外发并回读结果 |

执行协作沿用第 4 节：主代理协调和最终复核；Sol high/xhigh 负责账务/权限/事务；Terra medium/high 负责已冻结接口的 UI、工具和文档；关键批次由未参与实现者独立审查。最多主代理加三名子代理，共享文件单 writer，重型验证串行。已创建一个持续 Goal 并按本节接续实施。

外部依赖应提前准备：隔离数据库与 Redis/ClickHouse、指定测试账号及 webhook 接收端、声明支持的平台、部署目标与持久数据备份。权限、凭据或设备不足时继续独立开发，但保留待验收项；不以 SQLite/jsdom/静态合同代替外部验收。通知测试发送、真实支付、GitHub 同步与生产部署沿用各自授权边界。

自查：顺序先解决当前集成阻碍，再补产品闭环；复用已有实现与本轮有效结果；普通小改只补受影响验证；不重写项目、不回退 WIP、不仅修改门禁布尔值来制造可发布状态。本轮另由 Sol/ultra 只读规划代理核对 F3–F7 的代码和接线缺口；这属于规划复核，不是完成实现的独立质量验收。

### 11.5 2026-09-20 持续开发批次：F3/F4 安全收口

- **A2-F3 Tier/Profile 策略：** `access_profile_setting.account_tiers` 与既有 Profile registry 使用同一受管理快照。Tier/Profile 的 route/model allowlist 取交集，缺省继承、显式空拒绝；unknown/disabled stable ID 不会扩大权限。`Distribute` 在最终渠道确定后（含指定渠道）检查策略；普通 relay 重试、Task 重试和 durable task 均在再次设置渠道上下文前检查最终 group。自动 key 的 remix 从原任务持久 group 和当前渠道 Ability 恢复最终 group；如果该 group 已不在当前 key 的 auto 可用集合，返回 403，不会以字面量 `auto` 跳过 scope。配置读写均对 Profile+Tier 整体同代发布，避免多节点 reload 混读权限版本。
- **A2-F4 告警交付：** API 为 `GET /api/channel/quota/alerts`、`GET/PUT /api/channel/quota/alerts/delivery`、`POST /api/channel/quota/alerts/delivery/run`；前端位于 System Settings → Operations → Monitoring & Alerts。测试覆盖签名脱敏、503 重试、八次隔离、私网/DNS 回绑/重定向拒绝、逐条领取租约、状态/历史加载失败与重试、保存和手工执行。未向真实网络目标发送。
- **独立审查：** Astra 审查先后发现批量租约、自动重试、配置混代、auto remix 和错误状态包装缺口；均已局部修复并增加定向回归。本批尚未替代三库、真实 Redis/webhook、浏览器和设备验收。
- **本机验证：** 账务/权限/告警相关定向 Go test 与 vet、前端 95/451、typecheck/lint/format/build、根服务和 `relaykit` 独立构建均已运行通过。原生服务使用全新临时 SQLite 目录三次在 `127.0.0.1:39080/39081/39082` 启动，`GET /api/status` 返回 200，随后收到 SIGINT 干净退出；最后一次以临时 ldflags 注入 `0.2.0-dev.2`，回读确认 policy/sample/version/application 四张提示词学习表已迁移。这个临时开发版本不等于发行候选或公网可达证明。
- **A3-F5-P1 持久化与受信 ingress 基础：** 新增 policy/sample 主库记录和 `service/promptlearning` 内部持久化桥。策略由 user+scope 唯一键保护，default-off/revoke/stale generation 均拒绝后续样本；桥只接收包内构造的 server-observed turn，经过既有准入和两层脱敏后才将 HMAC occurrence/semantic/observation 指纹、可信度和脱敏文本写入。跨 scope 不可读取，精确重放零写入、异载荷同 occurrence 冲突；客户端来源声明不会写入。该批没有读取 Full Content、文件、凭据或网络，也没有对外 HTTP/API/worker 调用。
- **A3-F5-P3 版本基础：** `PromptInstructionVersion` 以 user+scope+command 唯一键持久化规范化后的 instruction 文本、内容 hash、父版本、来源和操作者。相同 command 的相同载荷返回原版本，异载荷冲突；版本拒绝 GORM update/delete，跨 scope 父版本和读取均拒绝。该批不生成文本、不把模型输出接入版本，也不执行文件应用或回滚。
- **A3-F5-P4 应用状态基础：** `PromptInstructionApplication` 只保存 scope 绑定的 version、HMAC target/backup receipt、授权 generation/到期、command、操作者和 CAS state；pending→applied/failed/unknown→rollback_pending→rolled_back 等转换受限，终态不能回到 pending。创建时复核当前 policy 与 version scope，撤销和跨 scope 引用均拒绝。当前没有文件路径、内容或凭据，也没有文件 I/O；实际受信文件适配和回滚仍依赖 F6 安装管理桥。
- **A3-F5-P2 运行状态基础：** 新增 `prompt_learning_runs` 记录一次分析的稳定 command、policy generation、基线版本、冻结样本上界/数量、指定 model/template、输入/输出上限、最多尝试数和 fenced lease。状态受限于 `pending → leased → preparing → submitted → succeeded`，准备阶段可取消/失败/跳过，已提交结果不明只能进入 `submission_unknown → outcome_unknown | manually_resolved`；租约恢复只会重排提交前状态。policy revoke 在同一事务取消 pending/leased/preparing run，但保留已提交未知态，不能伪称外部调用被撤回。当前没有 worker、调度 API、预算预扣或模型调用，因而不会产生费用；完整 model 包与 vet 已通过。
- **A3-F5-P3 首个用户 API/UI：** `GET/PUT /api/user/prompt-learning`、`GET/POST /api/user/prompt-learning/versions`、只读 `GET /api/user/prompt-learning/runs` 及 session-only `POST /api/user/prompt-learning/runs/:id/cancel` 固定到服务器生成的 self scope。每个 handler 额外要求 live dashboard session，API key 不能启用采集或创建指令；版本和 run 响应不包含 scope、command id、lease owner 或内部 user id。`/prompt-learning` 页面覆盖策略加载、启停、手工版本保存、版本/run 历史、空态和失败重试；仅 `pending`、`leased`、`preparing` 显示取消，提交/未知结果不能伪称已取消。个人资料页提供入口。每个已审版本可在浏览器本地导出为 Markdown，导出不触发文件应用或网络调用。文案通过七语言脚本+sync 写入，前端 API/组件交互测试、typecheck、lint 和 build 通过；完整 Vitest 为 97 文件、458 项通过。未加入最终全局导航信息架构，亦未提供模型生成、样本审阅或文件应用。
- **A3-F6 安装恢复 journal/暂存基础：** `cli/lib/installation-journal-store.mjs` 将 schema-1 fresh-install journal 接到调用者指定的受管理根 `state/`：journal 原子 rename、锁目录 owner record、owner 匹配释放、畸形 journal fail-closed，所有路径拒绝根目录和符号链接。持锁 executor 只能先从持久 journal 声明下一 effect，声明成功后才允许执行，并且只能从持久 pending effect 完成同一状态；已验证 identity 在持锁且已声明 `artifact_verified → staged` effect 时，才允许把调用者指定的本地普通文件复制到操作专属 `staging/`，并对复制结果重新 SHA-256 校验，失配会清理临时文件。新的持久暂存入口只读取持锁 root 内的 journal，复制和验 hash 后才原子记录完成的 `stage` effect；若在两者之间中断，journal 保持 pending，不能把暂存文件误报为已完成。它不读取下载制品、不形成 trust、没有 system service/Docker 操作，也不会清理运行文件。完整 CLI Node 回归通过 76 项；D11 受信 manifest 与真实安装执行仍待。
- **当前集成回归：** 本轮 `go test -p 1 ./...` 全包通过；`go vet -p 1 ./...` 无输出；`relaykit` 内 `GOWORK=off go build -p 1 ./...` 通过。结果覆盖当前 macOS 本机代码与合成服务，不等价于 MySQL/PostgreSQL、真实 Redis、Docker、真实支付/通知、浏览器或声明平台安装验收。
- **候选构建与发行检查：** `npm test` 为 72/72，前端完整 Vitest 为 97 文件/455 项；`typecheck`、`lint`（仅既有 footer `react/no-danger` warning）、`format:check` 和生产构建均通过。按当前 `VERSION=0.1.1` 重建前端，并用既有发行 `-ldflags` 注入同一版本后，隔离 SQLite 实例在 `127.0.0.1:39084` 启动，`/api/status` 回读 `0.1.1` 并收到 SIGINT 后退出。`npm run release:check` 的 CLI、升级、运行时、工作流和包内容子检查均通过，最终 `pack:check` 因 `SOURCE_MANIFEST.json` 检出 `sourceTreeDirty=true` 正确失败；当前有大量未封版工作树内容，且既有 `v0.1.1` tag 已受保护，不能把此源码伪称为该 tag 的候选或发布。下一候选版本、同一提交 SHA、干净工作树、manifest/签名/制品及外部验收仍是 R5 前置。
