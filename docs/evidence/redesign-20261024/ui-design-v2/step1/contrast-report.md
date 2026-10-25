# 第 1 步：方向 3 令牌对比度审计

- 日期：2026-10-25；分支 `dev`。
- 来源：直接解析落地后的 `web/src/styles/theme.css`（`:root`、`.dark`）与 `web/src/styles/theme-presets.css`（高对比亮 / 暗），`oklch()`、`var()` 与 `color-mix(in oklch, …)` 按 CSS 规则计算后转 sRGB；不是方案表 **[运行]**。
- 公式：WCAG 2.1 相对亮度。文字 ≥4.5:1；非文本（导航下划线、输入框边框、焦点环、连线、额度环）≥3:1。
- 生成脚本：会话 scratchpad 中的 `s1/contrast-v3.mjs`（`node contrast-v3.mjs web/src/styles`，依赖同目录上层的 `color-lib.mjs`）；本文件的表格是该脚本的原样输出。
- 与样稿取值的差异（为通过对比度而改）：
  - 亮色 `--input` 与 `--line` 用 `#808DA9`，不用样稿 `#8591AB`：后者放在浅灰面 `#F3F5FA` 上只有 2.90:1，流向板正是浅灰面。
  - 暗色 `--line` 用 `#5F6C8C`，不用样稿 `#3A4664`：后者在暗色卡片 `#131A2A` 上只有 1.85:1。
  - 高对比亮色补了 `--line: #5B6782`：继承的 `#808DA9` 在高对比浅灰面 `#E6EBF5` 上为 2.79:1。
- 规定沿用：Logo 青 `#00CFFC`（`--telemetry`）在亮色表面不作文字色，只作核心卡上的非文本标记；额度轨道 `--meter-track` 不承载信息，额度环总与百分比文字同时出现。"仅供参考"一栏不计入通过率。

## 汇总

| 主题 | 组合数 | 通过 | 不通过 |
| --- | --- | --- | --- |
| 方向 3 默认 · 亮色 | 43 | 43 | 0 |
| 方向 3 默认 · 暗色 | 43 | 43 | 0 |
| 高对比 · 亮色 | 43 | 43 | 0 |
| 高对比 · 暗色 | 43 | 43 | 0 |
| 合计 | 172 | 172 | 0 |

## 方向 3 默认 · 亮色

