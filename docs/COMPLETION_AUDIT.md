# MyAPI 完成度与证据矩阵

本文档把总体计划中的目标映射到可复核证据。`已验证` 只表示代码、合同或 CI
已经提供证据；真实设备、生产副本、法律和正式发布不会因静态检查而自动变成完成。

| 领域 | 已验证证据 | 当前状态 | 仍需外部条件 |
| --- | --- | --- | --- |
| API 兼容 | `relay/` 转换器与后端 CI | 已验证 | 上游版本变化时继续回归 |
| API/响应日志 | `web/src/features/usage-logs/`、移动集成测试、脱敏测试 | 代码已验证 | 真实手机视觉验收 |
| 渠道额度历史 | `controller/channel-billing.go`、历史/聚合测试、权限路由测试 | 已验证 | 真实登录账号和采样数据演练 |
| 概览额度变化 | `account-quota-changes-panel.tsx`、60 秒前台刷新、错误/plan type 测试 | 已验证 | 具备 `channel.read` 的真实管理员验收 |
| 账户等级/Key 访问方案 | `model/access_profile.go`、Key/UI/API 测试与策略注册表 | 兼容层已验证 | 强制路由迁移评审 |
| 设置引导 | Full/LAN Lite/权限条件、生命周期测试 | 已验证 | 多设备视觉检查 |
| 品牌与旧元数据 | `tools/branding/check.mjs`，最近运行 `blocking_count: 0` | 阻断项已清零 | NOTICE、源码头和兼容标识法律审查 |
| 静态官网 | `tools/website/check-static.mjs`、Chromium smoke、artifact workflow | 自动化已验证 | 真实移动视觉与独立域名发布决策 |
| LAN Lite/桌面 | `lan:check`、`desktop:check`、Electron 安全边界 | 合同已验证 | macOS/Windows 实机安装、LAN 请求、防火墙 |
| GHCR/升级 | `release:workflow:check`、`upgrade:check`、不可变 digest 合同 | 自动化已验证 | 脱敏副本升级、数据库恢复、人工审批 |
| Claude 组织用量 | `docs/CLAUDE_USAGE_REPORT.md`，官方 Usage Report 边界 | 设计已验证 | Admin 凭据、权限、保留策略和实际接入 |
| Google Antigravity | `docs/ANTIGRAVITY_INTEGRATION.md`，官方 Interactions 边界 | 能力边界已验证 | 稳定官方余额接口；不存在时保持 unsupported |
| NPM 正式发布 | CLI/打包/版本合同检查 | 发布前检查已验证 | 版本确认、tag、清单、用户明确确认与 `npm publish` |

## 最近 CI 证据

- `33334175656`：完成度证据矩阵提交后的完整 CI，Backend、Frontend、Desktop
  和 Distribution 四个作业全部成功。
- `33333993651`：README 多语言导航更新后的完整 CI，全部成功。
- `33333825592`：README 导航与合同更新后的完整 CI，全部成功。
- `33332856633`：静态官网 Chromium smoke，成功。

CI 运行号会随新提交变化；发布前应重新查询当前提交对应的运行结果，不应永久依赖
上述历史编号。

## 版本与远端 tag 只读核对（2026-08-31）

- 当前 `main` 与 `origin/main` 均指向 `6c934ed`。
- `origin` 的 `v0.1.1` 仍指向历史提交 `5007c6c`；本次工作没有移动或覆盖该 tag。
- 本地历史 `v0.1.0` 仍保留在旧提交；正式发布前仍需由负责人决定新版本号并创建
  指向目标提交的新 tag。

## 外部验收顺序

1. 在脱敏测试副本记录镜像 digest、数据库备份校验和及资源余量。
2. 使用具备 `channel.read` 的管理员账号，在手机浏览器验证日志和额度页面。
3. 分别在 macOS 和 Windows 验证默认回环、显式 LAN 绑定、API Key 请求和回滚。
4. 完成副本升级/恢复后，再由负责人决定是否进行生产变更、版本 tag、NPM 或其他发布。
