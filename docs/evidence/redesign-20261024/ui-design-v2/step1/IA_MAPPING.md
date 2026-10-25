# 第 1 步：现有路由到方向 3 外壳的位置映射

- 日期：2026-10-25；分支 `dev`；基线提交 `6b479884`。
- 依据：第一版 `ui-design/DESIGN_DIRECTION.md` 5.3 与 5.5 节的路由清单（63 个路由文件，`find web/src/routes -name '*.tsx'` 排除 `._*` 后仍为 63 个）、`ui-design-v2/DESIGN_DIRECTION_V2.md` 2.3 节、`TASKS.md` 的维护者决定。
- 本步没有改任何路由 URL，也没有删除路由。导航数据仍由 `web/src/hooks/use-sidebar-data.ts` 登记，`use-sidebar-view.ts` 按角色、渠道读权限、版本能力和 `sidebar_modules` 收窄；新外壳只重排位置，不新增权限路径。
- 证据标记：**[代码]** 从实现读出；**[测试]** 有单测断言；**[浏览器]** 本机 Chromium 实际点过或截图看过。

## 1. 外壳里的六种位置

| 位置 | PC（≥1024px） | 手机（<1024px） |
| --- | --- | --- |
| 顶栏 5 项 | 概览 / 上游 / API Key / 请求 / 设置，链接到该区第一个可见页面，当前区深蓝下划线 | 底部栏：概览 / 上游 / Key / 请求；设置进"更多" |
| 页内页签 | 顶栏 5 区之一有两个以上页面时，内容区顶部一行下划线页签（`<nav aria-label="区名">`）；设置区再加一行分区胶囊。"管理"与"更多"里的区（用户、商业、系统、工具）不显示页签，由菜单列出其页面 | 同 PC，页签行可横向滑动，当前页签自动滚入视野 |
| "管理"下拉 | 仅 Full 且角色 ≥ 管理员：用户 / 商业 / 系统三组 | 这些入口在"更多"里 |
| "更多" | 顶栏菜单：工具（Playground、提示词学习、聊天预设），Lite / Desktop 或普通用户时也收纳用户、商业、系统 | 底部"更多"抽屉：除 4 个底部页签之外的全部入口 + 个人资料，可搜索；在设置区时顶部列出全部设置分区与"返回控制台" |
| 头像菜单 | 个人资料、钱包（商业开启时）、系统设置（Root）、语言、外观、构建号、退出 | 同 PC |
| 命令搜索 | `Ctrl/⌘ K` 或顶栏搜索：全部可见入口 + 个人资料 + 主题；粘贴 Request ID 回车直接打开该请求详情 | 顶栏搜索按钮 |

版本与角色规则 **[代码][测试]**：

- `edition === 'full'` 且 `role >= ADMIN` 时出现"管理"；状态缺失按 Full 处理（`resolveProductEdition`）；`VITE_SELF_USE_MINIMAL` 构建期仍强制 Lite 能力。
- Lite / Desktop：没有"管理"，用户与运行状态进"更多"；`capabilities` 为 false 的入口（绘图、钱包、订阅、兑换码、模型部署分区等）隐藏。
- 普通用户：没有上游、设置、管理；`requiredRole` 与渠道读权限照旧。

## 2. 63 个路由文件逐一映射

