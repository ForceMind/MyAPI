# MyAPI UI 重新设计：设计方向方案（任务 6 前置，待维护者确认）

- 日期：2026-10-24；分支 `dev`；工作区 `.claude/worktrees/dev-work`。
- 角色：UI 与视觉。本阶段只写本文件和 `baseline/` 截图，未改 `web/` 源码，未 commit，未 push。
- 维护者要求（原话要点）：保留现有 Logo（`web/public/myapi-logo-v1.png`），整个 UI 重新设计，核心是易用，PC 版和手机版都要高度适配，必须使用设计 skill。
- 证据标记：**[查看]** = 亲自查看截图或浏览器；**[运行]** = 亲自运行命令或脚本得到；**[代码]** = 从代码看（读源码得出，未运行行为）；**[猜测]** = 推断，需要核实。
- 使用的设计技能：
  1. `shadcn-ui`（仓库 `.agents/skills/shadcn-ui/SKILL.md` 及 `vendor/shadcn/rules/` 下 composition、styling、forms、icons、base-vs-radix，`customization.md`）——组件组合、语义 token、表单、图标、Base UI 写法。
  2. `vercel-react-best-practices`（仓库 `.agents/skills/vercel-react-best-practices/SKILL.md`）——双端组合时的渲染与数据获取约束。
  3. `dataviz`（本会话可用的设计类技能）——图表配色用其 `validate_palette.js` 实际校验，统计块与图表规则按其 non-negotiables。
  4. `anthropic-skills:built-in-browser`——现状取证时使用内置浏览器的规则。

---

## 0. 结论摘要

1. 现有 UI 的主要问题不是"颜色不好看"，而是：首屏被说明卡和统计块占满、列表落到首屏以下；同一对象有 2–3 个名字；手机上同时固定两条栏并遮挡操作；七语言下导航断词；两套调色板并存且部分状态色不达 WCAG AA。见第 2 节，每条有截图或代码依据。
2. 推荐方向 A"工作台 + 检视器"：PC 是分组侧栏 + 列表 + 右侧检视器，手机是底部 5 项导航 + 列表 + 全屏详情；中等偏高密度，排错路径最短。备选方向 B"引导式任务中心"信息密度低、顶部横向导航，在七语言和 30 多个入口下不成立。理由见第 7 节。
3. 视觉系统从 Logo 两色推导：深蓝 `#0035FC` 作为主操作色（白字对比 7.20:1），青 `#00CFFC` 只用于遥测与数据强调（不作文字色，白底对比仅 1.85:1）。提议的亮/暗两套主要组合全部 ≥4.5:1，图表 5 色通过 dataviz 校验器（暗色有一对需要直接标签）。**[运行]**
4. 版本裁剪：Full 显示全部；Lite/Desktop 隐藏架构评估 C1–C12 对应入口，默认首页都是"概览"，Desktop 的概览首屏加"本机接入"卡片。依赖任务 4 在 `/api/status` 暴露版本与能力。
5. 实施风险最大的是 `tools/` 下 30 个浏览器验收脚本：10 个依赖 `data-testid`（97 处），13 个依赖 `.pc-*`/`data-myapi-*` 选择器，1,170 处按可访问名称或文本查询，改文案就会失效。**[运行]**

需要维护者拍板的决策点集中在第 11 节，共 6 项。

---

## 1. 用户与任务

### 1.1 谁在用

| 角色 | 是否进入控制台 | 关心什么 | 设计含义 |
| --- | --- | --- | --- |
| 管理者（Root/Admin；自用场景下通常只有一人） | 是，PC 为主，手机用于外出查看与应急 | 上游是否可用、额度窗口还剩多少、Key 发给谁、某次请求为什么失败、系统是否健康 | 控制台按"上游 → 访问 → 记录 → 系统"组织；排错路径最短；手机能完成查看、暂停 Key、查请求、看正文 |
| Key 使用者（同事、客户端程序） | 否，只持有 Key | Base URL、可用模型、额度、示例请求 | 设计"Key 交付卡"：创建 Key 时一次性展示 Key、Base URL、允许模型、curl/Python/Node 示例，可复制成一段纯文本发出去 |
| 普通注册用户（仅 Full 开放注册时） | 是 | 自己的 Key 和调用记录、资金 | 只见"概览（个人）/ API Keys / 调用记录 / 个人资料"，不见上游与系统 |

### 1.2 管理者前 10 个高频任务

"现有步数"= 从已登录的概览页出发、到达任务完成所需的点击/提交次数（不含输入文字）。来源：本次截图 **[查看]** 与组件代码 **[代码]** 推算，未逐项实测点击。"目标步数"是本方案的验收线。

| # | 任务 | 现有路径（PC） | 现有步数 PC / 手机 | 目标步数 PC / 手机 | 新路径 |
| --- | --- | --- | --- | --- | --- |
| 1 | 查一个 Key 的最近请求并看失败原因 | 侧栏 API 密钥 → 卡片"查看 Key 请求" → 行"详情" → 详情里打开正文 | 4 / 4（手机首屏看不到任何请求行，需先滚过筛选与统计，`key-requests-390.png`） | 2 / 3 | Key 列表点行 → 检视器"最近请求"点一条（详情含"诊断/正文/SSE"页签）；手机：Keys → Key 详情 → 请求 |
| 2 | 按 Request ID 定位请求 | 调用记录 → "展开"筛选 → 输入 → 搜索 → 详情 | 4 / 5 | 1 / 2 | 顶栏全局搜索（`Ctrl/⌘ K`）粘贴 Request ID，回车直接打开请求详情 |
| 3 | 创建 Key 并交给使用者 | API 密钥 → 创建 → 提交 → 复制 | 4 / 4 | 3 / 3 | 概览或 Keys 的"创建 Key" → 提交 → Key 交付卡"复制全部" |
| 4 | 新增一个上游连接并验证可用 | 连接 → "高级渠道管理" → 创建渠道 → 提交 → 行菜单 → 测试 | 6 / 6（`connections` 页没有新增和测试入口 **[代码]**：`features/connections/connections.tsx` 文案中无 Add/Test） | 3 / 3 | 上游 → 添加连接 → "保存并测试" |
| 5 | 看上游账号窗口还剩多少 | 上游账号 → 选账号 | 1–2 / 1–2 | 0 / 0 | 概览首屏"最紧张的窗口"列表直接给出百分比、观测时间、状态 |
| 6 | 测试一个已有连接 | 连接 → 高级渠道管理 → 行内测试图标 | 3 / 3 | 2 / 2 | 上游列表点行 → 检视器"测试" |
| 7 | 暂停或恢复一个 Key | API 密钥 → 卡片"暂停 Key" | 2 / 2 | 2 / 2（不变，加撤销 toast） | Keys 行操作或检视器 |
| 8 | 调整 Key 的模型范围或额度 | API 密钥 → "模型范围"或"窗口分额" → 对话框 → 保存 | 3 / 3 | 3 / 3（不变，改为检视器内编辑，手机为全屏表单） | Keys → 检视器"访问"页签 → 编辑 → 保存 |
| 9 | 看近期失败请求的汇总 | 调用记录 → 类型筛选"错误" → 搜索 | 3 / 4 | 1 / 1 | 概览"近期请求"区块的"失败 N"链接直接带筛选打开 |
| 10 | 查看后台任务与版本，或改日志采集设置 | 系统信息（1）；改正文采集：系统设置 → 运维 → 日志维护 → 开关 → 保存（5） | 1 与 5 / 1 与 6 | 1 与 3 / 1 与 4 | 正文页的"采集已关闭"状态旁直接给"去设置"链接，深链到对应设置分区 |

---

## 2. 现状问题（取证结果）

截图：`baseline/` 下 27 张，宽度 1280×900、390×844、320×700（俄语），合成预览 `http://127.0.0.1:4184`，中文界面。截图由本地 headless Chromium 通过 CDP 生成，拦截了所有非 127.0.0.1:4184 的请求（实际拦截数 0）；另用内置浏览器在 1280 宽度核对了 `lang` 属性与表格溢出。**[运行]**

### 2.1 信息层级

| # | 问题 | 证据 |
| --- | --- | --- |
| H1 | 列表页首屏被说明卡、统计块和筛选占满，主数据落到首屏以下。调用记录 1280 首屏只露出 2.5 行请求；390 首屏 0 行 | `key-requests-1280.png`、`key-requests-390.png` **[查看]** |
| H2 | 一个页面堆多块说明横幅。正文页有"在线请求浏览器"和"已启用新请求的正文采集"两块，加 3 个统计卡，才到筛选 | `content-logs-1280.png` **[查看]** |
| H3 | 概览用大号数字"1"做卡片主体，三张卡各占 1/3 宽度，手机首屏只放下 2 张卡，看不到近期请求 | `overview-1280.png`、`overview-390.png` **[查看]** |
| H4 | 渠道页把列表、额度消耗图、额度事件、路由预览堆在同一页；单张渠道卡只占 310px，名称被截成"Synthetic co…""0 T…"，右侧大面积空白 | `channels-1280.png` **[查看]** |
| H5 | 模型调用分析在有请求的情况下仍全显示 0 与空坐标轴（0–1），没有走空态组件 | `analytics-1280.png` **[查看]**；是数据范围还是统计口径导致，未核实 **[猜测]** |

### 2.2 导航与命名

