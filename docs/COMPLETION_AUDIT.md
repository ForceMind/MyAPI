# MyAPI 完成度与证据矩阵

本文档把总体计划中的目标映射到可复核证据。`已验证` 只表示代码、合同或 CI
已经提供证据；真实设备、生产副本、法律和正式发布不会因静态检查而自动变成完成。

| 领域 | 已验证证据 | 当前状态 | 仍需外部条件 |
| --- | --- | --- | --- |
| API 兼容 | `relay/` 转换器与后端 CI | 已验证 | 上游版本变化时继续回归 |
| API/响应日志 | `web/src/features/usage-logs/`、`web/src/features/full-content-logs/`、移动集成测试、脱敏测试、移动内容高度修复；列表与 Full Content Logs 查询缓存均按 user/session 隔离 | 代码已验证 | 真实手机视觉验收 |
| 运行构建可见性 | 管理员「系统信息」中的只读 Runtime build 标识、`build-metadata.ts` DOM/global 元数据及 `build-metadata.test.ts`；Docker/Release/Electron 构建注入 commit SHA；本机容器已切换到 `local/new-api:myapi-9dc11d4` 并健康 | 代码与本机副本已验证 | 真实管理员手机视觉验收仍待完成 |
| 渠道额度历史 | `controller/channel-billing.go`、`controller/codex_usage.go`、历史/聚合测试、权限路由测试；2xx 无有效 Codex rate_limit 时标记 unsupported；历史聚合按 metric/window/source/plan/unit/currency/window_seconds 隔离；快照按渠道/系列/观测时间桶幂等保留首条 | 已验证 | 真实登录账号和采样数据演练 |
| 概览额度变化 | `account-quota-changes-panel.tsx`、60 秒前台刷新、错误/plan type/只读告警状态测试；`a2528a2` 的跨登录身份查询缓存隔离与认证刷新回归测试；系列按计划/单位/窗口隔离 | 已验证 | 具备 `channel.read` 的真实管理员验收；真实手机视觉仍待完成 |
| 账户等级/Key 访问方案 | `model/access_profile.go`、Key/UI/API 测试、策略注册表及旧 Key profile 保留测试；Key 表单显式提交稳定 `access_profile_id` 并保留 legacy `group`；`setting/access_profile.go` 校验 fallback 目标存在、去空格后的 ID 唯一性和循环依赖 | 兼容层已验证 | 强制路由迁移评审 |
| 设置引导 | Full/LAN Lite/权限条件、生命周期测试 | 已验证 | 多设备视觉检查 |
| 品牌与旧元数据 | `tools/branding/check.mjs`，最近运行 `blocking_count: 0` | 阻断项已清零 | NOTICE、源码头和兼容标识法律审查 |
| 静态官网 | `tools/website/check-static.mjs`、Chromium smoke、artifact workflow | 自动化已验证 | 真实移动视觉与独立域名发布决策 |
| LAN Lite/桌面 | `lan:check`、`desktop:check`、Electron 安全边界；`electron/test/desktop-probe-contract.test.mjs`、`runtime-config.js` 和 `tools/desktop/check.mjs` | 合同已验证（生产探针为 `/api/status`，仅 2xx 视为就绪）；CLI 与托盘对通配监听均仅展示发现的 RFC1918 地址 | macOS/Windows 实机安装、LAN 请求、防火墙；真实设备运行结果不得由合同测试代替 |
| 多语言关键文案 | `web/src/i18n/locales/{fr,ja,ru,vi,zh-TW}.json`、`web/src/i18n/__tests__/locale-key-parity.test.ts` | English key parity 已验证（5 locales / 5 tests） | 真实设备文字长度与视觉审查 |
| GHCR/升级 | `release:workflow:check`、`upgrade:check`、不可变 digest 合同；版本与架构 tag 构建前检查并 fail-closed 拒绝覆盖 | 自动化已验证 | 脱敏副本升级、数据库恢复、人工审批 |
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

- 当前 `7c1b6c0` 增量复核：在 `web/` 以
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 完整运行前端回归，62 个测试
  文件、279 个测试全部通过（约 125 秒）。本条只更新当前提交的前端证据；CLI、品牌、官网、
  LAN Lite、Desktop、Upgrade 和打包合同仍以各自最近一次明确标注的成功运行作为证据，不能
  用本地前端测试替代这些合同或真实设备/副本验收。
- 当前 `7c1b6c0` 合同增量复核：`npm run lan:check -- --skip-docker` 通过 66/66，
  `npm run desktop:check` 通过 32/32，`npm run upgrade:check -- --json` 通过 18/18。
  LAN 检查跳过了 Docker Compose 解析，三项结果均不能替代真实跨平台安装、局域网请求、
  防火墙或脱敏数据库升级/恢复演练。
- 当前 `35ed22b` 后复核：`npm run release:workflow:check` 通过 16/16，确认语义版本
  Tag 自动触发 Full/LAN GHCR、既有 Tag 拒绝覆盖、manifest 使用已校验的不可变 digest，
  以及发布闸门和构建超时/并行度约束仍然生效；这不等于真实 GHCR 拉取或发布操作已执行。

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
- `24a44f1`/`f965e04`/`3338846` 后增量：release contract 15/15、upgrade contract
  18/18、Full Content Logs 相关 Vitest 14/14 和 tsgo 通过；新增 Codex Go 测试仅
  完成 gofmt/静态审阅，未完成依赖下载后的运行验证。
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
- 2026-08-31 后续只读核对确认该容器仍在运行且健康，但镜像标签/摘要仍对应
  `local/new-api:myapi-9dc11d4`，尚未重建到包含 `bdf1d62`/`c8a0681` 的最新源码；
  本轮没有重启或替换它，以避免未经当前授权改变正在使用的服务。
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
