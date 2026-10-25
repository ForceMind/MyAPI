# 第 4 步：上游页（卡片网格、连接对象页），以及三个收尾项

- 日期：2026-10-25；分支 `dev`；基线 `20ec63f1`（其后 `101266dd`、`cc2e191d` 只改文档）。提交见第 9 节。
- 范围：A 上游卡片网格 `/connections`、连接对象页 `/connections/<id>`、账号窗口页与模型目录的最小调整；B 概览"失败 N"接入 `result=failed`；C 共享分页组件的手机触控目标；D 个人资料页"我的订阅"只读卡片。没有改后端，没有新增依赖。
- 证据标记：**[代码]** 从实现读出；**[测试]** 有单测断言；**[浏览器]** 本机 Chromium 实际操作或截图看过。

## 1. 页面与路由

| 页面 | 地址 | 搜索参数 |
| --- | --- | --- |
| 上游卡片网格 | `/connections` | `page`、`filter`、`status`（原有）；`focusConnectionId`（从分享链接打开的连接页返回时要聚焦的卡片） |
| 连接对象页 | `/connections/<id>` | `tab`（`routing` / `requests` / `history`；默认"窗口"页签不写入）、`connectionReturn`（来源网格的 `page`/`filter`/`status`） |

- 网格：3 列（1280 起），1024–1279 两列，1024 以下单列 **[浏览器]**。卡片的名称是链接并铺满整张卡（伪元素），开关与"测试"在它之上，所以点卡片任何非控件处都进入对象页，点开关与测试只在卡片上生效 **[测试][浏览器]**。
- 返回：对象页登记到外壳（与第 3 步 Key 页同一套 `ObjectPage`），PC 用面包屑"上游 › 连接 › 名称"，手机用顶栏"返回"。从网格打开时返回等于浏览器后退，焦点回到来源卡片；网格还在加载时等待；卡片已不在（删除、筛掉）或列表读取失败时焦点落到"已配置的连接"标题；用户先有操作就放弃 **[测试][浏览器]**。从分享链接直接打开的连接页，返回时以 `replace` 回到 `connectionReturn` 记下的网格并聚焦该卡（`focusConnectionId`）**[测试]**。
- `/channels`（连接的表格视图）保留：名称是进入对象页的链接（表格、卡片、手机三种视图都有）；桌面表格行点击非控件处也进入；后退回到表格时焦点回到该名称 **[测试][浏览器]**。
- 手机上对象页隐藏"连接 / 账号窗口 / 模型目录"页签行（顶栏已有返回与名称）；PC 保留。

### 旧链接兼容

| 旧地址 | 现在 |
| --- | --- |
| `/connections?connection=7` | `/connections/7` |
| `/connections?connection=7&tab=routing` | `/connections/7?tab=routing` |
| `/connections?connection=7&tab=windows` 或 `tab=overview` | `/connections/7`（窗口为默认页签；第一版"概览"页签的内容拆到标题行、统计块与各页签） |
| 同时带 `filter`/`page`/`status` | 进入 `connectionReturn`，返回时还原 |
| `/connections/0`、`/connections/abc` | 回到 `/connections` |
| `/connections?status=disabled`（概览琥珀条） | 不变 |

全部由路由 `beforeLoad` 以 `replace` 重定向 **[测试][浏览器]**。说明：任务说明写的是 `?inspect=`，但第一版连接检视器实际使用的参数是 `?connection=`（`git` 历史与 `connection-state.ts` 可查），所以重定向的是 `connection`；没有为从未存在的 `inspect` 参数加处理。

站内指向旧检视器的链接都改为直达对象页：调用记录页的"返回连接"（原"返回连接列表"，现在落在连接页并带 `connectionReturn`）、"关联 Key 分额"工作区的返回链接、账号窗口页"管理账号连接"。

## 2. 每个展示字段的来源

打开网格或连接页只读已保存的配置、观测与调用记录，不向上游发请求；只有点"测试"才调用现有 `/api/channel/test/<id>` **[代码][测试]**。

