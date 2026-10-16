# 个人控制台运行证据

状态：已归档 22 张个人页面原始运行截图、2 张隔离真实应用截图及对应报告。未拼接、缩放或修改截图；运行驱动规定的遮敏保留。

应用候选 `0.2.0-beta.15`。前端源码 `c0e210377f1edb3ef1d892b085904c3456e970c6`，对应 CI 引用 `dd3859d9cf86189072fab25d0ee840d626d86c36`，二者文件树均为 `853a276b15feaf734b91119ecaf5ee76d201dacd`。截图与页面完整 revision 使用 CI 引用；它是自动验证引用，本批没有人工合并。

- [普通 CI 与个人页面](https://github.com/ForceMind/MyAPI/actions/runs/38768659712)：全部 10 项作业成功，包含后端/三库/Redis/race、分发合同和六个兼容浏览器脚本。个人页面使用真实 production build 与 Chromium，API 为隔离合成响应，禁止外部网络。221 个文件/1387 项单元测试、类型、lint、构建通过；概览/上游 27、资料/日志 16、分额 28 个旅程全过，产出 220 张图，选取 22 张入库。涵盖七语言、320/390/1280/1440、明暗、权限、显式 Key 交接、失败重试、成功缺 usage、Key 分额及键盘提交。
- [真实应用隔离验证](https://github.com/ForceMind/MyAPI/actions/runs/38761929914)：已通过，准确源码 `e8376f7054a9f61629bba9260862902ee48b23d5`。legacy/authoritative 两种 writer、Root/普通 owner、真实 UI 的五次 POST、回环测试上游、拒绝不增加上游请求、精确日志/计数和刷新均完成。使用全新 SQLite 与临时 CI 凭据；截图遮敏为驱动规定行为。

`e8376f7` 到 `c0e2103` 没有后端、Go 依赖、relaykit、VERSION 或 runtime 驱动变化；真实应用报告只声明它实际验证的源码，后继表单/标签/样式和取消边界由本轮单元与 Chromium 验证。

## 选图

| 页面或状态 | 桌面 | 手机 |
| --- | --- | --- |
| 概览 | [1280](personal-zh-1280-overview.png) | [320](personal-zh-320-overview.png) |
| 上游账号 | [1280](personal-zh-1280-upstream.png) | [320](personal-zh-320-upstream.png)、[390 陈旧窗口](personal-zh-390-upstream-stale.png) |
| Key 列表 | [1280](subscription-share-zh-1280-root-key-list.png) | [320](subscription-share-zh-320-root-key-list.png) |
| 分额编辑 | [1280](subscription-share-zh-1280-root-pool.png) | [输入区](subscription-share-zh-320-root-pool.png)、[预览与保存区](subscription-share-zh-320-root-pool-actions.png) |
| 个人资料 | [1280](console-profile-zh-1280.png) | [320](console-profile-zh-320.png)、[越南语长文](console-profile-vi-320.png) |
| 资料读取失败 | [1280](console-profile-error-zh-1280.png) | [320](console-profile-error-zh-320.png) |
| 成功且缺 usage | [1280](console-success-unknown-zh-1280.png) | [详情首屏](console-success-unknown-zh-320.png)、[Token 卡滚动状态](console-success-unknown-usage-zh-320.png) |
| 深色 | [概览](personal-en-1280-dark-overview.png)、[上游](personal-en-1280-dark-upstream.png) | 报告亦覆盖中文 320 深色 |
| 独立模块重试 | 报告记录 | [失败](personal-retry-zh-320-errors.png)、[恢复](personal-retry-zh-320-recovered.png) |
| 隔离真实应用 | [实际调用记录](runtime-e837-desktop-call.png) | [实际调用记录](runtime-e837-mobile-call.png) |

截图显示各自的真实滚动位置，不能把单张首屏当作全页。手机保存区图中未勾近似确认，保存禁用符合预期；七语言实际 Enter 和单次写入另由浏览器操作报告证明。桌面保存区位于首屏外，点击操作已通过，但该首屏图不证明底部完整布局。

非实现者独立实际审查最终五页面、深色、错误恢复、越南语长文和手机操作区，可见范围无 P1/P2。状态文字采样 62 项：浅色 56 项最低 5.35:1，深色 6 项最低 8.03:1，均达 4.5:1；只涵盖报告中的采样文字。

`verification-summary.json` 汇总检查，三份 `*-qualification.json` 保留完整个人旅程；`runtime-legacy.json` 与 `runtime-authoritative.json` 保留实际 writer 结果。`manifest.json` 记录入库文件原始 SHA256、PNG 尺寸和来源；报告指向的其余截图在 CI 完整产物中保留七天，未全部复制入库。

完整 API 与设计差距见 [交付记录](../../PERSONAL_CONSOLE_DELIVERY_20261015.md)。本目录不证明生产部署、真实外部上游、历史账务修复或物理手机安装。
