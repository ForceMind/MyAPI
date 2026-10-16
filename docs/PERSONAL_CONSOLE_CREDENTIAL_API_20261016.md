# 凭据维护 API 合同（beta.18 候选）

## 范围与身份

此合同对应已批准的个人控制台下一批。只允许启用的 Root 以真实控制台会话维护本人未删除的 Key；控制台 Access Token/PAT 不获得该入口。模型层再次核对当前角色、状态和 Key owner。跨账号指 Codex 上游服务商主体，不改变 MyAPI 用户、Token ID 或 API secret。

四个接口使用 `Cache-Control: no-store`。读取和预检只查询数据库，不调用上游、不刷新分额 projection、不写审计或别名。无法确认的状态标为 `unknown`，不能冒充可用容量。

| 方法与路径 | 作用 |
| --- | --- |
| GET `/api/token/:id/maintenance` | 脱敏 Key、轮换阻断、当前账号/窗口和目标候选 |
| POST `/api/token/:id/account-migration/preview` | 明确目标与全部窗口映射的只读预检 |
| POST `/api/token/:id/account-migration` | 原子迁移与不可变回执 |
| GET `/api/token/:id/maintenance/operations/:operation_id` | 按本人 Key 和操作 ID 回读脱敏结果 |

不存在轮换提交路由。普通 Token 更新不因此获得替换 secret 的能力。`rotation.commit_supported=false` 及稳定阻断码由后端返回，不依靠禁用按钮保护。

## 预检与提交

预检 body 恰有 `target_channel_id` 和 `target_shares`；后者为 1–2 项 `{source_pool_id,target_pool_id,share_bps}`。源账号由现有正分额推导，源全部启用原生主/次窗口完整匹配目标全部启用窗口，同槽位且本批保持原比例。目标必须是启用、单凭据 Codex 渠道，具有当前凭据关联、服务商确认主体与新鲜可信既有基线池。缺池由现有分额配置建立，迁移不自动建池或重置别人基线。

返回 `status=ready|blocked|unknown`、`preview_revision`、`expires_at`、`blockers:[{code,state}]` 和 `changes`。仅 ready 有 64 位小写 hex revision；其他状态 revision 为空。变更摘要描述 policy revision、source/target channel、窗口映射及目标 allocation 是否已存在，不包含 secret、原始账号 ID 或完整主体引用。

提交 body 恰有预检两项，以及 `operation_id`（64 位小写 hex）、`preview_revision` 和 `confirmed:true`。不接受客户端 owner、subject 或 policy JSON。重复字段、大小写别名、额外字段、null、空窗口、超过两窗和尾随 JSON 均拒绝。

POST 两路有真实会话 Origin guard 和限流；提交额外要求 `X-Security-Proof` 的 `token.account_migration` 专用 scope，使用既有 Passkey/2FA 验证、会话绑定和 5 分钟有效期。`channel.key.read` 等 scope 不授予迁移权限。

## 事务、回执与失败

提交锁当前用户/Token，按主体排序锁两个 account gate，再重新读取并锁定选中渠道、窗口、分额及策略。完整状态摘要包含当前凭据、证据、policy、generation、分额和用量。任一变化导致旧 preview 失效；目标容量不足、上层策略不允许目标、预算与分额互斥或未决/未知请求均阻断。

同一事务完成源分额归零、目标分额、必要的 Key assigned channel 替换和不可变 `TokenAccountMigrationReceipt`。目标已存在 allocation/usage 时保留其 ID 和历史用量；回迁不能通过重新建行清零。旧池、generation、snapshot、attempt、审计和金额记录不搬移或删除。提交不会关闭旧池或删除渠道。

同 operation ID、同规范请求摘要返回已提交结果并 `replayed:true`；异请求 409。提交与 GET 回执的 `data` 均为脱敏回执字段及 `replayed`，不嵌套秘密值。回执由 actor、Token 和 operation 共同限定。

错误使用 400 invalid、401 session required、403 denied/proof、404 not found、409 conflict/blocked、412 precondition unknown（事务在迁移写入前明确拒绝）、503 commit unknown。412要求重新读取并预检；DB/事务提交结果或已提交回执无法确认时仍503。未知提交不能自动重发 POST；客户端仅回读回执，失败保持写锁。pending operation ID 在 POST 前写入并回读 sessionStorage，按 actor/Token 隔离；关闭重开或同一标签页刷新先 GET 原回执，不生成新的提交。unknown/404 保留记录；确认 committed 或明确未受理的 deterministic commit 4xx 才 compare-and-clear。存储失败不能发送 POST。sessionStorage 不覆盖关闭标签或清除站点数据，不能声称跨设备恢复。404 回执不证明请求未提交。HTTP 408、499、5xx、网络失败或响应无法解码均按未知处理。没有自动回滚或旧凭据恢复承诺。

## 轮换资格与验证边界

本批轮换是有后端依据的只读预检。缓存批次可能持久化旧 secret 的 HMAC locator，同步在途请求也可能携带旧值后结算。完整生命周期阻断、旧/新缓存保护和兼容资格尚未证明，故不开放单值轮换。后续不得以清队列、补余额、复制已用量或改写历史取得资格。

接口、本机定向、三库合同及真实Chromium/Go维护流程已实现并取得验证证据；完整资格、准确源码及独立审查范围以[本批交付记录](PERSONAL_CONSOLE_CREDENTIAL_DELIVERY_20261016.md)为准。
