# 设计令牌对比度审计（批次 0 · 设计系统基础）

- 来源：直接解析 `web/src/styles/theme.css` 与 `web/src/styles/theme-presets.css` 中落地的 token（`oklch()` 与 `color-mix(in oklch, …)` 按 CSS 规则计算，超色域分量截断），不是提议表。
- 公式：WCAG 2.1 相对亮度。正文 ≥4.5:1；非文本（输入框边框、焦点环、数据标记）≥3:1。
- 生成脚本：会话 scratchpad 中的 `contrast-audit.mjs`（`node contrast-audit.mjs web/src/styles`）。

## MyAPI 默认 · 亮色

| 组合（前景 / 背景） | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| foreground / background | #0E1630 | #F6F8FC | 16.79:1 | ≥4.5 | 通过 |
| foreground / card | #0E1630 | #FFFFFF | 17.85:1 | ≥4.5 | 通过 |
| card-foreground / card | #0E1630 | #FFFFFF | 17.85:1 | ≥4.5 | 通过 |
| popover-foreground / popover | #0E1630 | #FFFFFF | 17.85:1 | ≥4.5 | 通过 |
| muted-foreground / background | #4D5975 | #F6F8FC | 6.58:1 | ≥4.5 | 通过 |
| muted-foreground / card | #4D5975 | #FFFFFF | 7.00:1 | ≥4.5 | 通过 |
| muted-foreground / muted | #4D5975 | #EEF2F9 | 6.23:1 | ≥4.5 | 通过 |
| primary-foreground / primary | #FFFFFF | #0035FC | 7.20:1 | ≥4.5 | 通过 |
| primary-foreground / myapi-brand-strong | #FFFFFF | #0C31D4 | 8.73:1 | ≥4.5 | 通过 |
| primary / background | #0035FC | #F6F8FC | 6.77:1 | ≥4.5 | 通过 |
| primary / card | #0035FC | #FFFFFF | 7.20:1 | ≥4.5 | 通过 |
| secondary-foreground / secondary | #24304D | #EEF2F9 | 11.65:1 | ≥4.5 | 通过 |
| accent-foreground / accent | #0029C4 | #E6ECFF | 8.42:1 | ≥4.5 | 通过 |
| destructive-foreground / destructive | #FFFFFF | #C8241B | 5.64:1 | ≥4.5 | 通过 |
| destructive / card | #C8241B | #FFFFFF | 5.64:1 | ≥4.5 | 通过 |
| destructive / destructive-muted | #C8241B | #FDECEA | 4.93:1 | ≥4.5 | 通过 |
| success-foreground / success | #FFFFFF | #067647 | 5.69:1 | ≥4.5 | 通过 |
| success / card | #067647 | #FFFFFF | 5.69:1 | ≥4.5 | 通过 |
| success / success-muted | #067647 | #E6F6EE | 5.09:1 | ≥4.5 | 通过 |
| warning-foreground / warning | #FFFFFF | #8F5200 | 6.22:1 | ≥4.5 | 通过 |
| warning / card | #8F5200 | #FFFFFF | 6.22:1 | ≥4.5 | 通过 |
| warning / warning-muted | #8F5200 | #FFF4DC | 5.70:1 | ≥4.5 | 通过 |
| info-foreground / info | #FFFFFF | #00628F | 6.68:1 | ≥4.5 | 通过 |
| info / card | #00628F | #FFFFFF | 6.68:1 | ≥4.5 | 通过 |
| info / info-muted | #00628F | #E3F7FF | 6.05:1 | ≥4.5 | 通过 |
| neutral-foreground / neutral | #FFFFFF | #5B6782 | 5.67:1 | ≥4.5 | 通过 |
| neutral / card | #5B6782 | #FFFFFF | 5.67:1 | ≥4.5 | 通过 |
| telemetry-foreground / telemetry | #0B1530 | #00CFFC | 9.73:1 | ≥4.5 | 通过 |
| sidebar-foreground / sidebar | #24304D | #EEF2F9 | 11.65:1 | ≥4.5 | 通过 |
| sidebar-primary-foreground / sidebar-primary | #FFFFFF | #0035FC | 7.20:1 | ≥4.5 | 通过 |
| sidebar-accent-foreground / sidebar-accent | #0029C4 | #DCE5FF | 7.90:1 | ≥4.5 | 通过 |
| foreground / table-header | #0E1630 | #F2F4F9 | 16.23:1 | ≥4.5 | 通过 |
| foreground / table-disabled | #0E1630 | #EEF0F5 | 15.69:1 | ≥4.5 | 通过 |
| muted-foreground / table-disabled | #4D5975 | #EEF0F5 | 6.15:1 | ≥4.5 | 通过 |
| foreground / skeleton-base | #0E1630 | #E6E9EF | 14.64:1 | ≥4.5 | 通过 |
| input / card | #808DA9 | #FFFFFF | 3.33:1 | ≥3（非文本） | 通过 |
| input / background | #808DA9 | #F6F8FC | 3.13:1 | ≥3（非文本） | 通过 |
| ring / card | #0035FC | #FFFFFF | 7.20:1 | ≥3（非文本） | 通过 |
| ring / background | #0035FC | #F6F8FC | 6.77:1 | ≥3（非文本） | 通过 |
| sidebar-ring / sidebar | #0035FC | #EEF2F9 | 6.42:1 | ≥3（非文本） | 通过 |
| table-disabled-border / background | #5B6782 | #F6F8FC | 5.33:1 | ≥3（非文本） | 通过 |
| telemetry / card | #00CFFC | #FFFFFF | 1.85:1 | ≥3（非文本） | **不通过** |
| chart-1 / card | #0035FC | #FFFFFF | 7.20:1 | ≥3（非文本） | 通过 |
| chart-2 / card | #B84A0C | #FFFFFF | 5.22:1 | ≥3（非文本） | 通过 |
| chart-3 / card | #0089AD | #FFFFFF | 4.05:1 | ≥3（非文本） | 通过 |
| chart-4 / card | #7442D6 | #FFFFFF | 6.01:1 | ≥3（非文本） | 通过 |
| chart-5 / card | #A8326E | #FFFFFF | 6.28:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 组合 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| border / card | #DCE2EE | #FFFFFF | 1.30:1 | ≥3 | **不通过** |
| chart-1 / card | #0035FC | #FFFFFF | 7.20:1 | ≥4.5 | 通过 |
| chart-2 / card | #B84A0C | #FFFFFF | 5.22:1 | ≥4.5 | 通过 |
| chart-3 / card | #0089AD | #FFFFFF | 4.05:1 | ≥4.5 | **不通过** |
| chart-4 / card | #7442D6 | #FFFFFF | 6.01:1 | ≥4.5 | 通过 |
| chart-5 / card | #A8326E | #FFFFFF | 6.28:1 | ≥4.5 | 通过 |

