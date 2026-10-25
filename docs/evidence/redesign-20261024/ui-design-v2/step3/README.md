# 第 3 步：Key 列表、Key 对象页、请求页

- 日期：2026-10-25；分支 `dev`；基线 `ff8f6da7`；实现 `67be4186` 与 `f3b29ffe`（看图后的修正），测试与浏览器脚本 `b2607555`。
- 范围：`/keys` 列表、Key 对象页、请求页，以及把旧链接、命令搜索、概览时间链接、调用记录"详情"接到新页面。上游页、设置、分析、正文日志页、任务等未改；没有改后端。
- 证据标记：**[代码]** 从实现读出；**[测试]** 有单测断言；**[浏览器]** 本机 Chromium 实际操作或截图看过。

## 1. 页面与路由

| 页面 | 地址 | 搜索参数 |
| --- | --- | --- |
| Key 列表 | `/keys` | 原有 `page`、`pageSize`、`filter`、`status`、`token`、`focusKeyId`、`connectionContext` |
| Key 对象页 | `/keys/<tokenId>` | `tab`（`scope` / `delivery`，默认请求页签不写入）、`requestLog`（无 Request ID 的旧记录，以对话框盖在页面上）、`keyReturn`（来源列表的筛选与页码） |
| 请求页（从 Key 打开） | `/keys/<tokenId>/requests/<requestId>` | `tab`（`sse` / `raw`，默认正文页签不写入） |
| 请求页（调用记录、命令搜索、概览） | `/usage-logs/request/<requestId>` | 同上 |

- 两种请求页是同一个组件，区别只在面包屑与返回去处：从 Key 打开时是"API Key › Key 名 › 请求 <短 ID>"，返回 Key 页；其余是"调用记录 › 请求 <短 ID>"，返回调用记录。Key 上下文的请求若属于另一个 Key，页面写明"此请求由另一个 Key 发出" **[测试]**。
- 选 path 形式的理由：对象是页面堆栈，地址本身就是面包屑；TanStack Router 的静态段 `request` 优先于 `/usage-logs/$section`，不影响原有调用记录地址。
- 返回：页面登记到外壳（`inspector-store` 增加 `page`、`parents`、`closeAll`），PC 用面包屑、手机用顶栏"返回"。从应用内打开时"返回"等于浏览器后退（历史条目带 `inspectorBack`），直接打开的链接退到上一级（Key 页回到 `/keys` 并带 `focusKeyId` 与原筛选，请求页回到 Key 页或按 Request ID 筛选的调用记录）**[测试][浏览器]**。
- 返回后的焦点：打开对象页时记下来源行（按历史条目），回到该条目后焦点落回那一行；列表还在加载时等待，用户先有操作就放弃 **[测试][浏览器]**。
- 列表在 Key 页被修改（暂停、编辑、删除）后作废列表缓存，返回时不会显示旧状态或已删除的 Key **[测试]**（应用的查询缓存保鲜 10 秒；这一条在浏览器脚本 `maintenance-session` 首轮暴露）。

### 旧链接兼容

| 旧地址 | 现在 |
| --- | --- |
| `/keys?inspect=21`、`/keys?keyId=21` | `/keys/21`（同时带来的 `page`/`pageSize`/`filter`/`status` 进入 `keyReturn`） |
| `/keys?inspect=21&tab=access` 或 `tab=shares` | `/keys/21?tab=scope` |
| `/keys?inspect=21&requestLog=55` | `/keys/21?requestLog=55`（旧记录对话框） |
| `/keys?inspect=21&request=R&requestTab=body\|sse\|diagnosis` | `/keys/21/requests/R`（`sse` 保留为 `?tab=sse`；`diagnosis` 现在总在左侧，落到默认页签） |
| `/usage-logs/common?requestId=R&inspect=request[&requestTab=…]` | `/usage-logs/request/R`（页签同上） |
| `/usage-logs/common?requestId=R`（不带 `inspect`） | 不变：按 Request ID 筛选调用记录 |
| `/keys?focusKeyId=21`、`/keys?connectionContext=…` | 不变 |
| 非法 Key ID、超长 Request ID | 回到 `/keys` 或该 Key 页 / 调用记录 |

