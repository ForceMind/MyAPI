# Claude 接手：My API 完整首个预发布

交接日期：2026-09-16（第二版，取代 2026-09-15 初版的中断状态描述）。本文件是本次交接的最新状态快照；逐批证据以 `docs/FULL_PRERELEASE_EXECUTION_PLAN.md` 第 5 节的 2026-09-15/16 进度条目为准。

## 1. 用户目标与执行方式

按 `docs/FULL_PRERELEASE_EXECUTION_PLAN.md` 完成最早完整计划全部必选目标，之后才形成首个预发布候选。完整目标包括：可靠调用/持久账务、账户与 Key 强制策略、额度/告警、提示词学习与版本中心、Full/Lite/Desktop、统一安装/更新/切换/恢复、完整独立 UI/官网、制品与文档。

用户要求加快实际功能开发，省略不必要模拟测试、重复全量检查和形式化审查。保留金额、权限、并发、未知提交、恢复和真实交互的必要验证。复用已有成果，不重新做全项目盘点，不以关闭功能或删测试制造通过。

本轮实际执行方式：主代理串行派单 coder 子代理（共享文件单 writer），每批主代理抽查关键账务代码并独立复跑定向测试；每批更新执行计划同一文件。

## 2. 实时 Git 和状态

- 目录：`/Users/wxx110/工作/Prive/MyAPI`。
- 分支：`codex/mac-durable-accounting`；HEAD：`504d4beb2f5ca72e5ebdb7462dc7416da5cd92ea`（自始未变，本轮无任何提交）。
- 交接只读核对：388 个文件有改动（修改+未跟踪合计）。`git diff --check` 通过。
- 本轮没有任何 commit、push、tag、发布或部署。旧限制"不要提交、推送、部署"继续保留；用户授权持续本地开发，不自动授权发布、生产切换或真实资金操作。
- 阶段：A0 已完成；A1 软件实现主线完成但未宣布关闭（隔离实例启用验收、三库、真实 Redis、真实浏览器、真实账号均未做）；A2–A5 未开始。

## 3. 本轮已完成（2026-09-15/16，全部本机 SQLite/miniredis 验证）

1. **WP3-B2 capability CAS 最终验收**：统一发布 helper（捕获 pool/binding generation/旧 snapshot、CAS、发布前后复核、失效只向当前 binding 修复）；旧 refresh/Ensure 交错切库不覆盖新快照；同 pool 发布不倒退；legacy 库不误拒。race/全包/vet 通过。
2. **前端第22批**：tag-batch-edit/model-mapping-editor/upstream-update-dialog 差异核对完整，oxlint/typecheck/vitest 通过。
3. **credit 边缘批**：新用户初始额度（authoritative 同事务 `user_init:<id>` receipt，三注册路径）、invitee 奖励幂等（`invite:invitee:<id>`）、inviter aff 经 `invite_reward_grants` 恰好一次、aff 转钱包经 `aff_quota_transfers` 持久操作身份（重放零写/异值冲突/同事务 debit-credit-receipt，前端每次尝试生成 request id 复用重试）。
4. **token/legacy relay + Task/Midjourney 批**：postConsumeQuotaWithResult 三层模式分发（billing 回退/realtime 序号键/violation fee）；token 回滚键固定 `billing-token-rollback:{requestID}`；Task legacy 结算/退款事实化（`task-settle/refund:{taskID}`）；Midjourney settle/refund 三模式+持久事实（`mj-billing/mj-refund:{mjId}`）。
5. **订阅/admin/注册批**：余额购 `subscription-wallet:{tradeNo}` receipt 与订单同事务；overflow 核实无新写入点；admin add/subtract/override 经 `admin_quota_adjustments`（锁内二次查重、并发收敛一次）；**writer 注册表逐 caller 核对后 10/13 置 true**，仅 `delta_update_user_quota` 无生产 caller 保持 false（AllWritersMigrated 仍 false）。
6. **受控切换操作面**：`quota_writer_mode_transitions` 证据表 + `ApplyQuotaWriterModeTransition`（epoch 行锁+LockVersion CAS、legacy→bridge 需集群排空确认、bridge→authoritative 重跑全部硬条件、失败落证据、authoritative 不可降级）；审计的 cluster_drain_ack/inflight_zero 改真实读取（Option epoch 绑定确认 + BillingSession 进程级在途计数）；`DriveQuotaWriterDrains` 有界排空；5 个 admin 端点 `/api/quota-writer/{status,plan,apply,transitions,drain}`；前端系统设置 Billing 分组「Quota Writer Mode」页面（审计清单/Plan/Apply 二次确认/Drain/历史）。
7. **KIMI 渠道**：Moonshot 渠道（type 25）模型列表五个旧模型官方全部下线，已更新为 kimi-k3/kimi-k2.7-code(-highspeed)/kimi-k2.6；前端显示名 "Kimi (Moonshot)"；抽屉 base URL 预设（moonshot.cn/.ai/kimi-coding-plan/自定义）；七语言同步。
8. **C09 全部子批**：
   - N2b 支付运行配置：46 键接入 PaymentRuntime 内核，17 条下单/webhook 路径入口单次快照，`stripe.Key` 全局态清零，发布失败整组中止停留旧代。
   - N4 成功硬限额（D05 落地）：reserve→commit/release 原子预留（Redis 3 Lua + 内存等价），committed+reserved 永不上限，失败释放、未知保留至过期，滚动升级兼容旧记录。
   - N3a typed bulk：`PUT /api/option/typed-bulk` + revision CAS（契约冻结于 `docs/OPTION_TYPED_BULK_API.md`），未知/重复键拒绝、单事务、按族一次发布失败中止。
   - N3b 前端：7 个白名单族表单迁单请求（checkin/ssrf/models 区四卡/passkey），共享 hook + 409 重载；混合型/排除族表单保持现状并记录。
   - N5b 受控修复：3 白名单动作、dry-run 零写入、apply 逐行 hash CAS+同事务备份、幂等、值不出 model 层；3 端点 RootAuth。
   - D14/D15 热读族：performance_setting/channel_affinity_setting 不可变 generation；磁盘目录与缓存容量/TTL 普通保存只更新配置代，经维护重建端点（`POST /api/option/disk_cache/rebuild`、`POST /api/option/channel_affinity_cache/rebuild`）或重启生效；修复 newDiskStorage 忽略 path 与单次决策混代两个既有缺陷。