## MyAPI 默认 · 暗色

| 组合（前景 / 背景） | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| foreground / background | #E8EDF7 | #0A0F1E | 16.26:1 | ≥4.5 | 通过 |
| foreground / card | #E8EDF7 | #121A2C | 14.78:1 | ≥4.5 | 通过 |
| card-foreground / card | #E8EDF7 | #121A2C | 14.78:1 | ≥4.5 | 通过 |
| popover-foreground / popover | #E8EDF7 | #162036 | 13.82:1 | ≥4.5 | 通过 |
| muted-foreground / background | #9BA7C0 | #0A0F1E | 7.89:1 | ≥4.5 | 通过 |
| muted-foreground / card | #9BA7C0 | #121A2C | 7.18:1 | ≥4.5 | 通过 |
| muted-foreground / muted | #9BA7C0 | #162036 | 6.71:1 | ≥4.5 | 通过 |
| primary-foreground / primary | #070B16 | #7B95FF | 7.10:1 | ≥4.5 | 通过 |
| primary-foreground / myapi-brand-strong | #070B16 | #8BA6FF | 8.44:1 | ≥4.5 | 通过 |
| primary / background | #7B95FF | #0A0F1E | 6.90:1 | ≥4.5 | 通过 |
| primary / card | #7B95FF | #121A2C | 6.27:1 | ≥4.5 | 通过 |
| secondary-foreground / secondary | #C9D2E6 | #1A2338 | 10.32:1 | ≥4.5 | 通过 |
| accent-foreground / accent | #C9D5FF | #1C2752 | 9.88:1 | ≥4.5 | 通过 |
| destructive-foreground / destructive | #1A0503 | #FF8A80 | 8.62:1 | ≥4.5 | 通过 |
| destructive / card | #FF8A80 | #121A2C | 7.60:1 | ≥4.5 | 通过 |
| destructive / destructive-muted | #FF8A80 | #322A39 | 6.03:1 | ≥4.5 | 通过 |
| success-foreground / success | #0A0F1E | #4ADE9B | 11.10:1 | ≥4.5 | 通过 |
| success / card | #4ADE9B | #121A2C | 10.10:1 | ≥4.5 | 通过 |
| success / success-muted | #4ADE9B | #1B343C | 7.62:1 | ≥4.5 | 通过 |
| warning-foreground / warning | #0A0F1E | #F2B544 | 10.43:1 | ≥4.5 | 通过 |
| warning / card | #F2B544 | #121A2C | 9.48:1 | ≥4.5 | 通过 |
| warning / warning-muted | #F2B544 | #2E2F35 | 7.28:1 | ≥4.5 | 通过 |
| info-foreground / info | #0A0F1E | #5CC8FF | 10.15:1 | ≥4.5 | 通过 |
| info / card | #5CC8FF | #121A2C | 9.23:1 | ≥4.5 | 通过 |
| info / info-muted | #5CC8FF | #1D3047 | 7.13:1 | ≥4.5 | 通过 |
| neutral-foreground / neutral | #0A0F1E | #9BA7C0 | 7.89:1 | ≥4.5 | 通过 |
| neutral / card | #9BA7C0 | #121A2C | 7.18:1 | ≥4.5 | 通过 |
| telemetry-foreground / telemetry | #04121F | #22D3FF | 10.64:1 | ≥4.5 | 通过 |
| sidebar-foreground / sidebar | #C9D2E6 | #070B16 | 12.96:1 | ≥4.5 | 通过 |
| sidebar-primary-foreground / sidebar-primary | #070B16 | #7B95FF | 7.10:1 | ≥4.5 | 通过 |
| sidebar-accent-foreground / sidebar-accent | #C9D5FF | #1C2752 | 9.88:1 | ≥4.5 | 通过 |
| foreground / table-header | #E8EDF7 | #141A29 | 14.83:1 | ≥4.5 | 通过 |
| foreground / table-disabled | #E8EDF7 | #161B2B | 14.55:1 | ≥4.5 | 通过 |
| muted-foreground / table-disabled | #9BA7C0 | #161B2B | 7.06:1 | ≥4.5 | 通过 |
| foreground / skeleton-base | #E8EDF7 | #181D2D | 14.27:1 | ≥4.5 | 通过 |
| input / card | #5F6C8C | #121A2C | 3.32:1 | ≥3（非文本） | 通过 |
| input / background | #5F6C8C | #0A0F1E | 3.65:1 | ≥3（非文本） | 通过 |
| ring / card | #8EA5FF | #121A2C | 7.42:1 | ≥3（非文本） | 通过 |
| ring / background | #8EA5FF | #0A0F1E | 8.16:1 | ≥3（非文本） | 通过 |
| sidebar-ring / sidebar | #8EA5FF | #070B16 | 8.40:1 | ≥3（非文本） | 通过 |
| table-disabled-border / background | #9BA7C0 | #0A0F1E | 7.89:1 | ≥3（非文本） | 通过 |
| telemetry / card | #22D3FF | #121A2C | 9.78:1 | ≥3（非文本） | 通过 |
| chart-1 / card | #5F7DFF | #121A2C | 4.85:1 | ≥3（非文本） | 通过 |
| chart-2 / card | #C9661E | #121A2C | 4.47:1 | ≥3（非文本） | 通过 |
| chart-3 / card | #0099C2 | #121A2C | 5.24:1 | ≥3（非文本） | 通过 |
| chart-4 / card | #9B7BF0 | #121A2C | 5.36:1 | ≥3（非文本） | 通过 |
| chart-5 / card | #D9558F | #121A2C | 4.67:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 组合 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| border / card | #26314A | #121A2C | 1.34:1 | ≥3 | **不通过** |
| chart-1 / card | #5F7DFF | #121A2C | 4.85:1 | ≥4.5 | 通过 |
| chart-2 / card | #C9661E | #121A2C | 4.47:1 | ≥4.5 | **不通过** |
| chart-3 / card | #0099C2 | #121A2C | 5.24:1 | ≥4.5 | 通过 |
| chart-4 / card | #9B7BF0 | #121A2C | 5.36:1 | ≥4.5 | 通过 |
| chart-5 / card | #D9558F | #121A2C | 4.67:1 | ≥4.5 | 通过 |

