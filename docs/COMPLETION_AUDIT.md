# MyAPI 完成度与证据矩阵

本文档把总体计划中的目标映射到可复核证据。`已验证` 只表示代码、合同或 CI
已经提供证据；真实设备、生产副本、法律和正式发布不会因静态检查而自动变成完成。

| 领域 | 已验证证据 | 当前状态 | 仍需外部条件 |
| --- | --- | --- | --- |
| API 兼容 | `relay/` 转换器与后端 CI | 已验证 | 上游版本变化时继续回归 |
| API/响应日志 | `web/src/features/usage-logs/`、`web/src/features/full-content-logs/`、移动集成测试、脱敏测试、移动内容高度修复；列表与 Full Content Logs 查询缓存均按 user/session 隔离 | 代码已验证 | 真实手机视觉验收 |
| 运行构建可见性 | 管理员「系统信息」中的只读 Runtime build 标识、`build-metadata.ts` DOM/global 元数据及 `build-metadata.test.ts`；Docker/Release/Electron 构建注入 commit SHA；本机容器已切换到 `local/new-api:myapi-3ea1e5b` 并健康 | 代码与本机副本已验证 | 真实管理员手机视觉验收仍待完成 |
| 完整独立 UI 系统 | 当前仅有 MyAPI 品牌资产、必要文案和局部额度/日志功能增量 | 尚未开始（代码层先行） | 需在代码合同稳定后另行完成信息架构、视觉系统、Full/LAN/移动端真实画面与设备验收；不得把局部增量误报为 UI 全量替换 |
| 渠道额度历史 | `controller/channel-billing.go`、`controller/codex_usage.go`、历史/聚合测试、权限路由测试；2xx 无有效 Codex rate_limit 时标记 unsupported；历史聚合按 metric/window/source/plan/unit/currency/window_seconds 隔离；快照按渠道/系列/观测时间桶幂等保留首条，并以 nullable SHA-256 唯一键抵抗并发重复写入 | 已验证 | 真实登录账号和采样数据演练 |
| 概览额度变化 | `account-quota-changes-panel.tsx`、`codex-account-quota-chart.tsx`、60 秒前台刷新、Codex 账户选择与 24 小时可用额度折线图、错误/plan type/只读告警状态测试；`a2528a2` 的跨登录身份查询缓存隔离与认证刷新回归测试；系列按计划/单位/窗口隔离；后台采样默认开启并可在监控设置中调整 | 代码、前端测试与本机新镜像已验证 | 需要由具备 `channel.read` 的真实管理员在概览页面验收；真实手机视觉仍待完成 |
| 账户等级/Key 访问方案 | `model/access_profile.go`、Key/UI/API 测试、策略注册表及旧 Key profile 保留测试；Key 表单显式提交稳定 `access_profile_id` 并保留 legacy `group`；`setting/access_profile.go` 校验 fallback 目标存在、去空格后的 ID 唯一性和循环依赖 | 兼容层已验证 | 强制路由迁移评审 |
| 设置引导 | Full/LAN Lite/权限条件、生命周期测试 | 已验证 | 多设备视觉检查 |
| 品牌与旧元数据 | `tools/branding/check.mjs`，最近运行 `blocking_count: 0` | 阻断项已清零 | NOTICE、源码头和兼容标识法律审查 |
| 静态官网 | `tools/website/check-static.mjs`、Chromium smoke、artifact workflow | 自动化已验证 | 真实移动视觉与独立域名发布决策 |
| LAN Lite/桌面 | `lan:check`、`desktop:check`、Electron 安全边界；`electron/test/desktop-probe-contract.test.mjs`、`runtime-config.js` 和 `tools/desktop/check.mjs` | 合同已验证（生产探针为 `/api/status`，要求 HTTP 2xx 且 JSON `success=true`；开发首页仍仅校验 HTTP 状态）；CLI 与托盘对通配监听均仅展示发现的 RFC1918 地址 | macOS/Windows 实机安装、LAN 请求、防火墙；真实设备运行结果不得由合同测试代替 |
| 多语言关键文案 | `web/src/i18n/locales/{fr,ja,ru,vi,zh-TW}.json`、`web/src/i18n/__tests__/locale-key-parity.test.ts` | English key parity 已验证（5 locales / 5 tests） | 真实设备文字长度与视觉审查 |
| GHCR/升级 | `release:workflow:check`、`upgrade:check`、不可变 digest 合同；版本与架构 tag 构建前检查并 fail-closed 拒绝覆盖 | 自动化已验证 | 脱敏副本升级、数据库恢复、人工审批 |
| 计费安全 | `service/violation_fee.go` 使用 checked quota rounding，并在饱和时拒绝收费、保留 `relayInfo.QuotaClamp`；对应正常值、溢出、`Inf`、`NaN` 与审计捕获回归测试 | 当前工作树代码与定向 Go 测试已验证 | 真实数据库/生产额度与完整 CI 仍待外部条件 |
| 新开发环境数据库默认值 | `48abce6`、`docker-compose.dev.yml`、`makefile` | 代码与模板已验证（新开发默认数据库为 `myapi`） | 接管既有数据库必须显式设置 `MYAPI_DEV_POSTGRES_DB`/`DEV_POSTGRES_DB` 并在副本验证；该变更不执行重命名或迁移 |
| Claude 组织用量 | `docs/CLAUDE_USAGE_REPORT.md`，官方 Usage Report 边界 | 设计已验证 | Admin 凭据、权限、保留策略和实际接入 |
| Google Antigravity | `relay/channel/gemini/antigravity_client.go`、`antigravity_client_test.go`、`docs/ANTIGRAVITY_INTEGRATION.md`、`docs/ANTIGRAVITY_PUBLIC_RELAY_GATE.md`；`1827358` | 专用 transport 代码、边界测试及有界 Docker Go 回归已交付（create/get/poll/cancel/delete、usage、动态 agent/continuation 约束、大小上限、终态和脱敏） | 仍需完成公共 Relay 闸门中的持久化、权限、计费、工具策略和完整测试评审；稳定官方余额接口不存在时保持 `unsupported` |
| NPM 正式发布 | CLI/打包/版本合同检查 | 发布前检查已验证 | 版本确认、tag、清单、用户明确确认与 `npm publish` |
| macOS 开发迁移 | `docs/DEVELOPMENT_ON_MACOS.md`、`docs/CODEX_HANDOFF_PROMPT.md`、README 导航 | 文档已补齐 | 新 Mac 的工具安装、依赖测试和实机 Electron/LAN 验收需在新设备执行 |

