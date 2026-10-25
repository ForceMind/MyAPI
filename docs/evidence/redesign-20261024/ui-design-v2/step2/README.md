# 第 2 步：首页流向板

- 日期：2026-10-25；分支 `dev`；基线 `f5770baa`（第 1 步外壳）；实现 `edf7ae1d`，测试与脚本 `cceb4aa5`。
- 范围：只改概览 `/dashboard/overview`。Key 列表、请求页、上游页不动。没有新增后端接口，没有改后端。
- 证据标记：**[代码]** 从实现读出；**[测试]** 有单测断言；**[浏览器]** 本机 Chromium 实际操作或截图看过。

## 1. 页面结构

从上到下：页头（标题、一句说明、"添加连接"、"创建 API Key"）→ "需要处理"琥珀条 → 流向板（API Key 列 → 网关核心卡 → 上游列）→ 最近请求（节点筛选标签、全部 / 失败 / 用量未提供、查看调用记录）→ 原有的资金摘要、性能健康、站点内容面板（按原条件显示，未改）。

- 选中节点写入 URL：`?key=<token_id>` 或 `?connection=<channel_id>`，两者同时出现时以 Key 为准，非法值忽略 **[测试]**。点同一节点或筛选标签的 × 清除；刷新、返回、分享链接后状态一致 **[浏览器]**。
- 节点是 `<button aria-pressed>`，Enter / Space 可选，键盘焦点有可见外框 **[浏览器]**。
- 每列超过 6 个节点时只显示前 6 个，加"查看全部 N 个"（Key 跳 `/keys`，上游跳 `/connections`）；URL 选中了第 6 个之后的 Key 时，它占最后一个位置 **[测试]**。上游列只读取按优先级排序的前 6 个连接，总数来自同一次读取的 `total`。
- 小于 1024px：纵向管道"Key 列 → 竖线箭头 → 核心卡 → 竖线箭头 → 上游列"；点节点后请求区滚到可见位置 **[浏览器]**。
- 普通用户（或没有渠道读权限的管理员）：没有上游列，只有 Key 列与核心卡，请求表没有"连接"列；不发任何 `/api/channel*` 请求 **[测试]**。

## 2. 每个数字的来源与口径

打开页面不向上游发请求，只读已保存的数据 **[代码]**。