| 位置 | 来源接口 | 口径 |
| --- | --- | --- |
| 卡片名称、类型、优先级、模型数、最近测试 | `/api/channel`、`/api/channel/search`（原有，按页 20 条） | 只缓存白名单字段，不含凭据 |
| 卡片环形与窗口行 | `/api/channel/quota/changes?channel_ids=<本页 ID>`（一次读取） | 环内写已用 %，旁边写各窗口剩余 % 与观测时间；采样失败/不支持/无时间显示状态，不画环、不补数 |
| 类型徽标 | 连接类型 + 是否有窗口 | 订阅（类型 57 或有窗口）/ 本地（Ollama、Xinference）/ API Key（其余，显示"按量计费，无订阅窗口"） |
| 状态 | 连接 `status` + 最近 24 小时最近 100 条调用记录（`/api/log`，与概览流向板同一口径） | 使用中 / 可用 / 自动禁用 / 已停用；记录读不到时不声称"使用中" |
| 开关 | 现有 `POST /api/channel/<id>/status`（需要 operate 授权） | 成功后重新读取列表与连接 |
| 统计块"24 小时请求" | `/api/log?channel=<id>&type=2|5&page_size=1` 的 `total` 相加 | 读不到显示"未知" |
| 统计块"24 小时失败" | `/api/log?channel=<id>&result=failed&page_size=1` 的 `total` | 不含待核对与发出前拒绝；数值链接到同口径的调用记录 |
| 第三个统计块 | 最紧的已存窗口；无窗口时为最近测试 | 窗口读不到显示"未知" |
| 窗口页签 | 同卡片的窗口读取 + 原 `WindowShareObservation`（Key 分额，Root 看池用量） | 大环形 + 剩余 % + 观测时间；Root 另有"关联 Key 分额"链接 |
| 路由与模型页签 | 连接记录 | 优先级、权重、分组、模型；链接到表格视图的路由预览 |
| 请求页签 | `/api/log?channel=<id>`（最近 20 条） | 与 Key 页同一列表组件（全部 / 失败 / 用量未提供），行进入请求页 |
| 历史页签 | 窗口读取里的本次与上一次采样 + 连接记录的最近测试 | 只列已保存的事件，按时间倒序；没有重置时间（接口不提供） |
| 概览"失败 N"、失败率、"失败"筛选 | `/api/log` 或 `/api/log/self` 加 `result=failed` | 见第 4 节 |
| 我的订阅 | `GET /api/subscription/self` 的 `all_subscriptions` | 套餐 #ID、状态、额度已用/总量、下次重置、有效期；套餐名该接口不提供，显示"未提供" |

## 3. 代码处置

- 新增：`features/connections/connection-card.tsx`、`connection-page.tsx`、`connection-format.ts`、`use-connection-switch.ts`、路由 `routes/_authenticated/connections/$connectionId/index.tsx`、`components/stat-tile.tsx`（从 Key 页统计块抽出共用）、`features/usage-logs/components/recent-request-list.tsx`（Key 页请求列表改为它的包装，DOM 不变）、`features/profile/components/my-subscription-card.tsx`。
- 改名并收窄：`connection-inspector.tsx` → `connection-sections.tsx`（删除检视器外层 `ConnectionInspector`，保留状态、最近测试、测试结果、路由内容区，新增历史）；`connection-windows.tsx` 改为接收查询结果并用大环形展示；`use-connections.ts` 拆为访问、列表、详情、窗口、流量、统计、请求几个读取；`use-connection-test.ts` 的结果放进按身份隔离的小 store，使"保存并测试"的结果在新连接页可见。
- 删除（`grep` 确认无引用）：`components/inspector/inspector-layout.tsx`、`use-inspector-focus-return.ts`、`use-inspector-presentation.ts`、`CONSOLE_INSPECTOR_SIDE_QUERY`、`INSPECTOR_TABS_LIST_CLASS`、样式里 `[data-inspector='page']` 的三条规则（改为手机对象页隐藏页签行的一条）。`inspector-history.ts` 仍被对象页使用。
- `useReturnFocus` 增加可选 `fallback`（目标卡片不在时聚焦的位置），Key 页不传，行为不变。
- `Switch` 增加 `lg` 尺寸（42×24，触屏命中区 ≥44px）。
- 模型目录：没有 `model_deployments` 能力（Lite）时不显示"部署"页签，`/models/deployments` 以 `replace` 回到元信息；"更多"按钮补可访问名称并在触屏上 ≥44px。
- 账号窗口页："查看连接历史"链接触屏 ≥44px；"管理账号连接"直达连接页。业务未改。

## 4. 收尾项

- **B 失败筛选**：`/usage-logs/$section` 接受 `result=failed` 并传给列表与统计接口；调用记录页显示"仅请求失败"提示与"显示全部错误记录"；筛选"搜索"在类型仍为错误或全部时保留它，"重置"清除。概览：核心卡失败率 = 24 小时 `result=failed` 总数 ÷ 24 小时请求记录总数（两者都精确）；琥珀条写"24 小时内 N 个请求失败"，链接带 `type=5&result=failed` 与同一时间窗，条数与链接目标一致；精确总数读不到时退回"最近请求中 N 条失败"（用与后端相同的规则在样本里计数）；最近请求的"失败"筛选改为向服务器读取 `result=failed`，"查看调用记录"随之带上参数；不传参数时旧链接不变。流向板节点的失败数也改用同一规则（`isRequestFailure`，与 `model/log.go` 的 `applyLogResultFilter` 对应）**[测试]**。
- **C 分页**：页码、上一页/下一页、首末页按钮在 `pointer: coarse` 下 44×44，桌面保持 32px；按钮组在窄容器里换到第二行而不是被裁掉 **[浏览器：320/390 下 Key 列表、调用记录、模型目录的分页按钮全部 ≥44px 且在视口内；1280 桌面 ≤36px]**。
- **D 我的订阅**：个人资料页右栏，只有接口返回至少一条订阅才显示；没有任何按钮或下拉；有商业订阅能力时只多一个"在钱包中管理"链接；Lite 无链接；不是本人的记录不显示 **[测试][浏览器]**。

