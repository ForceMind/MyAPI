# P2-A1 增量结构审查（2026-11-07）

基线：dev / `f46f19c71dbf6954620feb43c01b8d8aa87bb1d5`。本文接续原 architecture-review.md，只核对第二阶段 A1 的两个具体缺口：Lite 支付回调、仅外部登录账户的访问能力。主代理只读源码，没有运行行为测试、访问生产、查询真实账户或订单，也没有发送邮件、支付或模型请求。下表均为**从代码看**，不是新运行回归的结论。

## 本次结论

Full→Lite 不能当作普通镜像更新直接执行：回调路径会改变可达性，无密码账户也可能失去下一次登录入口。当前 Full→Full 的 beta.34→beta.36 打包不改变 edition，不需要为它扩大为降级修复。签名校验、资金 mode/epoch 与订单匹配仍必须保留，不能为恢复回调可达性而删门禁。

| ID | 从代码看 | 直接依据（本轮已读对应段落） | 影响与后续验收 |
| --- | --- | --- | --- |
| A1-01 | EditionGuard 在业务路由注册之前安装；Full 直接继续，Lite/lan 按路径阻断 | `main.go:203`、`middleware/edition.go:88` | 本文 Lite 路径结论不应推成 Full 当前线上失败；真实线上行为未测 |
| A1-02 | Stripe、Creem、Waffo webhook、Epay 充值 notify 和订阅 notify/return 被 Lite 前缀匹配提前返回403；签名及订单处理器还未执行 | `middleware/edition.go:131`、`:163`；`router/api-router.go:78`、`:98`、`:213` | 有未完成订单时直接降级可能切断履约；待合成真实路由回归，不联系供应商或重放现有通知 |
| A1-03 | `/api/waffo-pancake/webhook/:env` 不匹配 `/api/waffo` 加斜杠的前缀规则，且没有独立 Pancake 项；其可达性与其他提供方不同 | `router/api-router.go:83`、`middleware/edition.go:142`、`:163` | 这是版本路径差异，不能据此声称无签名可充值或直接封掉已付订单；下一轮统一降级策略后再决定处理方式 |
| A1-04 | Pancake 处理器仍检查test/prod、已配置签名、事件mode/type，再做资金决策；结算仍按订单种类分别处理 | `controller/topup_waffo_pancake.go:465`、`:476`、`:482`、`:487`、`:541` | 不把路径差异写成资金授权绕过；这些门禁必须由后续合成回归保持 |
| A1-05 | 资金策略与 edition 是不同维度；disabled 确认但不结算，retirement 只允许截止 epoch/ID 内历史订单，enabled 仍要求提供方、pending状态和epoch匹配 | `service/user_funding_policy.go:28`、`:74`；`model/user_funding_policy.go:381`、`:402` | 仅放行HTTP路径不足以证明会入账；也不能因禁购买而把retirement改成disabled、清空订单或放宽资金校验 |
| A1-06 | 订阅/充值的资金决策会在事务中重新校验，订阅订单使用跨库lockForUpdate并校验支付提供方与状态 | `model/subscription.go:632`、`:645`、`:651`、`:660`；`model/topup.go:326`、`:332`、`:341` | 后续 A2 必须保事务、状态、幂等及精确单次入账；本轮未审完整个结算子系统，不声称三库新行为已验 |
| A1-07 | 外部OAuth新用户从空User开始，不填密码；InsertWithTx允许空密码，插入默认设置及初始额度，不自动生成本地登录密码 | `controller/oauth.go:292`、`:333`、`:370`；`model/user.go:665`、`:674`、`:775`、`:132`；`model/credit_edge.go:100` | 无密码账户是实际支持的创建路径；不能假定每个外部登录用户已有本地凭据 |
| A1-08 | Lite封 `/api/oauth`；密码登录虽可达，还受PasswordLoginEnabled控制，空存储密码明确拒绝 | `middleware/edition.go:139`；`router/api-router.go:66`、`:75`、`:94`；`controller/user.go:40`；`model/user.go:1105`、`:1125` | 缺本地密码及可用Passkey时，重新登录可能不可达。此结论不表示现有会话立即失效；本轮未审所有会话续期路径 |
| A1-09 | Passkey登录不在Lite禁止前缀中，但需要开关/WebAuthn环境及已注册凭据；有API路径不能证明用户实际拥有可用凭据 | `router/api-router.go:95`；`controller/passkey.go:299`、`:309`、`:382`；`middleware/edition.go:131` | 降级前应实际确认所需账户的本地登录能力，不得仅以全局开关或凭据条数替代用户能登录的证明；浏览器/认证器未验 |
| A1-10 | 个人改密检查会拒绝旧密码为空；邮箱重置另有保留路径，通过唯一邮箱与重置码更新密码并撤销会话 | `controller/user.go:923`、`:934`；`router/api-router.go:63`、`:64`；`controller/misc.go:301`、`:334`；`model/user.go:1253` | 不能笼统建议无密码用户“在个人资料直接设密码”，也不能说邮箱恢复不存在；邮件投递、邮箱唯一性、密码登录开关及恢复后登录仍须确认，未发邮件 |
| A1-11 | CLI升级以已有.env中的edition选镜像并校验配置；该计划构造没有检查数据库pending订单或逐账户凭据。安装器既有保护主要核对目标身份和batch配置；受限运维服务仅支持Full | `cli/myapi.mjs:1183`、`:1190`、`:1197`；`deploy/install.sh:303`、`:314`；`tools/operations/docker.mjs:86` | 不把P1来源预检当作Full→Lite业务降级预检。受限运维服务不应扩展为Lite切换，本次也不安装它 |

## 建议的最小后续顺序（尚未改产品）

1. 继续 A1 的账务/异步任务恢复与部署资产审查，复用本报告追加证据，不新建另一份总账。
2. A2 先给精确方案：对真正的 edition 降级做失败关闭的预检，明确未完成充值/订阅订单、账户本地登录能力和未知/失败状态；不自动取消订单、不替用户设密码、不删除OAuth绑定、不回放通知。
3. 合成回归须分开验证 edition、funding mode、合法签名、pending/已成功/错误提供方/过期epoch，以及有无本地凭据；Full既有路径与单次结算不变。所有实际构建测试只在 Actions，真实服务与浏览器另行授权。
4. 本报告只记录发现；在具体范围确认前，不修改支付回调白名单、认证方式、资金状态、数据库或升级器。Full→Full生产更新继续使用既有严格镜像身份、最新数据保全与配置保留流程。

## 覆盖与未验范围

- 本轮逐段读过：edition guard、全局挂载与API路由、Stripe/Creem/Waffo/Pancake/Epay处理器对应入口、统一funding决策与订单事务校验、OAuth创建、用户插入/密码校验/改密/邮箱恢复、Passkey登录、CLI计划/安装器预检及Full运维限制。
- **P2-A1整体未完成**：账务与异步任务恢复其余逐文件审查、`tools/`全目录、`website/`、`deploy/`全资产仍待。790个单调用点与637个翻译键是历史候选数，未在本轮重新测量或认定可删除。
- 只读审查没有新增行为测试，不将既有CI绿当作上述所有降级场景已验证。没有运行本机测试/浏览器/Docker或安装依赖，没有下载构建产物，没有查询生产账户/订单或调用供应商。
