# 批次 0 第二步：应用外壳的截图与浏览器检查

- 日期：2026-10-24；分支 `dev`；构建：`web/` 下 `bun run build` 产物，由已运行的合成预览 `http://127.0.0.1:4184` 提供（读 `web/dist`）。
- 方式：`shell-shots.mjs`（本目录，副本；原件在会话临时目录）启动 Playwright 缓存中的 chrome-headless-shell 1243，经 CDP 截取视口并执行断言。只放行 `127.0.0.1:4184`，其余请求一律拦截：实际拦截数 0，页面异常 0。Lite 视图通过在响应阶段改写 `/api/status`（`product_edition: "lite"`，13 项 capabilities 全为 false）得到。
- 结果：27 张截图，51 项断言全部通过，原始结果见 `browser-checks.json`。
- 语言：除 `*-320-ru.png` 为俄语外均为简体中文。

## 截图清单

| 页面 | 1280 亮 | 1280 暗 | 390 亮 | 390 暗 |
| --- | --- | --- | --- | --- |
| 概览 | overview-1280-light.png | overview-1280-dark.png | overview-390-light.png | overview-390-dark.png |
| Key 请求 | key-requests-1280-light.png | key-requests-1280-dark.png | key-requests-390-light.png | key-requests-390-dark.png |
| 账号窗口 | upstream-accounts-1280-light.png | upstream-accounts-1280-dark.png | upstream-accounts-390-light.png | upstream-accounts-390-dark.png |
| 设置 | settings-1280-light.png | settings-1280-dark.png | settings-390-light.png | settings-390-dark.png |

其余：

| 文件 | 内容 |
| --- | --- |
| overview-320-ru.png、more-320-ru.png | 320 宽俄语：底部导航；"更多"抽屉 |
| key-requests-768-light.png | 768 宽仍为手机组合 |
| overview-1024-rail.png、overview-1024-expanded.png | 1024 宽默认 64px 图标栏；点"展开侧栏"后 240px |
| overview-1280-lite.png、more-390-lite.png | Lite：侧栏与"更多"均无商业组与绘图 |
| command-request-id-1280.png、request-id-result-1280.png | `Ctrl+K` 打开搜索，粘贴 Request ID；回车后的调用记录（只剩 1 行） |
| key-requests-1280-rail.png | 1280 宽点"收起侧栏"后 |
| key-requests-390-scrolled-end.png | 390 宽滚到底：分页随内容滚动，止于底部导航上沿 |

## 断言（全部通过，数值见 browser-checks.json）

1. 16 张主截图：页面无横向溢出；1280 为 PC 组合、390 为手机组合。
2. 320 俄语：底部 5 个目标（Обзор / Источник / Ключи / Журналы / Ещё），每个 62×56px（≥44×44），标签均为 1 行、无词内截断；无横向溢出。
3. 768：手机组合。1024：PC 组合，默认收起为 64px；点展开后 240px。
4. Lite 1280：侧栏分组为 控制台/上游/访问/记录/工具/系统，无"商业"，无 `/usage-logs/drawing`；Full 1280 有绘图入口。Lite 390 的"更多"里无钱包、订阅、兑换码、绘图。
5. `Ctrl+K` 打开搜索且输入框获得焦点；输入 `SYNTHETIC-CONTEXT-ERROR` 后选中项为"查找请求 SYNTHETIC-CONTEXT-ERROR"；回车跳到 `/usage-logs/common?requestId=SYNTHETIC-CONTEXT-ERROR`，表格 1 行。
6. 1280：点"收起侧栏"后 `data-collapsed=true`、焦点仍在该按钮、cookie `sidebar_state=false`；再点恢复。
7. 390 Key 请求：键盘聚焦"更多"按 Enter 打开，Escape 关闭后焦点回到"更多"；页面内 19 个可用按钮逐个滚到中间，`elementFromPoint` 均命中按钮本身（不被顶栏、底栏遮挡）；滚到底后分页底边 779 = 底栏上沿 779；外壳本身 `scrollTop` 为 0，顶栏与底栏位置不变。

## 逐张查看发现的问题

| # | 截图 | 问题 | 类别 | 处理 |
| --- | --- | --- | --- | --- |
| S1 | upstream-accounts-*、overview-* | 导航与面包屑已是"账号窗口""API Key"，页面 h1 和卡片仍是"上游账号""现有 API Keys""API 密钥" | 术语（页面内文案） | 按约定留到页面批次 |
| S2 | key-requests-1280-*、key-requests-390-* | "仅自己"在词内断成两行 | 既有（D6） | 页面批次 |
| S3 | key-requests-* | 错误、待核对行整行着色 | 既有（S2） | 页面批次 |
| S4 | overview-1024-expanded | 1024×768 展开时侧栏条目超过一屏，"系统"组被底部收起按钮遮住一半，需滚动（滚动条隐藏，沿用旧外壳行为） | 外壳 | 记录；可考虑在矮屏默认折叠"记录"之外的更多组 |
| S5 | 全部 PC 截图 | 顶栏"主题设置"按钮（调色板图标）仍是 lucide，其余外壳图标已是 Hugeicons；`config-drawer.tsx` 内 6 处勾选徽标依赖 lucide 的填充结构，整文件迁移需要视觉返工 | 图标一致性 | 未迁移，见汇报 |
| S6 | more-320-ru、more-390-lite | 用鼠标或键盘打开"更多"时焦点进搜索框；触摸打开时焦点放在列表（不弹键盘）。截图脚本发的是鼠标事件，触摸分支未在浏览器中实测 | 交互 | 已实现，触摸分支未实测 |
| S7 | more-*-* | "更多"里"分析"归在"控制台"小标题下 | 文案 | 可接受，后续可改为"概览与分析" |
| S8 | overview-*、upstream-accounts-* | 概览与账号窗口仍用原生 `<select>` | 既有（D3） | 页面批次 |
| S9 | key-requests-768-light | 768 宽页面边距 16px，设计 6.1 要求 24px（令牌在 1024 才切换） | 令牌（批次 0 第一步范围） | 记录 |

未发现文字与背景对比不足、元素错位、遮挡或页面横向溢出。