## 高对比 · 亮色

| 组合（前景 / 背景） | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| foreground / background | #05091A | #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| foreground / card | #05091A | #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| card-foreground / card | #05091A | #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| popover-foreground / popover | #05091A | #FFFFFF | 19.80:1 | ≥4.5 | 通过 |
| muted-foreground / background | #2B3550 | #FFFFFF | 12.16:1 | ≥4.5 | 通过 |
| muted-foreground / card | #2B3550 | #FFFFFF | 12.16:1 | ≥4.5 | 通过 |
| muted-foreground / muted | #2B3550 | #E6EBF5 | 10.17:1 | ≥4.5 | 通过 |
| primary-foreground / primary | #FFFFFF | #0029C4 | 9.93:1 | ≥4.5 | 通过 |
| primary-foreground / myapi-brand-strong | #FFFFFF | #0724A2 | 11.79:1 | ≥4.5 | 通过 |
| primary / background | #0029C4 | #FFFFFF | 9.93:1 | ≥4.5 | 通过 |
| primary / card | #0029C4 | #FFFFFF | 9.93:1 | ≥4.5 | 通过 |
| secondary-foreground / secondary | #05091A | #E6EBF5 | 16.57:1 | ≥4.5 | 通过 |
| accent-foreground / accent | #001A8C | #D6E0FF | 10.46:1 | ≥4.5 | 通过 |
| destructive-foreground / destructive | #FFFFFF | #A3180F | 7.79:1 | ≥4.5 | 通过 |
| destructive / card | #A3180F | #FFFFFF | 7.79:1 | ≥4.5 | 通过 |
| destructive / destructive-muted | #A3180F | #FDECEA | 6.81:1 | ≥4.5 | 通过 |
| success-foreground / success | #FFFFFF | #05603A | 7.66:1 | ≥4.5 | 通过 |
| success / card | #05603A | #FFFFFF | 7.66:1 | ≥4.5 | 通过 |
| success / success-muted | #05603A | #E6F6EE | 6.85:1 | ≥4.5 | 通过 |
| warning-foreground / warning | #FFFFFF | #6E3F00 | 8.85:1 | ≥4.5 | 通过 |
| warning / card | #6E3F00 | #FFFFFF | 8.85:1 | ≥4.5 | 通过 |
| warning / warning-muted | #6E3F00 | #FFF4DC | 8.10:1 | ≥4.5 | 通过 |
| info-foreground / info | #FFFFFF | #004B6E | 9.41:1 | ≥4.5 | 通过 |
| info / card | #004B6E | #FFFFFF | 9.41:1 | ≥4.5 | 通过 |
| info / info-muted | #004B6E | #E3F7FF | 8.52:1 | ≥4.5 | 通过 |
| neutral-foreground / neutral | #FFFFFF | #3D4966 | 8.96:1 | ≥4.5 | 通过 |
| neutral / card | #3D4966 | #FFFFFF | 8.96:1 | ≥4.5 | 通过 |
| telemetry-foreground / telemetry | #0B1530 | #00CFFC | 9.73:1 | ≥4.5 | 通过 |
| sidebar-foreground / sidebar | #05091A | #E6EBF5 | 16.57:1 | ≥4.5 | 通过 |
| sidebar-primary-foreground / sidebar-primary | #FFFFFF | #0029C4 | 9.93:1 | ≥4.5 | 通过 |
| sidebar-accent-foreground / sidebar-accent | #001A8C | #D6E0FF | 10.46:1 | ≥4.5 | 通过 |
| foreground / table-header | #05091A | #FBFBFB | 19.09:1 | ≥4.5 | 通过 |
| foreground / table-disabled | #05091A | #F6F6F7 | 18.39:1 | ≥4.5 | 通过 |
| muted-foreground / table-disabled | #2B3550 | #F6F6F7 | 11.29:1 | ≥4.5 | 通过 |
| foreground / skeleton-base | #05091A | #EDEEF0 | 17.05:1 | ≥4.5 | 通过 |
| input / card | #3D4966 | #FFFFFF | 8.96:1 | ≥3（非文本） | 通过 |
| input / background | #3D4966 | #FFFFFF | 8.96:1 | ≥3（非文本） | 通过 |
| ring / card | #0029C4 | #FFFFFF | 9.93:1 | ≥3（非文本） | 通过 |
| ring / background | #0029C4 | #FFFFFF | 9.93:1 | ≥3（非文本） | 通过 |
| sidebar-ring / sidebar | #0029C4 | #E6EBF5 | 8.31:1 | ≥3（非文本） | 通过 |
| table-disabled-border / background | #3D4966 | #FFFFFF | 8.96:1 | ≥3（非文本） | 通过 |
| telemetry / card | #00CFFC | #FFFFFF | 1.85:1 | ≥3（非文本） | **不通过** |
| chart-1 / card | #0035FC | #FFFFFF | 7.20:1 | ≥3（非文本） | 通过 |
| chart-2 / card | #B84A0C | #FFFFFF | 5.22:1 | ≥3（非文本） | 通过 |
| chart-3 / card | #0089AD | #FFFFFF | 4.05:1 | ≥3（非文本） | 通过 |
| chart-4 / card | #7442D6 | #FFFFFF | 6.01:1 | ≥3（非文本） | 通过 |
| chart-5 / card | #A8326E | #FFFFFF | 6.28:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 组合 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| border / card | #5B6782 | #FFFFFF | 5.67:1 | ≥3 | 通过 |
| chart-1 / card | #0035FC | #FFFFFF | 7.20:1 | ≥4.5 | 通过 |
| chart-2 / card | #B84A0C | #FFFFFF | 5.22:1 | ≥4.5 | 通过 |
| chart-3 / card | #0089AD | #FFFFFF | 4.05:1 | ≥4.5 | **不通过** |
| chart-4 / card | #7442D6 | #FFFFFF | 6.01:1 | ≥4.5 | 通过 |
| chart-5 / card | #A8326E | #FFFFFF | 6.28:1 | ≥4.5 | 通过 |