| # | 问题 | 证据 |
| --- | --- | --- |
| N1 | 同一对象多个名字：侧栏"连接"，页面标题"渠道"，按钮"高级渠道管理"；侧栏"API 密钥"、卡片"Key"、表头"令牌"；侧栏"调用记录"、面包屑"通用日志"；侧栏"模型"、页面标题"元信息" | `channels-1280.png`、`key-requests-1280.png`、`models-1280.png` **[查看]** |
| N2 | "系统信息"同时是运维页面（`/system-info`）和设置分区（`/system-settings/site/system-info`）的名字 | `system-info-1280.png`、`settings-1280.png` **[查看]**；`system-settings.config.ts` `SITE_SECTIONS` **[代码]** |
| N3 | 分页按钮译成"上一步/下一步"（步骤语义），应为"上一页/下一页" | `connections-1280.png`、`content-logs-1280.png` **[查看]** |
| N4 | 上游相关入口分散在三处：上游账号、连接、渠道（高级）。连接页能看不能加、不能测 | 截图 + `connections.tsx` 文案 **[代码]** |
| N5 | 提示词学习不在侧栏，只能从个人资料页的卡片进入 | `use-sidebar-data.ts` 无 `/prompt-learning` **[代码]**；`profile-1280.png` **[查看]** |
| N6 | 个人资料放在"运维"分组里，面包屑显示"运维 > 个人资料" | `profile-1280.png` **[查看]** |
| N7 | 手机"更多"抽屉重复列出底部已有的 4 项，且无搜索，列表很长 | `more-sheet-390.png` **[查看]** |
| N8 | 渠道页 `h1` 的可访问文本是"渠道最大重试: 0"，徽标混进了标题 | CDP 读取 `h1.textContent` **[运行]** |

### 2.3 密度与一致性

| # | 问题 | 证据 |
| --- | --- | --- |
| D1 | Key 用大卡片展示，每张卡 7 个操作按钮；Key 多于 10 个时不可扫视 | `keys-1280.png` **[查看]** |
| D2 | 表格在 1280 宽度横向溢出：正文页表格容器 `scrollWidth 1294 > clientWidth 964`，"耗时""操作"列被裁；系统信息实例表最后一列被裁 | 内置浏览器 JS 读取 **[运行]**；`system-info-1280.png` **[查看]** |
| D3 | 同类控件混用：原生 `<select>` 与 Base UI Select 并存（13 个文件用原生 `<select>`）；原生日期输入显示英文 `mm/dd/yyyy` | `grep` **[运行]**；`content-logs-1280.png` **[查看]** |
| D4 | 页面模式开关被当作主操作：渠道页头部并排"批量操作/标签模式/使用 ID 排序"三个开关 | `channels-1280.png` **[查看]** |
| D5 | 每页条数 PC 为 20、手机为 10，统计口径随宽度变化 | `models-1280.png` 与 `models-390.png` **[查看]** |
| D6 | 1280 宽度下"全部/仅自己"切换的"仅自己"被挤成两行 | `key-requests-1280.png` **[查看]** |
| D7 | 连接详情标题在 PC 上显示焦点框（程序化聚焦后可见轮廓），且 PC 并排布局里仍显示"返回连接列表" | `connections-1280.png` **[查看]** |

### 2.4 手机操作区

| # | 问题 | 证据 |
| --- | --- | --- |
| M1 | 底部导航 + 分页栏两条固定栏共占约 140px；调用记录页的"重置/搜索/查看"按钮被分页栏压住 | `key-requests-390.png`、`models-390.png` **[查看]** |
| M2 | 手机顶栏把构建号 `rv.0000.local.2…` 放在品牌下方并截断，占用顶栏，又没有返回按钮和页面标题 | `overview-390.png`、`connections-390.png` **[查看]** |
| M3 | 俄语 320 宽：底部导航"Аккаунты провайдеров"在词内断成"провайдер/ов"三行；顶栏搜索按钮压到品牌区 | `overview-320-ru.png` **[查看]** |
| M4 | 设置页手机端没有分区导航和返回，"保存更改"在顶部，滚动后不可见 | `settings-390.png` **[查看]** |

### 2.5 状态反馈

| # | 问题 | 证据 |
| --- | --- | --- |
| S1 | 错误状态只是纯文本加"重试"按钮（"无法加载账号窗口"），没有使用 Alert | `connections-1280.png` **[查看]** |
| S2 | 表格行用整行红/黄底表示错误与待核对，颜色是唯一编码之一 | `key-requests-1280.png` **[查看]** |
| S3 | 分析页统计为 0 时仍画空坐标轴，空态与"真的为 0"无法区分 | `analytics-1280.png` **[查看]** |

### 2.6 视觉系统与可访问性

| # | 问题 | 证据 |
| --- | --- | --- |
| V1 | 两套调色板：`theme.css` 定义 `--primary: oklch(0.52 0.2 260)`（#105FD9），`personal-console.css` 在无预设时用硬编码 hex 覆盖为 `#4f6bd8` 等 22 个值。截图里实际生效的是后者 | `personal-console.css:2-31` **[代码]**；截图主按钮颜色 **[查看]** |
| V2 | `theme.css` 的部分状态组合不达 AA：亮色 success 文字/白底 3.67:1、warning 文字/白底 2.93:1、info 白字 3.85:1、success 白字 3.51:1；暗色 destructive 白字 2.77:1；输入框边框 1.58:1、焦点环 2.58:1（非文本要求 3:1） | 对比度脚本 **[运行]**，见 4.6 |
| V3 | 组件内手写颜色：原始调色板类 477 处（66 个文件）、`dark:` 颜色覆盖 161 处、`space-x/y` 774 处、手写 `z-50`/`z-[n]` 28 处，均违反 shadcn styling 规则 | `grep`（排除 `._*` 与测试目录）**[运行]** |
| V4 | 页面 `<html lang>` 恒为 `en`，界面是中文或俄文时也不变；源码中没有任何设置 `documentElement.lang` 的代码，`index.html` 写死 `lang="en"`。影响读屏发音和中日文字形选择 | CDP 与内置浏览器读取 **[运行]**；`grep` **[运行]** |
| V5 | 正文字体 Public Sans 只带 latin、latin-ext、vietnamese 子集，没有西里尔与中日文，回落到通用 `sans-serif`；没有按语言给出中日文字体栈 | `node_modules/@fontsource-variable/public-sans/files` **[运行]**；`theme.css --font-sans` **[代码]** |
| V6 | 图标库不一致：`components.json` 与 `web/AGENTS.md` 规定 Hugeicons，实际 279 个文件用 `lucide-react`，32 个用 Hugeicons | `grep` **[运行]** |
| V7 | 9 个主题预设中有一个以第三方品牌命名（`anthropic`），且预设会整体替换品牌色 | `theme-presets.css` **[代码]** |
| V8 | 自用精简构建（Lite/Desktop 使用）只加载 en 与 zh 两种语言，不满足七语言要求 | `i18n/config.minimal.ts` `supportedLngs: ['en', 'zhCN']` **[代码]**；`rsbuild.config.ts` 以别名替换 **[代码]**（架构评估 3.6 已提到别名，本条为语言数影响） |

---

## 3. 设计原则（每条可检验）

| # | 原则 | 检验方法 |
| --- | --- | --- |
| P1 | **首屏见数据。** 任何列表页在 1280×800 与 390×844 下，首屏必须出现主对象的第一条数据或明确的空态；每页说明性横幅最多 1 块，高度 ≤72px（PC）。 | 浏览器脚本测量首行 `getBoundingClientRect().top < innerHeight`；横幅计数 |
| P2 | **一个对象一个名字。** 导航、面包屑、`h1`、按钮、表头使用第 5.4 节术语表中的同一名称；术语表的"禁用同义词"在 `zh.json`/`en.json` 的值中出现 0 次。 | 对 locale 值做 `grep` |
| P3 | **排错不超过两跳。** 任何显示 Request ID 或 Key 的位置，到该请求的"诊断 + 正文"不超过 2 次点击；全局搜索粘贴 Request ID 回车直接打开。 | 第 1.2 节任务 1、2 的浏览器旅程 |
| P4 | **只用语义 token。** 组件与 feature 代码中原始调色板类、hex、`oklch()`、`dark:` 颜色覆盖新增 0 处；状态必须是"Badge/Alert 变体 + 图标 + 文字"。 | `grep` 规则写入 lint 或 CI 检查 |
| P5 | **两套组合，不靠缩放。** `<1024px` 用移动组合；移动端同时固定的栏 ≤2（顶栏 + 底部导航，或顶栏 + 表单操作栏）；任何可交互元素不被固定栏遮挡；触控目标 ≥44×44px。 | 320/390/768 浏览器旅程：`elementFromPoint` 检查按钮中心可点；尺寸断言 |
| P6 | **状态诚实。** 未知、缺失、未启用、无权限、过期、处理中分别有固定表现（第 6.3 节）；数值未知不显示为 0，0 只表示有证据的零。 | 组件测试覆盖状态矩阵 |
| P7 | **七语言不破版。** 320 宽度下 ru/fr/vi/ja：无页面横向滚动；导航与按钮标签不在词内断行、不截断；`<html lang>` 与界面语言一致。 | 七语言 × 320/390 截图 + `scrollWidth` 断言 + `lang` 断言 |

---

## 4. 视觉系统

### 4.1 从 Logo 推导