全部由路由 `beforeLoad` 以 `replace` 重定向，刷新、返回、分享后状态一致 **[测试][浏览器]**。

## 2. 每个展示字段的来源

打开页面不向上游发请求；正文只读已保存、已脱敏的内容；没有轮询 **[代码]**。

| 位置 | 来源接口 | 口径 |
| --- | --- | --- |
| Key 列表 | `/api/token/`、`/api/token/search`（原有） | 默认列：名称、状态、API Key（掩码）、额度（非个人模式）、模型、最后使用、操作；访问方案、IP、创建、过期默认隐藏，可在"查看"里打开 |
| Key 页标题行 | `/api/token/<id>` | 名称、状态、`#ID`、掩码 Key、创建、过期、最后使用 |
| 统计块"24 小时请求" | `/api/log`（管理员）或 `/api/log/self`，`type=2` 与 `type=5` 各 `page_size=1`、`token_id`、`start_timestamp=现在-24h`，取 `total` 相加 | 精确值；读不到显示"未知" |
| 统计块"失败" | 该 Key 最近 20 条请求记录（`token_id`，只保留本人、type 2/5） | 未完成的请求数（含发出前拒绝）；说明写"最近 N 次请求中"，另列"用量未提供"条数 |
| 第三个统计块 | `/api/token/<id>/subscription-share` | 有分额时取用得最多的一个窗口，基线未知显示"未知"；没有分额时显示剩余额度（个人模式显示最后使用） |
| 请求页签 | 同"失败"的 20 条 | 全部 / 失败 / 用量未提供 切换只筛已读到的行；行进入请求页，无 Request ID 的旧行打开旧记录对话框 |
| 范围与分额页签 | Key 对象 + `subscription-share` | 原检视器的"访问"与"窗口分额"内容区 |
| 交付页签 | `/api/status` 的 `server_address`（为空用当前地址）+ Key 对象 | 接入地址可复制；Key 保持掩码，只有点"显示 / 复制"才读取完整 Key；Root 在此做凭据维护 |
| 请求页记录 | `/api/log` 或 `/api/log/self` `?request_id=`（不限日期，取最新一条请求记录） | 同一 Request ID 有多条时写明条数 |
| 四格结果 | 记录的 `other`：`request_outcome`/`preflight_rejected`、`http_status`、`usage_outcome`/`usage_accuracy`/用量证据、`settlement_status` | 缺失显示"未记录 / 未提供 / 结算未记录"，不补 0 **[测试]** |
| 诊断时间线 | 同一记录：Key、模型、`admin_info.relay_attempts`（仅管理员）、连接名（仅管理员）、HTTP 状态、首字节、上游错误码、结算、记录时间 | 记录里没有分步时间戳，所以只写记录时间；普通用户看到"路由细节仅管理员可见" **[测试]** |
| 已保存的记录 | 同一记录（原诊断内容区 `LogDetailsContent`，嵌入模式不再重复路由尝试） | 含输入 Token 估算及"本地估算，不作为实际用量或结算依据" |
| 正文 / SSE | `/api/full-content-logs/<requestId>`（仅管理员）；未保存时读 `/api/full-content-logs` 判断采集开关 | 沿用原组件与提示：未采集、采集关闭、需要管理员权限 |
| 原始 | 本页读到的这条记录（`other` 解码后显示） | 不含正文 |

## 3. 代码处置

- 新增：`components/object-page/`（对象页布局与返回焦点）、`features/keys/key-page.tsx`、`components/key-page/`（操作、统计块、交付页签）、`hooks/use-key-detail.ts`、`hooks/use-key-subscription-share.ts`、`lib/share-usage.ts`、`features/usage-logs/request-page.tsx`、`components/request-result-grid.tsx`（ResultGrid）、`components/diagnosis-timeline.tsx`（DiagnosisTimeline）、`lib/relay-attempts.ts`、`lib/request-page-search.ts`，以及三个路由文件。
- 删除（`grep` 确认无引用）：`keys/components/inspector/key-inspector.tsx`、`usage-logs/components/request-inspector.tsx`、`request-lookup-inspector.tsx`、`lib/request-inspector-tabs.ts`、`RequestFacts`、`CompactApiKeyRowActions`、表格的 `compact` 列选项、`inspectedKeyId`、`KeyAccessPanel` 的凭据维护入口（移到交付页签）。
- 保留并复用：`inspector/` 下的最近请求、访问、窗口分额内容区；`LogDetailsContent`、正文与 SSE 面板、`useRequestLookup`。`components/inspector/` 的检视器布局仍被上游页使用，未动。
- `knip` 对本步文件无新增未使用导出（`KEY_PAGE_TABS`、`REQUEST_PAGE_TABS`、`LogDetailsBadges` 已改为不导出）。