## 高对比 · 暗色

| 组合（前景 / 背景） | 前景 | 背景 | 对比 | 要求 | 结果 |
| --- | --- | --- | --- | --- | --- |
| foreground / background | #FFFFFF | #02040A | 20.50:1 | ≥4.5 | 通过 |
| foreground / card | #FFFFFF | #0B1020 | 18.93:1 | ≥4.5 | 通过 |
| card-foreground / card | #FFFFFF | #0B1020 | 18.93:1 | ≥4.5 | 通过 |
| popover-foreground / popover | #FFFFFF | #141B30 | 17.08:1 | ≥4.5 | 通过 |
| muted-foreground / background | #C9D2E6 | #02040A | 13.51:1 | ≥4.5 | 通过 |
| muted-foreground / card | #C9D2E6 | #0B1020 | 12.48:1 | ≥4.5 | 通过 |
| muted-foreground / muted | #C9D2E6 | #141B30 | 11.26:1 | ≥4.5 | 通过 |
| primary-foreground / primary | #02040A | #A9BBFF | 10.97:1 | ≥4.5 | 通过 |
| primary-foreground / myapi-brand-strong | #02040A | #B8C7FF | 12.39:1 | ≥4.5 | 通过 |
| primary / background | #A9BBFF | #02040A | 10.97:1 | ≥4.5 | 通过 |
| primary / card | #A9BBFF | #0B1020 | 10.13:1 | ≥4.5 | 通过 |
| secondary-foreground / secondary | #FFFFFF | #141B30 | 17.08:1 | ≥4.5 | 通过 |
| accent-foreground / accent | #FFFFFF | #1A2550 | 14.74:1 | ≥4.5 | 通过 |
| destructive-foreground / destructive | #02040A | #FFB3AB | 11.99:1 | ≥4.5 | 通过 |
| destructive / card | #FFB3AB | #0B1020 | 11.08:1 | ≥4.5 | 通过 |
| destructive / destructive-muted | #FFB3AB | #322A39 | 8.05:1 | ≥4.5 | 通过 |
| success-foreground / success | #02040A | #7DF0BC | 14.71:1 | ≥4.5 | 通过 |
| success / card | #7DF0BC | #0B1020 | 13.58:1 | ≥4.5 | 通过 |
| success / success-muted | #7DF0BC | #1B343C | 9.40:1 | ≥4.5 | 通过 |
| warning-foreground / warning | #02040A | #FFD27A | 14.40:1 | ≥4.5 | 通过 |
| warning / card | #FFD27A | #0B1020 | 13.30:1 | ≥4.5 | 通过 |
| warning / warning-muted | #FFD27A | #2E2F35 | 9.37:1 | ≥4.5 | 通过 |
| info-foreground / info | #02040A | #9ADCFF | 13.72:1 | ≥4.5 | 通过 |
| info / card | #9ADCFF | #0B1020 | 12.67:1 | ≥4.5 | 通过 |
| info / info-muted | #9ADCFF | #1D3047 | 8.98:1 | ≥4.5 | 通过 |
| neutral-foreground / neutral | #02040A | #C9D2E6 | 13.51:1 | ≥4.5 | 通过 |
| neutral / card | #C9D2E6 | #0B1020 | 12.48:1 | ≥4.5 | 通过 |
| telemetry-foreground / telemetry | #04121F | #22D3FF | 10.64:1 | ≥4.5 | 通过 |
| sidebar-foreground / sidebar | #FFFFFF | #02040A | 20.50:1 | ≥4.5 | 通过 |
| sidebar-primary-foreground / sidebar-primary | #02040A | #A9BBFF | 10.97:1 | ≥4.5 | 通过 |
| sidebar-accent-foreground / sidebar-accent | #FFFFFF | #1A2550 | 14.74:1 | ≥4.5 | 通过 |
| foreground / table-header | #FFFFFF | #090D15 | 19.40:1 | ≥4.5 | 通过 |
| foreground / table-disabled | #FFFFFF | #0B0F17 | 19.14:1 | ≥4.5 | 通过 |
| muted-foreground / table-disabled | #C9D2E6 | #0B0F17 | 12.62:1 | ≥4.5 | 通过 |
| foreground / skeleton-base | #FFFFFF | #0D1119 | 18.86:1 | ≥4.5 | 通过 |
| input / card | #A3AEC6 | #0B1020 | 8.50:1 | ≥3（非文本） | 通过 |
| input / background | #A3AEC6 | #02040A | 9.20:1 | ≥3（非文本） | 通过 |
| ring / card | #C9D5FF | #0B1020 | 13.02:1 | ≥3（非文本） | 通过 |
| ring / background | #C9D5FF | #02040A | 14.10:1 | ≥3（非文本） | 通过 |
| sidebar-ring / sidebar | #C9D5FF | #02040A | 14.10:1 | ≥3（非文本） | 通过 |
| table-disabled-border / background | #C9D2E6 | #02040A | 13.51:1 | ≥3（非文本） | 通过 |
| telemetry / card | #22D3FF | #0B1020 | 10.67:1 | ≥3（非文本） | 通过 |
| chart-1 / card | #5F7DFF | #0B1020 | 5.29:1 | ≥3（非文本） | 通过 |
| chart-2 / card | #C9661E | #0B1020 | 4.87:1 | ≥3（非文本） | 通过 |
| chart-3 / card | #0099C2 | #0B1020 | 5.71:1 | ≥3（非文本） | 通过 |
| chart-4 / card | #9B7BF0 | #0B1020 | 5.85:1 | ≥3（非文本） | 通过 |
| chart-5 / card | #D9558F | #0B1020 | 5.09:1 | ≥3（非文本） | 通过 |

