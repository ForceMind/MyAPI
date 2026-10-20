# MyAPI 平台重构隔离交付证据（2026-10-20）

状态：P1–P4批准范围的开发、隔离验收和本批原图自查完成。最终准确CI为11/11成功；本批未部署。版本 `0.2.0-beta.24`。维护者批准的P1–P4范围见[唯一计划与进度](../../MYAPI_PLATFORM_REBUILD_PLAN_20261020.md)。本批没有部署、真实供应商测试或生产数据操作。

## 来源与制品

| 对象 | 准确标识 |
| --- | --- |
| 最终候选源码 | `458f07a24ef6cbdd3055757cc428aa8c92cc20d3` |
| 最终源码 tree | `6f5d30621a64b4e7a6821d0ddfcd7f6554563965` |
| Actions checkout | `cca41246f68a2e2b2a7e1387f6025dd5e36b8f1c`，GitHub API确认与候选同tree |
| 准确 CI | [39420412383](https://github.com/ForceMind/MyAPI/actions/runs/39420412383)，最终11/11成功 |
| 前端构建工件 | `12262994177` / `myapi-console-build-cca41246f68a2e2b2a7e1387f6025dd5e36b8f1c` |
| 200文件构建清单 | [build-manifest.json](candidate-458f07a/build-manifest.json)，SHA256 `4e180d363ef851f7eff1d3a270df419535e8afba5671fc4393eb8522e5b8429b` |
| 隔离预览核对 | [preview-probe.json](candidate-458f07a/preview-probe.json)：HTTP200，全部5个入口JS与制品字节相同；HTML仅增加明确的合成预览提示条 |
| 分支及 PR | `codex/subscription-key-share-20261014` / [Draft #17](https://github.com/ForceMind/MyAPI/pull/17)，base `codex/personal-core-20261014` |

最终归档提交只含文档和证据，不冒充被构建源码。`candidate-2fcd34e/`属于前一准确候选：CI39415316504最终11/11成功、完整前端251文件/1588测试及所有既定浏览器检查通过；它仍有会话相对时间英文问题，最终源码另行修正并验收。`candidate-d77b02e/`为更早未通过候选，只用于诊断。

## 可查看结果与边界

- [控制台](http://127.0.0.1:4181/__preview/operator)、[上游账号](http://127.0.0.1:4181/upstream-accounts)、[调用记录](http://127.0.0.1:4181/usage-logs/common?pageSize=20)、[默认首页](http://127.0.0.1:4181/__preview/guest)。首页链接切换此隔离预览为访客，控制台链接恢复合成维护者。
- 预览在本机会话TTY46389，使用Actions准确构建；内存合成API、无真实账号/数据库/Redis/供应商、业务写操作拒绝。页面明显标记合成数据，CSP限制连接到自身。前端认证fixture不属于真实凭据。
- 运行依赖会话进程和`/tmp`工件；恢复时核对端口、进程、revision与SHA，不能假定在线。Actions工件有保留期，本目录报告和已归档原图用于长期交接。[预览脚本原始副本](preview-server.mjs)可复核，恢复仍需对应构建与本地路径。
- 本批为界面与已有合同整合；根Go源码、go.mod/go.sum及relaykit模块相对本轮起点c6641b9未变。原单值Key轮换/迁移与账务门槛继续有效，历史账务冻结，没有SIWC。
- PC分组导航与移动底部/更多共享权限登记；资金状态不再切换整个页面。成熟渠道/模型/Key/日志/内容/任务/系统/资料能力保留。窗口错误、过期、明确零值和证据缺失分开呈现，读取的是已存观测，不能据此证明真实上游可用或原生429/刷新行为。

## 验证与原图

准确候选458f07a当前已保存结果：

| 范围 | 实际结果 | 原始报告 |
| --- | --- | --- |
| 完整前端 | 252文件 / 1594测试通过；typecheck通过 | candidate-458f07a/myapi-frontend-tests.log、myapi-frontend-typecheck.log |
| 首页/入口/完整调用记录 | 20/20 | candidate-458f07a/browser/personal-console-qualification.json |
| 总览/权限/账号/窗口/历史导航 | 30/30；新增三旅程各一次合法历史读取 | candidate-458f07a/browser/personal-workbench-qualification.json |
| Key原生窗口分额 | 28/28 | candidate-458f07a/browser/subscription-share-qualification.json |
| 模型范围 / 高级窗口 / 安全 | 17/17、17/17、20/20 | candidate-458f07a/advanced/ |
| 隔离维护会话 / 迁移 / 轮换 | 12/12、4/4、4/4；临时SQLite API证明通过 | candidate-458f07a/maintenance/ |
| 全平台 / 资料 / 初始化 / 稳定性 / Playground / 个人状态 | 33/33、9/9、25/25、10/10、8/8、6/6 | candidate-458f07a/ui/ |
| CI全部作业 | 11/11成功，包含typecheck/lint/build、根模块/relaykit、三库、Redis、race及既定浏览器 | [ci-result.json](candidate-458f07a/ci-result.json) |

已保存全部对应原始JSON；截图仅归档本次需要的代表性原PNG，数量及实际查看清单以最终manifest与reviewed-originals.json为准，不冒称已查看所有Actions图片。构建、Go、三库/Redis/race及Chromium均使用既有Actions，本机仅定向前端回归、typecheck/lint/i18n及只读预览指纹核对。

本机有意义的原始红/绿与检查日志见[local/](local/)；最后的会话相对时间回归为旧实现法语失败→修正后七语言及移动布局共7例通过。主代理自查与独立审查分别记述，本批未新增独立审查或子智能体。最终结构化检查见[qualification-summary.json](candidate-458f07a/qualification-summary.json)：首页/入口、完整日志关联内容、双端连接/窗口、权限、设置/外观、完整资料和系统入口均有对应实际旅程。未发现所查看10张原图的布局阻断。

## 失败与必要修正

| 准确候选 | 已保留事实及处理 |
| --- | --- |
| e51940a / CI39402708217 | P2手机长连接名与编号同span，旧测试按独立名称精确定位失败；改按真实语义行定位，未改应用数据或超时。原JSON/PNG见diagnostics/e51940a |
| b14a212 / CI39404618156 | 浏览器通过后自查1024原图发现Enabled被长名称挤断；固定状态列与正常词边界换行，有红绿回归，见diagnostics/b14a212 |
| fe160cc / CI39407187058 | 成熟资料页恢复后旧驱动仍等待Identity/展开按钮；保留真实维护失败日志及安全报告，按实际Profile和完整安全卡片更新驱动，权限/请求次数等断言保留 |
| 77ee073 / CI39408424408 | 新顶部Search与日志筛选同名，旧驱动未限定main；原图又发现搜索图标继承宽度，已修紧凑宽度，见diagnostics/77ee073 |
| d77b02e / CI39410672493 | 最终10/11：法语长姓名夹具错误等待固定短名、外观radio前缀定位不符；修复真实夹具/可访问名称，未删旅程。原图发现法语会话标题挤压和限流单位拆行，局部修正并补原旅程断言，见diagnostics/d77b02e |
| 2fcd34e / CI39415316504 | 最终11/11，但自查会话相对时间仍为英文；仅按当前界面语言格式化该时间值，未改全局语言或时间戳；最终ad5f017另验 |

ad5f017核心20/20、分额28/28、安全与完整单元通过；工作台在第二旅程暴露合法渠道7历史读取未配置。原始失败见diagnostics/ad5f017。458f07a仅补Root/channel.read且实际/channels页面的这条GET合成空历史，并等待真实图表响应、验证ID和一次读取再返回；不更改应用、权限、超时、出站或写入断言。准确来源见上表；应用代码与ad5f017相同，458f07a仅变更夹具及进度记录。

## 已知范围外事项

- 缓存Token已有逐条记录/展示，调用记录统计接口目前只有quota/RPM/TPM，缺统一缓存命中率和独立总Token消耗；cache_ratio是计费倍率，不是命中率；上游缺usage保留未知。用户本轮询问记录为待办，没有借此新增统计后端、改历史日志或推断为零。
- 现有隔离run/动作预算/停止排空/准确来源及验证反馈体现部分Harness思想；统一运行观测尚不完整，不宣称已实现完整Harness平台。
- 公开仓库标准GitHub托管runner计算时间免费；存储/缓存规则与实际账单分别判断。[官方说明](https://docs.github.com/en/billing/concepts/product-billing/github-actions)。本机下载的预览副本和开发依赖仍占空间，本批未自动扩大清理范围。
- 未进行新生产部署、真实OAuth/refresh/429或窗口重置验收；既有beta.21部署记录继续只证明原版本。

## 保留与归档

维护者WIP未暂存/覆盖：MYAPI_MASTER_PLAN.md SHA256 `e058e179a828c752b5b389cde5a88d05c5999c61029948a88e270aad7131b243`，NEXT_SESSION_HANDOFF.md SHA256 `4e21c1e847db911b9143f580778ca63650337d7a330b658f8556aa1f499f1c39`；两组设计草稿及其他既有未提交文件保持。最终[manifest.json](manifest.json)覆盖本目录全部归档文件（不含自身），诊断/本机红绿子清单见diagnostics-manifest.json。最终候选只归档10张代表性原PNG，主代理已逐张查看，清单与判定见[candidate-458f07a/reviewed-originals.json](candidate-458f07a/reviewed-originals.json)；它们未经过缩放、拼接、涂改。更早候选原图只作为对应诊断，不代替最终来源。