## 4. 验证命令与结果 **[运行]**

前端（`web/`）：`bun run typecheck` 退出码 0；`bun run lint` 退出码 0、8 个 warning（与基线相同，0 error），两者在 `f3b29ffe` 上重跑；`bun run build`（`VITE_BUILD_ID=f3b29ffe50243d9ebd984e7e56496bc080355d58`）退出码 0；i18n 同步脚本（`sync-i18n.mjs` 副本，只多过滤 `._*`）完成。

全量单测（`f3b29ffe`）：1843 个用例（基线 1813，本步净增 30），1840 通过、3 失败：`credential-maintenance-dialog`、`credential-rotation-dialog`（基线既有的存储类失败）以及 `strict-parameter-selection`（高负载偶发，单独运行通过；前一轮偶发的是 `handoff-lifecycle`）。

新测试先红后绿：14 处变异（旧链接不重定向两类、返回不恢复焦点、列表筛选不带到 Key 页、HTTP 缺失显示 0、请求页把人人当管理员、24 小时总数读不到显示 0、手机行用 PC 列、请求面包屑丢 Key、行点击不进入、调用记录详情仍开对话框、普通用户读全员日志、路由尝试列两遍、删除后停在 Key 页），每一处都有测试失败，恢复后全部通过；"删除后返回仍显示已删 Key"另做了一次去掉修复的红灯检查。

### 单测改动逐条说明

| 文件 | 改动 | 理由 |
| --- | --- | --- |
| `keys/__tests__/key-inspector-url.test.tsx` → `key-page-url.test.tsx` | 每条旧断言改写为整页等价：点 Key 进入 `/keys/21`、只列本 Key 的请求；分享链接还原请求与正文页签；进入请求后返回 Key 页且焦点回到那一行；关闭（返回）后焦点回到来源行；手机由顶栏返回；手机首屏请求页签第一块就是请求行、没有文本筛选框；从调用记录返回时 Key 获得焦点；手机顶栏一级一级退出；旧链接与非法参数 | 新增：行上任意位置点击进入（复选框不进入）、页签写入 URL、返回保留筛选与焦点、分享链接返回到列表与行、请求在 Key 之下的面包屑、他人 Key 的请求有提示、旧链接对照表、旧记录对话框、非法地址回到列表 |
| `keys/__tests__/key-inspector-actions.test.tsx` → `key-page-actions.test.tsx` | 打开方式从检视器改为 Key 页；"访问"页签 → "范围与分额"；凭据维护在"交付"页签；删除后回到列表；个人模式下的"创建"在列表上检查 | 新增：范围页签显示"禁止所有模型"；10 秒保鲜缓存下删除后返回不显示已删 Key（去掉修复时失败） |
| `keys/__tests__/key-recent-requests.test.tsx` | 打开 Key 页；返回由外壳的返回动作触发；"首次读取失败"只让最近 20 条那次失败（24 小时总数是另一组读取） | 原断言（用量未提供不补 0 Token、只列本人、失败重试、繁简中文时间、调用记录链接带返回上下文）不变 |
| `keys/__tests__/keys-page-fixture.tsx` | 使用真实路由的 `validateSearch`/`beforeLoad`/`component`；日志模拟支持 `type` | 让测试走真实的重定向 |
| `keys/__tests__/key-page-stats.test.tsx`（新） | 24 小时总数来自 type 2 与 type 5 的精确 `total`、普通用户只读 `/self`；失败只数未完成的请求并写明用量未提供；读不到显示"未知"不为 0；分额块取用得最多的窗口、基线未知不参与；无分额显示剩余额度；手机为两行记录、无列标题 | 新行为 |
| `usage-logs/__tests__/request-direct-open.test.tsx` | 旧 `?inspect=request` 链接重定向到请求页且不限日期查找；未找到与读取失败分开；正文页签只读已保存正文、不发 POST；旧 SSE 页签保留；分享的请求页返回到按 Request ID 筛选的调用记录 | "列表扩到记录当天"一条随列表不再垫在详情后面而失去意义，改为断言重定向后的地址 |
| `usage-logs/__tests__/request-page.test.tsx`（新） | 四格结果缺失时为"未记录/未提供/结算未记录"、已知时取记录值；管理员看到按顺序的路由尝试且只出现一次；发出前拒绝没有路由与上游步骤；普通用户只读 `/self`、无路由细节、不读正文；原始页签显示记录并写入 URL | 新行为 |
| `usage-logs/__tests__/platform-workflow.test.tsx` | 详情进入请求页：无金额、未知不补零、已保存内容在正文页签、返回后地址与分页不变；切换登录身份后上一身份读到的详情（原始页签里的其他用户名）消失 | 夹具对完整 Request ID 精确返回并补 `content` 字段（真实接口总有） |
| `dashboard/.../overview-page.test.tsx`、`layout/.../command-navigation.test.tsx` | 期望地址改为 `/usage-logs/request/<id>`；概览测试的 Link 模拟替换所有路径参数 | 同一个请求，新地址 |

