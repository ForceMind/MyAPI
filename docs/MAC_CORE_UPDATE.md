# Mac 核心大更新：实施交接与阶段清单

更新日期：2026-09-10。本文记录本次获批 Mac 首批工作的实施边界，不是已完成、已迁移或已启用的证明。

## 当前基线与已知事实

- 实施分支：`codex/mac-durable-accounting`。
- 代码基线：`f6536ca96126415f74d239165fd688589bd54af9`。
- PR #1 已于 2026-09-08 合入 `main`；主代理已通过 API 核实 CI `34186651121` 的十个 job 成功。
- Mac 的 44 项 CLI 纯逻辑测试已通过。该结果不覆盖本次后续实现，也不等同于整体验收。
- 当前本机工具版本为 Go 1.27、Bun 1.4、Node 26.7；尚未进行整体验收。`Docker` 不在当前 `PATH`，因此未做容器相关验证。
- 主代理已创建本次核心更新的持久 Goal，状态为进行中；阶段验收以本表和实际证据为准。
- Git HTTPS 连接已复查恢复：`git ls-remote origin refs/heads/main` 成功，返回上述基线。另已找到 Node 22.23.2，可显式使用而不修改全局环境。

## 本次已批准范围

按以下顺序推进，保持 gate-off：

1. **B3-A1 / T1：reserve + receipt**。建立受理预留与不可变回执的最小闭环；不接通真实切换或生产扣款。
2. **B2、B3、C03b**：仅完成与上述闭环直接相关的实现和配置；不得借机重做 UI。
3. 所有新路径保持关闭，真实请求路径切换、历史迁移执行、生产启用、发布、部署 Linux、真实数据处理均不在本次授权内。

## 已选定的业务决策

以下是用户于 2026-09-10 在本次授权下选定的建议值，供实现与审查使用：

- **D02**：主库是唯一可消费权威；Redis 仅为可重建投影，不拥有最终扣减授权。
- **D03**：对历史不明差异执行人工核查；不得自动扣款、退款、补扣或补退。

这些决策不表示迁移已执行，也不授权真实切换。真实切换须完成相应验收，并取得单独明确授权。

## 资源、验证与协作边界

- 串行资源限制：`GOMAXPROCS=1`、`GOMEMLIMIT=768MiB`；按需使用串行 Go 执行方式。
- 计划验证：相关定向 Go 测试，随后 Go 全量、race 与 SQLite/MySQL/PostgreSQL 三库 CI；尚未执行的验证必须标为待验证。
- 本机没有可确认的 `systemd` 硬限额；不得把上述 Go 运行时设置描述为整个进程组的硬资源限制。
- 规划采用 Sol/high（当前环境不支持 ultra）；主代理负责协调，文档工作采用 Terra/medium；后续安排独立审查。
- 主代理统一安排测试与集成；实现由 Sol/high 承担，独立审查不参与相同实现。

## 简短阶段清单