Logo 实测（脚本解码 PNG 统计像素 **[运行]**）：1254×1254，主色深蓝 `#0035FC`、青 `#00CFFC`、节点填充近白 `#FAFAFA`。形状是两条圆头粗线交织成 "M"，三个圆形节点表示路由经过的点。

推导规则：

- **深蓝 = 操作**：主按钮、链接、选中态、焦点环。白字对比 7.20:1。
- **青 = 遥测**：只用于"正在流动的数据"——额度进度条、实时 RPM/TPM、图表第一辅助色、流式内容标记。青色在白底只有 1.85:1，**禁止做文字色**；青色面上的文字用深墨 `#0B1530`（9.73:1）。
- **圆头线条 → 圆角**：控件圆角 8px，卡片 12px，徽标全圆；不用直角，也不用夸张的大圆角。
- **节点 → 状态点**：状态徽标前的 6px 圆点沿用 Logo 节点形状。
- 品牌标记只出现在侧栏顶部（32px）与登录页（48px）；手机顶栏用 24px 标记 + 页面标题，不再显示构建号（构建号移到"系统 > 运行状态"和头像菜单底部）。

### 4.2 语义 token（亮/暗）

沿用 `web/src/styles/theme.css` 现有 shadcn 命名与 `@theme inline` 映射，只改值、补少量新 token。规则：组件里只出现 token 对应的工具类（`bg-primary`、`text-muted-foreground`、`bg-success-muted` 等），不出现原始色值（shadcn styling 规则"Semantic colors / No raw color values"）。

| token | 亮色 | 暗色 | 处理 |
| --- | --- | --- | --- |
| `--background` / `--foreground` | `#F6F8FC` / `#0E1630` | `#0A0F1E` / `#E8EDF7` | 保留名称，改值 |
| `--card` / `--card-foreground` | `#FFFFFF` / `#0E1630` | `#121A2C` / `#E8EDF7` | 保留 |
| `--popover` / `--popover-foreground` | `#FFFFFF` / `#0E1630` | `#162036` / `#E8EDF7` | 保留 |
| `--primary` / `--primary-foreground` | `#0035FC` / `#FFFFFF` | `#7B95FF` / `#070B16` | 保留，改为 Logo 蓝 |
| `--secondary` / `--secondary-foreground` | `#EEF2F9` / `#24304D` | `#1A2338` / `#C9D2E6` | 保留 |
| `--muted` / `--muted-foreground` | `#EEF2F9` / `#4D5975` | `#162036` / `#9BA7C0` | 保留 |
| `--accent` / `--accent-foreground` | `#E6ECFF` / `#0029C4` | `#1C2752` / `#C9D5FF` | 保留（悬停、选中行） |
| `--destructive` / `--destructive-foreground` | `#C8241B` / `#FFFFFF` | `#FF8A80` / `#1A0503` | 保留，修正暗色对比 |
| `--success` / `--warning` / `--info` / `--neutral`（含 `-foreground`） | `#067647` / `#8F5200` / `#00628F` / `#5B6782` | `#4ADE9B` / `#F2B544` / `#5CC8FF` / `#9BA7C0` | 保留名称；亮色改为可作文字色的深色值 |
| `--success-muted` `--warning-muted` `--info-muted` `--destructive-muted` | `#E6F6EE` `#FFF4DC` `#E3F7FF` `#FDECEA` | 对应色 15% 混入 `--card`（实施时以计算值写死在 token，并复算对比） | **新增**，供 Badge/Alert 柔和变体，替代组件内 `bg-green-100` 一类写法 |
| `--telemetry` / `--telemetry-foreground` | `#00CFFC` / `#0B1530` | `#22D3FF` / `#04121F` | **新增**，Logo 青 |
| `--border` / `--input` / `--ring` | `#DCE2EE` / `#8592AD` / `#0035FC` | `#26314A` / `#5F6C8C` / `#8EA5FF` | 保留；`--input` 提到 3:1 |
| `--chart-1..5` | 见 4.7 | 见 4.7 | 保留名称，改值与顺序 |
| `--sidebar-*` | `--sidebar #EEF2F9`，前景 `#24304D`，选中底 `#DCE5FF`，选中字 `#0029C4` | `--sidebar #070B16`，前景 `#C9D2E6`，选中底 `#1C2752`，选中字 `#C9D5FF` | 保留 |
| `--table-*`、`--skeleton-*` | 由 `--foreground` 与 `--background` 混色 | 同 | 保留 |
| `--myapi-brand*`、`--myapi-surface-*`、`--myapi-grid` | 改为引用 `--primary`、`--background`、`--muted`、`--border` | 同 | 保留别名，避免改动大量引用；新代码不再使用 |
| `--overview-accent-1..3` | — | — | **删除**（与 `--chart-*` 重复） |
| `personal-console.css` 第 2–31 行 22 个硬编码 hex 覆盖 | — | — | **删除**，这是两套调色板的来源（V1） |

实施格式：值写成 `oklch()`（shadcn customization 约定），上表 hex 为校验用的等价值；落地时把 hex 转 oklch 后再复算一次对比度。

### 4.3 字体与字号

- 字体栈（按 `:lang()` 选择，不下载额外 CJK 字体，控制包体积）：
  - 拉丁、越南文：`'Public Sans Variable'`（已打包）→ 系统 UI 字体。
  - 简体中文 `:lang(zh-CN)`：`'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei UI', 'Noto Sans SC', 'Source Han Sans SC'`。
  - 繁体中文 `:lang(zh-TW)`：`'PingFang TC', 'Microsoft JhengHei UI', 'Noto Sans TC'`。
  - 日文 `:lang(ja)`：`'Hiragino Sans', 'Yu Gothic UI', 'Meiryo UI', 'Noto Sans JP'`。
  - 俄文 `:lang(ru)`：`system-ui, 'Segoe UI', 'Roboto', 'Noto Sans'`（Public Sans 无西里尔，V5）。
  - 等宽（ID、Key、JSON、SSE）：`ui-monospace, 'SF Mono', 'Cascadia Mono', Menlo, Consolas, monospace`。
  - 前提：实施批次 0 必须让 `<html lang>` 随 i18n 切换（V4），否则 `:lang()` 不生效。
- 字号阶梯（rem 基于 16px；行高中日文正文取 1.6，拉丁/西里尔取 1.5）：

| 用途 | 字号 / 行高 | 字重 |
| --- | --- | --- |
| 辅助说明、表头 | 12px / 16px | 500 |
| 表格正文、次要文字 | 13px / 20px | 400 |
| 正文、表单 | 14px / 22px | 400 |
| 正文（手机） | 15px / 24px | 400 |
| 卡片标题、检视器标题 | 16px / 24px | 600 |
| 区块标题 | 18px / 26px | 600 |
| 页面标题（手机） | 20px / 28px | 600 |
| 页面标题（PC） | 24px / 32px | 600 |
| 关键数值 | 28px / 36px，`tabular-nums` | 600 |

- 不使用 30px 以上的展示字号；概览的大数字改为 28px 并配单位与对比（解决 H3）。
- 现有 `theme-presets.css` 的字体轴（sans/serif）与字号轴（sm/lg/xl）保留为用户偏好，见决策点 3。

### 4.4 间距、圆角、层级

- 间距：4px 栅格。常用 4/8/12/16/24/32/48。页面边距：手机 16、平板 16、PC 24、≥1440 为 32。区块间距 24，卡片内边距 16（手机）/20（PC）。按 shadcn 规则用 `gap-*`，不用 `space-x/y-*`。
- 圆角：`--radius: 0.5rem`（8px）→ 控件 8、卡片 12（`rounded-xl`）、对话框与底部抽屉顶部 16、徽标全圆。
- 层级（不手写 z-index，交给 overlay 组件）：

| 层 | 用途 | 表现 |
| --- | --- | --- |
| 0 | 页面背景 | `--background` |
| 1 | 卡片、表格容器、侧栏 | `--card` + 1px `--border`，**无阴影**（去掉 `product-ui.css` 的卡片阴影） |
| 2 | Popover、Dropdown、Tooltip、检视器（非模态） | `--popover` + 阴影 `0 8px 24px -12px` 墨色 20% |
| 3 | Dialog、Sheet、Drawer（模态） | 遮罩墨色 40%（暗色 60%）+ 阴影 `0 24px 48px -24px` 墨色 30% |

### 4.5 动效

- 时长：120ms（悬停、按下）、200ms（展开、页签切换、检视器出现）、280ms（Sheet/Drawer 进出）。缓动 `cubic-bezier(0.2, 0, 0, 1)`。
- 只做透明度与 8px 以内的位移；不做整页过渡动画（现有 `AnimatedOutlet` 页面过渡改为仅透明度 120ms）。
- `prefers-reduced-motion: reduce`：关闭位移与缩放，透明度过渡降为 0ms，骨架屏不闪烁（`index.css` 已有若干 reduced-motion 块，统一到一处）。

### 4.6 对比度检查结果

方法：脚本按 WCAG 2.1 相对亮度公式计算，oklch 先转线性 sRGB（超色域分量截断）。正文要求 ≥4.5:1，非文本（边框、焦点环、图形）≥3:1。**[运行]**

**提议的亮色**

