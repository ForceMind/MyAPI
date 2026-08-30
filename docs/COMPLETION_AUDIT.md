# MyAPI 完成度与证据矩阵

本文档把总体计划中的目标映射到可复核证据。`已验证` 只表示代码、合同或 CI
已经提供证据；真实设备、生产副本、法律和正式发布不会因静态检查而自动变成完成。

| 领域 | 已验证证据 | 当前状态 | 仍需外部条件 |
| --- | --- | --- | --- |
| API 兼容 | `relay/` 转换器与后端 CI | 已验证 | 上游版本变化时继续回归 |
| API/响应日志 | `web/src/features/usage-logs/`、移动集成测试、脱敏测试、移动内容高度修复（`2eae754`） | 代码已验证 | 真实手机视觉验收 |
| 运行构建可见性 | 管理员「系统信息」中的只读 Runtime build 标识、`build-metadata.ts` DOM/global 元数据及 `build-metadata.test.ts`；Docker/Release/Electron 构建注入 commit SHA；本机容器已切换到 `local/new-api:myapi-9dc11d4` 并健康 | 代码与本机副本已验证 | 真实管理员手机视觉验收仍待完成 |
| 渠道额度历史 | `controller/channel-billing.go`、历史/聚合测试、权限路由测试 | 已验证 | 真实登录账号和采样数据演练 |
| 概览额度变化 | `account-quota-changes-panel.tsx`、60 秒前台刷新、错误/plan type/只读告警状态测试；`a2528a2` 的跨登录身份查询缓存隔离与认证刷新回归测试 | 已验证 | 具备 `channel.read` 的真实管理员验收；真实手机视觉仍待完成 |
| 账户等级/Key 访问方案 | `model/access_profile.go`、Key/UI/API 测试、策略注册表及旧 Key profile 保留测试 | 兼容层已验证 | 强制路由迁移评审 |
| 设置引导 | Full/LAN Lite/权限条件、生命周期测试 | 已验证 | 多设备视觉检查 |
| 品牌与旧元数据 | `tools/branding/check.mjs`，最近运行 `blocking_count: 0` | 阻断项已清零 | NOTICE、源码头和兼容标识法律审查 |
| 静态官网 | `tools/website/check-static.mjs`、Chromium smoke、artifact workflow | 自动化已验证 | 真实移动视觉与独立域名发布决策 |
| LAN Lite/桌面 | `lan:check`、`desktop:check`、Electron 安全边界；`957d4ca` 的 `electron/test/desktop-probe-contract.test.mjs`、`runtime-config.js` 和 `tools/desktop/check.mjs` | 合同已验证（生产探针为 `/api/status`，仅 2xx 视为就绪） | macOS/Windows 实机安装、LAN 请求、防火墙；真实设备运行结果不得由合同测试代替 |
| GHCR/升级 | `release:workflow:check`、`upgrade:check`、不可变 digest 合同 | 自动化已验证 | 脱敏副本升级、数据库恢复、人工审批 |
| 新开发环境数据库默认值 | `48abce6`、`docker-compose.dev.yml`、`makefile` | 代码与模板已验证（新开发默认数据库为 `myapi`） | 接管既有数据库必须显式设置 `MYAPI_DEV_POSTGRES_DB`/`DEV_POSTGRES_DB` 并在副本验证；该变更不执行重命名或迁移 |
| Claude 组织用量 | `docs/CLAUDE_USAGE_REPORT.md`，官方 Usage Report 边界 | 设计已验证 | Admin 凭据、权限、保留策略和实际接入 |
| Google Antigravity | `relay/channel/gemini/antigravity_client.go`、`antigravity_client_test.go`、`docs/ANTIGRAVITY_INTEGRATION.md`、`docs/ANTIGRAVITY_PUBLIC_RELAY_GATE.md`；`1827358` | 专用 transport 代码、边界测试及有界 Docker Go 回归已交付（create/get/poll/cancel/delete、usage、动态 agent/continuation 约束、大小上限、终态和脱敏） | 仍需完成公共 Relay 闸门中的持久化、权限、计费、工具策略和完整测试评审；稳定官方余额接口不存在时保持 `unsupported` |
| NPM 正式发布 | CLI/打包/版本合同检查 | 发布前检查已验证 | 版本确认、tag、清单、用户明确确认与 `npm publish` |

## 最近 CI 证据