## 最近 CI 证据

- `33377504590`（提交 `4151c41`，2026-08-31）仍在 GitHub Actions runner 启动前失败：
  Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`，约 5 秒内结束。
  该结果继续按 GitHub Billing/runner 外部阻塞处理，不能据此判断当前文档提交或源码失败；
  runner 恢复后只需重跑最新提交。
- `33338149233`（提交 `a9dcbae`）、`33338069839`（提交 `2abb2e1`）、`33337364868`（提交 `f487ac0`）、`33337161936`（提交 `3bae897`）和 `33336527035`（提交 `eae3d30`）在 GitHub Actions runner 启动前失败：四个作业均无 steps，无法据此判断代码失败；需待 runner 恢复后重新运行同一提交。
- `33339052745`（提交 `149155b`）仍在 runner 启动前失败：Backend、Frontend、Desktop
  和 Distribution 四个作业均为 `steps: []`；这不能作为代码失败证据，需待 runner
  恢复后重新运行当前提交。
- `33339157977`（提交 `cf47ea7`）继续呈现相同 runner 启动前失败：四个作业均为
  `steps: []`，因此仍需 runner 恢复后重新运行当前提交。
- `33341868806`（提交 `f4fc17e`）仍是同一外部故障：Desktop、Backend、Frontend、
  Distribution 四个作业均在启动后立即失败且 `steps: []`，不能据此判定代码失败；
  本轮已用受限本机/Docker 回归替代验证，待 runner 恢复后仍应重跑远端 CI。
- `33342523430`（提交 `4c2267c`）及同期官网运行 `33339379741` 的检查注释已明确
  为账户付款失败或 spending limit 阻断；四个作业均 `steps: []`、无 runner，根因在
  GitHub Billing & plans，不是 workflow 或代码。额度恢复后只重跑最新提交，避免重跑历史。
- `33344526525`（提交 `1695688`）仍在 runner 启动前失败：Backend、Frontend、Desktop、
  Distribution 四个作业均为 `steps: []`；当前仍应按 GitHub Billing & plans 外部阻塞处理，
  不将其视为代码测试失败。
- `33344621039`（提交 `51f3fec`）及同提交的官网 workflow 均在 runner 启动前失败，
  CI 四个作业和官网检查/浏览器 smoke 均无执行 steps；继续按同一 Billing 外部阻塞处理。
- `33351658149`（提交 `94a9eba`）在本轮自动触发后仍呈现相同状态：Frontend、Backend、
  Desktop 和 Distribution 四个 job 均 `steps: []`，在启动阶段失败；该结果不能作为
  代码失败证据，需 GitHub Billing/runner 恢复后只重跑最新提交。
- `33352270443`（提交 `4524224`）和 `33352740848`（提交 `085e475`）继续呈现同一
  外部启动故障：Frontend、Backend、Desktop 和 Distribution 四个 job 均为 `steps: []`，
  无可用 job 日志。应按 GitHub Billing/runner 阻塞处理，不能视作代码失败；恢复后只重跑
  最新提交。
- `33356196350`（提交 `a56966f`）及其前序 `33356139117`、`33355935758`、
  `33355371586` 仍在 runner 启动阶段失败；最新 CI 的四个 job 均为 `steps: []`，
  约 2 秒内结束。该结果继续按 GitHub Billing/runner 外部阻塞处理，本轮以本机
  前端 62/278、Go 回归和发行合同检查作为替代证据，不重跑历史 workflow。
- `33357189957`（提交 `f04f97b`，2026-08-31）仍为同一外部启动故障：Backend、Frontend、
  Desktop 和 Distribution 四个 job 均为 `steps: []`，约 2 秒内结束；不能据此判断本轮
  文档提交或源码失败，待 GitHub runner/Billing 恢复后只需重跑最新提交。
- `33358768662`（提交 `7479b2a`，2026-08-31）继续呈现同一外部启动故障：Backend、
  Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`，约 3 秒内结束；当前仍以
  本机资源受限回归作为替代证据，不将该 CI 红灯归因于源码。
- 2026-08-31 历史记录：本机运行副本曾使用 `local/new-api:myapi-9dc11d4`，容器
  `new-api` healthy，回环 `GET /api/status` 返回成功且版本 `0.1.1`；该记录对应当时的
  只读复核，已由下方的迁移修复和新镜像演练更新。
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
- `957d4ca`：Electron 生产后端探针改为请求 `/api/status`，并仅接受 2xx；后续
  `94707b0` 增加 JSON `success=true` 校验，避免业务失败响应误判为就绪。
  HTTP 状态；对应 runtime-config、探针合同测试和 desktop check 已纳入源码证据。
  这些合同证据不等同于 macOS/Windows 实机安装、局域网请求或生产运行验证。
