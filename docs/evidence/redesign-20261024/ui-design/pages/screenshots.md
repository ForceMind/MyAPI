# 批次 0 第三步：三个关键页面的截图与浏览器检查

- 日期：2026-10-24；分支 `dev`；构建：`web/` 下 `bun run build` 的产物，由合成预览 `http://127.0.0.1:4184`（`tools/ui/page-layout-preview.mjs`，读 `web/dist`）提供。预览只读：写操作返回 405，所以"保存并测试"只验证到编辑器打开，提交流程由单元测试覆盖。
- 方式：`pages-shots.mjs`（本目录，副本）启动 Playwright 缓存中的 chrome-headless-shell 1243，经 CDP 截取视口并执行断言。只放行 `127.0.0.1:4184`，其余请求一律拦截：实际拦截 0 次，页面异常 0 次，写请求 0 次。Lite 视图在响应阶段改写 `/api/status`（`product_edition: "lite"`，13 项 capabilities 全为 false）。
- 结果：46 张截图，91 项断言全部通过，原始结果在 `browser-checks.json`（含每项的测量值和任务步数）。
- 语言：`*-320-ru.png` 为俄语，其余为简体中文。

## 截图清单

| 页面 | 1280 亮 | 1280 暗 | 390 亮 | 390 暗 |
| --- | --- | --- | --- | --- |
| 概览 | overview-1280-light | overview-1280-dark | overview-390-light | overview-390-dark |
| Key 检视器（最近请求） | keys-inspector-1280-light | keys-inspector-1280-dark | keys-inspector-390-light | keys-inspector-390-dark |
| 请求详情"正文"页签 | request-body-1280-light | request-body-1280-dark | request-body-390-light | request-body-390-dark |
| 连接检视器 | connections-inspector-1280-light | connections-inspector-1280-dark | connections-inspector-390-light | connections-inspector-390-dark |

其余：

| 文件 | 内容 |
| --- | --- |
| keys-1280-light、keys-390-light、connections-1280-light、connections-390-light、account-windows-1280-light、account-windows-390-light | 未打开检视器的列表；账号窗口页 |
| keys-inspector-1024-light、connections-inspector-1024-light、overview-1024-light | 1024 宽：检视器覆盖在列表上 |
| overview-768-light、keys-inspector-768-light | 768 宽：手机组合，检视器为整页 |
| overview-320-ru、keys-inspector-320-ru、request-body-320-ru、connections-320-ru | 320 宽俄语 |
| overview-1280-lite | Lite：无性能面板和站点内容面板 |
| task1-1280-key-open、task1-1280-request、task1-1280-body、task1-1280-sse | 任务 1 PC：点 Key 行 → 点请求 → 正文 → SSE |
| task1-390-key-page、task1-390-request、task1-390-body | 任务 1 手机 |
| task2-1280-command、task2-1280-details、task2-390-details | 任务 2：粘贴 Request ID 回车直达请求详情 |
| task4-1280-editor | 任务 4："添加连接"编辑器，底部"保存并测试" |
| task6-1280-test、task6-390-test | 任务 6：连接行 → "测试"，结果在检视器内显示（预览拒绝供应商调用，所以是失败结果） |
| task9-1280-failed | 任务 9：概览"失败 N"落到 24 小时内的错误记录 |

## 断言（全部通过）

1. 16 张主截图：无横向溢出；1280 为 PC 组合、检视器在列表右侧；390 为手机组合、检视器为整页。
2. 首屏：1280×800 概览同时看到健康条、需要处理、账号窗口和至少 3 条近期请求；390×844 概览首屏开始于健康条和需要处理；390×844 的 Key 列表、Key 检视器的第一条请求行、连接列表第一行都在首屏且不被底栏遮挡。
3. 触控：390 宽 Key 检视器里所有按钮、链接和页签至少 44×44；检视器内每个按钮滚到中间后 `elementFromPoint` 命中自身（不被顶栏、底栏遮挡）。
4. 任务 1 PC：点 Key 行 → 检视器出现最近请求 → 点请求得到结果与诊断 → "正文"页签显示已保存并脱敏的正文，期间没有任何写请求（不重发）→ 返回回到 Key，焦点在刚才那条请求上。
5. 任务 1 手机：Key 打开为整页、顶栏标题"API Key #21"并带返回；顶栏返回先回到 Key（焦点在请求行），再回到列表（焦点在 Key 卡片）。
6. 任务 2：`Ctrl+K` 粘贴 `SYNTHETIC-CONTEXT-ERROR`，选中项为"打开请求 SYNTHETIC-CONTEXT-ERROR"；回车到 `/usage-logs/common?...inspect=request` 并直接显示详情；手机为搜索图标 → 粘贴 → 回车。
7. 任务 6：连接行 → "测试" 调用现有渠道测试接口，结果显示在检视器内。任务 4："添加连接"打开标题为"添加连接"、带"保存并测试"的编辑器；编辑器头部按钮互不重叠。
8. 任务 9：概览"失败 N"链接带 `type=["5"]` 和 24 小时范围，一次点击落到调用记录，记录行可见。
9. 从 Key 检视器"在调用记录中查看全部"再点"返回 API Key 列表"：1280 焦点回到该 Key 的"查看 Key 请求"，390 焦点回到该 Key 卡片。
10. 1024 宽 Key 与连接检视器为覆盖层；768 宽为手机组合；320 俄语四页无横向溢出，所有页签文字完整显示在页签内。
11. Lite 概览无性能和站点内容面板；四个页面的 h1 与导航名称一致（概览 / API Key / 连接 / 账号窗口）。

