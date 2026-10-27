# 目录引导

按"想找什么"组织。路径依据：仓库 `AGENTS.md` 的目录说明、本批工作中读过的文件和各步骤报告。没有逐一核对每个文件，找不到时以 `git ls-files | grep` 为准。

## 1. 仓库顶层

| 路径 | 内容 |
| --- | --- |
| `AGENTS.md`、`CLAUDE.md` | 项目硬规则（后端、数据库、计费、前端、治理）。先读 |
| `main.go` | 启动入口；`InitResources` 里调用 `middleware.ValidateEdition()`；挂载 `EditionGuard` |
| `router/` | HTTP 路由（API、中继、面板）；Midjourney 中继在 `relay-router.go` |
| `controller/` | 请求处理；`misc.go` 的 `GetStatus` 输出 `/api/status`；`log.go`、`log_identity.go`、`log_token_statistics.go` 处理日志与 `result` 参数 |
| `service/` | 业务逻辑（计费、选渠道、故障切换、订阅、任务等） |
| `model/` | 数据模型与 GORM 访问；`log.go` 含日志查询；账务状态机大部分在这里 |
| `middleware/` | 鉴权、分发、限流、内容日志、版本门禁（`edition.go`） |
| `relay/` | 转发处理与 53 个供应商适配器（`relay/channel/*`） |
| `relaykit/` | 独立 Go 模块（协议 DTO 与转换），必须能 `GOWORK=off go build ./...` |
| `setting/`、`common/`、`constant/`、`dto/`、`types/`、`i18n/`、`oauth/`、`pkg/` | 配置、共享工具（JSON 包装、quota 换算）、常量、类型、后端多语言、OAuth、内部包 |
| `web/` | React 前端（见第 3 节） |
| `cli/` | 零依赖 Node CLI（`myapi.mjs`，`lib/` 子模块，`test/` 测试） |
| `deploy/` | `docker-compose.yml`、`.env.example`、`install.sh` |
| `electron/` | Desktop 外壳（`main.js` 传 `MYAPI_EDITION: 'lite'`、托盘、`runtime-config.js`、测试） |
| `tools/` | 验收与合同脚本（见第 4 节） |
| `website/` | 静态官网 |
| `Dockerfile`、`Dockerfile.dev`、`docker-compose*.yml`、`makefile`、`my-api.service`、`new-api.service` | 构建与部署资产（`my-api.service` 待修，见接手文档） |
| `patches/`、`bin/` | 历史补丁与旧迁移脚本（按维护者决定保留） |
| `.github/workflows/` | CI 与发布工作流（见第 5 节） |
| `docs/` | 文档（见第 2 节） |
| `.agents/skills/` | 仓库自带的设计与最佳实践技能：`shadcn-ui`、`vercel-react-best-practices`、`i18n-translate` |

## 2. 文档（`docs/`）

- `NEXT_SESSION_HANDOFF.md`、`MYAPI_MASTER_PLAN.md`、`MYAPI_PLATFORM_REBUILD_PLAN_20261020.md`、`PROJECT_COMPLETION_EXECUTION_PLAN.md`：历史计划与交接。内含大量历史阶段描述，**当前状态以本批接手文档、`git log` 与 GitHub Actions 为准**。`NEXT_SESSION_HANDOFF.md` 在早先的约定里属于受保护文件，不要覆盖。
- `RELEASE_MANIFEST.md`：三维产品模型（功能版 / 安装形态 / 访问模式）、能力矩阵、安装与更新合同、待负责人决定的 5 项（第 9 节）。
- `MYAPI_DISTRIBUTION.md`、`LAN_LITE.md`：发行层与 Lite / Legacy LAN 说明（已更新 `lite` 取值）。
- `UI_PAGE_INVENTORY.md`：旧页面清单。
- **`evidence/redesign-20261024/`：本批全部工作的证据与计划，见下。**
- `evidence/` 下其他目录：更早阶段的验收证据，保留不动。

### 2.1 `docs/evidence/redesign-20261024/`