| 用途 | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 正文 / 页面 | `--foreground` #0C1222 | `--background` #FFFFFF | 18.66:1 | ≥4.5 | 通过 |
| 正文 / 卡片 | `--foreground` #0C1222 | `--card` #FFFFFF | 18.66:1 | ≥4.5 | 通过 |
| 正文 / 浅灰面（搜索框、流向板） | `--foreground` #0C1222 | `--muted` #F3F5FA | 17.11:1 | ≥4.5 | 通过 |
| 菜单文字 / 菜单 | `--popover-foreground` #0C1222 | `--popover` #FFFFFF | 18.66:1 | ≥4.5 | 通过 |
| 次要文字、未选中导航 / 页面 | `--muted-foreground` #525E78 | `--background` #FFFFFF | 6.50:1 | ≥4.5 | 通过 |
| 次要文字 / 卡片 | `--muted-foreground` #525E78 | `--card` #FFFFFF | 6.50:1 | ≥4.5 | 通过 |
| 次要文字、搜索提示 / 浅灰面 | `--muted-foreground` #525E78 | `--muted` #F3F5FA | 5.96:1 | ≥4.5 | 通过 |
| 主按钮文字 / 主色 | `--primary-foreground` #FFFFFF | `--primary` #0035FC | 7.20:1 | ≥4.5 | 通过 |
| 链接 / 页面 | `--primary` #0035FC | `--background` #FFFFFF | 7.20:1 | ≥4.5 | 通过 |
| 链接 / 浅灰面 | `--primary` #0035FC | `--muted` #F3F5FA | 6.61:1 | ≥4.5 | 通过 |
| 次按钮文字 / 次按钮 | `--secondary-foreground` #24304D | `--secondary` #F3F5FA | 12.00:1 | ≥4.5 | 通过 |
| 选中项文字 / 选中底 | `--accent-foreground` #0029C4 | `--accent` #E6ECFF | 8.42:1 | ≥4.5 | 通过 |
| 选中分区标签（反色） | `--background` #FFFFFF | `--foreground` #0C1222 | 18.66:1 | ≥4.5 | 通过 |
| 网关核心卡文字 / 核心卡 | `--core-foreground` #FFFFFF | `--core` #0035FC | 7.20:1 | ≥4.5 | 通过 |
| 危险按钮 | `--destructive-foreground` #FFFFFF | `--destructive` #C8241B | 5.64:1 | ≥4.5 | 通过 |
| 失败文字 / 卡片 | `--destructive` #C8241B | `--card` #FFFFFF | 5.64:1 | ≥4.5 | 通过 |
| 失败徽标 | `--destructive` #C8241B | `--destructive-muted` #FDECEA | 4.93:1 | ≥4.5 | 通过 |
| 成功实底 | `--success-foreground` #FFFFFF | `--success` #146F37 | 6.25:1 | ≥4.5 | 通过 |
| 成功文字 / 卡片 | `--success` #146F37 | `--card` #FFFFFF | 6.25:1 | ≥4.5 | 通过 |
| 成功徽标 | `--success` #146F37 | `--success-muted` #E6F6EC | 5.59:1 | ≥4.5 | 通过 |
| 警告实底 | `--warning-foreground` #FFFFFF | `--warning` #8F5200 | 6.22:1 | ≥4.5 | 通过 |
| 警告文字 / 卡片 | `--warning` #8F5200 | `--card` #FFFFFF | 6.22:1 | ≥4.5 | 通过 |
| 警告徽标 | `--warning` #8F5200 | `--warning-muted` #FFF3DB | 5.66:1 | ≥4.5 | 通过 |
| 信息实底 | `--info-foreground` #FFFFFF | `--info` #00628F | 6.68:1 | ≥4.5 | 通过 |
| 信息文字 / 卡片 | `--info` #00628F | `--card` #FFFFFF | 6.68:1 | ≥4.5 | 通过 |
| 信息徽标 | `--info` #00628F | `--info-muted` #E3F3FC | 5.88:1 | ≥4.5 | 通过 |
| 中性徽标文字 / 卡片 | `--neutral` #5B6782 | `--card` #FFFFFF | 5.67:1 | ≥4.5 | 通过 |
| 表头 | `--foreground` #0C1222 | `--table-header` #FBFBFB | 18.02:1 | ≥4.5 | 通过 |
| 停用行次要文字 | `--muted-foreground` #525E78 | `--table-disabled` #F7F7F8 | 6.06:1 | ≥4.5 | 通过 |
| 顶部导航当前项下划线、分页标签下划线 | `--primary` #0035FC | `--background` #FFFFFF | 7.20:1 | ≥3（非文本） | 通过 |
| 输入框边框 / 卡片 | `--input` #808DA9 | `--card` #FFFFFF | 3.33:1 | ≥3（非文本） | 通过 |
| 输入框边框、开关关 / 浅灰面 | `--input` #808DA9 | `--muted` #F3F5FA | 3.06:1 | ≥3（非文本） | 通过 |
| 焦点环 / 页面 | `--ring` #0035FC | `--background` #FFFFFF | 7.20:1 | ≥3（非文本） | 通过 |
| 焦点环 / 浅灰面 | `--ring` #0035FC | `--muted` #F3F5FA | 6.61:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 浅灰面 | `--line` #808DA9 | `--muted` #F3F5FA | 3.06:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 卡片 | `--line` #808DA9 | `--card` #FFFFFF | 3.33:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 卡片 | `--meter` #0083A8 | `--card` #FFFFFF | 4.36:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 浅灰面 | `--meter` #0083A8 | `--muted` #F3F5FA | 4.00:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 轨道 | `--meter` #0083A8 | `--meter-track` #E1E7F1 | 3.51:1 | ≥3（非文本） | 通过 |
| 额度环告警 / 轨道 | `--meter-warning` #8F5200 | `--meter-track` #E1E7F1 | 5.01:1 | ≥3（非文本） | 通过 |
| 额度环耗尽 / 轨道 | `--meter-critical` #C8241B | `--meter-track` #E1E7F1 | 4.54:1 | ≥3（非文本） | 通过 |
| 核心卡柱状标记（青）/ 核心卡 | `--telemetry` #00CFFC | `--core` #0035FC | 3.89:1 | ≥3（非文本） | 通过 |
| 停用行左侧标记 | `--table-disabled-border` #5B6782 | `--background` #FFFFFF | 5.67:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 用途 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 装饰性分隔线（不承载信息） | `--border` #DDE2EC | `--card` #FFFFFF | 1.30:1 | ≥3（非文本） | **不通过** |
| 额度轨道（数值同时以文字显示） | `--meter-track` #E1E7F1 | `--card` #FFFFFF | 1.24:1 | ≥3（非文本） | **不通过** |
| Logo 青作文字 / 卡片（规定不作文字） | `--telemetry` #00CFFC | `--card` #FFFFFF | 1.85:1 | ≥4.5 | **不通过** |