## 5. 浏览器脚本

本机 Chromium（Playwright 1.61.1、Node 22），最终构建 `VITE_BUILD_ID=f3b29ffe50243d9ebd984e7e56496bc080355d58`（与提交内容相同的工作区）。"修复前"是新界面加未改的旧脚本；"中途"是脚本改完后的第一轮。

| 脚本 | 修复前 | 中途 | 最终 |
| --- | --- | --- | --- |
| Go `TestTokenMaintenanceRuntimeBrowser` / `TestTokenCredentialRotationRuntimeBrowser` | 1 / 1（停在 open-keys：找不到"访问"页签） | 0 / 0 | 0 / 0 |
| window-maintenance / model-scope / security-maintenance | 1 / 1 / 0 | 0 / 0 / 0 | 0 / 0 / 0 |
| browser-fixtures、connection-key-browser-fixtures、key-request-preview（`node --test`） | 0 | 0 | 0 |
| personal-workbench / subscription-share / personal-console / connections / key-request | 1 / 1 / 1 / 0 / 1 | 0 / 0 / 1 / 0 / 0 | 0 / 0 / 0 / 0 / 0 |
| profile / stability / setup / personal-status / browser-smoke | 0 / 0 / 0 / 1 / 1 | 0 / 0 / 0 / 0 / 1 | 0 / 0 / 0 / 0 / 0 |
| quota / routing / access-policy browser-smoke | 1 / 0 / 0 | 1 / 0 / 0 | 0 / 0 / 0 |
| credential-maintenance / credential-rotation / maintenance-session | 1 / 1 / 1 | 0 / 0 / 1 | 0 / 0 / 0 |
| playground（仓库原脚本 / 本地副本） | 1 / 1 | 1 / 0 | 1 / 0 |

- playground 仓库原脚本在 macOS 上停在第 326 行 `key.press('End')`（原生 `<select>` 不响应 End，第 1、2 步已记录，CI 的 Linux 通过）；本地副本只把两处 `press('End')` 换成 `selectOption('22')`，其余与仓库脚本相同，最终为 0。修复前副本的失败在"调用记录详情是对话框"一步。
- 中途四个失败与处理：personal-console（中文下"请求成功"在页面上出现三处，脚本 `waitFor` 触发 Playwright 严格模式，改为 `.first()`，计数断言不变）；browser-smoke（额度列与"访问方案"列的重叠检查找不到默认隐藏的访问方案列，改为与相邻列比较；结算状态的版面检查以对话框为容器，改为同时接受请求页，页面随外壳滚动所以只检查宽度）；quota（请求页默认读已保存正文，夹具没有正文时浏览器记录 404，脚本把任何控制台错误视为失败——夹具改为提供该请求的已保存正文）；maintenance-session（**产品缺陷**：在 Key 页删除后返回列表，列表从 10 秒保鲜的缓存显示已删除的 Key。已修：Key 页的改动与删除使列表缓存作废，并加了单测）。