| 路径 | 内容 |
| --- | --- |
| `handoff/` | 接手入口 `README.md`（先看零节）、`PROMPT.md`、`OPERATIONS.md`、本文件；新增 `CACHE_REVIEW_20261027.md`（缓存专项CI红绿证据、未批准修复与代码路径） |
| `TASKS.md` | 任务清单、已确认决定、各步进度与 CI 记录。每完成一步在这里追加 |
| `PLAN_PHASE2.md` | 第二阶段计划（结构审查与重构 A1–A6、部署脚本 B1–B7、收口 C1–C3） |
| `architecture-review.md` | 第一性原理架构评估（价值链、模块归属表、重复与死代码、版本差距、A/B/C/D 精简分级） |
| `ui-design/` | 第一版设计方向与截图（已被取代，仅留档）：`DESIGN_DIRECTION.md`、`baseline/`（改版前现状截图）、`foundation/`、`shell/`、`pages/` |
| `ui-design-v2/` | **第二版设计（当前依据）**：`DESIGN_DIRECTION_V2.md`、`RESEARCH.md`、`mockups/{1-switchboard,2-control-room,3-flow}/`（三个可点击静态样稿，维护者选定 `3-flow`）、`screenshots/` |
| `ui-design-v2/step1/` 到 `step4/` | 各实施步骤的说明与截图：`step1`（令牌与外壳，含 `IA_MAPPING.md`、`contrast-report.md`）、`step2`（流向板首页）、`step3`（Key 与请求页，含路由与旧链接兼容表）、`step4`（上游页与收尾项）。5a 之后没有写 README 与截图（本机不能运行浏览器） |

## 2.2 缓存专项代码索引（2026-10-27）

- `router/codex_session_headers_test.go`：新增合成出站取证，当前5个会话头子场景失败；不含产品修复。
- `relaykit/relayconvert/internal/oai_chat/to_oai_responses_req.go` 及其测试：保留策略已修并全绿。
- `setting/operation_setting/channel_affinity_setting.go`、`service/channel_affinity.go`、`service/channel_account_affinity.go`：默认规则、模板、渠道/账号亲和。
- `middleware/distributor.go`、`relay/responses_handler.go`、`relay/chat_completions_via_responses.go`：分发与正文/协议转换顺序。
- `relay/channel/codex/adaptor.go`、`relay/channel/api_request.go`、`relay/common/override.go`：出站头、运行时覆盖/删除和普通/严格门禁。
- `relay/channel/openai/usage.go`、`service/log_token_evidence.go`：原生缓存计数与未知/零证据；其他统计疑点未修。

## 3. 前端（`web/`）

技术栈与约定：`web/AGENTS.md`。

| 路径 | 内容 |
| --- | --- |
| `web/src/routes/` | TanStack Router 路由文件（63 个）。`_authenticated/` 下是登录后页面 |
| `web/src/features/` | 业务模块（下表） |
| `web/src/components/` | 通用组件：`ui/`（基础原语）、`data-table/`、`quota-ring.tsx`（环形额度）、`layout/components/`（外壳） |
| `web/src/components/layout/components/` | **方向 3 外壳**：`console-shell.tsx`、`console-header.tsx`（PC 顶栏与手机顶栏）、`mobile-bottom-nav.tsx`、`page-tabs.tsx`（页内页签与设置分区胶囊）、`app-breadcrumb.tsx`、`public-header.tsx` 与 `public-layout.tsx`（公共页） |
| `web/src/hooks/` | `use-sidebar-data.ts`（导航登记表）、`use-sidebar-view.ts`（按角色与能力收窄）、`use-console-navigation.ts`、`use-console-page.ts`、`use-product-edition.ts` |
| `web/src/lib/product-edition.ts` | 读取 `/api/status` 的 `product_edition` 与 `capabilities`；缺失按 Full 处理 |
| `web/src/styles/` | `theme.css`（设计令牌，亮暗两套）、`theme-presets.css`（只剩"MyAPI 默认"与"高对比"）、`personal-console.css`、`product-ui.css`、`index.css` |
| `web/src/i18n/` | `locales/{en,zh,zh-TW,fr,ru,ja,vi}.json`（平铺，英文原文为键）、`_reports`（由同步脚本生成）、`languages.ts`（含 `<html lang>` 同步） |
| `web/public/myapi-logo-v1.png` | Logo，**保留不改** |