仅供参考（不计入通过率）：

| 组合 | 前景 | 背景 | 对比 | 参考线 | 结果 |
| --- | --- | --- | --- | --- | --- |
| border / card | #8592AD | #0B1020 | 6.05:1 | ≥3 | 通过 |
| chart-1 / card | #5F7DFF | #0B1020 | 5.29:1 | ≥4.5 | 通过 |
| chart-2 / card | #C9661E | #0B1020 | 4.87:1 | ≥4.5 | 通过 |
| chart-3 / card | #0099C2 | #0B1020 | 5.71:1 | ≥4.5 | 通过 |
| chart-4 / card | #9B7BF0 | #0B1020 | 5.85:1 | ≥4.5 | 通过 |
| chart-5 / card | #D9558F | #0B1020 | 5.09:1 | ≥4.5 | 通过 |

Logo 青 #00CFFC 作文字 / 白：1.85:1，不通过，所以规定青色（--telemetry）不作文字色。

汇总：计入通过率的组合 188 个，不通过 2 个。

## 说明

- 不通过的 2 个都是亮色（默认与高对比）下 `--telemetry`（Logo 青 #00CFFC）相对卡片 1.85:1。这是 DESIGN_DIRECTION 4.1/4.2 已确认的取值；本次没有改动它，而是在 token 注释里规定"遥测填充必须同时给出文字数值"。若要求遥测图形本身达到 3:1，需要维护者决定改用更深的青（例如图表第 3 色 #0089AD，4.05:1）。
- 与提议表的差异：`--input` 亮色提议值 #8592AD 在 `--background` 上只有 2.94:1，已改为 #808DA9（背景 3.13:1、卡片 3.33:1）；`--table-disabled-border`（停用行左侧 4px 标记，属于状态指示）原为 16%/24% 混色，只有 1.46:1 / 1.74:1，已改为引用 `--neutral`。
- 暗色 `*-muted` 按"状态色 15% 混入 `--card`"在 OKLab 中计算后写死（在 OKLCH 中按色相插值会把色相拉向卡片的藏青，失去状态色）。暗色 warning-muted 因此接近中性灰 #2E2F35，文字对比 7.28:1。
- "仅供参考"表中的 `chart-* / card` 按 4.5:1 计，用于提醒旧组件（`components/status-badge.tsx`、`lib/colors.ts`）把图表色当文字色；按 dataviz 规则文字不应使用系列色。
- 修改前的不达标组合（DESIGN_DIRECTION 2.6 V2 与 4.6 所列：亮色 success 文字 3.67、warning 文字 2.93、info 白字 3.85、success 白字 3.51、input 1.58、ring 2.58；暗色 destructive 白字 2.77、sidebar-primary 白字 4.11）在上表对应行均已通过。