### 脚本改动逐条说明（均为等价改写，没有删除或放宽断言）

| 脚本 | 改动 | 理由 |
| --- | --- | --- |
| `tools/ui/key-request-browser.mjs` | 检视器定位换成 `key-page` / `request-page`；`?inspect`、`?request` 断言换成路径；PC 用面包屑返回、手机用顶栏返回；正文成为默认页签（URL 不带 `tab`），SSE 写 `tab=sse`；新增"旧 `inspect+request` 链接落到请求页 SSE"一项 | 原有 16 项检查（只列 Key #21 的 4 条、诊断文字、HTTP 200 的流内错误、SSE 原文、用量未提供、未采集正文、返回后焦点回到行、调用记录按 Key ID、分页、返回列表保留筛选与焦点、关闭后焦点、旧 `keyId`、Request ID 直达、无外部/写入/取 Key 请求）全部保留 |
| `tools/ui/browser-smoke.mjs` | 结算详情改读请求页；Escape 关闭改为浏览器后退，"关闭"按钮改为应用内返回；"刷新后不显示旧状态"改为"刷新后重新读取结算状态（等到该 GET）且仍只读"；版面检查接受请求页；额度列重叠比较相邻列；手机 Key 卡片进入 Key 页"范围与分额" | 请求详情不再是对话框；不变量（只读、无手动表单、两行文字不裁切、可滚到下方、刷新不残留旧状态）都保留 |
| `tools/quota/browser-smoke.mjs` | 待核对记录的详情改读请求页，后退离开；夹具提供该请求的已保存正文 | 手动核对表单与"未确认不能提交、提交一次、提交后不能再提交"断言不变 |
| `tools/quota/token-budget-browser.mjs` | 手机进入 Key 页"范围与分额"；夹具回答 `/api/token/1/subscription-share`（空） | Key 页第三个统计块读取分额 |
| `tools/ui/personal-console-browser.mjs` | 详情进入请求页；已保存内容在 SSE 页签读取（原为另一个对话框）；后退后 URL 不变；夹具对完整 Request ID 精确返回；`.first()` | 与真实接口一致：完整 Request ID 精确匹配 |
| `tools/ui/personal-status-browser.mjs`、`tools/ui/playground-browser.mjs` | 详情进入请求页；Escape 改后退；Request ID 在页面上出现两处，取第一处 | 文字、未知不补零、无金额、无余额断言不变 |
| `tools/ui/credential-*-browser.mjs`、`tools/runtime/credential-*-browser.mjs` | "访问"页签改为"交付"页签 | 凭据维护入口移到交付页签 |
| `tools/ui/maintenance-session-browser.mjs` | Key 页"范围与分额"；第一次后退回到列表；删除后落到空列表（检查路径） | 会话边界、不重放写入、不刷新登录的断言不变 |
| `tools/ui/model-scope-browser.mjs`、`subscription-share-browser.mjs`、`window-maintenance-browser.mjs` | 入口改为 Key 页"范围与分额" | 同上 |
| `tools/ui/personal-workbench-browser.mjs` | 仍用旧的 `/keys?inspect=<id>` 打开（必须重定向到 Key 页，新增路径断言）；Key 对象读取允许出现在 `/keys/*` | Key 页属于 Key 页面组；概览不得读 Key 对象的断言不变 |
| `tools/ui/browser-fixtures.mjs` | 管理员读取 `/api/full-content-logs/<id>` 时回答 404（未保存） | 普通用户读取仍记为违规 |
| `tools/runtime/personal-browser.mjs` 与其 `personal-browser.test.mjs` | 详情控件现在是链接；详情页用 `[data-testid="request-page"]`；离开用后退；假页面补 `goBack` | Docker 冒烟用，本机未运行；`node --test` 修改前后均为 140 通过 / 5 失败（同样 5 项，基线既有，不在 CI） |

## 6. 截图与浏览器检查

`screenshots/shots.mjs`（Playwright 1.61.1 + Chromium，只放行 `127.0.0.1:4184` 的合成预览，其余请求一律中止，本次中止 0 个；场景数据、暂停/恢复与复制的写入只在浏览器里应答，预览本身只读）。70 张截图，344 项检查，338 项通过；未通过的 6 项全部是同一个问题（见下表第 1 条）。截图逐张看过 **[浏览器]**。