| 路由文件 | URL | 新位置 | 第一版位置 |
| --- | --- | --- | --- |
| `__root.tsx` | — | 根布局，不在导航中 | 同 |
| `_authenticated/route.tsx` | — | 控制台外壳（`AuthenticatedLayout` → `ConsoleShell`） | 同（侧栏外壳） |
| `_authenticated/dashboard/index.tsx` | `/dashboard` | 重定向到概览 | 同 |
| `_authenticated/dashboard/$section.tsx` · overview | `/dashboard/overview` | 顶栏"概览" | 侧栏"概览" |
| `_authenticated/dashboard/$section.tsx` · models / flow / users | `/dashboard/models`、`/dashboard/flow`、`/dashboard/users` | "请求"区页签"分析"（分析页自身的三个页签不变；`users` 仍需管理员） | 侧栏"分析" |
| `_authenticated/connections/index.tsx` | `/connections` | 顶栏"上游"（落地页）+ 页签"连接" | 侧栏 上游 › 连接 |
| `_authenticated/channels/index.tsx` | `/channels` | "上游"区页签"连接"（`/channels` 作为连接的表格视图，页签与顶栏均标为当前） | 同（连接的 activeUrl） |
| `_authenticated/upstream-accounts/index.tsx` | `/upstream-accounts` | "上游"区页签"账号窗口" | 侧栏 上游 › 账号窗口 |
| `_authenticated/models/index.tsx`、`models/$section.tsx` · metadata | `/models`、`/models/metadata` | "上游"区页签"模型目录" | 侧栏 上游 › 模型目录 |
| `_authenticated/models/$section.tsx` · deployments | `/models/deployments` | "上游"区页签"模型目录"（deployments 是它的 activeUrl；从代码看由模型页自身的"元信息 / 部署"页签切换，按能力 `model_deployments`） | 同 |
| `_authenticated/keys/index.tsx` | `/keys` | 顶栏"API Key" | 侧栏 访问 › API Key |
| `_authenticated/usage-logs/index.tsx`、`usage-logs/$section.tsx` · common | `/usage-logs/common` | 顶栏"请求"（落地页）+ 页签"调用记录" | 侧栏 记录 › 调用记录 |
| `_authenticated/usage-logs/$section.tsx` · task | `/usage-logs/task` | "请求"区页签"任务" | 侧栏 记录 › 任务 |
| `_authenticated/usage-logs/$section.tsx` · drawing | `/usage-logs/drawing` | "请求"区页签"绘图"（按能力 `midjourney` 显隐） | 侧栏 记录 › 绘图 |
| `_authenticated/full-content-logs/index.tsx` | `/full-content-logs` | "请求"区页签"正文与 SSE"（管理员） | 侧栏 记录 › 正文与 SSE |
| `_authenticated/system-settings/route.tsx`、`index.tsx` | `/system-settings` | 顶栏"设置"（Root；落地 `/system-settings/site`） | 侧栏 系统 › 设置 |
| `system-settings/site/index.tsx`、`site/$section.tsx` | `/system-settings/site/*` | "设置"区页签"站点与品牌" + 分区胶囊（4 个） | 侧栏下钻视图 |
| `system-settings/auth/index.tsx`、`auth/$section.tsx` | `/system-settings/auth/*` | 页签"身份验证" + 分区胶囊（OAuth 两项按能力） | 同上 |
| `system-settings/billing/index.tsx`、`billing/$section.tsx` | `/system-settings/billing/*` | 页签"计费与支付" + 分区胶囊（支付、签到按能力与自用构建） | 同上 |
| `system-settings/models/index.tsx`、`models/$section.tsx` | `/system-settings/models/*` | 页签"模型与路由" + 分区胶囊（模型部署按能力） | 同上 |
| `system-settings/security/index.tsx`、`security/$section.tsx` | `/system-settings/security/*` | 页签"安全与限制" + 分区胶囊 | 同上 |
| `system-settings/content/index.tsx`、`content/$section.tsx` | `/system-settings/content/*` | 页签"控制台内容" + 分区胶囊（能力 `site_content`，自用构建隐藏） | 同上 |
| `system-settings/operations/index.tsx`、`operations/$section.tsx` | `/system-settings/operations/*` | 页签"运维" + 分区胶囊 | 同上 |
| `_authenticated/users/index.tsx` | `/users` | Full 管理员："管理 › 用户"；Lite：PC "更多"、手机"更多" | 侧栏 访问 › 用户 |
| `_authenticated/wallet/index.tsx` | `/wallet` | Full 管理员："管理 › 商业 › 钱包 / 资金历史"；普通用户：PC "更多"与头像菜单（商业开启时）；能力 `payments` 与资金模式不变 | 侧栏 商业 › 钱包 |
| `_authenticated/subscriptions/index.tsx` | `/subscriptions` | "管理 › 商业 › 订阅"（管理员，能力 `commercial_subscriptions`） | 侧栏 商业 › 订阅 |
| `_authenticated/redemption-codes/index.tsx` | `/redemption-codes` | "管理 › 商业 › 兑换码"（管理员，能力 `redemption_codes`；自用构建隐藏） | 侧栏 商业 › 兑换码 |
| `_authenticated/system-info/index.tsx` | `/system-info` | Full："管理 › 系统 › 运行状态"（Root）；Lite："更多" | 侧栏 系统 › 运行状态 |
| `_authenticated/playground/index.tsx` | `/playground` | "更多 › 工具 › Playground" | 侧栏 工具（折叠） |
| `_authenticated/prompt-learning/index.tsx` | `/prompt-learning` | "更多 › 工具 › 提示词学习" | 侧栏 工具（折叠） |
| `_authenticated/chat/$chatId.tsx` | `/chat/$chatId` | "更多 › 工具"下的聊天预设（web 类预设打开此页；外链预设新窗口打开） | 侧栏 工具 › 聊天 |
| `_authenticated/chat2link.tsx` | `/chat2link` | 由聊天预设与 Key 操作跳转，不在导航中 | 同 |
| `_authenticated/profile/index.tsx` | `/profile` | 头像菜单"个人资料"；手机"更多 › 账户"；命令搜索 | 头像菜单、手机"更多" |
| `_authenticated/errors/$error.tsx` | `/errors/*` | 控制台内错误页，不在导航中 | 同 |
| `(auth)/route.tsx`、`sign-in.tsx`、`sign-up.tsx`、`register.tsx`、`forgot-password.tsx`、`reset.tsx`、`user/reset.tsx`、`otp.tsx`、`oauth.tsx` | 登录流程 | 外壳之外，不变 | 同 |
| `oauth/$provider.tsx` | OAuth 回调 | 外壳之外，不变 | 同 |
| `(errors)/401.tsx`、`403.tsx`、`404.tsx`、`500.tsx`、`503.tsx` | 公共错误页 | 外壳之外，不变 | 同 |
| `setup/index.tsx` | `/setup` | 安装向导，外壳之外 | 同 |
| `index.tsx` | `/` | 公开首页，外壳之外 | 同 |
| `about/index.tsx`、`privacy-policy.tsx`、`user-agreement.tsx` | 公开页 | 外壳之外，不变 | 同 |
| `pricing/index.tsx`、`pricing/$modelId/index.tsx`、`rankings/index.tsx` | 公开页 | 外壳之外，不变 | 同 |

