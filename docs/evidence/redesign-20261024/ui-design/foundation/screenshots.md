# 批次 0 设计系统基础：截图与检查记录

- 日期：2026-10-24；分支 `dev`；构建：`web/` 下 `bun run build` 产物，由已运行的合成预览 `http://127.0.0.1:4184`（`tools/ui/page-layout-preview.mjs`，进程工作目录为本工作区）提供，页面引用的 CSS 与本次构建一致。
- 截图方式：本地 headless Chromium（Playwright 缓存中的 chrome-headless-shell 1243）经 CDP 截取视口，界面语言 `zhCN`，主题由 `vite-ui-theme` cookie 指定。所有非 `127.0.0.1:4184` 请求被拦截，实际拦截数 0；页面异常 0。内置浏览器无法把截图写成文件，因此另用内置浏览器在 1280 宽度实测了语言切换与颜色预设（见下文）。
- 文件名：`<页面>-<宽度>-<主题>.png`，视口 1280×900 与 390×844（移动模拟）。

| 页面 | 路由 | 1280 亮 | 1280 暗 | 390 亮 | 390 暗 |
| --- | --- | --- | --- | --- | --- |
| 概览 | `/dashboard/overview` | overview-1280-light.png | overview-1280-dark.png | overview-390-light.png | overview-390-dark.png |
| Key 请求 | `/usage-logs/common?tokenId=21&pageSize=20` | key-requests-1280-light.png | key-requests-1280-dark.png | key-requests-390-light.png | key-requests-390-dark.png |
| 上游账号 | `/upstream-accounts` | upstream-accounts-1280-light.png | upstream-accounts-1280-dark.png | upstream-accounts-390-light.png | upstream-accounts-390-dark.png |
| 设置 | `/system-settings/site/system-info` | settings-1280-light.png | settings-1280-dark.png | settings-390-light.png | settings-390-dark.png |

## 脚本读取的页面状态（16 张全部相同）

- `<html lang>` = `zh-Hans`；`body` 字体栈首项 `"PingFang SC"`。
- `document.documentElement.scrollWidth` 等于视口宽度（1280 / 390），无页面横向溢出。
- `.personal-console` 上 `--primary` 为亮色 `#0035FC`、暗色 `#7B95FF` 对应值，说明 `personal-console.css` 的硬编码覆盖已不再生效。

## 内置浏览器实测（1280×900）

- 通过界面语言菜单切换：俄语后 `<html lang="ru">`、字体栈首项 `system-ui`、行高 24px（1.5）；繁体中文后 `lang="zh-Hant"`、字体栈首项 `"PingFang TC"`、行高 25.6px（1.6）；切回简体中文后 `lang="zh-Hans"`。
- 主题设置抽屉的颜色预设只有"MyAPI 預設 / 高對比"两项；选高对比后 `body[data-theme-preset="high-contrast"]`，控制台内 `--primary`、`--foreground`、`--border` 取高对比值（此前控制台内的硬编码覆盖会让任何预设失效）；两个色块由 `[data-preset-swatch]` 规则绘制。
- 测试后已把预设恢复为默认、语言恢复为简体中文、视口恢复 desktop。

## 逐张查看发现的问题

本步只改 token，不改布局；以下问题中标"既有"的在 `baseline/` 截图中已存在，留给页面批次处理。

| # | 截图 | 问题 | 类别 | 处理 |
| --- | --- | --- | --- | --- |
| F1 | upstream-accounts-*-dark、overview-1280-dark | 额度进度条轨道（`--muted` 系）与卡片底色几乎相同，暗色下轨道不可辨，只能看到已用部分 | 对比度（非文本） | 待页面批次：进度条改用 `--telemetry` 填充 + 有边界的轨道，并始终显示百分比文字 |
| F2 | key-requests-1280-*、key-requests-390-* | "仅自己"在词内断成"仅自 / 己" | 既有（D6） | 页面批次 |
| F3 | key-requests-390-* | 筛选卡底部的"重置 / 搜索 / 查看"被固定分页栏压住 | 既有（M1） | 外壳批次 |
| F4 | key-requests-* | 错误 / 待核对行整行着红、黄底 | 既有（S2） | 页面批次 |
| F5 | overview-390-dark 等 | 暗色顶栏与侧栏中 Logo 的深蓝笔画在深底上对比偏低 | Logo 资产本身 | 维护者要求保留 Logo，未改 |
| F6 | 全部 1280 | 统计块数字仍是 `font-mono`，未用 28px `text-metric` | 既有（H3） | 页面批次使用新字号 token |
| F7 | overview-*、upstream-accounts-* | 账号窗口、Key 选择仍是原生 `<select>` | 既有（D3） | 页面批次 |
| F8 | settings-390-* | "关于"文本框占位文字在 2 行高度内被截断，需要滚动 | 既有 | 页面批次 |

未发现文字与背景对比不足、元素错位或页面横向溢出。