9. **controller fixture 统一批**：38 失败+1 panic 归零（310 顶层测试全 PASS）；3 处共享 fixture 修复；3 处生产最小修复（stripe webhook nil 防护、Waffo/Pancake 解析失败告警补回）。

每批的详细证据、语义差异和边界见执行计划文档第 5 节进度条目。

## 4. 关键语义与已知边界（新接手者必须知道）

- **三模式 writer**：legacy / bridge / authoritative（`quota_writer_epochs` 单例）。bridge=静默排空：新 BillingSession 拒绝、在途可结算；批一业务 credit 在 bridge=legacy；订阅余额购与 admin 调整在 bridge fail-closed（有意差异，切换运维需知晓）。authoritative 不可降级。
- **有意的模式语义差异**：authoritative 下 admin 减法拒绝透支、余额购要求用户 enabled；legacy 保持逐字旧行为。
- **发布中止边界**：typed bulk/支付 bulk/受控修复均为"DB 提交后 runtime 发布失败→旧代保留+SysError"，不伪装跨 DB/runtime 原子。
- **统计列崩溃窗口**：Task/MJ 事实应用与统计列非同一事务，崩溃可残留 used_quota 偏高（不双退）。
- **多进程在途**靠操作员 cluster ack 背书，无自动聚合；authoritative 下常驻审计显示 cluster_drain_ack 缺失属设计。
- **Redis Cluster**：N4 的双键 Lua 需同 slot；请求超窗口时成功数保守不补记。
- **MySQL/PostgreSQL、真实 Redis、真实支付渠道、真实浏览器全部未验收**——所有"通过"均为 SQLite fixture/miniredis/组件测试。

## 5. 下一步（按优先级）

1. **A2-F3 账户/Key 权益策略**（D04 合同：Tier/Key/Profile/渠道能力取交集；inherit 不增加限制、显式空名单拒绝；最多一次且仅首次 dispatch 前 fallback；off→audit→范围 enforce）：现状 `service/authz` 仅覆盖渠道资源；用户模型已有 `AccountTierID`/`EffectiveAccountTierID`。需策略快照、缓存失效、audit/enforce、旧 Key 可审计迁移。
2. **A2-F4 渠道额度/告警闭环**：多 Key 身份与系列隔离（不能默认合并为一个上游账号）；告警持久记录与投递历史（首版=站内记录+管理员配置 HTTPS webhook，持久重试、SSRF/重定向约束）；渠道额度当前/历史准确性已有部分成果保留。
3. **A1 关闭前验收**：隔离实例实际启用 legacy→bridge→authoritative 切换演练；MySQL 5.7.8+/PostgreSQL 9.6+ 迁移与事务验证（可用 CI 临时服务）；真实 Redis 故障/恢复；真实浏览器关键路径。
4. **A3**：提示词学习与版本中心（F5，默认关闭、授权样本→脱敏→调度预算→不可变版本→人工应用/回滚）、安装/更新/切换/恢复（F6）。
5. **A4/A5**：完整 UI/官网、联合验收、封版。

## 6. 验证与工作区保护

- 读根 AGENTS.md 和目录规则；前端遵循 web/AGENTS.md。翻译用脚本写七语言并 `bun run i18n:sync`。
- 改文件前读当前内容；保留所有 WIP，不 reset、stash、clean、广泛回退或批量提交。
- 本机 Go 用 `GOMAXPROCS=1 GOMEMLIMIT=768MiB GOWORK=off`、`-p 1`；复用现有缓存；不删共享缓存。
- 前端：Bun；Vitest 用 `NODE_OPTIONS=--no-experimental-webstorage`（Node 26 默认 webstorage 会挂）或 Node 22（`/opt/homebrew/opt/node@22/bin/node`）。
- 当前全量基线：`go test ./model ./service ./controller ./router` 均全绿（controller 310 顶层测试）；`go build ./...` 与 `cd relaykit && GOWORK=off go build ./...` 通过；前端 typecheck/build 通过；全仓 lint 仍有既有项未清（A4 候选阻断保留）。
- 子代理单轮约 100 步上限，大批次需 resume 续跑；共享文件（model/option.go、controller/option.go、router/api-router.go 等）串行派工。
- 只有相关输入变化或具体失败才重跑/扩大测试；保留金额/权限/幂等/恢复断言；不为格式微调加机械测试。

## 7. 可直接使用的启动指令

请接手当前工作区，先阅读根 AGENTS.md、docs/FULL_PRERELEASE_EXECUTION_PLAN.md（唯一执行计划，含逐批证据）与本文件。保留全部未提交改动，从 A2-F3 账户/Key 权益策略开始（不按本文件重做已完成的 A1 批次）。核对并停止遗留测试进程时仅操作已确认属于本任务的进程。A1 未宣布关闭：隔离实例切换演练与三库/真实 Redis/浏览器验收仍欠账，排入 A2 并行或 A4 前完成。不得因局部通过声明 A1/完整首版完成，不执行未授权提交、推送或部署。