- `a2528a2`：额度概览和渠道额度面板恢复统一的 `401` 认证刷新路径，并将用户
  ID、session SID 与权限能力纳入查询缓存 key；新增跨登录身份回归测试，避免
  同一标签页复用上一会话的额度数据。该修复不绕过服务端权限检查。
- `cb419c1`：CLI `up`/`upgrade`/`doctor` 与 installer 统一拒绝未显式允许的
  非回环 LAN 监听；installer 同时严格校验 Full edition HTTPS origin、LAN 私网
  origin 和至少 48 字符的 `SESSION_SECRET`。新增 CLI 回归 21/21、LAN Lite
  合同 66/66，并为 NPM workflow 增加旧 `v0.1.0` tag 保护；概览额度汇总按
  provider `source`/`plan_type` 隔离，避免不同账户语义混算。
- `0e77a85`：渠道页移动端关闭固定高度表格，避免额度面板与渠道列表形成
  嵌套滚动或内容裁剪；该提交当时的前端有界回归为 59 个测试文件、266 个测试（历史证据）。
- `1827358`：Claude adaptor 增加 nil/base URL 防护和默认 JSON/Anthropic 版本头
  测试；Antigravity transport 固定 dynamic agent/continuation 字段边界、限制
  interaction 响应大小、支持 `requires_action` 终态并保持错误正文脱敏。Go 回归已在
  有界 Docker 容器中执行：`GOWORK=off go test ./relay/channel/gemini ./relay/channel/claude`
  通过（gemini 0.140s、claude 0.018s）。
- `8a7741a`：PR 质量检查补齐 anti-slop 所需的最小 PR/issue 写权限，并监听
  `pull_request_target.synchronize`，确保后续提交重新审查；未授予 contents 写权限。
- `618327c`：LAN 通配监听移除 `<private-LAN-IP>` 占位，按 RFC1918 IPv4
  地址发现并输出候选端点；补齐五个非中文 locale 的额度/账户/访问方案关键文案，
  并加入 locale key parity 回归测试（5/5）。
- `24a44f1`：Docker、Release、NPM workflow 和 release-state 统一保护已存在的
  `v0.1.0`/`v0.1.1` tag，并把 SemVer tag 自动 GHCR 构建、Full/LAN 仓库和版本固定
  拉取纳入 release contract（15/15）。
- `f965e04`：Codex 额度采样不再把 2xx 登录页/无 rate_limit JSON 误计为成功样本，
  新增 unsupported 分类回归；Go 测试因依赖下载资源限制未宣称通过。
- `3338846`：Full Content Logs 查询缓存加入 user/session 身份隔离，新增 query-key
  回归；相关 Vitest 5 文件/14 测试、tsgo 均通过。
- `9871564`：Codex WHAM usage/reset/consume 响应统一限制为 1 MiB，并以额外 1 字节
  探测超限；边界测试覆盖三条接口。gofmt 通过，Go 测试因依赖下载资源限制未宣称通过。
- `5cfb046`：Electron 托盘和 LAN 状态对 `0.0.0.0` 展开实际 RFC1918 IPv4 端点，
  无候选时显示明确提示并移除占位符；Desktop 合同更新为 32/32。
- `bdf1d62`：旧的未过期管理员会话若缺少 `admin_permissions`，认证引导现在会
  强制进入一次服务端 refresh，避免额度变化面板因旧权限快照被误判为无权限；新增
  `requiresCapabilityRefresh` 回归测试，明确的空权限矩阵仍按拒绝处理。相关前端测试
  （认证会话与额度面板）21/21、`tsgo -b` 和格式检查均通过。

CI 运行号会随新提交变化；发布前应重新查询当前提交对应的运行结果，不应永久依赖
上述历史编号。

## 当前源码合同复核（2026-08-31）

### 当前工作树阶段证据（2026-08-31）

