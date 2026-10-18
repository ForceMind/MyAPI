# beta.20 维护会话证据

候选源码 `1dc806ee369753ebf9776e2e57eb019c4b88b488`，CI checkout `109907180d4ed124c8d081c15f88a508ba98772b`，相同tree `6a65752cdb868f902c1e8c22e7a0200a93a777b3`。来源为 [CI39148878978](https://github.com/ForceMind/MyAPI/actions/runs/39148878978) 的 `myapi-credential-maintenance-109907180d4ed124c8d081c15f88a508ba98772b` 原始制品；最终11/11作业success，新12旅程/38图及独立审查通过。

验证仅使用隔离合成API、临时SQLite和回环测试环境；本目录不含数据库副本或原始服务日志，不包含真实凭据，也没有真实上游调用。原PNG未裁切、编辑、重画或重编码。截图中只有明确的synthetic身份和Key名称。

## 最终原图与报告

`final/` 包含38张原PNG及同次 [12旅程报告](final/maintenance-session-qualification.json)。英文1440×1000、中文390×900分别覆盖单删、预算、Key策略的正常成功和发送后离开页面：delete10图、budget14图、policy14图。8张header补充标题/说明，review和success显示动作区；navigated/settled显示离开后的个人资料。

- 桌面：[删除确认](final/maintenance-delete-success-en-1440-review.png)、[预算标题](final/maintenance-budget-success-en-1440-header.png)、[策略成功](final/maintenance-policy-success-en-1440-success.png)。
- 手机：[删除成功](final/maintenance-delete-success-zh-390-success.png)、[预算标题](final/maintenance-budget-success-zh-390-header.png)、[策略成功](final/maintenance-policy-success-zh-390-success.png)、[离开后无旧反馈](final/maintenance-delete-navigate-away-zh-390-settled.png)。

每条旅程写入一次，认证刷新数不增加；delete确实交付旧401，budget/policy确实由导航取消请求。策略保存后owner/admin精确两处显示已分配。上述动态结论来自驱动断言；PNG只证明拍摄时可见布局与状态。

[独立审查](review-summary.json)记录非实现者逐张实际查看最终38/38及全部12旅程，主代理另看4张最终代表图；无P1/P2。budget标题图有右上关闭X，policy保持既有底部关闭按钮，不能把截图滚动位置误称为标题缺失。

## 中间候选与生成摘要

`intermediate-71ae535/`保留两个旧策略成功原图及其原报告。来源CI39148275311后来被后继运行自动取消；原driver报告12旅程通过，实图审查却发现owner GET fixture始终未分配，故这些文件不是最终验收。修正夹具而非产品后重新取得上方准确原图，不修改旧证据。

[原始文件索引](original-artifact-manifest.json)由主代理生成，列出42个原始文件：最终38PNG+1JSON、中间2PNG+1JSON；逐个记录原制品路径、checkout、运行、字节数与SHA256，复制后均验证一致。索引本身不是Actions原制品。

原始文件索引及[单元摘要](unit-test-summary.json)、[审查摘要](review-summary.json)、[预览核验](preview-verification.json)均由主代理生成，不能称作原始运行报告。本片前端243文件/1562单元通过，其中新增22项；本机无Vitest/Chromium安装，相关完整验证在Actions执行。

[基线实红摘要](baseline-red-summary.json)记录固定旧源码的十个预期失败用例及原报告SHA256；完整旧源码报告在同次 `myapi-ui-browser-109907180d4ed124c8d081c15f88a508ba98772b` 制品。该摘要与[最终CI来源核验](verification-summary.json)也由主代理生成，和原始文件分别分类。

## 实际运行

[4177合成预览](http://127.0.0.1:4177/keys)服务同次准确前端，HTML与下载产物逐字一致，200/no-store。仅临时内存假Key，可演示本片维护行为；不连接Go、数据库或provider。本机核验范围为HTTP，实际Chromium运行和原图由Actions提供。服务依赖当前环境会话，不是部署或长期托管。

完整验收、范围及失败修正见[交付记录](../../PERSONAL_CONSOLE_SESSION_DELIVERY_20261019.md)和[执行记录](../../PERSONAL_CONSOLE_SESSION_EXECUTION_20261019.md)。历史账务、生产数据/凭据/Redis/备份冻结，不合并main、不发布、不部署。