| 显示位置 | 来源接口 | 口径 |
| --- | --- | --- |
| Key 节点：名称、状态 | `/pg/keys`（本人 Key 的安全列表，分页读完；不读 `/api/token`） | 本人的 Key。样本里出现、但不在本人列表里的 Key（管理员能看到其他用户的请求）也成为节点：名称取日志里的 `token_name`，状态显示"状态未提供"或"其他用户" |
| Key 节点：N 次、失败 N | 同一次日志样本（见下一行），前端按 `token_id` 聚合 | 最近 24 小时内最近 100 条日志中的请求记录（type 2/5）；`token_id` 为 0 的旧记录不归入任何节点。日志读不到时显示"未知"，不补 0 |
| 日志样本 | 管理员 `/api/log?p=1&page_size=100&start_timestamp=<24h 前>`，普通用户 `/api/log/self` 同参数 | 管理员读全部用户记录、普通用户读本人记录，沿用第一版概览。返回的 `total` 大于本页条数时视为被截断，流向板说明文字与柱状图会写明"只含最近抽取的请求" |
| 核心卡"24 小时" | 两次一行读取：`type=2` 与 `type=5` 各 `page_size=1`，取 `total` 相加 | 最近 24 小时成功与失败的请求总数（精确值）；读取失败或 `total` 非法显示"—" |
| 核心卡"失败率"、"耗时中位数" | 日志样本 | 失败 = 未完成的请求（含发出前拒绝），沿用第一版；中位数只取 `use_time > 0` 的记录；样本为空显示"—" |
| 核心卡每小时柱状 | 日志样本按本地整点分桶，24 个桶 | 被截断时，早于样本最早一条的小时标为"不在样本范围内"（虚线），不画成 0。没有使用 `/api/data` 小时聚合：它只记成功计费、按 `DataExportInterval` 延迟写入、关闭数据看板时为空，会和"24 小时"总数对不上 |
| 核心卡"用量未提供 N 条" | 日志样本 | 已完成但没有用量证据的请求数；不计入任何 Token 合计 |
| 核心卡接入地址 | `/api/status` 的 `server_address`；为空时用 `window.location.origin` | 显示 `<地址>/v1`，下方注明"服务器地址设置"或"当前访问地址" |
| 核心卡版本行 | `/api/status` 的 `product_edition`、`version` | Full：`Full · 版本`。Lite：另加"本机访问 / 网络访问"，由当前浏览器访问的主机名是否为回环地址判断（只描述本次访问方式，后端没有访问模式字段）。版本缺失显示"版本未提供" |
| 上游节点：名称、类型、优先级、状态 | `/api/channel?p=1&page_size=6&sort_by=priority&sort_order=desc`（经 `parseConnections` 只保留元数据）+ `/api/channel?page_size=1&status=enabled` 取已启用总数 | 状态：已启用 / 已禁用 / 自动禁用（虚线连线）。连接记录里没有保存测试结果，所以不显示"测试失败" |
| 上游节点：环形额度 | `/api/channel/quota/changes`（`useUpstreamAccounts`，已保存快照） | 每个连接取已用最多的 `codex_rate_limit` 窗口；环内写已用百分比，右下写"窗口 · 剩余 N%"。没有可信样本的窗口显示其状态（如"用量采样失败"），不画环；快照读不到时列内提示并可重试 |
| 上游节点："按量 / 订阅 / 本地"、"使用中 · 最近 N 次" | 渠道类型（57 为订阅，4、47 为本地，其余按量）；日志样本按 `channel` 聚合 | 次数口径同 Key 节点 |
| 需要处理 | 日志样本、已启用总数、账号窗口快照、正文采集状态 | 沿用第一版条目与跳转；Root 始终能在这里打开"待核对请求" |

## 3. 版本、权限与状态

- Full 管理员：三列；Lite（`product_edition: "lite"`、能力全 false）：顶栏无"管理"，核心卡显示本机访问，性能与站点内容面板按能力隐藏 **[测试][浏览器]**。
- Full 的"用户 / 分组"摘要：概览现有数据里没有，本步没有新增读取，只显示地址与版本。
- 状态：加载（列骨架、核心卡数值骨架、Root 的琥珀条显示"加载中..."）、空（"还没有 API Key → 创建 API Key"，"还没有上游连接 → 添加连接"）、错误（Key 列、上游列、账号窗口、请求表各自重试）、无权限（无上游列）、未知（"未知"、"—"、"状态未提供"、"版本未提供"）**[测试][浏览器]**。

## 4. 截图与浏览器检查

`screenshots/shots.mjs`（Playwright 1.61.1 + Chromium，只放行 `127.0.0.1:4184` 的合成预览，其他请求一律中止，本次中止 0 个；场景数据只在浏览器里改写预览的响应）。`browser-checks.json` 166 项检查全部通过，30 张截图逐张看过 **[浏览器]**。

| 组 | 文件 |
| --- | --- |
| 主矩阵 | `overview-{1280,390}-{light,dark}-{zh,en}.png` |
| 其他宽度 | `overview-{320,768,1024}-light-{zh,en}.png` |
| Lite | `lite-overview-1280-light-{zh,en}.png`、`lite-overview-390-light-zh.png` |
| 普通用户 | `user-overview-{1280,390}-light-zh.png` |
| 空 / 加载 / 折叠 | `empty-overview-1280-light-{zh,en}.png`、`loading-overview-1280-light-zh.png`、`folded-overview-{1280,390}-light-zh.png`（8 个 Key、9 个连接） |
| 选中 | `selected-overview-1280-light-zh.png`（Key）、`selected-overview-1280-dark-en.png`（上游） |
| 实际操作 | `ops-key-selected-1280-light-zh.png`、`ops-key-focus-1280-light-zh.png`、`ops-request-detail-1280-light-zh.png`、`ops-phone-selected-390-light-zh.png` |

