# 第二版样稿截图清单与自查

- 生成方式：Playwright 缓存里的 `chrome-headless-shell`（1243）经 CDP 打开 `file://` 样稿；脚本只放行 `file://`，其余请求全部拦截并计数；每张截图同时记录横向溢出、页面异常与被拦截请求数到 `browser-checks.json`。脚本在 scratchpad（`v2/shots.mjs`、`v2/jobs.mjs`），不在仓库内。**[运行]**
- 结果：43 张截图，`horizontalOverflow` 全部为 `false`，`pageErrors` 全部为空，`blocked` 全部为 0（样稿不依赖任何网络资源）。**[运行]**
- 宽度：PC 1280×800，手机 390×844（触摸模拟）；暗色通过 `?theme=dark` + `prefers-color-scheme: dark` 两条路径同时生效。
- 所有数据均为合成：名称带"示例"、域名用 `example.invalid`、Key 为 `sk-····xxxx`、Request ID 为 `req_01HX…`。

## 文件清单（文件名 = 方向-页面-宽度-主题[-动作]）

| 方向 1 切换台 | 看什么 |
| --- | --- |
| `d1-home-1280-light.png` / `d1-home-390-light.png` | 接入首页：需要处理 → 本机接入卡 → 小统计 → 最近请求 + 上游 |
| `d1-home-1280-dark.png` / `d1-home-390-dark.png` | 暗色首页 |
| `d1-keys-1280-light.png` / `d1-keys-390-light.png` | Key 行卡，第一行展开显示最近请求页签 |
| `d1-keys-request-1280-light.png` / `d1-keys-request-390-light.png` | 点一条请求后的详情对话框（手机全屏） |
| `d1-keys-request-body-1280-light.png` | 对话框切到"正文"页签 |
| `d1-upstream-1280-light.png` / `d1-upstream-390-light.png` | 上游行卡，第一行展开（窗口、路由、操作） |
| `d1-upstream-toggle-1280-light.png` | 把"示例本地模型 C"开关打开并展开第二行 |
| `d1-upstream-390-expanded.png` | 手机上收起第一行、展开第四行后滚动 |

| 方向 2 控制室 | 看什么 |
| --- | --- |
| `d2-home-1280-light.png` / `d2-home-390-light.png` | 运行板：统计带 + 上游矩阵 + 请求流 + 事件 / 后台任务 / 实例 |
| `d2-home-1280-dark.png` / `d2-home-390-dark.png` | 暗色 |
| `d2-keys-1280-light.png` | 三栏：Key 列表 / 请求表 / 请求检视 |
| `d2-keys-request-1280-light.png` | 选中第一条请求（行高亮） |
| `d2-keys-request-body-1280-light.png` | 检视栏切到"正文" |
| `d2-keys-390-light.png` / `d2-keys-390-key.png` / `d2-keys-request-390-light.png` | 手机三屏：列表 → Key 的请求（表格转两行记录）→ 请求检视 |
| `d2-upstream-1280-light.png` / `d2-upstream-detail-1280-light.png` | 矩阵 + 右侧连接检视（选中"示例 API 连接 B"） |
| `d2-upstream-390-light.png` / `d2-upstream-390-detail.png` | 手机列表与详情屏 |

| 方向 3 流向图 | 看什么 |
| --- | --- |
| `d3-home-1280-light.png` / `d3-home-390-light.png` | 流向板：Key 节点 → 网关核心卡 → 上游节点；手机为纵向管道 |
| `d3-home-1280-dark.png` / `d3-home-390-dark.png` | 暗色 |
| `d3-home-filtered-1280-light.png` | 点"示例订阅账号 A"节点后，请求表出现可清除的筛选标签 |
| `d3-keys-1280-light.png` / `d3-keys-390-light.png` | Key 表格（手机转记录） |
| `d3-keys-key-1280-light.png` / `d3-keys-key-390-light.png` | Key 对象页：统计块 + 页签 |
| `d3-keys-request-1280-light.png` / `d3-keys-request-390-light.png` / `d3-keys-request-body-390-light.png` | 请求页：四格结果 + 诊断 + 正文 |
| `d3-upstream-1280-light.png` / `d3-upstream-390-light.png` | 上游卡片网格（环形剩余、开关、测试） |
| `d3-upstream-detail-1280-light.png` / `d3-upstream-detail-390-light.png` | 连接对象页"窗口"页签（大环形） |

## 自查发现并已修掉的问题 **[查看]**

| # | 方向 | 问题 | 处理 |
| --- | --- | --- | --- |
| 1 | 1 | 手机顶栏把"创建 API Key"整段文字塞进 36px 图标按钮，文字溢出 | 顶栏改为专用图标按钮 + `aria-label` |
| 2 | 1 | 手机首页"本机接入"卡被 `pre` 片段撑宽，复制按钮与片段被裁 | 网格子项加 `min-width:0`，片段 `max-width:100%` 内部滚动 |
| 3 | 1 | 请求对话框的四格结果里，内层 `.l/.v` 也被画了边框，出现"框套框" | 选择器改为 `.results > div` |
| 4 | 1 | 手机上游行展开后右侧开关被裁掉 | 手机下右侧区域允许换行，额度条独占一行 |
| 5 | 1 | 手机请求列表模型名被截成"synthe…" | 手机下列表项换行，模型名独占第二行 |
| 6 | 2 | Key 页中栏表格在 1280 下 HTTP / 耗时列被裁；选中行的 Request ID 单元格画到了右侧检视栏上 | 表格套 `overflow-x:auto` 容器；Key 页表格去掉连接 / HTTP / Request ID 列（检视栏已有） |
| 7 | 2 | 手机下表格横向被裁（5 小时列、耗时列不可见） | 手机下表格转为"时间 · 结果 · 耗时 / 模型"两行记录，上游矩阵转"名称 · 开关 / 类型 · 状态 · 进度" |
| 8 | 2 | 手机进入 Key / 请求屏后，页头的"API Key · 3 个 · 创建 Key"仍显示，和顶栏标题重复 | 非列表屏隐藏页头 |
| 9 | 3 | 流向板两侧列标题高度不齐（"上游 · 4"在顶、"API KEY · 3"在中） | 列顶对齐，核心卡单独垂直居中 |
| 10 | 3 | 手机请求页"诊断"卡被右侧 `pre` 撑宽，时间线文字被裁 | `.two > * {min-width:0}` |
| 11 | 3 | 手机页头：副标题挤在左侧窄列，按钮堆在右侧 | 手机下页头改为纵向块 |

## 仍存在的小问题（未修，列入已知）

1. 方向 3 手机 Key 对象页的操作行有 4 个控件，"…"按钮单独换到第二行（`d3-keys-key-390-light.png`）。
2. 方向 2 手机首页的页头"运行板 / 创建 Key"按钮换到第二行（`d2-home-390-light.png`），落地时应把主操作移到顶栏。
3. 方向 1 的手机首页统计块 2×2 与第一版形态相近，只是尺寸缩小；若维护者认为仍像第一版，可改为一行横向滚动的小统计。
4. 只截了 1280 与 390；1024–1279 的中间宽度（方向 2 三栏退化、方向 3 顶部导航收起）没有截图验证。
