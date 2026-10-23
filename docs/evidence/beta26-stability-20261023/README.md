# beta.26 本地稳定候选（2026-10-23）

本批在 `dev` 开发，基于 `6efd467255c5637277dbaffc1584c682503b056b` 的未提交工作区。当前结果是本地候选；尚未推送、运行新云端 CI、合入 `main` 或部署生产。上次核验的生产仍为 beta.25，本批没有改变生产配置或消耗供应商订阅。

## 首次云端回归与修正

两轮合并提交 `f99aef811c3707b0f5f1e4e4a532c78e9f288163` 已推送 `dev`。[首次CI](https://github.com/ForceMind/MyAPI/actions/runs/39936836364)的后端完整测试发现：输入估算提前初始化 `RelayInfo.ChannelMeta`，改变了首次渠道选择和初次账务预留的渠道字段。相关失败断言保留：[原始失败节选](ci-router-red.log)。

修复将输入估算/上限检查改为只读当前选中渠道的配置，让渠道状态仍由原有派发路径初始化；重试时读取当前渠道，保留首次预扣前拒绝的行为。没有放宽既有账务或故障切换断言，另补充了预检不能改变路由状态的测试。[预算与故障切换定向回归](ci-router-targeted-green.log)、[输入预检回归](ci-input-targeted-green.log)已通过；修复后路由、控制器、服务三个包[完整回归通过](ci-backend-fix-regression.log)。新CI以修复提交关联的Actions为准。

下方源码manifest保留各轮最初验收时点；本次修正文件的SHA另见 [修复源码manifest](ci-fix-source-manifest.json)。

## 可操作预览

[打开只读合成预览](http://127.0.0.1:4183/__preview)。可操作 Key → 精确请求 → 错误诊断 → 已保存正文/原始 SSE → 返回 Key；包含上游上下文超限及本地预检拦截。连接 → 编辑连接 → 高级设置中可查看和编辑输入估算上限，预览不保存修改。

预览使用合成身份、连接、Token 和正文，不接生产 API。`920000` 等数值只是演示配置，不是默认值或模型能力结论。会话结束后服务可能停止。冻结构建位于 `.local-tests/beta26-stability-20261023/final-preview-dist`，重启：

```sh
MYAPI_PREVIEW_PORT=4183 MYAPI_UI_DIST="$PWD/.local-tests/beta26-stability-20261023/final-preview-dist" node tools/ui/key-request-preview.mjs
```

冻结目录未纳入 Git。[构建 manifest](build-manifest.json)记录文件 SHA256；[源码 manifest](source-manifest.json)记录本批工作区代码 SHA256。HEAD 本身不包含这些未提交修改。

## 本批变化

- 统一侧边栏和顶部栏高度、页面边距、标题及按钮尺寸，改善 Key 卡片换行、收起 Logo 和手机顶部布局。侧边栏隐藏常驻滚动轨道，仍支持滚轮、键盘和末项导航。
- 修复空闲初次挂载留下进度条，以及进度条跨越侧边栏的问题。加载条位于工作区，导航完成后清除。
- 文本请求单独记录 `request_input_estimate`；实际 usage、账务 Token 和结算算法保持原有语义。本地估算不是上游精确输入量。
- 渠道可按客户端精确模型名配置 `setting.input_token_limits`。空配置沿用原有准入；超过估算上限时返回 HTTP400，在首次预扣前、每次重试派发前检查。媒体、不透明历史等无法完整估算的请求在启用上限时拒绝。该上限约束输入估算，不自动证明输入加输出满足供应商上下文窗口。
- 日志详情展示输入估算、已配置上限、已知 HTTP 状态，以及安全白名单中的上游流错误码；缺失 usage 仍为未知，不补写成零。正文入口移至核对信息之前。
- 修复发行合同对旧开发分支的断言，以及安装器合同对安全 JSON artifact 的过度禁止，保留禁止发布、权限扩大及敏感上传的约束。

## 验证范围

| 检查 | 结果与证据 |
| --- | --- |
| 受影响 Go 包 | controller、service、model、relay/common、openai 全部通过：[日志](backend-regression.log)；错误码白名单最终改动另经 [openai 回归](openai-final-regression.log) |
| 前端受影响回归 | 43 文件、294 项通过：[日志](frontend-regression.log)；最后局部修改后 3 文件、17 项通过：[日志](final-focused-tests.log) |
| 发行/恢复合同 | 与失败 CI 作业相同命令的 78 项通过：[日志](distribution-green.log)；安装器检查通过：[日志](installer-green.log) |
| 模块与构建 | `relaykit` 下 `GOWORK=off go build ./...`、前端 lint、typecheck、beta.26 build 均退出0；空日志代表命令无输出：[模块](relaykit-build.log)、[lint](lint.log)、[类型](context-typecheck.log)、[构建](candidate-build.log) |
| 翻译 | 新文案覆盖 7 种语言；同步后各语言缺失键0、额外键0：[日志](i18n-context.log) |
| 实际浏览器 | [5 组布局](console-layout-qualification.json)、[3 组 Key 流程](key-request-qualification.json)、[桌面/手机编辑器](input-policy-qualification.json)均通过，未发出供应商请求或业务写入 |

后端测试覆盖首次预检拒绝不派发、不预扣，估算边界及缺失 usage 的账务语义。未增加数据库迁移，复用现有 JSON 设置及 GORM 日志写入；本批没有重新运行原生 MySQL/PostgreSQL 实例验证，也没有做真实供应商端到端验收。旧云端 CI 39807929357 的失败不能据本地结果改称成功。

## 渲染证据

- 桌面：[调整前](before-keys-desktop.png) / [调整后](console-layout-keys-zh-1280.png)。
- 手机：[连接详情](console-layout-connections-zh-390.png)、[调用记录](console-layout-requests-zh-390.png)。
- 诊断：[上游流内错误](key-request-stream-error-zh-390.png)、[预检拦截](key-request-preflight-rejected-zh-390.png)、[正文](key-request-preflight-body-zh-390.png)。
- 输入上限：[桌面](input-policy-zh-1280.png) / [手机](input-policy-zh-390.png)。

本批 UI 范围是控制台外壳及 Key、连接、请求排错流程；全平台页面的最终视觉接受仍待后续迭代。现有 `geekmind-context-myapi-20261023` 真实测试日志包保留，未混入本批合成验收。