| 组 | 文件 |
| --- | --- |
| 主矩阵（列表、Key 页、请求页） | `{keys,key,request}-{1280,390}-{light,dark}-{zh,en}.png` |
| 其他宽度 | `{key,request}-{320,768,1024}-light-{zh,en}.png`、`keys-{320,768,1024}-light-zh.png` |
| 页签 | `key-{scope,delivery}-{1280,390}-light-zh.png`、`request-{sse,raw}-{1280,390}-light-zh.png`、`request-full-1280-light-zh.png` |
| Lite | `lite-keys-1280-light-zh.png`、`lite-key-1280-light-zh.png`、`lite-request-390-light-zh.png` |
| 普通用户 | `user-key-1280-light-zh.png`、`user-request-{1280,390}-light-zh.png`（无路由细节、正文需要管理员） |
| 空 / 未找到 / 错误 | `empty-keys-1280-light-zh.png`、`empty-keys-390-light-en.png`、`empty-key-requests-1280-light-zh.png`、`missing-request-{1280-light-zh,390-light-en}.png`、`error-key-1280-light-zh.png`、`error-request-1280-light-zh.png` |
| 正文未采集 | `no-body-capture-on-1280-light-zh.png`（采集开启但该请求无文件）、`no-body-capture-off-1280-light-en.png`（采集关闭） |
| 发出前拒绝 | `rejected-request-1280-light-zh.png` |
| 实际操作 | `ops-back-to-list-1280-light-zh.png`、`ops-request-from-key-1280-light-zh.png`、`ops-paused-1280-light-zh.png`、`ops-legacy-request-1280-light-zh.png`、`ops-command-search-1280-light-zh.png`、`ops-phone-actions-390-light-zh.png` |

每张页面截图检查：无横向溢出、无页面错误、标题在视口内、可见文字没有被无省略号地截断；小于 1024px 时（触屏模拟）页面内按钮、链接、页签 ≥44px，滚到底后最后的内容在底部导航之上。实际操作检查（`ops:` 开头 16 项，全部通过）：点 Key 行进入 `/keys/22`；面包屑返回后 `filter=Synthetic`、`page=2` 保留且焦点回到第 22 行；点行上非控件处也进入；从 Key 页进入请求再用浏览器后退，焦点回到那条请求；暂停→恢复→复制（剪贴板为 `sk-synthetic-copied-secret`）；`?inspect=21&tab=access` 落到范围页签；旧请求链接落到请求页 SSE；旧调用记录链接落到请求页；命令搜索粘贴 Request ID 回车进入请求页；概览时间链接进入请求页；调用记录"详情"进入请求页、返回后筛选还在；手机操作区在底部导航之上；手机顶栏一级一级退出；320px 请求页签横向滚动而不是换行。

### 看图发现的问题

| # | 现象 | 位置 | 处理 |
| --- | --- | --- | --- |
| 1 | 手机 Key 列表分页的页码按钮 32px 宽（高 44px） | `keys-{320,390,768}-*.png`（检查项未通过） | 未修：共享的 `data-table` 分页组件，所有列表页都一样；改它会动到其他页面，留到其余页面批次 |
| 2 | 列表"操作"列被挤成竖排，每行很高（第一版就如此） | 中间构建 | 已修：操作单元保持一行，名称列最宽 15rem（完整名称在 `title`），状态、模型、最后使用设最小宽度 |
| 3 | Key 页"更多"按钮在手机上只有 36px 宽 | 中间构建，检查项未通过 | 已修：外壳的 `.myapi-page-actions` 把按钮最小宽度清为 0，Key 页给这个按钮单独补 44px |
| 4 | 英文"Rejected before sending"在结果列折成两行 | `key-1280-dark-en` 中间构建 | 已修：结果列加宽到 11rem |
| 5 | 320px 英文下三个统计块挤在一行，单词被拆开 | 中间构建 | 已修：小于 360px 时统计块改为单列 |
| 6 | 空请求卡片右边框被裁掉；不可用 Key 的标题下重复 `#21` | 中间构建 | 已修（`f3b29ffe`） |
| 7 | 读取 Key 失败时全局还会弹出接口错误提示 | `error-key-1280-light-zh.png` | 未修：接口层对 `success:false` 的统一提示，第一版检视器同样如此 |
| 8 | 统计块"失败"的说明较长，390px 下占三行 | `key-390-*.png` | 未修：信息需要（"最近 N 次中"+"用量未提供"），不影响阅读 |
| 9 | 手机行里耗时"2 秒"偶尔单独折到下一行 | `key-390-*.png` | 未修：已让"2 秒"本身不拆开；整行折行由模型名长度决定 |