- 本阶段提交为 `a620246`（基于 `HEAD=9b65ac4`）；代码、测试与文档已写入本地提交，远端同步状态需以本阶段 push 后的核对为准。
- `deploy/install.sh` 已移除 macOS 系统 Bash 3.2 不支持的 `${var,,}` 展开，并将 `MYAPI_PORT` 限制为 `1..65535`；`bash -n deploy/install.sh` 通过。
- 本机合同回归：CLI 22/22、LAN Lite 68/68、Desktop 32/32、Upgrade 18/18、Runtime probe 13/13（测试 4/4）、Release workflow 16/16、Brand 2/2；Website 静态检查通过。
- 本机前端回归（Node 22）：typecheck、Vitest 62 个测试文件/280 个测试和 production build 全部通过。Node 26 的 localStorage 不兼容只属于不符合项目要求的运行环境，改用 Node 22 后未重现。
- 本机 Go 回归：`GOWORK=off go test ./... -count=1` 通过；额度/渠道与 Gemini/Claude 定向回归通过；`cd relaykit && GOWORK=off go build ./...` 通过。计费饱和回归覆盖正常值、溢出、无穷与 NaN 输入。
- 上述均为源码、合同和本机受限资源验证；尚未替代 Docker Desktop Compose 实跑、SQLite/MySQL/PostgreSQL 副本恢复、真实管理员手机、macOS/Windows 安装、局域网/防火墙和生产环境验收。
- 本阶段没有执行 push、tag、GHCR/NPM 发布、生产重启或生产数据操作。GitHub Actions 仍可能受 runner/Billing 启动阶段故障影响；恢复后应只重跑最新提交。NOTICE/法律审查和正式版本号仍需负责人确认。
- 阶段提交 `e0ca670` 推送后的 CI run `33397392112`（2026-08-31）中，Frontend、Desktop、Backend 和 Distribution 四个 job 均在 runner 启动阶段失败且 `steps: []`；按既有规则归类为 GitHub Billing/runner 外部阻塞，不归因于源码。恢复后只需重跑最新提交。
- 随后的文档同步提交 `52f1d03` 对应 CI run `33397495873` 仍为同一启动阶段故障，四个 job 均为 `steps: []`；继续按 GitHub Billing/runner 外部阻塞处理，不修改无关源码。
- 阶段 2 的 macOS/Docker/发行合同只读审查未发现新的可直接修复缺陷；Docker Desktop/Compose、真实 LAN 启动、健康检查和数据库演练仍待本机安装与外部验收。
- 当前执行批次已把代码优先目标写入总体计划；本机尝试安装 Docker Desktop 时下载长时间无进度后中止，Docker/Compose 仍视为未安装的外部环境阻塞。
- 在干净提交 `54fe197` 上重新生成被忽略的 `SOURCE_MANIFEST.json`，`npm run pack:check` 通过（2119 个文件，19,238,938 bytes）；清单只作为发布前证据，不代表已执行 NPM/GHCR 发布。
- 新增 `.github/workflows/docker-smoke.yml`：GitHub runner 使用 `push: false`、不登录 GHCR 的 Buildx 构建临时 LAN 镜像，启动隔离 SQLite 容器并检查 `/api/status`；`tools/release/check.mjs` 已增加对应静态合同。该 workflow 仅用于测试，不会创建 tag 或发布镜像，首次真实运行需等待 GitHub runner/Billing 恢复。
- Docker smoke 手动运行 `33403818364`（提交 `0e4f912`）在 runner 启动阶段失败，唯一 job 为 `steps: []`；已将 workflow 限定为 `workflow_dispatch`，避免普通文档/代码 push 重复触发同一外部阻塞。恢复 runner 后再手动重跑。
- JSON wrapper 阶段提交 `83ba011` 将 common 工具、额度告警和 model JSON 持久化路径统一到 `common/json.go`，新增 strict decoder 并保留原有 `JsonRawMessageToString` 回归；完整根 Go 回归与 relaykit 独立构建通过。
- Provider 阶段提交 `6b160c8` 修复 Midjourney 上传响应 fallback 丢失结果、Tencent 签名 payload 错误被忽略和 Cohere JSON wrapper 绕过；`service`、Tencent、Cohere 定向回归及完整根 Go 回归通过。
- Provider 阶段推送后的 CI run `33404395878`（提交 `46697c6`）仍在 runner 启动阶段失败，Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`；不归因于本轮源码。
- 迁移/Provider 阶段提交 `b90a342` 对应 CI run `33406547782` 延续同一 runner 启动故障，四个 job 均为 `steps: []`；待 Billing/runner 恢复后只重跑最新提交。
- JSON helper 增量提交 `f6655ca` 将 `common.Any2Type` 的序列化/反序列化改用项目 wrapper，并保留 `0`、`false`、嵌套值和错误传播回归；common/model 测试通过。
- 迁移与 Provider 增量提交 `2fab8a3`：SQLite 旧 `subscription_plans` 表新增必需列时使用兼容默认值并支持重复迁移；`migrateDBFast` 改为串行并补齐 Casbin/Authz 模型；Cloudflare/Dify 业务 JSON 统一走 wrapper，Dify 附件字段、上传失败和 SSE 错误不再静默吞掉。完整根 Go 回归、model/cloudflare/dify 定向测试与 relaykit 独立构建通过。
- model 锁/迁移增量提交 `5ec3822`：`lockForUpdate` 按实际 `tx.Dialector` 选择方言，避免全局配置误加 SQLite `FOR UPDATE`；`migrateSubscriptionPlanPriceAmount` 传播 DDL 错误，防止迁移失败后继续启动；model 全量回归和完整根 Go 回归通过。
- Dify/迁移增量提交 `f201f6d`：Dify 缺失 usage 时仅按实际输出文本估算，不再按 reasoning 事件数虚构 token；完整上游 usage 保持不被额外增加；迁移转换 helper 按实际连接方言执行，model_sync JSON 走 wrapper。Dify、controller、model 和完整根 Go 回归通过。
- Controller JSON 增量提交 `791ed6c`：io.net 部署测试与 Creem 支付 products/checkout 路径改用项目 JSON wrapper，保留原错误语义；controller 完整回归通过。
- 邮箱一致性阶段提交 `9801b57`：新增 nullable `users.email_normalized`，迁移先回填并检测含软删除记录的冲突/超长值，再创建可重复的唯一索引；创建、更新、支付/OAuth 内部 map 更新和解绑同步规范化值，读路径保留缺列时的旧库回退。model 全量与完整根 Go 回归通过；MySQL/PostgreSQL 实库并发和恢复演练仍待外部副本验证。
- 邮箱一致性阶段推送后的 CI run `33413426111`（提交 `2ff9f3e`）四个 job 均在 runner 启动阶段失败且 `steps: []`；继续按 GitHub Billing/runner 外部阻塞处理。
- 渠道测试 billing 阶段提交 `e528d34`：`settleTestQuota` 使用 checked quota 转换，饱和值拒绝写入并通过集中化 helper 保留 admin-only 审计标记；controller 与完整根 Go 回归通过。该路径仍不替代真实上游/生产计费验收。
- Claude billing 阶段提交 `f004bd6`：统一 `max_tokens`/`max_tokens_to_sample` 有效值，限制默认值与 thinking budget 比例，拒绝 thinking 请求中低于 1280 的显式上限，并防止超界配置绕过 validator；relay、setting、relaykit 及完整根 Go 回归通过。
- 图片计数边界阶段提交 `f53ce34`：MiniMax 与 Vertex Imagen 适配器复用 `dto.MaxImageN`，拒绝超限/负数/非整数/Inf/NaN，并保留合法 0/默认值语义；Provider 定向与完整根 Go 回归通过。
- Baidu/Coze JSON 阶段提交 `b265e6f`：Provider 响应、SSE 和访问令牌路径统一使用项目 JSON wrapper，malformed 响应不会继续输出或进入 usage 处理；Baidu/Coze 定向与完整根 Go 回归通过。
- 数据库并发/控制器增量提交 `2ae74db`：规范化邮箱锁与可用性检查统一使用 `LOWER(email)`，快照去重索引在 MySQL 并发创建时回检并传播真实错误，io.net 部署测试请求改用 JSON wrapper；model/controller 定向与完整根 Go 回归通过。
- 代码优先阶段提交 `9427656` 修复并覆盖了 OpenRouter cache-create quota 饱和、topup ratio 原子更新与有限值校验、Gemini Imagen `N` 边界以及图片 token 面积/最终 quota 转换；`go test ./common ./service ./controller ./relay/channel/gemini`、完整根 Go 回归与 `cd relaykit && GOWORK=off go build ./...` 均通过。
- 同一阶段的 Node 合同复核：LAN Lite 68/68、Desktop 32/32、Upgrade 18/18、Release workflow 17/17；Docker workflow 版本写入已断言为无 `v` 的 SemVer。真实 Docker Compose、数据库副本和跨平台设备仍未验证。
- 代码阶段提交 `d0cd478` 推送后的 CI run `33402289671`（2026-08-31）仍在 runner 启动阶段失败，Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`；继续按 GitHub Billing/runner 外部阻塞处理。