## 方向 3 默认 · 暗色

| 用途 | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 正文 / 页面 | `--foreground` #E8ECF6 | `--background` #090D18 | 16.41:1 | ≥4.5 | 通过 |
| 正文 / 卡片 | `--foreground` #E8ECF6 | `--card` #131A2A | 14.69:1 | ≥4.5 | 通过 |
| 正文 / 浅灰面（搜索框、流向板） | `--foreground` #E8ECF6 | `--muted` #182136 | 13.56:1 | ≥4.5 | 通过 |
| 菜单文字 / 菜单 | `--popover-foreground` #E8ECF6 | `--popover` #182136 | 13.56:1 | ≥4.5 | 通过 |
| 次要文字、未选中导航 / 页面 | `--muted-foreground` #9AA6C0 | `--background` #090D18 | 7.94:1 | ≥4.5 | 通过 |
| 次要文字 / 卡片 | `--muted-foreground` #9AA6C0 | `--card` #131A2A | 7.11:1 | ≥4.5 | 通过 |
| 次要文字、搜索提示 / 浅灰面 | `--muted-foreground` #9AA6C0 | `--muted` #182136 | 6.56:1 | ≥4.5 | 通过 |
| 主按钮文字 / 主色 | `--primary-foreground` #070B1A | `--primary` #88A0FF | 7.95:1 | ≥4.5 | 通过 |
| 链接 / 页面 | `--primary` #88A0FF | `--background` #090D18 | 7.87:1 | ≥4.5 | 通过 |
| 链接 / 浅灰面 | `--primary` #88A0FF | `--muted` #182136 | 6.51:1 | ≥4.5 | 通过 |
| 次按钮文字 / 次按钮 | `--secondary-foreground` #C9D2E6 | `--secondary` #182136 | 10.57:1 | ≥4.5 | 通过 |
| 选中项文字 / 选中底 | `--accent-foreground` #C9D5FF | `--accent` #1C2752 | 9.88:1 | ≥4.5 | 通过 |
| 选中分区标签（反色） | `--background` #090D18 | `--foreground` #E8ECF6 | 16.41:1 | ≥4.5 | 通过 |
| 网关核心卡文字 / 核心卡 | `--core-foreground` #E8ECF6 | `--core` #1C2752 | 12.15:1 | ≥4.5 | 通过 |
| 危险按钮 | `--destructive-foreground` #1A0503 | `--destructive` #FF8A80 | 8.62:1 | ≥4.5 | 通过 |
| 失败文字 / 卡片 | `--destructive` #FF8A80 | `--card` #131A2A | 7.61:1 | ≥4.5 | 通过 |
| 失败徽标 | `--destructive` #FF8A80 | `--destructive-muted` #3C1E1E | 6.59:1 | ≥4.5 | 通过 |
| 成功实底 | `--success-foreground` #090D18 | `--success` #4ADE9B | 11.28:1 | ≥4.5 | 通过 |
| 成功文字 / 卡片 | `--success` #4ADE9B | `--card` #131A2A | 10.10:1 | ≥4.5 | 通过 |
| 成功徽标 | `--success` #4ADE9B | `--success-muted` #153527 | 7.78:1 | ≥4.5 | 通过 |
| 警告实底 | `--warning-foreground` #090D18 | `--warning` #F2B544 | 10.59:1 | ≥4.5 | 通过 |
| 警告文字 / 卡片 | `--warning` #F2B544 | `--card` #131A2A | 9.49:1 | ≥4.5 | 通过 |
| 警告徽标 | `--warning` #F2B544 | `--warning-muted` #3A2D14 | 7.33:1 | ≥4.5 | 通过 |
| 信息实底 | `--info-foreground` #090D18 | `--info` #5CC8FF | 10.31:1 | ≥4.5 | 通过 |
| 信息文字 / 卡片 | `--info` #5CC8FF | `--card` #131A2A | 9.23:1 | ≥4.5 | 通过 |
| 信息徽标 | `--info` #5CC8FF | `--info-muted` #15314A | 7.11:1 | ≥4.5 | 通过 |
| 中性徽标文字 / 卡片 | `--neutral` #9AA6C0 | `--card` #131A2A | 7.11:1 | ≥4.5 | 通过 |
| 表头 | `--foreground` #E8ECF6 | `--table-header` #131723 | 15.07:1 | ≥4.5 | 通过 |
| 停用行次要文字 | `--muted-foreground` #9AA6C0 | `--table-disabled` #151925 | 7.16:1 | ≥4.5 | 通过 |
| 顶部导航当前项下划线、分页标签下划线 | `--primary` #88A0FF | `--background` #090D18 | 7.87:1 | ≥3（非文本） | 通过 |
| 输入框边框 / 卡片 | `--input` #5F6C8C | `--card` #131A2A | 3.32:1 | ≥3（非文本） | 通过 |
| 输入框边框、开关关 / 浅灰面 | `--input` #5F6C8C | `--muted` #182136 | 3.06:1 | ≥3（非文本） | 通过 |
| 焦点环 / 页面 | `--ring` #88A0FF | `--background` #090D18 | 7.87:1 | ≥3（非文本） | 通过 |
| 焦点环 / 浅灰面 | `--ring` #88A0FF | `--muted` #182136 | 6.51:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 浅灰面 | `--line` #5F6C8C | `--muted` #182136 | 3.06:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 卡片 | `--line` #5F6C8C | `--card` #131A2A | 3.32:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 卡片 | `--meter` #22D3FF | `--card` #131A2A | 9.79:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 浅灰面 | `--meter` #22D3FF | `--muted` #182136 | 9.03:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 轨道 | `--meter` #22D3FF | `--meter-track` #263249 | 7.24:1 | ≥3（非文本） | 通过 |
| 额度环告警 / 轨道 | `--meter-warning` #F2B544 | `--meter-track` #263249 | 7.01:1 | ≥3（非文本） | 通过 |
| 额度环耗尽 / 轨道 | `--meter-critical` #FF8A80 | `--meter-track` #263249 | 5.63:1 | ≥3（非文本） | 通过 |
| 核心卡柱状标记（青）/ 核心卡 | `--telemetry` #22D3FF | `--core` #1C2752 | 8.10:1 | ≥3（非文本） | 通过 |
| 停用行左侧标记 | `--table-disabled-border` #9AA6C0 | `--background` #090D18 | 7.94:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 用途 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 装饰性分隔线（不承载信息） | `--border` #222C42 | `--card` #131A2A | 1.25:1 | ≥3（非文本） | **不通过** |
| 额度轨道（数值同时以文字显示） | `--meter-track` #263249 | `--card` #131A2A | 1.35:1 | ≥3（非文本） | **不通过** |
| Logo 青作文字 / 卡片（规定不作文字） | `--telemetry` #22D3FF | `--card` #131A2A | 9.79:1 | ≥4.5 | 通过 |

