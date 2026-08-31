# Claude 组织用量观测边界

本文档记录 MyAPI 对 Anthropic 官方组织级 Usage Report 的接入边界。它描述的是
可验证的后续设计，不代表当前版本已经向生产请求该接口。

## 官方接口

Anthropic 官方文档提供：

```text
GET https://api.anthropic.com/v1/organizations/usage_report/messages
```

参考：[Get Messages Usage Report](https://platform.claude.com/docs/en/api/admin/usage_report/retrieve_messages)。
接口支持 `bucket_width=1m|1h|1d`、时间范围、workspace/account/API key/model 等
分组和分页，返回每个时间桶的 token 与请求使用量。

官方文档同时区分产品形态和凭据类型：Claude Platform 使用 Usage & Cost Admin
API，要求 Admin API key、`org:admin` OAuth token 或同等组织级凭据；Claude
Enterprise（claude.ai）使用独立的 Analytics API，要求带 `read:analytics` 权限的
Analytics API key。Claude Platform on AWS 当前不提供这些组织用量/成本端点，不能
仅凭“渠道类型为 Claude”推断接口可用。参见 Anthropic 的
[Usage and Cost API](https://platform.claude.com/docs/en/manage-claude/usage-cost-api)、
[Analytics API](https://platform.claude.com/docs/en/manage-claude/analytics-api) 和
[Admin API 概览](https://platform.claude.com/docs/en/manage-claude/overview)。

## 与账户额度的区别

- Usage Report 是组织级使用量，不是预付费余额、订阅剩余额度或速率限制剩余量。
- 它不能填充当前“账户额度变化”面板，也不能从 token 使用量推算余额。
- 普通 Claude Messages 或 workspace 渠道密钥不能假定拥有该权限；必须显式配置
  与组织产品形态匹配的 Admin/Analytics 凭据或官方支持的等价授权。

## MyAPI 接入门槛

实现前必须同时满足：

1. 管理员明确选择启用组织用量观测，并提供专用 Admin 凭据；不读取本机 Claude
   配置、浏览器 profile、keychain 或个人订阅会话。
2. 凭据进入受保护的服务端密钥存储，日志、错误和响应均脱敏；前端永不返回凭据。
3. 新增独立权限（建议 `provider_usage.read`），与 `channel.read` 和余额历史权限
   分离；所有查询写入审计事件。
4. 只持久化规范化时间桶和数值字段，例如 input/output/cache/tool tokens、请求数、
   workspace/model 维度、数据质量和来源时间；不保存完整上游响应。
5. 对 `1m` 请求设置明确的时间范围、分页上限、超时、重试和缓存，避免把 Admin API
   当作高频余额采样器。
6. 组织用量使用独立表和独立 UI，标题必须写明“Claude 组织用量”；不与渠道余额、
   MyAPI 用户余额或单个 API Key 限额合并。

## 建议的后续交付

先实现只读、管理员可见的历史查询和 CSV/JSON 脱敏导出，再评估预算告警。告警应
基于组织用量或成本报告的明确指标，并单独配置阈值、冷却和通知通道。若 Admin
凭据、授权范围或保留策略未确定，继续显示“未配置组织用量观测”，而不是尝试使用
普通 Claude 渠道密钥。

## 验收条件

- 无凭据时不发起请求，页面显示未配置状态。
- 403、429、5xx、超时和分页错误分别显示为可诊断状态，不覆盖已有数据。
- 单位、时区、时间桶和分页结果可重复；跨桶聚合不伪造余额变化。
- 普通用户和无 `provider_usage.read` 权限的管理员看不到组织数据。
- 测试夹具只使用合成响应，不包含真实 API key、OAuth token 或组织标识。