| 阶段 | 范围 | 状态 | 完成依据 |
| --- | --- | --- | --- |
| M0 | 基线、授权边界、D02/D03 决策入档 | 已完成（文档） | 本文及两个总计划的 2026-09-10 交接记录 |
| M1 | B3-A1 T1 reserve + receipt，gate-off | 已完成（代码/测试/审查） | 余额版本、三库迁移、事务内核、回执不可变保护、三库测试夹具；定向测试、-race 检测、relaykit 独立构建、go vet 全过；独立审查 P2/P3 项已全部闭环修复 |
| M2 | B3-A2（T3/T4 终态结算与全额释放）+ B2-2B/C（异步调度出站与状态机） | 已完成（代码/测试/审查） | `model/quota_mutation_settle.go` 终态差额结算（多退少补等额）与全额退款释放；复合唯一索引 `(operation_id, mutation_type)` 演进与迁移测试；`service/task_submission_service.go` 调度流水线与事务隔离；独立审查 P2 全修复（严格 User->Token->Sub->Op->Attempt 锁序、超时/断网 fail-closed、严禁 DB 事务内网络 I/O）；5 大矩阵与 12 调度场景测试 100% PASS，-race 0 竞态，relaykit 独立编译通过 |
| M3 | 异步任务轮询恢复引擎、日志 Outbox 投递与一致性审计（B2-2D / B3-B / B3-C），gate-off | 已完成（代码/测试/审查） | 1. Model 层扫描基元（ListStaleDispatchingOperations、ListUnfinishedPreparedOrReservedOperations、ListClaimableTaskBillingLogOutboxes 等）；2. TaskRecoveryWorker（超时 dispatching 严格 fail-closed 隔离至 submission_unknown，未出站 reserved 安全取消释放，过期 lease 回收）；3. TaskBillingOutboxService（Outbox 租约抢占、去重投递至 logs、指数退避重试）；4. TaskResolutionService（人工审计闭环，AuditCommandID 幂等重放与冲突拒绝）；5. 轮询终态持久对账桥接（DurableSettleTaskOnComplete 与 DurableReleaseTaskOnFailure，自动流转至 succeeded/failed 并投递 Outbox）；全部测试通过，-race 0 竞态，relaykit 独立编译通过 |
| M4 | 系统任务注册、恢复执行引擎、持久入口协议与控制器接入（B2-2A / B2-1 / SystemTask），gate-off | 已完成（代码/测试/审查） | 1. `model/system_task.go` 注册 `task_recovery` 与 `task_billing_outbox` 系统任务；2. `controller/system_task_handlers.go` 挂接 `service.TaskEngineRunner` 门面执行恢复巡检与 Outbox 投递，绑定 `IsTaskRecoveryObligationRecoveryEnabled` 开关；3. `service/task_ingress_service.go` 完成路由动作识别（video.create, video.remix, suno.music, suno.lyrics）、三种指纹提取（JSON, Form, Multipart）、意图创建与重放判定；4. `controller/relay_task_durable.go` 实现持久入口控制器，提供 Location、Cache-Control: no-store、HTTP 202 响应规范；5. 独立审查发现的 P1（头修改导致重拒）、P2（别名头逃逸、nil resp、500 误报、饱和 clamp 审计、全局 DB 污染）全部彻底修复；6. 包含 端到端全新提交、幂等重放、指纹冲突、别名拦截、传统链路 gate-off 短路 的单元及集成测试 100% PASS，-race 0 竞态，relaykit 独立编译通过 |
| M5 | 统一账务与余额投影收口（C03b: C03b-1 Redis 余额投影防回滚 + C03b-2 通用权威账务变更内核与收据），gate-off | 已完成（代码/测试/审查） | 1. `model/user_cache.go` 升级 `userCacheSchemaVersion = 4`，引入 `QuotaVersion`；2. `model/user_auth_cache.go` 升级 Lua 脚本实现严格防回滚（`incomingQV <= currentQV` 拦截重写，冷缓存防残缺哈希）；3. `model/token_cache.go` 同步引入 `QuotaVersion` 与防回滚；4. `model/user_quota_mutation.go` 实现 `UserQuotaMutationReceipt` 模型、不可变 Hook、GORM 全局防写 Guard、`MutateUserQuotaAuthoritative`（用户行锁 + 双重检查 CAS + 幂等重放/冲突拒绝）与提交包装 `MutateUserQuota`；5. 独立审查发现的 P1（MySQL 5.7 TEXT default 语法错误）、P2（等版本覆盖实时扣费、双重检查加锁防并发冲突、零 delta 校验）全部闭环修复；6. 单元、高并发 CAS 竞争、重放与投影防回滚、三库迁移测试 100% PASS，-race 0 竞态，relaykit 独立编译通过 |
| M6 | 真实切换评审与上线准备 | 未授权 | 独立验收完成后另行明确授权 |

## 禁止事项

- 不完整重做 UI。
- 不部署 Linux、不发布、不处理真实数据。
- 不自动处理历史不明差异。
- 不把 gate-off 基元、历史 CI 或纯逻辑测试表述为真实迁移、真实切换或全量验收已完成。
