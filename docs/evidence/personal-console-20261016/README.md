# beta.16 个人控制台实图与运行证据

应用源码 `630f905b4d19758d2eab236808b6867bcbc0a37b`；CI checkout `b34cc0055cdae0150121f40ee50139e58d75006a`，文件树均为 `57e3722edfad7138a14600b5efc2823c0fce2e6b`。[准确 CI](https://github.com/ForceMind/MyAPI/actions/runs/38787088937)。候选版本 `0.2.0-beta.16`，未发布或部署。

本目录根部 PNG 是上述生产前端构建在实际 Chromium 页面中的原始截图，使用明确合成 API。未缩放、裁切或美化；截图中的账号、Key 和数值不属于生产。只读本地预览 `http://127.0.0.1:4173/upstream-accounts` 也使用此构建与合成数据，选择示例 Key 后查看窗口；业务写入返回 405，不连接 Go 或生产数据库。完整浏览器原图 artifact 只保留 7 天，本目录保留代表原图和报告供长期复核。每个文件的 SHA256、尺寸和出处见 [manifest.json](manifest.json)。

## 桌面和手机

| 页面或状态 | 桌面 | 手机 |
| --- | --- | --- |
| 概览外壳 | [1440](personal-zh-1440-overview.png) | [390](personal-zh-390-overview.png) |
| 上游账号与窗口入口 | [1440](personal-zh-1440-upstream.png) | [320](personal-zh-320-upstream.png) |
| 原生窗口完整用量分解 | [1440](personal-zh-1440-upstream-pool-summary.png) | [390](personal-zh-390-upstream-pool-summary.png) |
| 全池全部 Key 与管理动作 | [1440](personal-zh-1440-upstream-pool-actions.png) | [390](personal-zh-390-upstream-pool-actions.png) |
| 个人资料 | [1440](console-profile-zh-1440.png) | [390](console-profile-zh-390.png) |
| 完整回答、Token 未知 | [1440](console-success-unknown-zh-1440.png) | [390 用量卡](console-success-unknown-usage-zh-390.png) |
| Key / 分额入口 | [1280 编辑](subscription-share-zh-1280-root-pool.png) | [320 列表](subscription-share-zh-320-root-key-list.png) |

## 边界与恢复

- [缺失初始基线](personal-en-320-pool-baseline_unknown-pool-summary.png)、[不一致证据](personal-en-320-pool-inconsistent-pool-summary.png)：合法值保留，未知不补零，不画完整分解。
- [真实零配置保留历史用量](personal-en-320-pool-zero-share-pool-key-2.png)、[已删除 Key](personal-en-320-pool-zero-share-pool-key-2.png)：配置比例与用量分别显示。
- [HTTP 失败](personal-en-320-pool-error.png)、[超时后恢复](personal-en-320-pool-timeout-recovered-pool-summary.png)、[失败后恢复](personal-en-320-pool-recovered-pool-summary.png)、[资料失败](console-profile-error-zh-390.png)。
- [Enter 进入 Key 管理](personal-en-320-pool-manage-keyboard.png)：驱动验证聚焦后 Enter 激活，不代表完整 Tab 顺序或全部辅助技术已验证。
- [320 暗色](personal-zh-320-dark-upstream-pool-summary.png)、[320 越南文完整分解](personal-vi-320-upstream-pool-summary.png)、[320 越南文管理区](personal-vi-320-upstream-pool-actions.png)。
- [普通 owner 分额视图](subscription-share-zh-320-owner-pool.png)：只读合同与 Root 全池权限由 API/组件测试另行验证，截图不单独证明权限安全。

## 可复验报告

- [概览与上游报告](personal-workbench-qualification.json)：27 个旅程，七语言、手机/桌面、失败/未知/恢复、池与会话关联、键盘重试/导航；几何与采样对比度断言随报告保留。
- [资料与调用报告](personal-console-qualification.json)：16 个旅程，成功且缺 usage、真实零与失败区分、详情及资料恢复。
- [Keys / 分额报告](subscription-share-qualification.json)：28 个旅程，Root/owner、配置、保存、关闭及到限等现有流程。
- [单元汇总](frontend-test-summary.txt)：226 文件、1447 项全部通过。24 项新增 Key 会话测试包括真实 Axios 401 不重放；36 项新增窗口观测测试覆盖数据契约和组件行为。[8 项旧源码回归](key-maintenance-baseline-red-summary.json)取得预期失败，说明所修缺陷可复现。
- [六个兼容浏览器报告汇总](compatibility-browser-summary.json)：共享页面、设置、角色、资料失败恢复、安装入口及 Playground 的既有旅程；保留原报告哈希，不外推生产。
- [隔离真实应用](runtime/README.md)：固定另一准确源码 `78c617b`，新的临时 SQLite、真实 Go/前端登录和有限 Key 写入、回环测试上游。使用 reported usage；不能把普通 Key 配额拒绝当作原生窗口分额拒绝，也不覆盖缺 usage。最后前端守卫修订单独由本目录源码验证，后端等运行输入相同才复用此证据。

非实现者分别复核代码、数据语义及代表原图，完整恢复分解条的证据缺口已补齐；可见范围无未解决 P1/P2。首轮类型、分发版本和第二轮 lint 失败及修正均保留在 [执行记录](../../PERSONAL_CONSOLE_REMAINDER_EXECUTION_20261016.md)。历史账务、生产数据、凭据与备份继续冻结，不从这些合成或隔离验证推断生产已修复。