- `33338149233`（提交 `a9dcbae`）、`33338069839`（提交 `2abb2e1`）、`33337364868`（提交 `f487ac0`）、`33337161936`（提交 `3bae897`）和 `33336527035`（提交 `eae3d30`）在 GitHub Actions runner 启动前失败：四个作业均无 steps，无法据此判断代码失败；需待 runner 恢复后重新运行同一提交。
- `33339052745`（提交 `149155b`）仍在 runner 启动前失败：Backend、Frontend、Desktop
  和 Distribution 四个作业均为 `steps: []`；这不能作为代码失败证据，需待 runner
  恢复后重新运行当前提交。
- `33339157977`（提交 `cf47ea7`）继续呈现相同 runner 启动前失败：四个作业均为
  `steps: []`，因此仍需 runner 恢复后重新运行当前提交。
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

- `48abce6`：新开发环境的 compose 与 make 默认 PostgreSQL 数据库标识切换为
  `myapi`；保留显式变量覆盖旧数据库的路径，不代表既有生产数据库已迁移或重命名。
- `957d4ca`：Electron 生产后端探针改为请求 `/api/status`，并仅接受 2xx
  HTTP 状态；对应 runtime-config、探针合同测试和 desktop check 已纳入源码证据。
  这些合同证据不等同于 macOS/Windows 实机安装、局域网请求或生产运行验证。
- `a2528a2`：额度概览和渠道额度面板恢复统一的 `401` 认证刷新路径，并将用户
  ID、session SID 与权限能力纳入查询缓存 key；新增跨登录身份回归测试，避免
  同一标签页复用上一会话的额度数据。该修复不绕过服务端权限检查。
- `1827358`：Claude adaptor 增加 nil/base URL 防护和默认 JSON/Anthropic 版本头
  测试；Antigravity transport 固定 dynamic agent/continuation 字段边界、限制
  interaction 响应大小、支持 `requires_action` 终态并保持错误正文脱敏。Go 回归已在
  有界 Docker 容器中执行：`GOWORK=off go test ./relay/channel/gemini ./relay/channel/claude`
  通过（gemini 0.140s、claude 0.018s）。

CI 运行号会随新提交变化；发布前应重新查询当前提交对应的运行结果，不应永久依赖
上述历史编号。

## 当前源码合同复核（2026-08-31）

- `npm run release:check` 在提交 `c8cd228` 上通过：CLI 19/19、品牌 2/2（115 条分类
  引用、0 blocking）、Website、LAN Lite 60/60、Desktop 28/28、Upgrade 18/18、
  Release workflow 11/11，以及 SOURCE_MANIFEST/package check 均通过。
- 本轮有界复核：Go `gemini`/`claude` 测试通过；前端 `tsgo -b` 通过，Vitest
  通过 59 个测试文件、265 个测试。测试容器限制为 `--cpus=1.5 --memory=3g
  --memory-swap=4g`；图表零尺寸和 React 非布尔属性仅为既有测试环境警告，不影响
  断言结果。真实手机/桌面设备仍按外部验收顺序执行。

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

## 本机部署更新与诊断（2026-08-31）

- `/root/new-api/docker-compose.yml` 当前配置的是本地镜像
  `local/new-api:myapi-9dc11d4`，容器名为 `new-api`，监听回环地址。
- 本轮在既有一次本机更新授权下完成受限构建与切换；构建使用
  `MYAPI_BUILD_PARALLELISM=1`，未拉取或发布外部镜像，也未读取生产环境密钥。
- 镜像摘要为 `sha256:a8b222d494cad235ef5d1b0c31addad5e42a050c65ae6cc5696b49c8b5d72662`，
  容器健康检查通过，`/api/status` 返回 HTTP 200、`version=0.1.1`。
- 前端主 bundle 已确认包含 `Account quota changes`、`/api/channel/quota/changes`
  和 `Runtime build`，说明最新额度面板代码已进入本机运行副本。
- 未携带凭据请求额度接口返回 HTTP 401（`AUTH_UNAUTHORIZED`），权限门禁正常；本轮
  没有使用真实登录凭据，因此仍不能证明管理员账户在手机上已看到数据或样本。
- 仍需使用具备 `channel.read` 的管理员账号在手机浏览器登录，核对 Runtime build
  revision、额度面板和 API 日志正文；本轮没有使用真实登录凭据，因此未替代该项
  真实视觉/权限验收。