每张页面截图检查：无横向溢出、无页面错误、流向板在视口内、节点文字未被裁切（省略号除外）、每个环都写了百分比、手机节点高度 ≥44px。实际操作检查（`ops:` 开头 12 项）：点 Key 节点后表格只剩该 Key 的行且 URL 带 `?key=`；刷新后保持；选另一节点再"返回"恢复上一个；筛选标签 × 清除行与 URL；键盘焦点外框可见、Enter 选中、Space 取消；复制按钮把 `http://127.0.0.1:4184/v1` 放进剪贴板；时间链接打开请求详情；手机点节点后请求区顶部在视口上半部分。

### 看图发现的问题

| # | 现象 | 位置 | 处理 |
| --- | --- | --- | --- |
| 1 | 俄语 320px 下"全部 / 失败 / 用量未提供"切换比卡片宽 31px | 修复前 `personal-workbench-browser.mjs` 实测失败（`personal-ru-320: cards remain inside the viewport`），探针找到超出的是该切换组 | 已修：切换项允许换行、可收缩，手机上最小高度 44px；修复后同脚本通过 |
| 2 | Root 加载中时琥珀条只有一个按钮、左侧空白 | `loading-overview` 中间构建 | 已修：加载中显示"加载中..." |
| 3 | 核心卡标签"请求（24 小时）"在 300px 卡内折成两行 | 中间构建 | 已修：卡内标签改为"24 小时" |
| 4 | Lite 的版本行在 1280 下于"用量未提供"中间折行 | `lite-overview-1280-light-zh.png` | 未修：已让"N 条"不拆开，中文仍可能在词中折行；可加 `break-keep`，改动需要重跑全部浏览器脚本，留到上游页批次一起处理 |
| 5 | 英文计数没有单复数（"1 requests"） | `overview-*-en.png` | 未修：项目现有计数文案同样没有复数形式（如"1 requests completed without a usage report"），统一处理需要改 i18n 约定 |
| 6 | 1024px 下上游节点名称与类型被省略号截断 | `overview-1024-light-*.png` | 未修：完整名称在 `title` 与连接页；1024–1279 的列宽方案随上游页批次再定 |
| 7 | 预览数据只有 6 条请求，柱状图只有一根柱子 | 全部截图 | 数据如此，不是缺陷；被截断时的虚线态由单测覆盖 |

## 5. 首屏 1280×800（中文、亮色、Full Root）能看到什么

顶栏；标题"概览"、说明、"添加连接"与"创建 API Key"；琥珀条（2 件事 + "待核对请求"）；流向板整块：说明文字、Key 列 2 个节点（次数、失败、ID、状态）、核心卡（地址与复制、24 小时 6 / 失败率 33.3% / 耗时中位数 2 秒、每小时柱状、版本行）、上游列 1 个节点（68% 环、7 天剩余 32%）、连线；最近请求的标题行、筛选切换与第一行请求。

与样稿 `d3-home-1280-light.png` 的差异：

| 样稿 | 实现 | 原因 |
| --- | --- | --- |
| 右上"今天 / 7 天"周期切换 | 没有 | 现有概览口径是最近 24 小时；7 天需要另一套读取，本步未做 |
| 核心卡"今天 / 失败 / P50" | "24 小时 / 失败率 / 耗时中位数" | 沿用第一版口径与术语 |
| Key 节点掩码尾号 `sk-····a8f3` | `#21`（Key ID） | 安全列表 `/pg/keys` 不含掩码；读取 `/api/token` 会拿到 Key 对象，第一版测试明确禁止，未放宽 |
| 上游"测试失败 401" | "自动禁用"状态 | 连接记录只保存测试时间与耗时，不保存结果 |
| 每小时柱状为 24 根 | 只有有请求的小时有柱，其余是基线；未覆盖的小时是虚线 | 样本口径如实呈现 |
| 请求区标题"请求"、链接"全部请求" | "最近请求"、"查看调用记录" | 沿用现有文案与浏览器脚本合同 |
| 样稿空筛选时也露出一个空的蓝色标签（样稿缺陷） | 未选中时不显示标签 | — |