## 7. 首屏（1280×800，中文，亮色，Full Root）与样稿对照

Key 页首屏：顶栏；面包屑"API Key › 名称"；标题、状态徽标、`#21`、掩码 Key、创建时间、过期、最后使用；操作"暂停 Key / 复制 / 打开 Playground / 更多"；三个统计块（24 小时请求 4、失败 2 与"最近 4 次请求中 · 用量未提供 1 条"、5 小时额度窗口分额已用 75%）；页签"请求 / 范围与分额 / 交付"；请求卡片（全部 4 / 失败 2 / 用量未提供 1、"该 Key 的最近请求"、列标题、4 行、"在调用记录中查看全部"）。

请求页首屏：面包屑"API Key › Key 名 › 请求 <短 ID>"；短 ID 标题、结果徽标、时间、连接、模型、完整 ID；"复制 ID"、"在调用记录中查看"；四格结果；左"诊断"（耗时、时间线、已保存的记录开头），右"正文 / SSE / 原始"（已保存正文的摘要、错误、请求文本）。

| 样稿 | 实现 | 原因 |
| --- | --- | --- |
| Key 页操作：启用开关、复制交付文本、编辑范围、更多 | 暂停/恢复按钮、复制 Key、打开 Playground、更多 | 按任务要求沿用第一版检视器的操作；编辑与范围在"范围与分额"页签 |
| 统计块"今天请求 128 · 较昨天 +8%" | "24 小时请求"精确总数，无环比 | 没有按天读取，环比需要另一组读取；口径与概览核心卡一致 |
| "失败 2 · 1 条来自上游" | "失败"取最近 N 条中未完成的请求，另列用量未提供 | 失败不是独立的记录类型（type 5 也可能是完成但用量未知），精确的 24 小时失败数需要后端支持 |
| 标题行 `sk-····a8f3 创建于 10/12 不过期` | 接口给出的掩码 Key、`#ID`、完整时间、过期、最后使用 | 掩码格式由后端决定 |
| 请求页签"今天" | "该 Key 的最近请求"（最近 20 条） | 沿用第一版读取 |
| 交付页签"Base URL / Key / 模型" | 接入地址、掩码 Key（显示/复制时才读完整 Key）、Root 的凭据维护 | Key 不绑定单个模型；凭据维护按任务放在交付 |
| Key 列表"今天 / 失败 / 分额"列 | 没有；默认列为名称、状态、API Key、额度、模型、最后使用、操作 | 列表接口不含每个 Key 的请求数与分额，逐个读取会是 N 次请求 |
| 请求页"在请求页打开" | "在调用记录中查看"（按 Request ID 筛选） | 本页已是请求页 |
| HTTP 状态"500（上游）" | 只写状态码 | 记录没有区分状态来自网关还是上游 |
| 时间线每步带毫秒时间 | 只有记录时间；耗时与首字节写在卡片头 | 调用记录没有分步时间戳，不虚构 |
| 正文页签显示原始请求 JSON | 已保存的请求文本与模型返回文本（原组件的清洗模式），原始 JSON 在 SSE / 原始页签 | 沿用第一版正文组件 |

## 8. 语言

新增 33 个文案键，提供 en 与 zh；其余五种语言由同步脚本以英文回填（各 33 个）。同步报告"疑似未翻译"计数：fr 23→31、ja 108→140、ru 107→139、vi 23→31、zh 15→16（`HTTP {{status}}` 中英相同），zh-TW 不参与该检测。视觉与浏览器验收只覆盖中英文；`key-request-browser.mjs` 与 `personal-workbench-browser.mjs` 原有的多语言矩阵保留并通过。