| 组合 | 前景 | 背景 | 对比 | 结果 |
| --- | --- | --- | --- | --- |
| foreground / background | #0E1630 | #F6F8FC | 16.79:1 | 通过 |
| foreground / card | #0E1630 | #FFFFFF | 17.86:1 | 通过 |
| muted-foreground / background | #4D5975 | #F6F8FC | 6.58:1 | 通过 |
| muted-foreground / muted | #4D5975 | #EEF2F9 | 6.23:1 | 通过 |
| primary-foreground / primary | #FFFFFF | #0035FC | 7.20:1 | 通过 |
| primary-foreground / primary 悬停 | #FFFFFF | #002BD0 | 9.31:1 | 通过 |
| primary 链接 / background | #0035FC | #F6F8FC | 6.78:1 | 通过 |
| accent-foreground / accent | #0029C4 | #E6ECFF | 8.42:1 | 通过 |
| telemetry-foreground / telemetry | #0B1530 | #00CFFC | 9.73:1 | 通过 |
| info / card | #00628F | #FFFFFF | 6.68:1 | 通过 |
| info / info-muted | #00628F | #E3F7FF | 6.05:1 | 通过 |
| success / card | #067647 | #FFFFFF | 5.69:1 | 通过 |
| success / success-muted | #067647 | #E6F6EE | 5.09:1 | 通过 |
| warning / card | #8F5200 | #FFFFFF | 6.22:1 | 通过 |
| warning / warning-muted | #8F5200 | #FFF4DC | 5.70:1 | 通过 |
| destructive / card；destructive-foreground / destructive | #C8241B ↔ #FFFFFF | — | 5.64:1 | 通过 |
| destructive / destructive-muted | #B01F17 | #FDECEA | 6.01:1 | 通过 |
| sidebar-foreground / sidebar | #24304D | #EEF2F9 | 11.65:1 | 通过 |
| sidebar 选中字 / 选中底 | #0029C4 | #DCE5FF | 7.90:1 | 通过 |
| input 边框 / card（非文本） | #8592AD | #FFFFFF | 3.13:1 | 通过（≥3） |
| ring / card（非文本） | #0035FC | #FFFFFF | 7.20:1 | 通过（≥3） |
| Logo 青 作文字 / 白 | #00CFFC | #FFFFFF | 1.85:1 | 不通过 → 规定青色不作文字 |

**提议的暗色**

| 组合 | 前景 | 背景 | 对比 | 结果 |
| --- | --- | --- | --- | --- |
| foreground / background | #E8EDF7 | #0A0F1E | 16.26:1 | 通过 |
| foreground / card | #E8EDF7 | #121A2C | 14.79:1 | 通过 |
| muted-foreground / card | #9BA7C0 | #121A2C | 7.18:1 | 通过 |
| muted-foreground / background | #9BA7C0 | #0A0F1E | 7.89:1 | 通过 |
| primary-foreground / primary | #070B16 | #7B95FF | 7.10:1 | 通过 |
| primary 链接 / card | #8EA5FF | #121A2C | 7.42:1 | 通过 |
| accent-foreground / accent | #C9D5FF | #1C2752 | 9.88:1 | 通过 |
| telemetry-foreground / telemetry | #04121F | #22D3FF | 10.64:1 | 通过 |
| info / card | #5CC8FF | #121A2C | 9.22:1 | 通过 |
| success / card | #4ADE9B | #121A2C | 10.09:1 | 通过 |
| warning / card | #F2B544 | #121A2C | 9.48:1 | 通过 |
| destructive / card | #FF8A80 | #121A2C | 7.60:1 | 通过 |
| destructive-foreground / destructive | #1A0503 | #FF8A80 | 8.62:1 | 通过 |
| sidebar-foreground / sidebar | #C9D2E6 | #070B16 | 12.96:1 | 通过 |
| input 边框 / card（非文本） | #5F6C8C | #121A2C | 3.31:1 | 通过（≥3） |
| ring / card（非文本） | #8EA5FF | #121A2C | 7.42:1 | 通过（≥3） |

**现有 token 对照（说明为什么要改）**：`theme.css` 亮色 success 文字/白 3.67:1、warning 文字/白 2.93:1、success 白字 3.51:1、info 白字 3.85:1、input 边框 1.58:1、ring 2.58:1；暗色 destructive 白字 2.77:1、sidebar-primary 白字 4.11:1，均不通过。`personal-console.css` 的硬编码组合均通过（例如 primary 白字 4.74:1），但它绕开了 token 体系。

### 4.7 图表与统计块（dataviz 技能）

- 分类色固定顺序，不循环：亮色 `#0035FC`（蓝）→ `#B84A0C`（橙）→ `#0089AD`（青）→ `#7442D6`（紫）→ `#A8326E`（洋红）；暗色 `#5F7DFF` → `#C9661E` → `#0099C2` → `#9B7BF0` → `#D9558F`。
- 校验：`validate_palette.js` 亮色（白底）全部 PASS；暗色（`#121A2C` 底）全部 PASS，其中第 3、4 色在绿色弱视下 ΔE 6.7（WARN，6–8 区间），按技能规则这两色同时出现时必须有直接标签或图例旁数值。**[运行]**
- 状态色（success/warning/destructive）保留给状态，不作为第 N 个系列色。
- 单一系列不画图例框；≥2 个系列必有图例；不使用双 Y 轴；数值文字用 `--foreground`/`--muted-foreground`，不用系列色。
- 零数据不画空坐标轴，改用 `Empty`（解决 H5/S3）；"有证据的 0"显示 0 并注明覆盖范围。
- 统计块：标签（12px muted）+ 数值（28px tabular）+ 一行对比或覆盖说明；同一行最多 4 个（PC）/ 2 个（手机）。

---

## 5. 信息架构与导航

### 5.1 PC（≥1024px）

```text
┌───────────────┬──────────────────────────────────────────────────────────────┐
│ [M] MyAPI     │ 上游 / 连接                [⌘K 搜索 Request ID、Key、连接]  🔔 ◐ 头像 │
│               ├──────────────────────────────────────────────────────────────┤
│ 概览          │ 页面标题                                     主操作  次操作 ⋯ │
│ 分析          │ 筛选条（一行）                                                │
│ 上游          ├────────────────────────────────┬─────────────────────────────┤
│   连接        │ 列表 / 表格                    │ 检视器（选中行的详情）       │
│   账号窗口    │                                │ 页签：概要 诊断 正文 …       │
│   模型        │                                │ 操作区                       │
│ 访问          │                                │                             │
│   API Keys    │                                │                             │
│   用户        │                                │                             │
│ 记录          │                                │                             │
│   调用记录    │                                │                             │
│   正文与 SSE  │                                │                             │
│   任务        │                                │                             │
│ 工具 ▸        │                                │                             │
│ 系统 ▸        │                                │                             │
│ 商业 ▸ (Full) │                                │                             │
│ ⟨ 收起        │ 分页（表格容器内）                                            │
└───────────────┴────────────────────────────────┴─────────────────────────────┘
```

- **侧栏分组**（固定顺序）：概览、分析（单项）；上游；访问；记录；工具；系统；商业（仅 Full）。"工具、系统、商业"默认折叠。宽度 240px；1024–1279 默认收起为 64px 图标栏（悬停出 Tooltip 名称）；≥1280 默认展开；收起状态沿用 `sidebar_state` cookie。
- **顶栏职责**：左侧面包屑（分组 / 页面 / 对象）；中间全局搜索（`Ctrl/⌘ K`，命令面板用现有 `command.tsx`，支持 Request ID、Key 名或 ID、连接名、页面名、设置分区名）；右侧通知、主题切换、头像菜单（个人资料、语言、外观偏好、关于、构建号、退出）。个人资料移出侧栏（解决 N6）。
- **面包屑**：只有三级——分组、页面、当前对象（例如"记录 / 调用记录 / req_8f2…"）。分组不可点击，页面可点击并保留原筛选。
- **返回规则**：检视器关闭回到列表并恢复滚动位置与焦点（沿用已有"返回保留筛选、页码和焦点"的行为）；从别的页面深链进入的对象详情，面包屑页面项即返回；浏览器后退始终可用，检视器打开状态写入 URL 查询参数（`?inspect=<id>`）。
- **设置**：系统设置保留现有"钻入式侧栏"（`SYSTEM_SETTINGS_VIEW`），但设置首页改为分区目录（7 组卡片 + 搜索），不再默认落在"站点信息"表单。

### 5.2 手机与窄平板（<1024px）

```text
┌──────────────────────────┐
│ ←  调用记录        🔍  ⋯ │  顶栏：返回（非一级页）/ 标题 / 搜索 / 页面菜单
├──────────────────────────┤
│ [搜索框        ] [筛选 2]│  一行；筛选进底部抽屉
│ 今天 · 全部 Key（可清除）│  当前条件以徽标显示
│ ┌──────────────────────┐ │
│ │ 失败 · gpt-x · 2.1s  │ │  卡片列表（同一列定义生成）
│ │ Key #21 · 04:06      │ │
│ └──────────────────────┘ │
│ …                        │
│ [ 加载更多 ]             │  列表末尾；不固定分页栏
├──────────────────────────┤
│ 概览  上游  Keys  记录 更多│  底部导航 5 项
└──────────────────────────┘
```