- 当前 `02bcc16` 增量复核：在 `web/` 以
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 完整运行前端回归，62 个测试
  文件、279 个测试全部通过（约 125 秒）。本条只更新当前提交的前端证据；CLI、品牌、官网、
  LAN Lite、Desktop、Upgrade 和打包合同仍以各自最近一次明确标注的成功运行作为证据，不能
  用本地前端测试替代这些合同或真实设备/副本验收。
- 当前 `02bcc16` 合同增量复核：`npm run lan:check -- --skip-docker` 通过 66/66，
  `npm run desktop:check` 通过 32/32，`npm run upgrade:check -- --json` 通过 18/18。
  LAN 检查跳过了 Docker Compose 解析，三项结果均不能替代真实跨平台安装、局域网请求、
  防火墙或脱敏数据库升级/恢复演练。
- 当前 `35ed22b` 后复核：`npm run release:workflow:check` 通过 16/16，确认语义版本
  Tag 自动触发 Full/LAN GHCR、既有 Tag 拒绝覆盖、manifest 使用已校验的不可变 digest，
  以及发布闸门和构建超时/并行度约束仍然生效；这不等于真实 GHCR 拉取或发布操作已执行。
- 审计提交 `ac2a1d8` 的发行包复核：在允许 Node 子进程的受限环境中重新运行
  `npm run source:manifest && npm run pack:check`，清单记录当前源提交，包含 2110 个文件；
  打包检查通过（2111 个文件，19,180,475 bytes），未发现敏感文件、构建目录或凭据模式。
- 当前工作树快照幂等增量：`ChannelQuotaSnapshot` 使用可迁移的 nullable `dedupe_key`
  唯一索引，并在并发唯一冲突时回读赢家记录；模型定向回归在 `--cpus=1.5 --memory=3g`
  的 Go 容器中通过；随后完整 `GOWORK=off go test ./model -count=1` 也在同样资源限制下
  通过（约 7 秒），额度历史/同步/Codex quota 相关的 `GOWORK=off go test ./controller
  -run "ChannelQuota|CodexQuota|Quota" -count=1` 也通过（约 0.19 秒）。旧数据的完整字段
  查询仍作为兼容回退。
- 访问方案注册表增量复核：`setting/access_profile.go` 的 JSON 解析统一使用
  `common.UnmarshalJsonStr`，不再绕过项目 JSON wrapper；`GOWORK=off go test ./setting ./model
  -run "AccessProfile|AccountTier" -count=1` 在资源受限 Go 容器中通过。

- 在代码提交 `02bcc16` 的验证时点，`HEAD` 与 `origin/main` 已核对为同一提交；上述
  前端、发行合同、清单和访问方案回归证据均对应该提交。`SOURCE_MANIFEST.json` 仍是
  被忽略的生成文件，发布前应在最终版本提交上重新生成，不要将其加入 Git。

