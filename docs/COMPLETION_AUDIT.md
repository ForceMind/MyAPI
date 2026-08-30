# MyAPI 完成度与证据矩阵

本文档把总体计划中的目标映射到可复核证据。`已验证` 只表示代码、合同或 CI
已经提供证据；真实设备、生产副本、法律和正式发布不会因静态检查而自动变成完成。

| 领域 | 已验证证据 | 当前状态 | 仍需外部条件 |
| --- | --- | --- | --- |
| API 兼容 | `relay/` 转换器与后端 CI | 已验证 | 上游版本变化时继续回归 |
| API/响应日志 | `web/src/features/usage-logs/`、移动集成测试、脱敏测试 | 代码已验证 | 真实手机视觉验收 |
| 运行构建可见性 | 管理员「系统信息」中的只读 Runtime build 标识、`build-metadata.ts` DOM/global 元数据及 `build-metadata.test.ts`；Docker/Release/Electron 构建注入 commit SHA | 代码与合同已验证 | 更新测试镜像后由现场核对实际运行 revision |
| 渠道额度历史 | `controller/channel-billing.go`、历史/聚合测试、权限路由测试 | 已验证 | 真实登录账号和采样数据演练 |
| 概览额度变化 | `account-quota-changes-panel.tsx`、60 秒前台刷新、错误/plan type/只读告警状态测试 | 已验证 | 具备 `channel.read` 的真实管理员验收 |
| 账户等级/Key 访问方案 | `model/access_profile.go`、Key/UI/API 测试与策略注册表 | 兼容层已验证 | 强制路由迁移评审 |
| 设置引导 | Full/LAN Lite/权限条件、生命周期测试 | 已验证 | 多设备视觉检查 |
| 品牌与旧元数据 | `tools/branding/check.mjs`，最近运行 `blocking_count: 0` | 阻断项已清零 | NOTICE、源码头和兼容标识法律审查 |
| 静态官网 | `tools/website/check-static.mjs`、Chromium smoke、artifact workflow | 自动化已验证 | 真实移动视觉与独立域名发布决策 |
| LAN Lite/桌面 | `lan:check`、`desktop:check`、Electron 安全边界 | 合同已验证 | macOS/Windows 实机安装、LAN 请求、防火墙 |
| GHCR/升级 | `release:workflow:check`、`upgrade:check`、不可变 digest 合同 | 自动化已验证 | 脱敏副本升级、数据库恢复、人工审批 |
| Claude 组织用量 | `docs/CLAUDE_USAGE_REPORT.md`，官方 Usage Report 边界 | 设计已验证 | Admin 凭据、权限、保留策略和实际接入 |
| Google Antigravity | `relay/channel/gemini/antigravity_client.go`、`antigravity_client_test.go`、`docs/ANTIGRAVITY_INTEGRATION.md` | 专用 transport 生命周期已验证（create/get/poll/cancel/delete、usage、脱敏） | 公开 relay/channel 接入的计费/权限/工具策略评审；稳定官方余额接口不存在时保持 `unsupported` |
| NPM 正式发布 | CLI/打包/版本合同检查 | 发布前检查已验证 | 版本确认、tag、清单、用户明确确认与 `npm publish` |

## 最近 CI 证据

- `33335484167`：完成度矩阵一致性修正后的完整 CI，Backend、Frontend、Desktop
  和 Distribution 四个作业全部成功。
- `33335299578`：本机部署旧镜像诊断证据提交后的完整 CI，Backend、Frontend、
  Desktop 和 Distribution 四个作业全部成功。
- `33334940514`：安装环境导出安全修复、官网 metadata 和 LAN 注入回归后的完整 CI，
  Backend、Frontend、Desktop 和 Distribution 四个作业全部成功。
- `33334940499`：同一提交对应的静态官网 Chromium smoke，成功。
- `33334175656`：完成度证据矩阵提交后的完整 CI，Backend、Frontend、Desktop
  和 Distribution 四个作业全部成功。
- `33333993651`：README 多语言导航更新后的完整 CI，全部成功。
- `33333825592`：README 导航与合同更新后的完整 CI，全部成功。
- `33332856633`：静态官网 Chromium smoke，成功。

CI 运行号会随新提交变化；发布前应重新查询当前提交对应的运行结果，不应永久依赖
上述历史编号。

## 版本与远端 tag 只读核对（2026-08-31）

- 本轮只读核对确认 `main` 与 `origin/main` 指向同一提交；精确提交值应以
  `git rev-parse HEAD origin/main` 的当前输出为准，避免文档提交后产生漂移。
- `origin` 的 `v0.1.1` 仍指向历史提交 `5007c6c`；本次工作没有移动或覆盖该 tag。
- 本地历史 `v0.1.0` 仍保留在旧提交；正式发布前仍需由负责人决定新版本号并创建
  指向目标提交的新 tag。

## 外部验收顺序

1. 在脱敏测试副本记录镜像 digest、数据库备份校验和及资源余量。
2. 使用具备 `channel.read` 的管理员账号，在手机浏览器验证日志和额度页面。
3. 分别在 macOS 和 Windows 验证默认回环、显式 LAN 绑定、API Key 请求和回滚。
4. 完成副本升级/恢复后，再由负责人决定是否进行生产变更、版本 tag、NPM 或其他发布。

## 本机部署只读诊断（2026-08-31）

- `/root/new-api/docker-compose.yml` 当前配置的是本地镜像
  `local/new-api:myapi-0684ee3`，容器名为 `new-api`，监听回环地址。
- 该容器当前为 healthy，但并非本仓库 `main` 的最新构建；本轮没有重建、重启、
  拉取镜像或读取生产环境密钥。
- 因此，若登录本机看不到额度变化面板或移动端日志，必须先确认实际运行镜像已
  更新到包含当前 `main` 构建的镜像，并在管理员「系统信息」核对 Runtime build
  revision 后，再进行权限和浏览器验收。
