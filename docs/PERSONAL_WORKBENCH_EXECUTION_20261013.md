# 个人工作台与个人核心推进（2026-10-13）

## 目标与有效边界

用户要求把独立个人工作台做成实际页面，并按既有个人核心计划继续开发；历史账务保持冻结，暂不部署。
承接[个人状态批次的工作台交付卡](PERSONAL_STATUS_EXECUTION_20261012.md)和[个人核心卡](PERSONAL_CORE_DELIVERY_CARD.md)。
本记录是当前实施入口，旧批次记录保留其各自代码和验收范围。

已授权正常源码开发、必要回归、文档、独立审查、多代理及现有 Draft PR #12 的提交推送。
不合并、不发布、不部署，不操作生产、历史数据库副本、旧 pending/applied、金额或缓存。
不改变普通 Key 与严格 Token/USD 的单位，不扩大严格预算的协议/模型资格，不把未知 usage 当成零值或估算结算。

本批沿用 `codex/ui-stability-20261012` / [Draft PR #12](https://github.com/ForceMind/MyAPI/pull/12)。
起点 `f54ebf9e7bf3eaf41d2b278fb5e9e7f20c62ada0` 的 [CI 38268270399](https://github.com/ForceMind/MyAPI/actions/runs/38268270399)
已回读为十项 success；工作区起始干净。远端 main 仍为 `92fa1182`，冻结 base 为 `0b51bb4`，
beta.10 标签仍为 `631b3205`，beta.11 标签不存在。VERSION 继续为未发布 beta.11，以构建提交标识候选。

已建立本批持久 Goal；没有用户指定的资源或 token 预算。构建、依赖安装、测试和浏览器均使用 GitHub Actions。
本机只做源码编辑、静态检查、七语同步及有界报告/截图回读，不构建镜像、不复制数据库。

## 实施顺序与验收

1. 核对现有概览、模式门禁、用户权限、Key/预算、账户窗口和日志接口，明确不能推断的状态。
2. 实现独立个人工作台：请求路径、显式 Key 选择和限额读取、上游账户窗口、本人最近请求及待处理提醒、常用操作。
   中文 320px 单列，英文 1280px 主体与侧栏，明暗主题；使用项目语义颜色、字体、Base UI/shadcn 组件。
   只有服务端已确认、ready、有效 epoch、双 mode 均为 disabled 才进入个人工作台；
   已确认 enabled/retirement 保留原页面，未确认/冲突/失败只显示中性加载或只读重试，不挂任一数据面。
   两个数据面按模式动态加载，保留商业及历史义务。
   不自动选更宽权限 Key、不泄露 Key、不自动发请求或恢复账务。空、失败、失联、未知、无权限都有可见反馈及合适的重试。
3. 本轮个人核心阶段为“新请求最终派送前的撤销边界”：严格预算先保持 prepared，
   最后一次派送资格及请求取消检查之后、生成请求的 client.Do 之前才提交 sent。
   既有 Responses 预检仍可调用独立 input_tokens 计数端点；“未派送”只指生成请求，不能写成完全无上游网络。
   已有 send 事务的 Token 锁和资格检查保持；确定未发送沿用 prepared 取消及原 writer 退款路径，
   提交后传输失败或取消继续保留 unknown，不承诺数据库与外部接收原子化。
   无新增 schema/迁移，不自动取消旧记录，不扩严格协议资格；并发准入已有实现，只补必要相关回归。
   新请求未知恢复组合验收保留后续，不操作历史实例。
   若涉及新增迁移、历史恢复、协议扩展或超出原计划的数据合同，先隔离并记录，不混入本批。
4. 七语言、针对性行为回归、实际 Chromium 截图和主要交互；后端改动运行相关两 writer、三库和 race 门禁。
   不降低现有断言或用测试双替代实际应用链。独立代码/视觉审查通过后同步原 Draft PR，并核对准确源码 Actions。

完成依据为真实可运行页面、可判定的个人核心阶段、相关检查与独审、文档及远端同步。
合成验收不能证明真实 provider/OAuth/429/费用或生产问题已经解决。核心范围已由源码调用链核实并冻结。

## 分工与资源

主代理负责接口约定、统一翻译、CI 接线、集成与交付；其他代理不写其文件。
`personal_workbench_plan` 使用 `gpt-5.6-sol/ultra` 做只读规划；
`personal_core_boundary` 使用 `gpt-5.6-sol/high` 诊断核心调用链和实际缺口。
规划代理随后实现独立 personal-workbench 目录；核心代理实现 service/token_budget、relay/channel/api_request
及相关测试；`personal_workbench_browser` 使用 `gpt-5.6-sol/high` 实现新 Chromium 脚本与旧概览断言兼容。
`personal_dispatch_review` 使用 `gpt-5.6-sol/xhigh` 独立只读审查核心改动。
主代理持有模式 hook、概览选择入口、标题/导航、个人资料展示、共享 locale、CI 和文档。
一并修正日常导航的一致性：retirement 保留历史入口，disabled 从桌面与移动日常导航隐藏钱包；
个人资料仅在已确认商业 enabled 时展示金额，其他状态保留已持久化请求次数，不改用户额度或策略。
后续实现按独立文件分工，并由不同实现者交叉审查。不递归派工，同一文件写入串行。
并行最多遵循会话四槽限制；不在本机产生独立编译缓存、依赖或测试 DB。

## 状态

| 阶段 | 状态 | 证据/下一步 |
| --- | --- | --- |
| 基线回读与规划 | 已完成 | 基线十项 CI 成功，真实接口及预发状态缺口已核实 |
| 独立工作台页面 | 待验证 | 17 份新组件/行为测试完成，真实读取、选择、刷新与操作入口待云端执行 |
| 下一个人核心阶段 | 待验证 | 预发撤销边界已实现，双 writer/三库相关回归待 Actions |
| 执行验收与独立审查 | 进行中 | 核心、UI 与浏览器脚本独立静态复审通过；新准确源码尚未运行 |
| 文档与 Draft PR 同步 | 进行中 | 准备正常提交同一分支/草稿 PR，不合并、不发布、不部署 |

## 本地静态结果及待执行项

核心初审发现三库竞争错误被忽略、transport 证据未绑定当前身份；已修正并独立复审通过。
SQLite 仅明确接受 typed BUSY/LOCKED，其他方言必须为具体状态冲突；Chat/Responses generation 均按
当前 token 与 pending request 精确验证 sent。旧代码将在新 prepared 断言失败，但未在本机执行红灯。

UI 初审发现未知模式挂旧数据面、Profile 签到入口遗漏、较弱商业导航门禁和多余 landmark；
已针对性修正并补 wrapper/导航/Profile 回归，独立静态复审通过；长策略徽标已允许断词，另有法语窄屏场景。
新工作台对超出 JavaScript 安全整数的内部额度显示未知并停止宣称 Key 可用，保持原值及后端不变。
其行为回归属于当前候选执行范围，不能凭静态检查称已运行通过。
Root 显式打开原用量策略对话框时保留存量政策证据；默认工作台/Profile 不展示旧用户额度金额。

首轮七语新增 97 key / 679 条；后继弃用两个不准确的错误分组键，最终相对起点新增 95 key / 665 条，
起点既有 locale 值未变。脚本写入后 `bun run i18n:sync` 七语 missing/extras 均为 0，
插值错误 0；既有 untranslated 提醒保留。临时翻译脚本已删除。本机未安装依赖或运行构建、Go/前端测试、Chromium。
新浏览器脚本预期 10 journeys / 13 图，覆盖 Root/owner/无渠道权限、明暗/320/1280、失败重试/空/未知预算，
并增加法语 320px 存量策略长文案的实际断词与边界检查；图片分别记录初始页、限额和滚动后的操作区。
并核对本人日志、无自动 Key 选择、无密钥读取/业务写/外联。此处为脚本范围，不是执行成功记录。

## 云端执行记录

首轮候选 `8f17d883f41cb4174b6b1437b1264314cfc539af` 已正常推送并回读。
[CI 38383780356](https://github.com/ForceMind/MyAPI/actions/runs/38383780356) 的前端 Typecheck 失败：
账户窗口状态误用了共享 StatusBadge 不支持的 `destructive`，已改为其实际支持的 `danger`。
该轮没有运行前端 lint/test/build/Chromium，不能写为页面通过。其他作业以各自终态为准。
原失败日志保留；修正不降低规则或改变错误颜色语义，后继准确源码必须重新验收。
同次复核把近期记录的 type=5 分组明确标为“错误与待核对”，复用已验七语键，避免把缺 usage 的完成记录又称为请求失败。
两项局部修正已独立静态复核通过，弃用键通过规定脚本删除；尚未运行的后继不标为通过。

首轮最终为 cancelled：前端 Typecheck failure、Backend cancelled、其余八项 success。
第二轮源码 `2cd720d8e20227af5c940fb43733dd8ae33cd618` 的
[CI 38384435207](https://github.com/ForceMind/MyAPI/actions/runs/38384435207) 已通过 Typecheck，
Lint 报七个错误（测试按钮缺 type、重复 React import、fixture 非空断言），已作局部修正；
没有关闭 lint 规则或删除行为断言。本轮前端 test/build/Chromium 未执行。
三库专项等八项 success，Backend 随后因后继运行 cancelled；整轮最终 cancelled，保留前端 failure。

第三轮源码 `cd6cb9fd35072af81365c6776d8039c24e71aa3c`，tree
`58ccdf080bb5ec84e80d0784dff6ae2cb77d6b67`，已正常推送并回读；
[CI 38385305585](https://github.com/ForceMind/MyAPI/actions/runs/38385305585) 已通过 Typecheck/Lint，
前端 1,262 passed / 2 failed，均在刷新失败后的 hook 状态断言，build/Chromium 未执行。
refetch Promise 返回后 observer 通知仍可异步，后继将首个错误状态断言改为 `waitFor`，
保留撤销旧身份/权限/Key 确认的全部原断言，不加固定等待、不改生产逻辑；需由后继执行核实。
其他八项专项 success、Backend cancelled，整轮最终 cancelled。坏 epoch fixture 采用必填 capabilities 类型并直接赋值，
不允许静默跳过，独立静态复审通过。

第四轮 `0d86c75226aaa1dea4b1f0e76afcb7b5ef19679c`，tree
`f9d4988d995e075729b2d4766b4ec54935561f8a`，
[CI 38385952524](https://github.com/ForceMind/MyAPI/actions/runs/38385952524) 的 Typecheck/Lint、
204 文件 / 1,264 前端测试及生产构建通过。实际 checkout `b6543c3590f54ea766154c3b032ea222e98e1400`
的 parents 为冻结 base 与该候选，API 回读 tree 与候选一致。
前端浏览器有两项失败：旧 quota overview 在默认 disabled 下等待商业图表；setup 手机导航
用全页第一个 `/profile`，误选新工作台快捷链接并被已打开的 sidebar sheet 遮挡。
后继仅将商业图表旅程临时设 enabled 后恢复原模式，并将 setup 的点击限定侧栏链接。
没有改变共享 fixture 默认或移除图表/几何/关闭侧栏断言，不使用 force click。
该轮 Profile 和稳定性浏览器已执行，setup 提前失败阻断后续个人状态/新工作台/完整 UI；
不能称新工作台已验。八项专项 success，Backend 终态另记。失败日志与 3.5 MB 合成 UI 产物保留，
artifact `11807256263` 的 ZIP SHA-256 为 `8236bf32ae4ab79374fbe53b0b1ff9d101b5fc15153e8adbdb10437d1b090940`。

## 剩余个人核心与下一片合同

本批只推进最终生成派送边界，不宣称个人核心全部完成。现有严格预算入口仍只确认无 query 的
POST `/v1/chat/completions`、`/v1/responses`，并受原生上游、模型、字段及 usage 证据限制；
个人无钱包端点范围与严格预算资格是不同合同，不能互相外推。

后续优先补“新请求 unknown 跨重启”组合：新建、已有资格的原生 Responses 严格 Token 请求，
两 writer 下 sent 后无可信 usage，确认 durable unknown；关闭并重开测试 DB、清除进程态后，
同 Key 新请求仍返回 typed pending 且不调用 generation transport；Root 以精确 input/output
证据核对两次，只产生一次结算/消费记录并释放 pending。接入现有三库门禁，先补行为测试，
仅在暴露缺口时修改生产代码。这是新建合成请求合同，不触碰冻结历史记录。
现有 `model/token_budget_test.go` 的 reopen 和 `service/token_budget_test.go` 的同进程 reconcile
各自有保护，但尚未形成上述组合证据。完整协议资格、逐 Key 共享账户窗口百分比和商业代码
物理拆除均未交付，保持原计划边界，不以本批 UI 或局部回归替代。