- **底部导航 ≤5 项**：管理员为"概览 / 上游 / Keys / 记录 / 更多"；无上游权限的用户为"概览 / Keys / 记录 / 更多"。标签用独立的短文案 key（如 ru "Аккаунты"），两行以内、不在词内断行（解决 M3）。
- **"更多"规则**：不重复底部已有 4 项；顶部带搜索框；按侧栏分组列出其余入口（分析、模型、用户、正文与 SSE、任务、工具、系统、商业、个人资料）；版本隐藏的入口不出现。
- **页头**：一级页面显示品牌标记 24px + 页面标题；二级及以下显示返回箭头 + 标题；页面主操作放在页头右侧图标按钮或页面底部操作栏，不放构建号（解决 M2）。
- **操作区**：列表页无固定操作栏；表单页隐藏底部导航，改为固定在底部的"取消 / 保存"操作栏（含安全区 `env(safe-area-inset-bottom)`）；任何时刻固定栏 ≤2 条（解决 M1）。
- **抽屉/底部弹层**：筛选、排序、单选列表用底部 `Drawer`（shadcn composition 规则"Mobile-first bottom panel"）；详情与编辑用全屏路由页或全屏 `Sheet`，带标题与返回；破坏性确认用 `AlertDialog`。
- **同一数据观察者**：PC 与手机组合由同一查询驱动，只切换呈现组件；切换宽度不重新请求（`MYAPI_PLATFORM_REBUILD_PLAN` 第 5 节要求）。用 `useMediaQuery(CONSOLE_MOBILE_QUERY)` 一处判断，断点常量统一为 1024（现有 `console-layout.ts` 已是 `max-width: 1023px`）。

### 5.3 按版本显隐

依据：架构评估 5.3 节 C1–C12 与 4.1–4.2 节；Lite/Desktop 隐藏的是入口，不删数据，后端门禁由任务 4 负责。"—" 表示隐藏。

| 导航项 | 路由 | Full | Lite | Desktop | 依据 |
| --- | --- | --- | --- | --- | --- |
| 概览 | `/dashboard/overview` | ✓ | ✓ | ✓（加"本机接入"卡） | 价值链 5 |
| 分析 | `/dashboard/models`、`/flow`、`/users` | ✓ | ✓ | ✓ | `users` 段仍需 Admin |
| 上游 · 连接 | `/connections`、`/channels` | ✓ | ✓ | ✓ | 价值链 2 |
| 上游 · 账号窗口 | `/upstream-accounts` | ✓ | ✓ | ✓ | |
| 上游 · 模型目录 | `/models/metadata` | ✓ | ✓ | ✓ | |
| 上游 · 模型部署（io.net） | `/models/deployments` | ✓ | — | — | C9 |
| 访问 · API Keys | `/keys` | ✓ | ✓ | ✓ | 价值链 1 |
| 访问 · 用户 | `/users` | ✓ | ✓ | ✓（默认折叠在"更多"） | 架构评估 2.2：Lite 保留 |
| 记录 · 调用记录 | `/usage-logs/common` | ✓ | ✓ | ✓ | |
| 记录 · 正文与 SSE | `/full-content-logs` | ✓ | ✓ | ✓ | |
| 记录 · 任务 | `/usage-logs/task` | ✓ | ✓ | ✓ | |
| 记录 · 绘图（Midjourney） | `/usage-logs/drawing` | ✓ | — | — | C8 |
| 工具 · Playground | `/playground` | ✓ | ✓ | ✓ | 架构评估 2.2 |
| 工具 · 提示词学习 | `/prompt-learning` | ✓ | ✓ | ✓ | 能力矩阵；新增导航入口（N5） |
| 工具 · 聊天 / 聊天跳转 | `/chat/$chatId`、`/chat2link` | ✓ | — | — | 自用构建已移除 `/chat` |
| 系统 · 运行状态 | `/system-info` | ✓ | ✓ | ✓ | Root |
| 系统 · 设置 | `/system-settings/*` | ✓（42 分区） | ✓（隐藏 payment、checkin、model-deployment、content 全组、email、worker、update-checker、oauth、custom-oauth） | 同 Lite | C1、C4、C7、C9、C10、C11 |
| 商业 · 钱包 | `/wallet` | ✓（按资金模式） | —（存量资金历史只读入口放到个人资料） | 同 Lite | C1 |
| 商业 · 订阅计划 | `/subscriptions` | ✓ | —（"我的订阅"只读待任务 4 决定） | 同 Lite | C2 |
| 商业 · 兑换码 | `/redemption-codes` | ✓ | — | — | C3 |
| 头像菜单 · 个人资料 | `/profile` | ✓ | ✓ | ✓ | |
| 公开首页 | `/` | ✓ | 未登录直达登录；已登录到概览 | 同 Lite | 架构评估 2.2 home |
| 公开定价 / 排行 | `/pricing`、`/pricing/$modelId`、`/rankings` | ✓（按配置） | — | — | C6 |
| 关于 / 隐私 / 用户协议 | `/about`、`/privacy-policy`、`/user-agreement` | ✓ | 关于 ✓；协议 —（不开放注册） | 同 Lite | |
| 注册 / 外部登录 | `/sign-up`、`/register`、`/oauth`、`/oauth/$provider` | ✓（按配置） | 注册 —；外部 OAuth — | 同 Lite | C7 |

注意：现有设置配置中 `oauth`、`custom-oauth` 分区没有 `selfUseHidden`，而后端 LAN 门禁已封 `/api/oauth`，Lite 下会出现能看到、不能用的分区 **[代码]**。本方案把它们列入 Lite 隐藏，需任务 4 一并处理。

**默认首页**：三个版本都落在"概览"。理由：概览是唯一能同时回答"上游是否可用、有没有失败、下一步做什么"的页面（第 3 节 P1），且三个版本共用同一套业务（TASKS.md 约束 Desktop 不维护第二套业务）。差异只在概览内容：Full 多一个"用户与资金摘要"区块（有权限时）；Lite/Desktop 不显示金额；Desktop 首屏第一块是"本机接入"卡（Base URL `http://127.0.0.1:<port>/v1`、访问模式本机/LAN、复制按钮），因为桌面用户的第一件事是把地址填进客户端。首次安装时仍先进 `/setup`。

### 5.4 术语表（解决 N1–N3）

| 统一名称（zh / en） | 禁用同义词 | 说明 |
| --- | --- | --- |
| 连接 / Connection | 渠道、频道（用户可见文案中） | 后端与 URL 仍是 channel，不改 |
| 账号窗口 / Account windows | 上游账户窗口、订阅窗口观测 | 指上游订阅账号的额度窗口 |
| API Key / API Key | 令牌、API 密钥、密钥（指 Key 时） | 中文界面保留英文 "API Key"，简称 "Key" |
| 调用记录 / Requests | 通用日志、调用日志 | 面包屑与标题一致 |
| 正文与 SSE / Bodies & SSE | 请求正文与 SSE、在线请求浏览器 | |
| 模型目录 / Model catalog | 元信息 | |
| 运行状态 / System status | 系统信息（指 `/system-info` 时） | 设置分区"系统信息"改名"站点信息 / Site info" |
| 上一页、下一页 | 上一步、下一步（分页场景） | |
| 订阅计划（商业） / 订阅分额（Key） | 混用"订阅" | 架构评估 B9 |

### 5.5 feature 与路由到新 IA 的映射

**28 个 feature**（`web/src/features` 实际为 28 个目录，与任务说明中的 25 不同，以目录为准 **[运行]**）：

| feature | 新位置 | 处理 |
| --- | --- | --- |
| about | 公开 `/about`；头像菜单"关于" | 保留 |
| access-policy | 访问 › API Keys 检视器"访问"页签 | 合并入口（组件保留） |
| auth | 登录流程（身份批次） | 保留；外部 OAuth 按版本 |
| channels | 上游 › 连接的"表格视图/批量"模式；额度消耗、额度事件、路由预览移入连接检视器页签与"分析" | 合并（并行实现收敛，架构评估 B2） |
| chat | 工具 › 聊天（Full） | 保留，按版本 |
| connections | 上游 › 连接（主列表 + 检视器），新增"添加""测试"入口 | 保留并补齐 |
| dashboard | 概览；分析（models/flow/users 三个页签） | 保留，概览重做；旧版总览与个人工作台收敛为一套 |
| errors | 错误页 | 保留，统一状态样式 |
| full-content-logs | 记录 › 正文与 SSE；同时作为请求详情"正文/SSE"页签 | 保留 |
| home | 公开首页（Full）；Lite/Desktop 直达登录/概览 | 保留，按版本 |
| keys | 访问 › API Keys（表格 + 检视器；手机列表 + 详情） | 保留，卡片列表改表格；个人 Key 列表与表格收敛 |
| legal | 公开（Full 开放注册时） | 保留，按版本 |
| models | 上游 › 模型目录；部署进"高级"（Full） | 保留 |
| performance-metrics | 公开定价/排行内（Full） | 保留，按版本 |
| personal-upstream | 上游 › 账号窗口 | 合并（与 connections 共用检视器） |
| playground | 工具 › Playground | 保留 |
| pricing | 公开（Full） | 保留，按版本 |
| profile | 头像菜单 › 个人资料；Lite 资金历史只读入口放这里 | 移出侧栏 |
| prompt-learning | 工具 › 提示词学习 | 新增导航入口 |
| rankings | 公开（Full） | 保留，按版本 |
| redemption-codes | 商业 › 兑换码（Full） | 保留，按版本 |
| setup | 安装向导 | 保留（身份批次） |
| subscriptions | 商业 › 订阅计划（Full） | 保留，按版本 |
| system-info | 系统 › 运行状态 | 改名 |
| system-settings | 系统 › 设置（42 分区全部保留，Lite 隐藏见 5.3） | 保留，首页改分区目录 |
| usage-logs | 记录 › 调用记录 / 任务 / 绘图（Full）；个人调用记录与通用日志收敛 | 保留 |
| users | 访问 › 用户 | 保留 |
| wallet | 商业 › 钱包（Full） | 保留，按版本 |