- 推送后的 CI `33359526462`（提交 `d422511`，2026-08-31）仍在 runner 启动阶段失败：
  Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`，约 3 秒内结束。
  该结果与前序记录一致，不能归因于源码；GitHub Billing/runner 恢复后只需重跑最新提交。

- 最新 CI `33359611153`（提交 `cf5d326`，2026-08-31）仍在 runner 启动阶段失败：四个 job
  均为 `steps: []`，约 2 秒内结束；继续按 GitHub Billing/runner 外部阻塞处理，不将其
  视为源码测试失败。

- `npm run release:check` 在提交 `4169778` 上通过：CLI 21/21、品牌 2/2（115 条分类
  引用、0 blocking）、Website、LAN Lite 66/66、Desktop 31/31、Upgrade 18/18、
  Release workflow 12/12，以及 SOURCE_MANIFEST/package check 均通过。该命令在
  外部受限执行环境中运行，避免 CLI 子进程被沙箱拒绝。
- 本轮有界复核：Go `gemini`/`claude` 测试通过；前端 `tsgo -b` 通过，Vitest
  通过 61 个测试文件、274 个测试。测试容器限制为 `--cpus=1.5 --memory=3g
  --memory-swap=4g`；图表零尺寸和 React 非布尔属性仅为既有测试环境警告，不影响
  断言结果。真实手机/桌面设备仍按外部验收顺序执行。
- `618327c` 后增量复核：CLI 22/22、LAN Lite 66/66、Desktop 31/31、Website
  静态检查通过；国际化 parity 测试 5/5 通过。前端完整构建与真实设备视觉仍待
  受限环境/外部设备执行。
- `24a44f1`/`f965e04`/`3338846` 历史增量：release contract 15/15、upgrade contract
  18/18、Full Content Logs 相关 Vitest 14/14 和 tsgo 通过；当时新增 Codex Go 测试仅
  完成 gofmt/静态审阅，未完成依赖下载后的运行验证；该缺口已由后续 Go 容器回归补齐。
- `5cfb046` 后增量：Electron runtime tests 当前 17/17 个 Node 子测试（分布在 2 个
  test files）、Desktop contract 32/32；真实平台安装和局域网请求仍待实机验收。
- `28e7bb2` 后增量：在允许 Node 子进程的受限环境中重新运行完整
  `npm run release:check`，CLI 22/22、品牌 2/2、Website、LAN Lite 66/66、Desktop
  32/32、Upgrade 18/18、Release workflow 15/15 以及 SOURCE_MANIFEST/pack check
  全部通过；未执行任何发布、tag 或生产操作。
- `c8a0681`：权限快照刷新仅针对非超级管理员的旧会话；`SUPER_ADMIN` 继续使用后端
  隐式全权限路径，不因缺少矩阵而增加不必要的 refresh。认证会话回归 11/11、格式
  检查通过；该边界不会把缺失权限当作 allow。
- `c4793ae` 后增量：在单 CPU、Node 堆上限 2GB 的受限环境中运行完整前端回归，
  61 个测试文件、273 个测试全部通过；仅有既有图表零尺寸和 React 非布尔属性警告，
  没有断言失败。真实手机视觉仍需按实机清单执行。
- `085e475` 后增量：额度变化面板在管理员面板挂载时按 `user.id + session.sid`
  最多刷新一次 `/api/user/self`，解决 SPA 内权限策略更新后旧快照导致的误隐藏；新增
  会话刷新回归测试。单 CPU、Node 堆上限 2GB 的完整前端回归通过 61 个测试文件、274
  个测试；仅有既有图表零尺寸和 React 非布尔属性警告。
- `8478f0f` 后增量：额度历史聚合与概览变化按 metric/window/source/plan/unit/currency/
  window_seconds 隔离；未指定系列的频道历史会锁定最新系列并支持显式筛选，避免多订阅
  计划混合绘图和速率计算。Full Content Logs 与 Usage Logs 在 401/403/网络错误时清空
  旧缓存行/正文并保留 Retry；单 CPU、Node 堆上限 2GB 的完整前端回归通过 62 个测试
  文件、278 个测试。
- `ee04de8` 后增量：Key 表单显式维护并提交稳定 `access_profile_id`，同时保留 legacy
  `group`；账户等级和访问方案的映射测试扩展后，完整前端回归通过 62 个测试文件、279
  个测试。
- `98133ce` 后复核：在 `web/` 使用
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 重新执行完整前端回归，62 个
  测试文件、279 个测试全部通过（约 125 秒）；该结果仍只证明代码和状态分支回归，不能
  替代真实管理员手机上的登录、权限和视觉验收。
- 当前提交 `fc4c7b4` 后复核：在 `web/` 使用 `taskset -c 0` 和
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 重新执行完整前端回归，62 个
  测试文件、279 个测试全部通过（约 158 秒）。本次限制为单 CPU，未出现断言失败；该结果
  仍不能替代真实管理员手机上的登录、权限和视觉验收。

## 版本与远端 tag 只读核对（2026-08-31）

- 本轮只读核对确认 `main` 与 `origin/main` 指向同一提交；精确提交值应以
  `git rev-parse HEAD origin/main` 的当前输出为准，避免文档提交后产生漂移。
- `origin` 的 `v0.1.1` 仍指向历史提交 `5007c6c`；本次工作没有移动或覆盖该 tag。
- 本地历史 `v0.1.0` 仍保留在旧提交；正式发布前仍需由负责人决定新版本号并创建
  指向目标提交的新 tag。
- `npm run release:state` 当前按预期 fail-closed：工作树版本 `0.1.1` 对应的 `v0.1.1`
  是受保护历史 tag，因此工具拒绝继续，要求先决定新的版本号；本轮没有修改
  `VERSION`/`package.json`，也没有移动或覆盖任何 tag。

## 外部验收顺序

1. 在脱敏测试副本记录镜像 digest、数据库备份校验和及资源余量。
2. 使用具备 `channel.read` 的管理员账号，在手机浏览器验证日志和额度页面。
3. 分别在 macOS 和 Windows 验证默认回环、显式 LAN 绑定、API Key 请求和回滚。
4. 完成副本升级/恢复后，再由负责人决定是否进行生产变更、版本 tag、NPM 或其他发布。

## 代码优先阶段增量（2026-09-01）

- Palm、Zhipu、Ali rerank 和 MiniMax 的业务 JSON 序列化/反序列化已统一走
  `common/json.go` wrapper；Palm/Zhipu/Ali 增加 malformed 响应边界回归，Zhipu
  malformed `meta:` SSE 与 scanner 错误不再继续输出或伪造 usage。
- Provider 定向回归：`go test ./relay/channel/palm ./relay/channel/zhipu
  ./relay/channel/ali ./relay/channel/minimax -count=1` 通过；在允许本地测试端口的
  条件下，`GOWORK=off go test ./... -count=1` 全量通过。受限沙箱首次运行的 SMTP
  测试因 `127.0.0.1:0` bind 权限失败，升级权限后已复核通过。
- relaykit 独立构建 `cd relaykit && GOWORK=off go build ./...` 通过（依赖下载在允许网络
  的执行环境完成）。
- 前端/发行合同本轮未能启动：本机 `/opt/homebrew/bin/node` 及 `merve/tsgo` 因缺失
  `simdutf` 动态库而在启动阶段 SIGABRT；代码未因此修改。修复 Homebrew Node 运行时后
  需重跑 `bun run typecheck`、前端测试、production build 与 `bun/npm run release:check`。
- 本阶段没有执行 tag、NPM/GHCR publish、生产重启或生产数据操作；完整 UI 替换仍为
  `尚未开始（代码层先行）`。真实 Docker Desktop、MySQL/PostgreSQL 副本、手机及
  macOS/Windows 安装仍按外部验收清单待验证。
- 提交 `2db1a58` 已推送到 `origin/main`；GitHub Actions CI run `33419182037` 的
  Backend、Frontend、Desktop、Distribution 四个 job 均在 runner 启动前失败且
  `steps: []`。按规则归类为 GitHub Billing/runner 外部阻塞，不归因于本轮源码；恢复后
  只需重跑该提交的 CI。
- 随后使用已安装的 Node 22.23.2（`PATH=/opt/homebrew/opt/node@22/bin:$PATH`）并将
  `NPM_CONFIG_CACHE` 指向临时目录复核：`bun run typecheck` 通过，前端 Vitest 62 个文件/
  280 个测试通过，`bun run build` 通过；`npm run release:check` 通过（CLI 22/22、LAN
  68/68、Desktop 32/32、Upgrade 18/18、Runtime 13/13+测试 4/4、Release workflow
  18/18、源码清单和打包检查通过）。原生 `/opt/homebrew/bin/node` 的 simdutf 链接问题仍
  需在开发主机永久修复，当前验证通过不代表 Node 26 环境符合项目要求。

## Provider JSON 审计增量（2026-09-01，第二轮）

- Xunfei 与 Volcengine 的业务 JSON 编解码已统一走 `common/json.go` wrapper；Xunfei
  增加响应解码错误边界，Volcengine 保留 `json.RawMessage` 类型依赖并补充 TTS/metadata
  malformed 输入测试。
- 定向回归 `GOWORK=off go test ./relay/channel/xunfei ./relay/channel/volcengine -count=1`
  通过；未改变上游协议、发布流程或生产数据。
- 随后在允许本地 SMTP/回环测试端口的环境中重新运行 `GOWORK=off go test ./... -count=1`，
  全部根模块包通过。提交 `f0b2195` 已推送；CI run `33420183217` 的四个 job 仍均为
  runner 启动前失败、`steps: []`，继续归类为 GitHub Billing/runner 外部阻塞。
- 按请求手动触发 Docker smoke workflow `33420575494`（提交 `5493cca`）；唯一 job
  `Build local image and probe SQLite runtime` 同样在 runner 启动前失败且 `steps: []`。
  workflow 仍保持 `push: false`、不登录 GHCR 的安全边界；待 GitHub runner/Billing 恢复后
  重跑即可，当前不能把该结果当作镜像或 `/api/status` 验收。

## 本机部署更新与诊断（2026-09-01）

## Provider JSON 审计增量（2026-09-01，第三轮）

- AWS、Jimeng、MokaAI 的业务 JSON 路径已统一使用 `common/json.go` wrapper；保留必要的
  `json.RawMessage` 类型依赖，并补充 malformed 响应/输入回归。
- `GOWORK=off go test ./relay/channel/aws ./relay/channel/jimeng ./relay/channel/mokaai -count=1`
  通过；未改变 Provider 协议、发布闸门或生产环境。

- `/root/new-api/docker-compose.yml` 当前配置的是本地镜像
  `local/new-api:myapi-3ea1e5b`，容器名为 `new-api`，监听回环地址。
- 2026-09-01 远端同步：本地 `main` 已快进到 `3ea1e5b`，与 `origin/main` 一致；该批次包含
  provider JSON 解码路径收敛、请求包装/参数边界、计费与并发安全、数据库迁移和 CI/发布合同更新。
- 2026-09-01 本机更新：使用 `MYAPI_BUILD_PARALLELISM=1` 构建
  `local/new-api:myapi-3ea1e5b`，并通过 compose 强制重建本机容器；容器健康、`/api/status`
  返回 HTTP 200、`success=true`、`version=0.1.1`、`system_name=MyAPI`。
  数据和日志绑定仍为 `/root/new-api/data` 与 `/root/new-api/logs`，没有修改其中内容。
- 后台额度采样默认开启；本机 Compose 不注入覆盖环境变量，因此管理员可在「系统设置 → 运维 →
  监控与告警」修改采样开关、间隔和每轮最大渠道数。首次任务已成功执行并记录 1 个 Codex 采样点；
  后续按默认 15 分钟间隔继续产生历史点。
- 2026-08-31 Codex 额度折线图更新：源码提交 `ff5feb8` 已使用单核、2GB 内存构建为
  `local/new-api:myapi-ff5feb8`，并通过 compose 强制重建本机容器；容器健康、首页返回
  HTTP 200，`/api/status` 返回 `success=true`、`version=0.1.1`、`brand=MyAPI`。
  数据和日志绑定仍为 `/root/new-api/data` 与 `/root/new-api/logs`，本次没有修改或复制其中内容。
- 2026-08-31 后续核对确认该容器仍在运行且健康；该镜像包含 SQLite 迁移修复，并在
  临时 SQLite 副本上完成新镜像启动、旧镜像回滚和 `/api/status` 健康演练。
- 新镜像构建使用 `MYAPI_BUILD_PARALLELISM=1`，未拉取或发布外部 MyAPI 镜像，也未读取
  生产环境密钥。
- 容器健康检查通过，`/api/status` 返回 HTTP 200、`version=0.1.1`；本次源码提交和镜像
  构建分别由 `ff5feb8` 与 `local/new-api:myapi-ff5feb8` 记录。
- 新镜像内嵌前端已确认包含 `Account quota changes`、`/api/channel/quota/changes`
  和 `Runtime build`，并已在本机正式回环容器中运行。
- 未携带凭据请求额度接口返回 HTTP 401（`AUTH_UNAUTHORIZED`），权限门禁正常；本轮
  没有使用真实登录凭据，因此仍不能证明管理员账户在手机上已看到数据或样本。
- 仍需使用具备 `channel.read` 的管理员账号在手机浏览器登录，核对 Runtime build
  revision、额度面板和 API 日志正文；本轮没有使用真实登录凭据，因此未替代该项
  真实视觉/权限验收。
- 最终镜像运行时链路增量探针：在 `local/new-api:myapi-921ca38` 上使用 1 CPU、1 GB
  内存和匿名 SQLite 数据卷完成初始化，使用仅用于探针的合成管理员登录后，
  `/api/user/self`、`/api/channel/quota/changes?range=24h` 与 `/api/log/` 均返回
  HTTP 200 且 `success=true`；该副本没有真实渠道因此额度项为 0，日志列表返回 1 条
  结果。探针结束后容器、数据卷和临时文件均已清理；这验证认证/权限链路，不替代真实
  管理员手机上的视觉与敏感日志正文验收。
- 针对移动端日志可见性和额度面板的增量回归：在 `web/` 使用 `taskset -c 0` 运行移动
  日志卡片、日志表格集成、概览额度面板和渠道额度面板 4 个测试文件，共 22 个测试全部
  通过（约 13 秒）。这验证移动 slot、加载/错误/空态与权限分支，但仍不能替代真实
  手机浏览器的触控、滚动和视觉验收。
- 当前后端增量回归：使用本机缓存的 Go 1.26.1 容器，在 `--cpus=1.5`、`--memory=3g`
  限制下运行 `GOWORK=off go test ./controller -run 'ChannelQuota|CodexQuota|Quota' -count=1`，
  结果 `ok github.com/ForceMind/MyAPI/controller`；该结果验证额度采样、聚合和 Codex
  用量相关控制器回归，不替代真实上游账户或生产数据库演练。
- 当前提交 `0735b20` 后完整发行检查：在单 CPU 限制下运行 `npm run release:check`，CLI
  22/22、品牌审计 115 条分类引用且 0 blocking、Website、LAN Lite 66/66、Desktop
  32/32、Upgrade 18/18、Runtime probe contract 13/13、Runtime probe 测试 3/3、Release
  workflow 16/16、源码清单和发行包检查全部通过。该结果不替代真实跨平台安装、手机视觉
  验收、数据库恢复或外部 GitHub runner 验证。
- `fb920c5` 后增量：新增 SQLite 迁移回归测试，在 Go 1.26.1 Alpine 容器中以单 CPU、1 GiB
  内存运行通过；验证旧表添加 `dedupe_key`、唯一索引创建及重复迁移幂等性。
- `80f722e` 后本机副本演练：`local/new-api:myapi-4b08bdb` 完成 SQLite 迁移并健康，恢复
  原数据库副本后旧镜像 `local/new-api:myapi-9dc11d4` 也通过健康检查；临时容器、端口和
  副本已清理，正式容器未使用副本数据。
- 当前文档基线（`b9cf87e`）：`main` 与 `origin/main` 已同步；在该干净提交上重新运行
  `npm run release:check`，CLI 22/22、LAN Lite 66/66、Desktop 32/32、Upgrade 18/18、
  Runtime probe 4/4、Release workflow 16/16、品牌/官网检查和 2115 文件打包检查均通过。
  `SOURCE_MANIFEST.json` 已生成但按约定保持 ignored。
- 推送 `4cc5e2b` 后的最新 CI `33376520189` 仍在 runner 启动前结束，四个 job 均为
  `steps: []`；该结果继续按 GitHub Billing/runner 外部阻塞处理，不能归因于源码。
- 推送 `7f4e184` 后的 CI `33376622066` 延续相同状态：四个 job 均在 runner 启动前结束且
  `steps: []`；待 GitHub Billing/runner 恢复后只需重跑最新提交。
- 当前 HEAD `6e96836` 的完整 `npm run release:check` 已重新通过：CLI 22/22、LAN Lite
  66/66、Desktop 32/32、Upgrade 18/18、Runtime probe 4/4、Release workflow 16/16，
  品牌/官网检查和 2115 文件打包检查均通过；该命令在单 CPU 限制下执行。