### 3.1 `web/src/features/` 与本批改动

| 模块 | 本批状态 |
| --- | --- |
| `dashboard/` | 首页流向板：`components/flow-board/`（`flow-board`、`flow-nodes`、`gateway-core-card`），`lib/overview-flow.ts`（数据聚合）；分析页图表读设计令牌（`web/src/lib/chart-palette.ts`） |
| `keys/` | Key 列表、对象页、请求页、`RecentRequestList`、`ObjectPage`、`useReturnFocus`；检视器内容区（访问、窗口分额、凭据维护）；5a 之前的 `inspect` 参数重定向 |
| `connections/` | 上游卡片网格（`connection-card.tsx`）、对象页与页签内容（`connection-sections.tsx`） |
| `personal-upstream/`、`models/` | 账号窗口页、模型目录（5c 做了页签与卡片对齐） |
| `usage-logs/`、`full-content-logs/` | 调用记录、任务、绘图、正文与 SSE（5a） |
| `system-settings/`、`users/`、`system-info/`、`profile/` | 设置共享布局、用户手机记录、运行状态、个人资料卡片（5b） |
| `wallet/`、`subscriptions/`、`redemption-codes/`、`auth/`、`setup/`、`home/`、`about/`、`legal/`、`pricing/`、`rankings/`、`errors/`、`playground/`、`chat/`、`prompt-learning/` | 5c |
| `channels/` | 渠道管理（`/channels` 是"连接"的表格视图），本批基本未动 |
| `access-policy/`、`performance-metrics/` | 未动 |

## 4. 验收与合同脚本（`tools/`）

| 路径 | 用途 |
| --- | --- |
| `tools/ui/*-browser.mjs`、`browser-smoke.mjs`、`browser-fixtures.mjs` | Chromium 验收脚本与合成接口夹具；按 `data-testid`、可访问名称、标题文字、`.pc-*` 选择器定位 |
| `tools/ui/page-layout-preview.mjs` | 只读合成预览服务（读 `web/dist`，本批用于截图，本机已不再使用） |
| `tools/runtime/` | Docker smoke、SQLite 恢复 smoke、凭据维护与轮换真实后端步骤、上游验收离线脚本 |
| `tools/quota/`、`tools/routing/` | 额度与路由验收脚本 |
| `tools/lan/check.mjs` | Lite / LAN 合同检查（已接受 `lite`） |
| `tools/desktop/`、`tools/upgrade/`、`tools/release/`、`tools/branding/`、`tools/website/` | Desktop、升级、发行、品牌、官网合同检查 |

## 5. CI（`.github/workflows/ci.yml`）

主要作业：后端 vet / build / test（含 linux/386 与若干竞态检测）；S1 身份与选项、S2-A 支付与订阅、B2 durable submission 的数据库回归（MySQL 与 PostgreSQL 临时服务）；S2-C 与 S2-C09 Redis 回归；B2 ClickHouse 日志迁移；Desktop 跨平台合同；发行与合同检查；"Isolated credential maintenance browser and real API"；"Frontend typecheck and test"（类型检查、Lint、全量单测、六个 Chromium 验收步骤）。

其他工作流：`docker-build.yml`、`docker-smoke.yml`（镜像构建与冒烟，不发布）、`electron-build.yml`（macOS 与 Windows）、`release.yml`、`npm-publish.yml`、`pr-check.yml`、`website-*.yml`、`sync-release-to-gitcode.yml`。发布类工作流只允许维护者手动触发，本批没有触发任何发布。

## 6. 本机之外的资源

- 生产实例（beta.25，已启用正文日志，见 `docs/NEXT_SESSION_HANDOFF.md` 历史记录）：本批没有访问、没有改动。
- 供应商订阅预算已用完（历史记录）：不得调用真实供应商。