计数核对：上表覆盖 `_authenticated` 下 39 个文件、认证组 9、错误组 5、根 1、其余公开与安装 9，共 63 个 **[代码]**。

## 3. 第一版侧栏条目的归宿

| 第一版侧栏条目 | 新归宿 | 验证 |
| --- | --- | --- |
| 概览 | 顶栏"概览" | **[测试]** `console-top-bar.test.tsx`；**[浏览器]** 截图 |
| 分析 | "请求"区页签"分析"；面包屑"请求 › 分析 › 流向" | **[测试]** `authenticated-navigation.test.tsx` |
| 上游 › 连接 / 账号窗口 / 模型目录 | 顶栏"上游" + 三个页签 | **[测试]** `console-shell-modules.test.tsx`、`sidebar-visibility.test.tsx` |
| 访问 › API Key | 顶栏"API Key" | **[测试]** |
| 访问 › 用户 | "管理 › 用户"（Full）/"更多"（Lite） | **[测试]** `console-top-bar.test.tsx` |
| 记录 › 调用记录 / 正文与 SSE / 任务 / 绘图 | 顶栏"请求" + 四个页签（绘图按能力） | **[测试]**；**[浏览器]** `personal-console-browser.mjs` |
| 工具 › Playground / 提示词学习 / 聊天 | "更多 › 工具" | **[测试]**；**[浏览器]** 触屏点"更多 › 游乐场"跳转 |
| 系统 › 运行状态 | "管理 › 系统"（Full Root）/"更多"（Lite） | **[测试]** |
| 系统 › 设置 | 顶栏"设置"（Root）+ 设置页签与分区胶囊 | **[测试]** `authenticated-navigation.test.tsx`（键盘逐级到"系统维护"） |
| 商业 › 钱包 / 订阅 / 兑换码 | "管理 › 商业"（Full 管理员）/"更多"（普通用户的钱包） | **[测试]** `console-top-bar.test.tsx`、`authenticated-layout.test.tsx` |
| 个人资料（手机"更多"与头像菜单） | 头像菜单；手机"更多 › 账户" | **[测试]** `console-shell.test.tsx` |
| 侧栏折叠按钮、侧栏样式与布局偏好 | 删除（没有侧栏）；保留"内容宽度"偏好：默认居中 1200px，"全宽"让顶栏与内容铺满 | **[浏览器]** `browser-smoke.mjs` 的 `platform-shell-layout-preferences` |

面包屑（PC）：最多三级"区 › 父页面 › 页面"，检视器打开时加一级对象名并保留最后三级；当前区已显示页签时，只在有第三级（例如"请求 › 分析 › 流向"、"设置 › 运维 › 系统维护"、"请求 › 调用记录 › 请求 …"）时出现，避免与页签重复。手机不显示面包屑，顶栏显示页面标题与返回。

滚动模型：PC 与手机一样，只有顶栏（手机另加底部栏）固定，页签、面包屑、页头、内容和分页作为一列整体滚动，与方向 3 样稿一致；自带内部滚动的页面（`data-page-scroll='contained'`，如 Playground）保持原样。第一版 PC 上页头固定、内容区单独滚动，本步改为整列滚动，原因是顶栏下新增的页签与面包屑占去固定区高度后，1280×600 下连接页的行操作被页头遮住（`tools/quota/browser-smoke.mjs` 的遮挡断言实测失败），整列滚动后该断言通过。

## 4. 已知限制

1. 顶栏导航在 1024–1279px 靠收紧间距、搜索收成图标放下 7 个入口；本机测得七种语言在 1024 / 1280 / 1440 宽度最后一项都不压到右侧操作区 **[浏览器]**（`/keys`，见本步汇报）。没有实现"超出时自动收进更多"。
2. 设置分区"侧边栏模块"（`/system-settings/site/sidebar-modules`）的名称还沿用"侧边栏"，它控制的已是顶栏与"更多"里的入口；改名属于设置页批次，本步未改。
3. 英文下调用记录页的手机顶栏标题是导航名"Requests"，页面 h1 是"Call records"（第一版已如此，术语统一留给请求页批次）。