**63 个路由文件**（`web/src/routes`，逐一列出，无删除）：

| 路由文件 | 新位置 / 处理 |
| --- | --- |
| `__root.tsx` | 根布局：负责设置 `<html lang>`、主题 class |
| `_authenticated/route.tsx` | 控制台外壳（PC/手机两套组合） |
| `_authenticated/dashboard/index.tsx`、`dashboard/$section.tsx`（overview、models、flow、users） | 概览；分析三页签 |
| `_authenticated/upstream-accounts/index.tsx` | 上游 › 账号窗口 |
| `_authenticated/connections/index.tsx` | 上游 › 连接 |
| `_authenticated/channels/index.tsx` | 上游 › 连接（表格/批量视图），URL 保留 |
| `_authenticated/models/index.tsx`、`models/$section.tsx`（metadata、deployments） | 上游 › 模型目录；部署（Full） |
| `_authenticated/keys/index.tsx` | 访问 › API Keys |
| `_authenticated/users/index.tsx` | 访问 › 用户 |
| `_authenticated/usage-logs/index.tsx`、`usage-logs/$section.tsx`（common、task、drawing） | 记录 › 调用记录、任务、绘图（Full） |
| `_authenticated/full-content-logs/index.tsx` | 记录 › 正文与 SSE |
| `_authenticated/playground/index.tsx` | 工具 › Playground |
| `_authenticated/prompt-learning/index.tsx` | 工具 › 提示词学习 |
| `_authenticated/chat/$chatId.tsx`、`_authenticated/chat2link.tsx` | 工具 › 聊天（Full） |
| `_authenticated/system-info/index.tsx` | 系统 › 运行状态 |
| `_authenticated/system-settings/route.tsx`、`index.tsx` | 系统 › 设置（首页改分区目录） |
| `system-settings/site/index.tsx`、`site/$section.tsx` | 设置 › 站点与品牌（4 分区） |
| `system-settings/auth/index.tsx`、`auth/$section.tsx` | 设置 › 身份验证（5 分区） |
| `system-settings/billing/index.tsx`、`billing/$section.tsx` | 设置 › 计费与支付（7 分区） |
| `system-settings/models/index.tsx`、`models/$section.tsx` | 设置 › 模型与路由（8 分区） |
| `system-settings/security/index.tsx`、`security/$section.tsx` | 设置 › 安全与限制（4 分区） |
| `system-settings/content/index.tsx`、`content/$section.tsx` | 设置 › 控制台内容（7 分区，Full） |
| `system-settings/operations/index.tsx`、`operations/$section.tsx` | 设置 › 运维（7 分区） |
| `_authenticated/profile/index.tsx` | 头像菜单 › 个人资料 |
| `_authenticated/wallet/index.tsx` | 商业 › 钱包（Full） |
| `_authenticated/subscriptions/index.tsx` | 商业 › 订阅计划（Full） |
| `_authenticated/redemption-codes/index.tsx` | 商业 › 兑换码（Full） |
| `_authenticated/errors/$error.tsx` | 控制台内错误页 |
| `(auth)/route.tsx`、`sign-in.tsx`、`sign-up.tsx`、`register.tsx`、`forgot-password.tsx`、`reset.tsx`、`user/reset.tsx`、`otp.tsx`、`oauth.tsx` | 登录流程（身份批次）；注册与 OAuth 按版本 |
| `oauth/$provider.tsx` | OAuth 回调（Full） |
| `(errors)/401.tsx`、`403.tsx`、`404.tsx`、`500.tsx`、`503.tsx` | 公共错误页 |
| `setup/index.tsx` | 安装向导 |
| `index.tsx` | 公开首页（Full）；Lite/Desktop 重定向 |
| `about/index.tsx`、`privacy-policy.tsx`、`user-agreement.tsx` | 公开页 |
| `pricing/index.tsx`、`pricing/$modelId/index.tsx`、`rankings/index.tsx` | 公开页（Full） |

计数核对：认证组 9、错误组 5、根 1、`_authenticated` 39、其余公开与安装 9，共 63。**[运行]**（`find src/routes -type f` 排除 `._*`）

---

## 6. 布局、响应式与组件规范

### 6.1 断点与容器

| 宽度 | 组合 | 页面边距 | 栅格 | 内容宽度 | 侧栏 / 导航 | 检视器 |
| --- | --- | --- | --- | --- | --- | --- |
| 320 | 手机 | 16 | 4 列，gap 12 | 100% | 底部导航 | 全屏页 |
| 390 | 手机 | 16 | 4 列，gap 12 | 100% | 底部导航 | 全屏页 |
| 768 | 窄平板（仍为手机组合） | 24 | 8 列，gap 16 | 100%，表单最大 640 | 底部导航 | 全屏页（≥768 可用右侧 Sheet 480px） |
| 1024 | PC | 24 | 12 列，gap 16 | 100% | 侧栏收起 64px | 覆盖式右侧面板 400px |
| 1440 | PC | 32 | 12 列，gap 24 | 最大 1440（表格可铺满） | 侧栏 240px 展开 | 并排面板 440px（≥1280 起并排） |

- 表单最大宽 720px；正文阅读区最大 880px；数据表不设上限。
- Tailwind 断点映射：`sm` 640、`md` 768、`lg` 1024、`xl` 1280、`2xl` 1440（新增 `2xl` 自定义值）。双端切换只看 `lg`。

### 6.2 表格、表单、对话框、筛选、分页

| 元素 | PC | 手机 |
| --- | --- | --- |
| 表格 | 现有 `components/data-table` 一处实现；行高 44px（可切换 36px 紧凑，仅 `pointer: fine`）；首列与操作列固定；列有优先级，1024–1279 自动隐藏低优先级列（列选择器可恢复）；表头 12px；数字 `tabular-nums` 右对齐；状态列用 Badge；整行不着色（S2），只用状态徽标 + 左侧 2px 状态条 | 用同一列定义生成卡片列表：主文本 + 状态徽标 + 时间 + 最多 2 个关键数值；点卡片进全屏详情。只有代码/JSON/SSE 区可横向滚动，且在独立容器内 |
| 表单 | `FieldGroup` + `Field`（shadcn forms 规则）；标签在上；仅短而相关的字段在 ≥1024 时两列；设置页用 `Field orientation="horizontal"`；校验错误在字段下方，`data-invalid` + `aria-invalid` 同时设置 | 单列；固定底部操作栏；输入框 16px 字号（避免 iOS 缩放） |
| 选择控件 | 统一 Base UI `Select`/`Combobox`（替换 13 个文件的原生 `<select>`，D3）；2–5 个选项用 `ToggleGroup` | 选项多时用底部 `Drawer` 列表 |
| 日期范围 | 组件化日期范围选择器，按界面语言格式化 | 底部 `Drawer` 内预设（今天/24 小时/7 天）+ 自定义 |
| 对话框 | 短任务 `Dialog`（≤560px）；编辑与详情用检视器或右侧 `Sheet`（480–640px）；破坏性操作 `AlertDialog`；都有 Title（可 `sr-only`） | 短选择用 `Drawer`；编辑为全屏页 |
| 筛选 | 一行工具条：搜索 + 3 个最常用筛选 + "更多筛选"（Popover）；已生效条件显示为可移除 Badge；统计块移出筛选卡，放到列表上方一行（解决 H1） | 搜索框 + "筛选（数量）"按钮 → `Drawer`，底部"应用 / 重置" |
| 分页 | 表格容器内底部：总数、每页条数、页码 | 列表末尾"加载更多"或页码，不固定（解决 M1）；每页条数与 PC 相同（D5） |
| 触控目标 | `pointer: fine` 下控件高 36px | 全部 ≥44×44px（沿用 `product-ui.css` 的 `pointer: coarse` 合同，并扩展到底部导航与卡片操作） |

### 6.3 组件与状态

**按钮层级**（只用 Button 内置变体，不写颜色 className；图标用 `data-icon`，不加尺寸类）：

| 层级 | 变体 | 每个视图数量 |
| --- | --- | --- |
| 主操作 | `default` | ≤1 |
| 次操作 | `outline` | ≤2，其余进 `⋯` 菜单 |
| 低频/行内 | `ghost` | 不限 |
| 破坏性 | `destructive`，必须经 `AlertDialog` 确认 | — |
| 加载中 | `disabled` + `Spinner data-icon="inline-start"`（shadcn 规则：Button 无 isLoading） | — |

**卡片**：只用于"一个对象的摘要"或"一组相关设置"；用完整组合 `CardHeader/CardTitle/CardDescription/CardContent/CardFooter`；列表不用卡片网格（D1、H4）。

**徽标**：状态一律 `Badge`，前置 6px 圆点或图标 + 文字；新增 `success`、`info`、`neutral` 变体（基于 `--*-muted` 与 `--*` token），沿用已有 `warning`、`destructive`、`outline`、`secondary`。

**提示**：页内提示用 `Alert`（补 `warning`、`info` 变体），配 `AlertTitle`/`AlertDescription`/`AlertAction`；临时反馈用 `sonner` toast；每页最多 1 个页内提示（P1）。