## 任务步数（对照 DESIGN_DIRECTION.md §1.2）

"步数"= 点击或回车次数，不含输入文字。§1.2 的"现有步数"把侧栏导航算作一步，"新路径"则从列表开始写，所以两种口径都列出。

| # | 任务 | 目标 PC / 手机 | 实测 PC | 实测手机 | 说明 |
| --- | --- | --- | --- | --- | --- |
| 1 | 查 Key 的最近请求并看失败原因 | 2 / 3 | 从 Keys 列表 2（行 → 请求）；从概览 3（加侧栏） | 3（底栏 Keys → Key → 请求） | 结果、HTTP 状态、用量、结算和诊断在第 2 步直接可见；看正文再 +1 |
| 2 | 按 Request ID 定位请求 | 1 / 2 | 1（`Ctrl+K` 粘贴后回车） | 2（搜索图标 → 回车） | 达标 |
| 4 | 新增上游连接并验证 | 3 / 3 | 从连接页 2（添加连接 → 保存并测试）；从概览 3 | 同 PC | 预览拒绝写入，提交与随后的测试由 `upstream-actions.test.tsx` 覆盖 |
| 6 | 测试已有连接 | 2 / 2 | 从连接页 2（行 → 测试）；从概览 3 | 同 PC | 按 §1.2 现有步数的口径（含导航）是 3 |
| 9 | 看近期失败请求汇总 | 1 / 1 | 1 | 未单独测 | 见问题 P5：落地列表是"错误与待核对"（type 5），会包含已完成但待核对的请求 |

## 逐张查看发现的问题

| # | 截图 | 问题 | 处理 |
| --- | --- | --- | --- |
| P1 | keys-inspector-320-ru | 俄语页签"Недавние запросы"超出页签行左边，被检视器裁掉（页面本身不溢出，原断言没抓到） | 已修：检视器页签允许换行并按语言断字；新增"页签文字完整"断言（该断言与修复同批构建，未在旧构建上跑过） |
| P2 | keys-390-light | Key 卡片的状态"已启用"被截成"已…" | 已修：状态徽标不收缩；新增断言，旧构建上测得两处被截 |
| P3 | overview-1280-light | 1280×800 首屏只能看到 1–2 条近期请求 | 已修：概览近期请求表格行高 44px、窗口条目改两行；断言通过 |
| P4 | task4-1280-editor | 编辑器头部"粘贴连接信息"按钮压住右上角关闭按钮 | 已修：按钮右侧留出关闭按钮位置；新增断言，旧构建上测得重叠 |
| P5 | task9-1280-failed | "失败 2"落到的列表有 3 行，其中含"请求成功"但待核对的行：后端 `type=5` 同时包含失败与待核对，没有只按失败筛选的参数 | 未改：不动后端；落地页筛选框显示"错误与待核对"，语义可见。需要维护者决定是否加后端筛选 |
| P6 | task6-1280-test、task6-390-test | 测试失败原因显示服务端原文（预览返回英文） | 沿用现有测试接口的消息，未改 |
| P7 | connections-inspector-*、connections-320-ru | 未测试时，操作按钮与页签之间有一段约 16px 的空白（测试结果的实时区域为空时仍占位） | 记录，未改 |
| P8 | task4-1280-editor | 复用的渠道编辑器内仍有"添加新的渠道""启用或禁用此渠道""API 密钥"等旧术语，且有 24 个 lucide 图标 | 编辑器不在三页范围内，记录 |
| P9 | request-body-390-light | 请求正文在窄屏按字符断行（"provider i / s"） | 沿用现有正文组件，记录 |
| P10 | task9-1280-failed | 调用记录页列名仍为"渠道""令牌"，"仅自己"在按钮内换行 | 调用记录页本身不在本次范围，记录 |
| P11 | overview-390-light | 手机"需要处理"列出全部条目，设计写的是"最多 3 条 + 全部 n"；但没有可承接"全部"的页面 | 记录，待定 |
| P12 | account-windows-390-light | 手机上账号列表是横向滚动条，右侧条目露出一半 | 有意为之（提示可横向滚动），保留 |

未发现文字与背景对比不足、页面横向溢出或亮暗主题下的错位。

## 未覆盖

- `tools/ui/` 下 13 个浏览器脚本已按新合同更新并通过 `node --check`，但本机没有 Playwright 包，未运行。
- 触摸事件只用 CDP 触摸模拟验证了 44px 目标和遮挡，未在真机上点按。
- 1024–1279 覆盖层只截了 1024 宽，没有截 1279 宽。
