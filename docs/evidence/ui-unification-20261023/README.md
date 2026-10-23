# 控制台其余页面 UI 统一（2026-10-23，北京时间）

本批继续在 `dev` 开发，基于 `6efd467255c5637277dbaffc1584c682503b056b` 及上一轮 beta.26 未提交工作区。没有提交、推送、合入 main 或部署生产，也没有改变生产配置、账务或供应商调用。上一轮后端修改和真实请求日志包保留。

## 查看入口

[打开可操作预览](http://127.0.0.1:4184/__preview)。入口包含概览、上游账号、模型、模型调用分析、个人资料、系统信息、站点设置，以及上一轮 Key → 请求 → 诊断 → 正文流程。

全部为合成身份、账号窗口和正文，只读；账号窗口不是实际供应商额度。设置页可以查看，预览拒绝业务写入。会话内已回读入口、概览和合成窗口接口均为 HTTP200，带 `synthetic-readonly` 标识：[回读记录](preview-readback.json)。不保证环境关闭后服务继续运行。

冻结构建在 `.local-tests/ui-unification-20261023/preview-dist`，对应 [构建 SHA256](build-manifest.json)；[本批源码 SHA256](source-manifest.json)区分未提交候选与 Git HEAD。重启命令：

```sh
MYAPI_PREVIEW_PORT=4184 MYAPI_UI_DIST="$PWD/.local-tests/ui-unification-20261023/preview-dist" node tools/ui/page-layout-preview.mjs
```

## 实际变化

- 概览和上游账号接入共用页面布局。标题距页面顶部统一24px，正文与标题使用相同水平边距；上游账号移除重复标题行，保留账号选择、刷新和连接管理。
- 概览卡片统一间距及排列；账号窗口与连接、Key 卡片保持边界对齐。资料页取消额外的独立最大宽度，使其跟随页面内容边界；卡片间距统一。
- 共用列表筛选工具栏允许操作组自然换行，手机输入框和按钮提供一致的最小尺寸；长语言操作按钮可换行，设置控件和分页沿用同一页面边距。
- 分析统计卡使用明确的分隔和留白。手机图表切换组随40px按钮扩展高度，避免原固定高度裁切按钮；页内标签组同样随内容扩展。
- 正文日志统计在常见手机宽度并排，数字使用紧凑字号；列表固定操作列的表头与单元格一同固定，横向滚动时保持对齐。
- 系统信息、模型、分析、正文日志、资料和设置页通过共用规则统一标题、内容、输入控件与手机操作。保留现有文案及7种语言翻译，没有新增产品文案或翻译键。

## 验证

| 范围 | 证据 |
| --- | --- |
| 受影响前端 | [31文件185项通过](regression.log)，最终表格行/筛选[4项通过](final-row-regression.log)，图表时间粒度[1项通过](chart-regression.log)；后两者为局部改动后的复测，不按独立新增测试累计 |
| 类型、lint、构建 | 最终命令依次运行并退出0：[typecheck](typecheck.log)、[lint](lint.log)、[beta.26 build](build.log)。lint无输出；无新增依赖 |
| 国际化 | [同步记录](i18n.log)，原7语言文件和上一轮翻译修改未被本批改变 |
| 真实浏览器 | [50项页面验收](page-layout-qualification.json)：8页面×5组语言/尺寸/主题，另含每组账号空数据及读取失败；验证内容/标题对齐、输入尺寸、长账号选择、手机筛选展开、图表按钮不被裁切、正文固定列表头对齐，无页面异常或外部/业务写入 |
| 原流程回归 | [3组Key流程通过](key-request-qualification.json)：英文1280、中文390、俄文320；保留精确Key范围、分页、未知用量、流内错误、正文/SSE、未采集提示和返回焦点 |
| 复现 | [统一布局失败用例](red.log)、[固定列表头错位](table-red.log)、[手机图表按钮裁切](chart-red.log)，均已由最终页面检查转绿 |

页面组合：中文1280、中文390、俄文320、英文768、中文暗色1024。全量原始截图保留在 `.local-tests/ui-unification-20261023/final-pages`，本目录归档代表截图。浏览器测试中排除了 Base UI 裁切隐藏的1px表单代理，尺寸断言针对实际输入组件；这不放宽用户可见控件的40px要求。

本批是 UI 验收，不替代真实供应商、生产数据或全仓 CI。没有修改 Go 代码，因此复用上一轮后端验证；本批未运行新云端 CI。历史管理页通过共用规则受益，但未逐一进行完整业务验收。俄文极窄屏底部导航的长标签仍会多行，后续可单独细化导航文案呈现。

## 截图

- 上游账号：[原桌面](before-accounts-zh-1280.png) / [新桌面](accounts-zh-1280.png)，[原手机](before-accounts-zh-390.png) / [新手机](accounts-zh-390.png)，[长语言窄屏](accounts-ru-320.png)。基线账号为空，新预览额外提供合成窗口，不能将数据差异当成 UI 改进。
- 概览：[桌面](overview-zh-1280.png) / [手机](overview-zh-390.png)。
- 模型：[原手机](before-models-zh-390.png) / [新手机](models-zh-390.png)。
- 分析：[原手机](before-analytics-zh-390.png) / [新手机](analytics-zh-390.png) / [暗色](analytics-zh-1024.png)。
- 正文：[原手机](before-content-zh-390.png) / [新手机](content-zh-390.png) / [展开筛选](content-filters-zh-390.png) / [桌面固定列](content-zh-1280.png)。
- 其余：[资料平板](profile-en-768.png)、[系统信息手机](system-zh-390.png)、[设置手机](settings-zh-390.png)、[账号空状态](accounts-empty-zh-390.png)、[账号失败状态](accounts-error-zh-390.png)。