**状态矩阵**（每个数据区块都要覆盖）：

| 状态 | 表现 | 组件 |
| --- | --- | --- |
| 加载 | 超过 300ms 才显示骨架；骨架与最终布局同形；后台刷新不清空已有数据，只在区块标题旁显示小 Spinner | `Skeleton`、`Spinner` |
| 空 | 说明为什么空 + 下一步按钮（例如"还没有连接 → 添加连接"） | `Empty` |
| 错误 | `Alert` destructive：发生了什么、可以做什么、重试按钮；保留上一次成功数据并标注"数据可能过期" | `Alert` + `Button` |
| 无权限 | `Empty` + 锁图标 + "需要 Root 权限"一类准确原因；不显示数据骨架 | `Empty` |
| 版本未提供 | `Empty`："当前版本（Lite）不提供此功能"，无操作按钮 | `Empty` |
| 未启用 | `Empty` + "去设置"深链（例如正文采集关闭） | `Empty` |
| 处理中 | `Badge info` + Spinner + 已用时间；可取消时给取消按钮 | `Badge` |
| 未知 | `Badge neutral` "未知" + Tooltip 解释来源缺失；数值显示 "—" 而非 0（P6） | `Badge`、`Tooltip` |
| 过期 | `Badge warning` "已过期" + 观测时间 | `Badge` |
| 暂停/停用 | `Badge outline` "已暂停"；行内文字正常色，不整行置灰 | `Badge` |
| 成功 | toast，可撤销的操作带"撤销" | `sonner` |
| 冲突 | `Alert warning`："已被他人修改"，按钮"重新加载 / 查看差异" | `Alert` |

**七语言长文本策略**：

1. UI 标签不截断、不固定宽度；按钮与页签允许换行到两行，`overflow-wrap: normal`、`word-break: keep-all`（中日文）/`hyphens: auto`（fr/ru，需 `lang` 正确）。
2. 只有数据值（名称、ID、模型名、URL）可以单行截断，截断必带 Tooltip 或可展开全文；ID 与 Key 用等宽字体，中间省略（`sk-…a8f3`）。
3. 空间受限的位置（底部导航、表头、徽标）使用单独的短文案 key（如 `nav.short.upstream`），七语言各自给出短译文，并在 320 宽度验证。
4. 数字、日期、相对时间用 `Intl` 按当前语言格式化；原生日期输入不再直接暴露。
5. 验收：七语言 × 320/390/1024 截图 + 页面 `scrollWidth === innerWidth` + 底部导航每项高度 ≤2 行。

---

## 7. 两个备选设计方向

### 方向 A：工作台 + 检视器（推荐）

- 气质：运维控制台，安静、密，品牌蓝只用在操作上，青色只出现在流动数据上。
- 密度：中等偏高。表格行 44px，PC 首屏 ≥12 行数据；统计压成一行。
- 导航：PC 分组侧栏（可收起）+ 顶栏全局搜索；手机底部 5 项 + "更多"。
- 主要形态：每个对象页都是"列表 + 检视器"。选中一行，右侧检视器显示概要、诊断、正文、关联对象，不离开列表；手机上检视器变成全屏详情页。
- Key → 请求 → 正文在同一检视器里用页签串起来。
- 优点：排错路径最短（任务 1、2 目标 2 步/1 步）；与现有 `data-table`、`Sheet`、侧栏注册表复用度高；30 多个入口放得下；七语言下侧栏纵向排列不受长标签影响。
- 缺点：新用户第一眼信息多，需要概览页的"下一步"区块引导；检视器在 1024–1279 宽度只能覆盖式显示；实现上要做好检视器状态与 URL 同步。

### 方向 B：引导式任务中心

- 气质：产品化、轻松，大卡片、大留白，品牌色用作区块底色。
- 密度：低。卡片网格，首屏 4–6 个任务卡。
- 导航：PC 顶部横向 5 个分区标签（总览 / 上游 / 访问 / 记录 / 系统），二级用页内页签；手机底部导航。
- 主要形态：首页是"你下一步要做什么"的任务卡（添加连接、创建 Key、查请求）；每个对象有独立详情页，进入后整页切换；复杂操作用分步向导。
- 优点：新用户上手快；单个页面视觉简单；适合偶尔使用者。
- 缺点：排错要多次整页跳转（任务 1 估计 4 步，与现状相当）；顶部横向导航放不下七语言长标签（俄语"Аккаунты провайдеров"已在底部导航断行，M3），二级入口要藏进页签；大卡片在 Key 多于 10 个、请求上千条时不可扫视（现有 Keys 卡片已出现此问题，D1）；与"上游、访问、记录、系统"30 多个入口的规模不匹配。

**推荐 A**。理由：MyAPI 的主要使用者是同一个管理者长期、高频地查请求和维护上游（第 1 节），方向 A 在任务 1、2、4、9 上把步数降到目标值，方向 B 只改善了任务 3 的首次体验；方向 A 复用现有 `data-table`、`Sheet`、`sidebar-view-registry`、`command.tsx`，不需要重写数据层；方向 B 的顶部导航与七语言长文本冲突已在现状截图中出现。B 中"下一步引导"的优点以"概览 · 下一步"区块的形式吸收进 A。

---

## 8. 关键页面样稿计划（线框级）

三个页面用于验证方向 A：概览、Key → 请求排错、上游（连接 + 账号窗口）。样稿阶段做成可交互预览（S6-1 要求"真实可交互预览"），使用现有合成预览数据，不接生产。

### 8.1 概览

PC（主次从上到下、从左到右）：

```text
┌ 页面标题：概览                         [创建 Key] [添加连接] ┐
├ 健康条（一行 4 个统计块）：近 24h 请求 · 失败率 · P50 耗时 · 可用连接 n/m ┤
├──────────────────────────────┬─────────────────────────────┤
│ 需要处理（按对象归并）        │ 最紧张的账号窗口（前 3）     │
│ · 连接 #3 测试失败 · 2h 前 →  │ 账号A 5h 窗口 ███░ 82% 观测… │
│ · 3 个请求用量未知 →          │ 账号B 7d 窗口 ██░░ 55%       │
│ · 正文采集未开启 → 去设置     │ [查看全部账号窗口]           │
├──────────────────────────────┴─────────────────────────────┤
│ 近期请求（表格 8 行：时间 结果 Key 模型 连接 耗时）  失败 3 → │
├─────────────────────────────────────────────────────────────┤
│ 下一步（仅新实例显示，完成项不占位）：添加连接 → 创建 Key → 复制接入地址 │
└─────────────────────────────────────────────────────────────┘
Desktop：健康条上方加"本机接入"卡（Base URL、访问模式、复制）
Full + 有权限：近期请求下方加"用户与资金摘要"
```

手机：

```text
顶栏：[M] 概览                 🔍 ⋯
健康条：2×2 统计块
需要处理（最多 3 条，"全部 n"）
最紧张窗口（1 条 + "全部"）
近期请求（5 张卡片，"查看全部"）
下一步（新实例）
底部导航
```

验收点：1280×800 首屏同时可见健康条、需要处理、最紧张窗口和至少 3 条近期请求；390×844 首屏可见健康条与需要处理；无数据时每块是对应空态而不是 0。

### 8.2 Key → 请求排错

PC（`/keys?inspect=21` → 检视器 → 请求 → 详情）：

```text
┌ API Keys                                     [创建 Key] ┐
│ 搜索 · 状态 · 连接 · 更多筛选                           │
├───────────────────────────────┬────────────────────────┤
│ 表格：名称 · ID · 状态 · 模型范围 · 分额 · 最后使用    │ 检视器：Key #21  [暂停][⋯]  │
│ ▸ Synthetic … #21  ●启用 …     │ 页签：概要 | 最近请求 | 访问 | 分额 │
│   Synthetic … #22  ●启用 …     │ 最近请求：                 │
│                               │  ✕ 04:06 流内错误 2.0s →   │
│                               │  ✓ 04:05 用量未知 2.0s →   │
│                               │ [在调用记录中查看全部]     │
└───────────────────────────────┴────────────────────────┘
点一条请求 → 检视器切换为请求详情（面包屑：访问 / API Keys / #21 / req_…）：
  结果区：业务结果 · HTTP 状态（无记录显示"未记录"） · 用量可信度 · 结算状态（四项分开，P6）
  页签：诊断（路由尝试时间线）| 正文 | SSE | 原始
  返回：回到 Key 检视器的"最近请求"，焦点回到该行
```

手机：Keys 列表 → Key 详情页（页签同上）→ 请求详情页（全屏，顶栏返回）→ 正文/SSE 页签内代码区独立横向滚动、可切换折行。

验收点：任务 1 PC 2 步、手机 3 步；全局搜索粘贴 Request ID 回车直接到请求详情；返回后筛选、滚动位置、焦点恢复。

### 8.3 上游（连接 + 账号窗口）

PC（`/connections`，表格/批量视图在同页切换，`/channels` URL 保留并映射到表格视图）：

```text
┌ 连接                         [视图：列表|表格] [添加连接] ┐
│ 搜索 · 状态 · 类型 · 分组                                 │
├──────────────────────────────┬────────────────────────────┤
│ 列表（按上游账号分组）        │ 检视器：连接 #1   [测试][编辑][⋯] │
│ 账号A（订阅）                 │ 状态：已启用 · 最近测试 未测试     │
│   #1 Synthetic … ●启用 82%    │ 页签：概要 | 账号窗口 | 路由 | 请求 | 额度历史 │
│ 普通 API                      │ 账号窗口：5h 55% · 7d 68%（观测时间、采样状态） │
│   #2 …                        │ 路由：优先级 · 权重 · 分组 · 模型（可折叠长列表） │
└──────────────────────────────┴────────────────────────────┘
```

