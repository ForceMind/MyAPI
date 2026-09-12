# Mac 核心大更新：实施交接与阶段清单

更新日期：2026-09-12。本文记录本次获批 Mac 首批工作的实施边界和最新审计；不是已完成、已迁移或已启用的证明。

## 当前基线与已知事实

- 实施分支：`codex/mac-durable-accounting`。
- WP2-B 审计范围：`953428d6b2b85a6230ed01e3efa2b214f6222ba5..33e14c063a3c9128f48f3acf9a0cd0e8683afb19`，共 1 个提交。
- 本地开发分支指向 `33e14c063a3c9128f48f3acf9a0cd0e8683afb19`，远端开发分支仍为基线 `953428d6b2b85a6230ed01e3efa2b214f6222ba5`；`VERSION` 为 `0.1.1`。
- 基线及此前 `main` CI 仅为历史证据，不能替代 `33e14c063a3c9128f48f3acf9a0cd0e8683afb19` 推送后产生的精确 SHA CI。
- Sol 最终独立审查已完成；Terra 已通过 181 项定向测试、4 项 race、`go vet`、`relaykit` 独立构建和 SQLite 验证。该结果不等同于外部方言或整体验收。
- 当前本机工具版本为 Go 1.27、Bun 1.4、Node 26.7。MySQL 5.7、PostgreSQL 9.6 和 ClickHouse 24.8 实库验证尚未执行；`Docker` 不在当前 `PATH`，因此未做容器相关验证。
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

2026-09-12 独立审计将状态拆分为“代码 / 接线 / 验证 / 生产启用”。“实现子范围存在”只说明范围内有代码，不能替代可恢复的账务闭环、独立复核或生产资格。

| 阶段 | 范围 | 代码 | 接线 | 验证 | 生产启用 |
| --- | --- | --- | --- | --- | --- |
| M0 | 基线、授权边界、D02/D03 决策入档 | 已入档 | 不适用 | 文档一致性已检查 | 未启用 |
| M1 | B3-A1 T1 reserve + receipt，gate-off | 已落盘 | 仅限 gate-off 范围 | 本机验证通过；外部方言待验 | 未启用 |
| M2 | B3-A2 与 B2-2B/C：终态结算、释放、异步调度与状态机 | 已落盘 | gate-off 范围内已接线 | 本机验证通过；外部方言待验 | 未启用 |
| M3 | 异步恢复、日志 Outbox、一致性审计与消费去重 | 已落盘 | gate-off 范围内已接线 | 本机验证通过；外部方言待验 | 未启用 |
| M4 | 系统任务、恢复执行、持久入口协议、backfill 与控制器接入 | 已落盘 | gate-off 范围内已接线 | 本机验证通过；外部方言待验 | 未启用 |
| M5 | C03b：Redis 余额投影与权威全写入 | P1-1 未处理 | 共享 schema/cache 逻辑仍会生效 | 待 WP3 | 未启用 |
| M6 | 真实切换评审与上线准备 | 未开始 | 未开始 | 未开始 | 未授权 |

### 2026-09-12 Phase A / WP1、WP2-B 状态

- 基线为 `953428d6b2b85a6230ed01e3efa2b214f6222ba5`；WP2-B 代码已提交为 `33e14c063a3c9128f48f3acf9a0cd0e8683afb19`。精确 SHA 的 CI 尚待推送后触发，远端基线不能替代该证据。
- Phase A / WP1 历史记录保留：`da6a59a` 为代码基础，文档基础为 `ba80b8b`。P1-2 固定 wallet、P1-6 Task candidate 原子落库及恢复、P1-7 quota clamp 后拒绝上游请求已在该基础上落盘。
- WP2-A-core `779901cf0a2f3a793980785ee6b5612fc58a575b` 与 WP2-A-entry `f9bd6c04b1153390d2d9a0e423480925dc4e5c9d` 已完成 P1-3/P1-4/P1-5 的 durable core 和全部 polling/realtime terminal 入口；WP2-B 在其上完成日志 `billing_event` 消费去重。
- WP2-B 已落盘稳定 canonical、row key 和 digest，事件清理，关系库/ClickHouse quarantine，轻量 startup fail-closed，持久 backfill 的 SystemTask、fence 和索引阶段，以及 ClickHouse identity 与 materialize 恢复。
- Sol 已完成最终独立审查。Terra 已通过 181 项定向测试、4 项 race、`go vet`、`relaykit` 独立构建和 SQLite 验证。
- P2 日志消费去重标为当前代码完成；MySQL 5.7、PostgreSQL 9.6 和 ClickHouse 24.8 实库验证尚未执行。P2 settlement/refund 静默截断 core 已关闭。
- P1 仅剩 Redis 投影与权威全写入，归入 WP3。不得将外部方言待验、精确 SHA CI 待推送或 gate-off 状态表述为生产资格。

默认 gate 保持关闭。Linux、生产、M6 真实切换、发布、真实数据处理和 UI 重做均未获授权。

下一执行点：**WP3 C03b writer inventory 迁移**，随后处理 Redis **bridge / drain / epoch**。之后才继续 WP4 恢复操作面、WP5 独立验证与计划同步。Sol 负责主要实现，Terra 负责测试/文档，主代理负责最终复核。

## 禁止事项

- 不完整重做 UI。
- 不部署 Linux、不发布、不处理真实数据。
- 不自动处理历史不明差异。
- 不把 gate-off 基元、历史 CI 或纯逻辑测试表述为真实迁移、真实切换或全量验收已完成。
