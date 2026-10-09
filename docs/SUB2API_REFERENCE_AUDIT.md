# sub2api 有界对照：个人模式、路由与 Key 限额

核对日期：2026-10-09。MyAPI 基线：`7492ec36edf68716953cc81c304f4c902d392751`；
发行源码：`c45df8c6c6bf2e66d39b58b8c2bc8d61984f901b`。本报告是源码与文档对照，
未运行 sub2api、未接入真实账户，不是两项目功能等价或性能测试结论。

## 固定原始来源与许可

主计划原链接确定为 **Wei-Shaw/sub2api**，不是同名镜像或 MyAPI 的 sub2api adapter。
本次固定 [commit 3a6fd1c9](https://github.com/Wei-Shaw/sub2api/commit/3a6fd1c9db07203ca308aaba69e502bc1f35b307)，
提交时间 2026-10-09 03:36:07 UTC，版本同步为 0.2.15，tree
`6b38d6e60f2b930a6c8073f25a4ac51c3f882de5`。原始来源：

- [README：Simple Mode / Project Structure](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/README.md)
- [复合路由文档](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/docs/COMPOSITE_GROUPS.md)
- [实际路由解析器](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/backend/internal/service/composite_route_resolver.go)
- [Simple Mode 准入测试](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/backend/internal/service/billing_cache_service_simple_mode_test.go)
- [Simple Mode 用量写入测试](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/backend/internal/service/gateway_usage_billing_simple_mode_test.go)
- [LICENSE](https://github.com/Wei-Shaw/sub2api/blob/3a6fd1c9db07203ca308aaba69e502bc1f35b307/LICENSE)：GNU LGPL v3；README 声明 v3 或以后版本。

本轮只记录行为与边界，没有导入对方代码、页面、资产或文案。任何后续代码复用须另行核对
具体文件来源、许可和通知义务；不能从“参考设计”推导出任意复制授权。保留 MyAPI 既有法律通知。

## 对照及取舍

| 方面 | 固定上游证据 | MyAPI 实际位置与差距 | 取舍 |
|---|---|---|---|
| 接入 | README 将账户接入、API Key、前后端职责分开 | `relay/channel/sub2api/adaptor.go` 仅继承 `newapi.Adaptor`；实际 OAuth、转发在现有 `oauth/`、`relay/`。adapter 不是整个 sub2api 已融合的证明 | 复用已有 provider 接入与身份；不导入第二套网关或扫描凭据 |
| 路由 | 明确 public model、真实 provider/upstream model、endpoint、preview；解析器显式规则优先、再账户证据/检测 | `model/channel_model_route.go`、`controller/channel_upstream_update.go`、`relay/helper/model_mapped.go`；beta.4 已接发现证据、映射、预览、真实派送及日志 | 已部分采纳明确映射/解释。MyAPI 同优先歧义拒绝；不照搬上游最终按 route ID 选中，也不把其多平台资格外推到 MyAPI |
| 个人模式 | Simple Mode 隐藏 SaaS，跳过余额/订阅扣减；可选 Key 窗口 | `model/user_usage_policy.go`、`service/funding_source.go`、`service/self_use_request.go` 已有显式无钱包策略，`web/src/features/users/components/user-usage-policy-dialog.tsx` 已有 Root 操作入口 | 采纳更简单的模式表达和入口。沿用已有策略、幂等与持久审计，不新造并行模式、不自动改变旧用户 |
| 计量与限额 | README 明确可选 5小时/日/7日 Key 费用窗口为请求后软上限，并发可能超过；测试只记 Key 窗口，不扣用户余额/订阅/账户额度 | `model/token_budget.go`、`service/token_budget_chat.go` 与现有 reservation/journal 是受资格限制的严格预算；普通 Key 的内部额度与 Token/USD 不同 | 借鉴单位/窗口的清楚表达；不以软限额替换严格预算、不跳过未知保留或以估算宣称实际账单 |
| 商业边界 | Simple Mode 隐藏 SaaS，不等于删除整个项目的商业实现 | `setting/operation_setting/user_funding_setting.go` 默认 disabled；现有充值/订阅/支付与历史恢复代码仍在 controller/model/service | 自用消费与可选商业隔离；不删历史账目、回调/恢复义务或解除服务端权限 |
| 结构 | README 列 backend/internal 的 config/model/service/handler/gateway 与 frontend；目录分层本身不是复杂度指标 | MyAPI 仍是 controller/service/model/relay 单体；`relaykit` 已独立且不依赖 HTTP/DB/计费，前端已 feature 化；尚未完成个人核心瘦身 | 先明确核心准入/预算/用量与商业资金接口及依赖测试，再按一个边界提取；不为目录好看全库搬迁或另建业务核心 |

## 当前完成度与停止条件

- 已完成这次固定 commit、许可、六方面有界对照；路由参考点已有 beta.4 实现与合同。
- 尚未完成“像 sub2api 一样简单”的 MyAPI 个人使用闭环，也未完成整个代码结构精简。
- 不继续无限搜索参考项目。下一步将可用取舍落实到[个人核心交付卡](PERSONAL_CORE_DELIVERY_CARD.md)，
  每条需求必须关联实际页面、服务端 caller、回归和验收。上游今后变化不自动扩大范围。
- 限额资格必须据 MyAPI 真实证据声明：原生合格 Responses 文本与精确 `gpt-6.1-sol` 原生 Chat 文本；
  Codex 百分比是账户窗口安全阈值，尚非逐 Key 百分比账本。兼容转发、媒体、模型发现都不自动授予严格资格。