## 高对比 · 亮色

| 用途 | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 正文 / 页面 | `--foreground` #05091A | `--background` #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| 正文 / 卡片 | `--foreground` #05091A | `--card` #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| 正文 / 浅灰面（搜索框、流向板） | `--foreground` #05091A | `--muted` #E6EBF5 | 16.57:1 | ≥4.5 | 通过 |
| 菜单文字 / 菜单 | `--popover-foreground` #05091A | `--popover` #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| 次要文字、未选中导航 / 页面 | `--muted-foreground` #2B3550 | `--background` #FFFFFF | 12.16:1 | ≥4.5 | 通过 |
| 次要文字 / 卡片 | `--muted-foreground` #2B3550 | `--card` #FFFFFF | 12.16:1 | ≥4.5 | 通过 |
| 次要文字、搜索提示 / 浅灰面 | `--muted-foreground` #2B3550 | `--muted` #E6EBF5 | 10.17:1 | ≥4.5 | 通过 |
| 主按钮文字 / 主色 | `--primary-foreground` #FFFFFF | `--primary` #0029C4 | 9.93:1 | ≥4.5 | 通过 |
| 链接 / 页面 | `--primary` #0029C4 | `--background` #FFFFFF | 9.93:1 | ≥4.5 | 通过 |
| 链接 / 浅灰面 | `--primary` #0029C4 | `--muted` #E6EBF5 | 8.31:1 | ≥4.5 | 通过 |
| 次按钮文字 / 次按钮 | `--secondary-foreground` #05091A | `--secondary` #E6EBF5 | 16.57:1 | ≥4.5 | 通过 |
| 选中项文字 / 选中底 | `--accent-foreground` #001A8C | `--accent` #D6E0FF | 10.46:1 | ≥4.5 | 通过 |
| 选中分区标签（反色） | `--background` #FFFFFF | `--foreground` #05091A | 19.80:1 | ≥4.5 | 通过 |
| 网关核心卡文字 / 核心卡 | `--core-foreground` #FFFFFF | `--core` #0035FC | 7.20:1 | ≥4.5 | 通过 |
| 危险按钮 | `--destructive-foreground` #FFFFFF | `--destructive` #A3180F | 7.79:1 | ≥4.5 | 通过 |
| 失败文字 / 卡片 | `--destructive` #A3180F | `--card` #FFFFFF | 7.79:1 | ≥4.5 | 通过 |
| 失败徽标 | `--destructive` #A3180F | `--destructive-muted` #FDECEA | 6.81:1 | ≥4.5 | 通过 |
| 成功实底 | `--success-foreground` #FFFFFF | `--success` #05603A | 7.66:1 | ≥4.5 | 通过 |
| 成功文字 / 卡片 | `--success` #05603A | `--card` #FFFFFF | 7.66:1 | ≥4.5 | 通过 |
| 成功徽标 | `--success` #05603A | `--success-muted` #E6F6EC | 6.84:1 | ≥4.5 | 通过 |
| 警告实底 | `--warning-foreground` #FFFFFF | `--warning` #6E3F00 | 8.85:1 | ≥4.5 | 通过 |
| 警告文字 / 卡片 | `--warning` #6E3F00 | `--card` #FFFFFF | 8.85:1 | ≥4.5 | 通过 |
| 警告徽标 | `--warning` #6E3F00 | `--warning-muted` #FFF3DB | 8.04:1 | ≥4.5 | 通过 |
| 信息实底 | `--info-foreground` #FFFFFF | `--info` #004B6E | 9.41:1 | ≥4.5 | 通过 |
| 信息文字 / 卡片 | `--info` #004B6E | `--card` #FFFFFF | 9.41:1 | ≥4.5 | 通过 |
| 信息徽标 | `--info` #004B6E | `--info-muted` #E3F3FC | 8.28:1 | ≥4.5 | 通过 |
| 中性徽标文字 / 卡片 | `--neutral` #3D4966 | `--card` #FFFFFF | 8.96:1 | ≥4.5 | 通过 |
| 表头 | `--foreground` #05091A | `--table-header` #FBFBFB | 19.09:1 | ≥4.5 | 通过 |
| 停用行次要文字 | `--muted-foreground` #2B3550 | `--table-disabled` #F6F6F7 | 11.29:1 | ≥4.5 | 通过 |
| 顶部导航当前项下划线、分页标签下划线 | `--primary` #0029C4 | `--background` #FFFFFF | 9.93:1 | ≥3（非文本） | 通过 |
| 输入框边框 / 卡片 | `--input` #3D4966 | `--card` #FFFFFF | 8.96:1 | ≥3（非文本） | 通过 |
| 输入框边框、开关关 / 浅灰面 | `--input` #3D4966 | `--muted` #E6EBF5 | 7.50:1 | ≥3（非文本） | 通过 |
| 焦点环 / 页面 | `--ring` #0029C4 | `--background` #FFFFFF | 9.93:1 | ≥3（非文本） | 通过 |
| 焦点环 / 浅灰面 | `--ring` #0029C4 | `--muted` #E6EBF5 | 8.31:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 浅灰面 | `--line` #5B6782 | `--muted` #E6EBF5 | 4.74:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 卡片 | `--line` #5B6782 | `--card` #FFFFFF | 5.67:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 卡片 | `--meter` #0083A8 | `--card` #FFFFFF | 4.36:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 浅灰面 | `--meter` #0083A8 | `--muted` #E6EBF5 | 3.65:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 轨道 | `--meter` #0083A8 | `--meter-track` #E1E7F1 | 3.51:1 | ≥3（非文本） | 通过 |
| 额度环告警 / 轨道 | `--meter-warning` #6E3F00 | `--meter-track` #E1E7F1 | 7.12:1 | ≥3（非文本） | 通过 |
| 额度环耗尽 / 轨道 | `--meter-critical` #A3180F | `--meter-track` #E1E7F1 | 6.27:1 | ≥3（非文本） | 通过 |
| 核心卡柱状标记（青）/ 核心卡 | `--telemetry` #00CFFC | `--core` #0035FC | 3.89:1 | ≥3（非文本） | 通过 |
| 停用行左侧标记 | `--table-disabled-border` #3D4966 | `--background` #FFFFFF | 8.96:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 用途 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 装饰性分隔线（不承载信息） | `--border` #5B6782 | `--card` #FFFFFF | 5.67:1 | ≥3（非文本） | 通过 |
| 额度轨道（数值同时以文字显示） | `--meter-track` #E1E7F1 | `--card` #FFFFFF | 1.24:1 | ≥3（非文本） | **不通过** |
| Logo 青作文字 / 卡片（规定不作文字） | `--telemetry` #00CFFC | `--card` #FFFFFF | 1.85:1 | ≥4.5 | **不通过** |