## 6. 第一版概览代码处置

删除（无其他引用，`grep` 确认）：`overview-health.tsx`、`overview-windows.tsx`、`overview-next-steps.tsx`，以及只被它们使用的 12 个文案键（七种语言同时删除）。改写：`overview-page.tsx`、`overview-attention.tsx`（琥珀条）、`overview-recent-requests.tsx`（节点筛选与结果切换）、`use-overview-data.ts`（Key 列表、前 6 个连接、样本截断标记）。保留：资金摘要、性能健康、公告、FAQ、API 信息、可用性面板与其测试，以及仍被上游账号页使用的 `account-quota-changes-panel` 一组。

## 7. 验证命令与结果 **[运行]**

前端（`web/`）：`bun run typecheck` 退出码 0；`bun run lint` 退出码 0、8 个 warning（与基线相同，0 error）；`bun run build` 退出码 0；同步脚本（`sync-i18n.mjs` 的副本，只多过滤 `._*`）完成；全量单测 1813 个用例，3 个失败，与基线的 3 个存储类失败相同（credential-maintenance-dialog、credential-rotation-dialog、strict-parameter-selection）。中途两次全量运行里 `playground/__tests__/account-switch.test.tsx` 各失败一次，单独运行与最后一次全量运行通过，判断为与概览无关的偶发失败。

新测试先红后绿：对实现逐项做破坏（选中不写 URL、取消折叠、未知变 0、按连接而非 Key ID 聚合、去掉 Lite 访问方式、无权限也显示上游列、筛选标签 × 不清除、表格忽略 Key 筛选、环内不写数值），每一项都有对应测试失败，恢复后全部通过。

浏览器脚本（最终构建 `VITE_BUILD_ID=f5770baa231e7b24b7e4f4ecd4643996156a3ed7`，与提交内容相同的工作区；Playwright 1.61.1、Node 22）：

| 脚本 | 首轮 | 最终 |
| --- | --- | --- |
| window-maintenance / model-scope / security-maintenance | 0 / 0 / 0 | 0 / 0 / 0 |
| browser-fixtures、connection-key-browser-fixtures、key-request-preview（`node --test`） | 0 | 0 |
| personal-workbench | 1（俄语 320px 切换组溢出，见第 4 节问题 1） | 0 |
| subscription-share / personal-console / connections / key-request | 0 / 0 / 0 / 0 | 0 / 0 / 0 / 0 |
| profile / stability / setup / personal-status / browser-smoke | 0 / 0 / 0 / 0 / 0 | 0 / 0 / 0 / 0 / 0 |
| playground（仓库原脚本） | 1 | 1：macOS 上原生 `<select>` 对 End 键不响应（第 1 步已记录，CI Linux 通过）；本地副本把两处 `press('End')` 换成 `selectOption('22')` 后为 0 |
| quota / routing / access-policy browser-smoke | 0 / 0 / 0 | 0 / 0 / 0 |
| credential-maintenance / credential-rotation / maintenance-session | 0 / 0 / 0 | 0 / 0 / 0 |
| Go：`TestTokenMaintenanceRuntimeBrowser`、`TestTokenCredentialRotationRuntimeBrowser` | 1 / 1（首轮构建号用了短 SHA，脚本要求 40 位） | 0 / 0 |

## 8. 语言

新增 40 个文案键，提供 en 与 zh；其余五种语言由同步脚本以英文回填（每种 40 个）。同步报告的"疑似未翻译"计数：fr 16→23、ja 74→108、ru 73→107、vi 16→23，zh-TW 不参与该检测。视觉与浏览器验收只覆盖中英文，`personal-workbench-browser.mjs` 原有的七语言矩阵保留并通过。