- "账号窗口"页（`/upstream-accounts`）保留，作为按账号聚合的视图：左侧账号列表，右侧窗口卡与 Key 分额；与连接检视器共用窗口组件。
- "批量操作""标签模式""ID 排序"三个开关移入表格视图的工具条菜单（D4）。
- 额度消耗图、额度事件、路由预览从列表页移到检视器页签和"分析"（H4）。

手机：连接列表（按账号分组的卡片）→ 连接详情（页签横向可滚动的 `TabsList`）→ 编辑为全屏表单，底部固定"取消 / 保存并测试"。

验收点：任务 4 三步内完成；错误窗口区分"读取失败 / 不支持 / 无可信数据 / 过期"（沿用 P2 已有语义）；打开页面不触发供应商请求。

---

## 9. 实施批次与风险

### 9.1 批次（对齐 S6-2..5 顺序）

| 批次 | 内容 | 前置依赖 |
| --- | --- | --- |
| 0 设计系统基础 | `theme.css` token 改值与新增；删除 `personal-console.css` 硬编码调色板与 `--overview-accent-*`；`<html lang>` 同步；字体栈；Badge/Alert 新变体；断点常量；新外壳（侧栏分组、顶栏搜索、手机底部导航与"更多"）；导航登记表增加版本字段 | 决策点 1–4；导航按版本显隐需任务 4 的 `/api/status` 能力字段，未就绪前用 `SELF_USE_MINIMAL` 过渡 |
| 1 身份 / 布局 / 引导 | 登录、注册、2FA、Passkey、OAuth 回调、找回、安装向导、错误页、概览 | 批次 0 |
| 2 Key / 调用 / 用户 | API Keys（表格 + 检视器 + Key 交付卡）、访问方案、调用记录、请求详情、用户 | 批次 0；Key 与个人 Key 收敛（B2） |
| 3 连接 / 额度 / 日志 / 任务 | 连接（合并 channels 入口）、账号窗口、模型目录、正文与 SSE、任务日志、绘图（Full）、分析 | 批次 2 的检视器组件 |
| 4 钱包 / 配置 / 运维 | 设置分区目录与 42 分区外壳、运行状态、钱包/订阅/兑换码（Full）、个人资料 | 任务 4 的 Lite 隐藏清单 |
| 5 提示词学习 | 版本中心页面 | 批次 0 |
| 6 安装 / 更新 / Lite / Desktop / 官网 | Desktop 本机接入卡、版本与访问模式展示、公开首页与官网 | 任务 5 |

每批：组件测试与页面集成分开；七语言用仓库 i18n 工具同步；代表宽度 320/390/768/1024/1440 截图并实际查看；`bun run typecheck`、`bun run lint`、受影响测试、`bun run build`。

### 9.2 会使 `tools/` 浏览器验收脚本失效的范围（估计）

统计对象：`tools/` 下使用 Playwright/Chromium 的 30 个脚本，合计 18,155 行。**[运行]**

| 依赖类型 | 脚本数 | 出现次数 | 失效条件 | 对策 |
| --- | --- | --- | --- | --- |
| `data-testid` / `getByTestId` | 10 | 97 | 重写组件时漏掉 testid | 实施前导出 testid 清单，新组件保留同名 testid；确需改名时同批更新脚本 |
| 外壳类与属性（`.pc-mobile-nav` 6、`.pc-sidebar` 4、`data-myapi-shell` 8、`.myapi-page-*` 6 等） | 13 | 约 40 | 批次 0 换外壳 | 外壳保留 `data-myapi-shell`、`data-myapi-header`，导航改用 `role="navigation"` + `aria-label`，脚本同批改为按角色定位 |
| `data-slot`（`card` 14、`dialog-close` 7 等） | 11 | 约 30 | 卡片改表格、对话框改检视器 | 按页面批次同步改 |
| 按可访问名称/文本（`getByRole`/`getByLabel`/`getByText`） | 多数脚本 | 1,170 | 术语表改名（5.4）、按钮文案变化 | 术语改名单独一批提交，脚本同批更新；定位优先用角色 + 稳定 key |
| 截图与布局断言（如 `page-layout-browser.mjs`、`console-layout-browser.mjs`） | 至少 2 | — | 布局整体变化 | 按新布局合同重写断言，不删除原保护的行为（例如 44px、无横向溢出） |

结论：批次 0 预计影响约 13 个脚本，批次 2、3 预计影响其余大部分 **[猜测]**（按脚本名与页面对应估计，未逐个阅读）。

### 9.3 i18n 影响

- 七种 locale 各 6,955 行键值 **[运行]**。新增：术语统一后的新键、短导航键、状态矩阵文案、Key 交付卡、概览新区块，估计每批 50–150 个键 **[猜测]**。
- 术语改名会改动大量既有值，按 `web/AGENTS.md` 3.1 用英文源串作键，改名等同新增键 + 废弃旧键，需配合架构评估 B6 的未用键复核。
- Lite/Desktop 精简构建只有 en/zh（V8），要满足七语言须改 `config.minimal.ts` 加载方式（可按需懒加载语言包以控制体积）——见决策点 5。

### 9.4 与任务 4（版本门禁）的依赖

- 导航显隐、默认首页、设置分区隐藏需要运行期版本与能力（架构评估 4.2 第 2、8 条）。批次 0 先把"版本"做成导航登记表的字段，数据源在任务 4 完成前读 `SELF_USE_MINIMAL`，完成后切到 `/api/status`。
- 路由层拦截（`beforeLoad`）由任务 4 负责；UI 只负责隐藏入口与"版本未提供"空态。
- TASKS.md 已定顺序：任务 4、5 完成后再开始任务 8 的大批量页面改造；批次 0 与第 8 节样稿可先做。

### 9.5 其他风险

- 方向 A 的检视器需要与 URL 同步，处理不当会造成返回、刷新、书签失效（平台方案第 10 节验收要求）。
- 颜色改为 Logo 深蓝后，用户已保存的主题预设会覆盖品牌色（决策点 3）。
- 删除 `personal-console.css` 的硬编码覆盖会改变所有页面的实际颜色，需全页截图回归。
- 图标库统一（决策点 4）若选 Hugeicons，涉及 279 个文件，必须随页面批次迁移，不单独大改。

---

## 10. 没做到 / 未覆盖

1. 未做可交互样稿；本文只有线框级文字与 ASCII 线框（第 8 节），样稿是任务 7。
2. 截图只覆盖 12 个页面（概览、上游账号、连接、渠道、模型、分析、正文与 SSE、API Keys、Key 请求、个人资料、系统信息、设置）和手机"更多"抽屉，语言与宽度只有中文 1280/390、俄语 320；未截 768/1024/1440、暗色、其余 5 种语言、登录与安装页、钱包等 Full 商业页、对话框与检视器打开状态。
3. 第 1.2 节"现有步数"由截图与代码推算，未逐个任务实际点击计数。
4. 对比度只算了表中列出的主要组合；`color-mix` 生成的 token（表格行、骨架屏、侧栏背景）未计算；提议值落地为 oklch 后需复算。
5. 字体栈未在 Windows/Linux 实机渲染验证；`:lang()` 方案依赖先修 `<html lang>`。
6. 未逐个阅读 30 个浏览器脚本，失效范围按选择器 `grep` 估计。
7. 未核实分析页统计为 0 的原因（H5，可能是时间范围或合成数据口径）。
8. 未评估公开首页与官网（`website/`）的设计，留到批次 6。
9. 未统计每个页面实际使用的 `components/ui` 原语，A3（10 个未引用原语）是否删除留到批次 0 决定。

---

## 11. 需要维护者确认的决策点

| # | 决策 | 推荐 | 理由 |
| --- | --- | --- | --- |
| 1 | 设计方向 | A：工作台 + 检视器 | 第 7 节 |
| 2 | 合并"上游账号 / 连接 / 渠道"三处入口为"上游"区（URL 全部保留，`/channels` 成为连接的表格视图） | 合并 | N4、H4；维护者此前反馈渠道管理体验不合格（平台方案"维护者反馈"一节） |
| 3 | 9 个主题预设（含以第三方品牌命名的 `anthropic`）如何处理 | 保留亮/暗/跟随系统、字号、圆角、内容宽度偏好；颜色预设只保留"MyAPI 默认"与"高对比"，其余停止提供但不报错（已保存的偏好回落默认） | 预设会整体替换品牌色，与"保留 Logo 识别"冲突；改动用户已有偏好，所以需确认 |
| 4 | 图标库统一 | Hugeicons（随页面批次迁移，单文件内不混用） | `components.json` 与 `web/AGENTS.md` 都规定 Hugeicons；若维护者更想少改动，备选是改文档为 lucide |
| 5 | Lite/Desktop 精简构建的语言 | 七种语言全部提供，语言包按需懒加载 | 七语言是 S6 硬要求；当前精简构建只有 en/zh（V8） |
| 6 | 术语表（5.4）是否采用，尤其"渠道 → 连接""API 密钥/令牌 → API Key" | 采用 | P2；改名会让依赖文本的浏览器脚本失效，需同批更新 |