## 5. 验证（本机已运行的部分）

维护者在本步中途要求构建与浏览器验收改在 GitHub 上进行、不再占用本机空间，所以下列结果来自中途的本机运行，最终提交上的完整验证交给 CI。

- `bun run typecheck`：最终工作区上退出码 0（最后一次本机运行）。
- `bun run lint`：退出码 0，8 个 warning（与基线相同，0 error）；在 `focusConnectionId` 改动之前运行，之后的改动只经过 typecheck。
- 全量前端测试（中途一轮）：1875 个用例，3 失败：`credential-maintenance-dialog`、`credential-rotation-dialog`（基线既有存储类失败）、`strict-parameter-selection`（单独重跑通过）。之后新增 3 个用例（分享链接返回、表格视图行点击两例），只单独运行过所在文件（通过）。
- 新测试先红后绿：19 处变异，18 处有测试失败（旧链接不重定向、环内不写数值、卡片不带列表状态、无焦点回退、开关无效、保存并测试不测试、失败数读不到写成 0、`result` 不传给接口、预检拒绝算作失败、概览失败数不用 `result`、最近请求"失败"只筛样本、流量把拒绝算作失败、无订阅也显示卡片、显示他人订阅、缺失重置时间写成日期、表格行不进入、点行内控件也进入、分享链接返回不聚焦），其中"最近请求'失败'只筛样本"第一次未变红，补了"样本之外的较早失败也要列出"的断言后变红；"默认页签写入地址"的变异被搜索参数校验吸收，可观察行为不变。
- `bun run build`：中途构建退出码 0；最终提交未在本机构建。
- i18n：新增 47 个键（en、zh），同步脚本（过滤 `._*` 的副本）回填其余五种语言；"疑似未翻译"计数 fr 31→39、ja 140→179、ru 139→178、vi 31→39、zh 16→16，zh-TW 不参与该检测。
- 截图与浏览器检查：`screenshots/shots.mjs` 在本机合成预览上跑过一轮（截图存于本机临时目录，未提交 PNG）：页面主矩阵（上游、连接页 × 1280/390 × 亮暗 × 中英）、320/768/1024、页签、状态（空、错误、无权限、普通用户、Lite、窗口读取失败）、实际操作、概览失败、分页、我的订阅，共约 300 项检查；修正后各组最后一轮只剩 `models-390` 的"更多"按钮 40px（已改为 `min-w-11!`，未重跑）。
- 浏览器验收脚本：只完成"修复前"一轮的前半部分（Go 两个运行时测试 0/0，window-maintenance / model-scope / security-maintenance 0/0/0，`node --test` 0，personal-workbench 1（网格卡片定位器，已改），subscription-share 0），随后按要求停止。更新后的 connections-browser 跑过一次，停在"请求"页签的翻译键（已改为 `Requests_section`，未重跑）。

## 6. 首屏与样稿对照（1280，中文，亮色，Full Root）

网格首屏：页签行"连接 / 账号窗口 / 模型目录"；标题"连接"与"N 个连接 · 正在使用：…"；"卡片视图 / 表格视图"与"添加连接"；搜索框与状态分段；3 列卡片（环形或类型图标、名称、优先级 · 模型数 · 提供方、窗口行、状态与类型徽标、最近测试、开关与"测试"）；底部"打开此页不会向上游发请求；只有点「测试」才会联网。"

| 样稿 | 实现 | 原因 |
| --- | --- | --- |
| 标题"上游"，筛选"全部 / 订阅 / API / 本地" | 标题"连接"（与页签一致），筛选"全部状态 / 已启用 / 已禁用" + 搜索 | 列表接口按页返回，按种类筛选只能筛当前页；保留原有服务器端筛选 |
| 卡片"测试失败 401" | 最近测试时间与耗时；本次会话的测试结果显示在卡片上 | 连接记录不保存测试结果 |
| 卡片"更多"按钮 | 无（更多操作在连接页） | 卡片只放开关与测试 |
| 对象页无统计块 | 三个统计块 | 按任务要求（与 Key 页一致） |
| 窗口"预计重置 22:10" | 观测时间 | 已存窗口观测不含重置时间 |
| 历史"5 小时窗口超过 80%""重置" | 已存的本次 / 上次采样与最近测试 | 不从两次采样推断事件 |
| 面包屑"上游 › 名称" | "上游 › 连接 › 名称" | 外壳三级面包屑（区域 › 页面 › 对象） |

## 7. 没做的与未验证

- 表格视图的卡片模式只有名称链接进入连接页，整卡点击没有做（`DataTableCardGrid` 无行点击）。
- 网格的类型筛选（订阅 / API / 本地）没有做，见上表。
- 调用记录页"仅请求失败"提示与筛选栏保留/清除 `result` 只有浏览器检查，没有单测。
- `tools/ui/console-layout-browser.mjs`、`input-policy-browser.mjs`、`tools/quota/token-budget-browser.mjs` 不在 CI 中，没有更新也没有运行；它们打开的 `/connections?connection=1` 会被重定向到连接页。
- 七语言：只提供 en、zh 译文；其余五种语言为英文回填。
