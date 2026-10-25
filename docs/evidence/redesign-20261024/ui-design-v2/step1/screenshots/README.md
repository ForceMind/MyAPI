# 第 1 步截图与浏览器检查

- 来源：本机合成预览 `http://127.0.0.1:4184`（`tools/ui/page-layout-preview.mjs`，只读合成数据，读 `web/dist`）；构建带 `VITE_BUILD_ID=6b479884979f3403fe59b655a5b886d6156ca2ca`。
- 工具：`shots.mjs`（Playwright 1.61.1 + Chromium，`MYAPI_PLAYWRIGHT_MODULE` 指向本机安装）；只放行 `127.0.0.1:4184`，其余请求一律中止，本次被中止 0 个。
- 结果：`browser-checks.json` 记录 263 项检查，全部通过 **[运行]**；74 张截图逐张看过 **[查看]**。

## 截图清单

| 组 | 文件 | 说明 |
| --- | --- | --- |
| 六个任务页 | `{overview,key-requests,upstream,account-windows,settings,profile}-{1280,390}-{light,dark}-{zh,en}.png` | 48 张；每张检查：无横向溢出、PC / 手机组合正确且无侧栏、页面不在顶栏之下、无页面错误 |
| 补充页 | `keys-*-light-zh.png`、`system-info-*-light-zh.png` | API Key 列表与运行状态，1280 / 390 |
| 其他宽度 | `{overview,key-requests}-{320,768,1024}-light-{zh,en}.png` | 320 / 768 检查底部 5 项 ≥44px、标签一行不截断；1024 检查顶栏导航不压到右侧操作区 |
| Lite | `lite-overview-{1280,390}-light-zh.png`、`lite-more-menu-1280-light-zh.png`、`lite-more-390-light-zh.png` | 改写 `/api/status` 注入 `product_edition: "lite"` 与全 false 能力：无"管理"，用户 / 运行状态 / 工具在"更多" |
| 管理下拉 | `admin-menu-1280-light-{zh,en}.png`、`more-menu-1280-dark-zh.png` | Full 管理员的"管理"展开（预览数据的资金模式未开启，所以没有商业组）与"更多"展开 |
| 实际操作 | `command-request-id-1280-light-zh.png`、`request-id-result-1280-light-zh.png`、`more-390-light-zh.png` | 键盘 `Ctrl/⌘ K` → 粘贴 Request ID → 回车直接打开该请求详情；手机"更多"抽屉 |

实际操作检查（`browser-checks.json` 中 `ops:` 开头的 6 项）：粘贴 Request ID 回车打开请求详情；键盘打开"管理"、焦点进入菜单、Escape 回到按钮；触屏（`pointer: coarse`）点"更多"、菜单项高度 ≥44px、点项跳转；手机"更多"Escape 关闭且焦点回到"更多"、选项跳转并关闭抽屉。

## 看图发现的问题

| # | 现象 | 位置 | 处理 |
| --- | --- | --- | --- |
| 1 | 第一版 Key 列表操作列"查看 Key 请求"按钮高度固定、`requests` 被拆成 `reques ts` | `keys-1280-light-zh.png`（修复后）；旧构建上 `tools/ui/key-request-browser.mjs` 新增检查实测失败（`fits:false, wordsWhole:false`） | 本步已修：按钮 `h-auto` + `wrap-break-word` + `break-keep` |
| 2 | 深色下分页栏是一条比内容卡片更宽的色带 | 中间构建的截图（未保留） | 本步已修：外壳内分页栏背景透明，只保留分隔线 |
| 3 | 1024×768 下调用记录页表格区只露出表头 | `key-requests-1024-light-*.png` | 未修：该页筛选区与内部滚动表格属于请求页批次（第 3 步）；外壳侧已在有页签时省去两级面包屑、改为整列滚动 |
| 4 | 调用记录页"全部 / 仅自己"切换在 1280 与 390 都折成两行 | `key-requests-*.png` | 未修：第一版同样存在（见 `ui-design/shell/key-requests-1280-light.png`），属于页面批次 |
| 5 | Key 列表在 1280 下表格宽于内容区，右侧固定的"操作"列表头盖住"IP 限制"表头 | `keys-1280-light-zh.png` | 未修：从代码看是表格的列固定行为，未在第一版构建上对照；属于 Key 页批次 |
| 6 | 手机"更多"用触屏打开时焦点落在列表，抽屉滚过了标题与搜索框 | `more-390-light-zh.png` | 未修：从代码看沿用第一版的 `initialFocus`（触屏打开时聚焦列表，避免弹出键盘），记录待交互批次评估 |
| 7 | 英文下调用记录页手机顶栏标题是导航名"Requests"，页面 h1 是"Call records" | `key-requests-390-*-en.png` | 未修：术语统一属于请求页批次 |
| 8 | 设置分区仍叫"侧边栏模块" | `settings-1280-*.png` | 未修：设置页批次改名 |