## 高对比 · 暗色

| 用途 | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 正文 / 页面 | `--foreground` #FFFFFF | `--background` #02040A | 20.50:1 | ≥4.5 | 通过 |
| 正文 / 卡片 | `--foreground` #FFFFFF | `--card` #0B1020 | 18.93:1 | ≥4.5 | 通过 |
| 正文 / 浅灰面（搜索框、流向板） | `--foreground` #FFFFFF | `--muted` #141B30 | 17.08:1 | ≥4.5 | 通过 |
| 菜单文字 / 菜单 | `--popover-foreground` #FFFFFF | `--popover` #141B30 | 17.08:1 | ≥4.5 | 通过 |
| 次要文字、未选中导航 / 页面 | `--muted-foreground` #C9D2E6 | `--background` #02040A | 13.51:1 | ≥4.5 | 通过 |
| 次要文字 / 卡片 | `--muted-foreground` #C9D2E6 | `--card` #0B1020 | 12.48:1 | ≥4.5 | 通过 |
| 次要文字、搜索提示 / 浅灰面 | `--muted-foreground` #C9D2E6 | `--muted` #141B30 | 11.26:1 | ≥4.5 | 通过 |
| 主按钮文字 / 主色 | `--primary-foreground` #02040A | `--primary` #A9BBFF | 10.97:1 | ≥4.5 | 通过 |
| 链接 / 页面 | `--primary` #A9BBFF | `--background` #02040A | 10.97:1 | ≥4.5 | 通过 |
| 链接 / 浅灰面 | `--primary` #A9BBFF | `--muted` #141B30 | 9.14:1 | ≥4.5 | 通过 |
| 次按钮文字 / 次按钮 | `--secondary-foreground` #FFFFFF | `--secondary` #141B30 | 17.08:1 | ≥4.5 | 通过 |
| 选中项文字 / 选中底 | `--accent-foreground` #FFFFFF | `--accent` #1A2550 | 14.74:1 | ≥4.5 | 通过 |
| 选中分区标签（反色） | `--background` #02040A | `--foreground` #FFFFFF | 20.50:1 | ≥4.5 | 通过 |
| 网关核心卡文字 / 核心卡 | `--core-foreground` #E8ECF6 | `--core` #1C2752 | 12.15:1 | ≥4.5 | 通过 |
| 危险按钮 | `--destructive-foreground` #02040A | `--destructive` #FFB3AB | 11.99:1 | ≥4.5 | 通过 |
| 失败文字 / 卡片 | `--destructive` #FFB3AB | `--card` #0B1020 | 11.08:1 | ≥4.5 | 通过 |
| 失败徽标 | `--destructive` #FFB3AB | `--destructive-muted` #3C1E1E | 8.80:1 | ≥4.5 | 通过 |
| 成功实底 | `--success-foreground` #02040A | `--success` #7DF0BC | 14.71:1 | ≥4.5 | 通过 |
| 成功文字 / 卡片 | `--success` #7DF0BC | `--card` #0B1020 | 13.58:1 | ≥4.5 | 通过 |
| 成功徽标 | `--success` #7DF0BC | `--success-muted` #153527 | 9.59:1 | ≥4.5 | 通过 |
| 警告实底 | `--warning-foreground` #02040A | `--warning` #FFD27A | 14.40:1 | ≥4.5 | 通过 |
| 警告文字 / 卡片 | `--warning` #FFD27A | `--card` #0B1020 | 13.30:1 | ≥4.5 | 通过 |
| 警告徽标 | `--warning` #FFD27A | `--warning-muted` #3A2D14 | 9.42:1 | ≥4.5 | 通过 |
| 信息实底 | `--info-foreground` #02040A | `--info` #9ADCFF | 13.72:1 | ≥4.5 | 通过 |
| 信息文字 / 卡片 | `--info` #9ADCFF | `--card` #0B1020 | 12.67:1 | ≥4.5 | 通过 |
| 信息徽标 | `--info` #9ADCFF | `--info-muted` #15314A | 8.95:1 | ≥4.5 | 通过 |
| 中性徽标文字 / 卡片 | `--neutral` #C9D2E6 | `--card` #0B1020 | 12.48:1 | ≥4.5 | 通过 |
| 表头 | `--foreground` #FFFFFF | `--table-header` #090D15 | 19.40:1 | ≥4.5 | 通过 |
| 停用行次要文字 | `--muted-foreground` #C9D2E6 | `--table-disabled` #0B0F17 | 12.62:1 | ≥4.5 | 通过 |
| 顶部导航当前项下划线、分页标签下划线 | `--primary` #A9BBFF | `--background` #02040A | 10.97:1 | ≥3（非文本） | 通过 |
| 输入框边框 / 卡片 | `--input` #A3AEC6 | `--card` #0B1020 | 8.50:1 | ≥3（非文本） | 通过 |
| 输入框边框、开关关 / 浅灰面 | `--input` #A3AEC6 | `--muted` #141B30 | 7.67:1 | ≥3（非文本） | 通过 |
| 焦点环 / 页面 | `--ring` #C9D5FF | `--background` #02040A | 14.10:1 | ≥3（非文本） | 通过 |
| 焦点环 / 浅灰面 | `--ring` #C9D5FF | `--muted` #141B30 | 11.75:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 浅灰面 | `--line` #5F6C8C | `--muted` #141B30 | 3.26:1 | ≥3（非文本） | 通过 |
| 流向板连线 / 卡片 | `--line` #5F6C8C | `--card` #0B1020 | 3.62:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 卡片 | `--meter` #22D3FF | `--card` #0B1020 | 10.67:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 浅灰面 | `--meter` #22D3FF | `--muted` #141B30 | 9.62:1 | ≥3（非文本） | 通过 |
| 额度环填充 / 轨道 | `--meter` #22D3FF | `--meter-track` #263249 | 7.24:1 | ≥3（非文本） | 通过 |
| 额度环告警 / 轨道 | `--meter-warning` #FFD27A | `--meter-track` #263249 | 9.02:1 | ≥3（非文本） | 通过 |
| 额度环耗尽 / 轨道 | `--meter-critical` #FFB3AB | `--meter-track` #263249 | 7.51:1 | ≥3（非文本） | 通过 |
| 核心卡柱状标记（青）/ 核心卡 | `--telemetry` #22D3FF | `--core` #1C2752 | 8.10:1 | ≥3（非文本） | 通过 |
| 停用行左侧标记 | `--table-disabled-border` #C9D2E6 | `--background` #02040A | 13.51:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 用途 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| 装饰性分隔线（不承载信息） | `--border` #8592AD | `--card` #0B1020 | 6.05:1 | ≥3（非文本） | 通过 |
| 额度轨道（数值同时以文字显示） | `--meter-track` #263249 | `--card` #0B1020 | 1.47:1 | ≥3（非文本） | **不通过** |
| Logo 青作文字 / 卡片（规定不作文字） | `--telemetry` #22D3FF | `--card` #0B1020 | 10.67:1 | ≥4.5 | 通过 |

Logo 青 #00CFFC 作文字 / 白：1.85:1，不通过；规定青色在亮色表面不作文字色，只作核心卡上的非文本标记。
