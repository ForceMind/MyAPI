## 当前执行：beta.6 官方 Chat 文本资格候选（2026-10-05）

beta.5 已按准确源码与文档 HEAD 收口。本轮承接既有主计划，仅实现官方原生
Chat Completions 的首个精确模型 `gpt-6.1-sol`，以及默认关闭的官网来源检查；
资格合同、官方证据、验证和停止边界见 [beta.6 交付卡](BETA_6_DELIVERY_CARD.md)。
实现以已核对 `ab8da0e` 为基线，保留 beta.3–5、MyAPI 命名和法律通知。

- Chat 按官方 1,050,000 总上下文上界预留，显式输出上限不重复相加；本地 tokenizer
  不冒充实际计数。缺失、矛盾、非文本、非适用档位或未完整结束的流保留待核对
- USD 复用发布/冻结来源并冻结两档价格，实际输入选档；明确缓存读/写和 reasoning
  包含关系。来源保存、价格发布、预算资格和真实账单核对分别对待
- 来源检查复用现有 SystemTask 和固定安全客户端，默认关闭，启用后 24 小时检查，
  72 小时无成功核验提示过期。只保存来源及差异待确认，不自动发布或替换锁定价
- 原入口 `/keys`、`/usage-logs/common`、`/channels` 与
  `/system-settings/models/openai-pricing-source` 同版补齐解释和七语言

独立资金合同/实现复核、本地根 Go 全测/vet/build、独立 relaykit、定向 race、
19 个实际合成 HTTP 场景、七语言及前端定向测试/typecheck/build均已通过。
准确新 HEAD 三库/Chromium/全 CI 仍须完成后才标本版限定交付。
未合并 main、未创建 tag、未发行或部署，VERSION/默认镜像仍为已发布 beta.3；
真实账号、真实提供方账单、目标环境和生产升级/回退保持未验。
不在本版扩媒体、第三方估价、逐 Key Codex 百分比账本或完整 UI 重构。

下方 beta.5 及更早的“停止/等待下一版”是保留历史，不覆盖本节当前 beta.6 执行。

## 当前交付：beta.5 限定源码候选通过（2026-10-05）

源码候选 [`2720806`](https://github.com/ForceMind/MyAPI/commit/2720806390bf44732da4b07d276f4e0f03d5778e)（tree `bb4d0ac17b67258412a2b05248dd8e5604e5ae8b`）已在原开发分支推送并回读。[CI 十作业](https://github.com/ForceMind/MyAPI/actions/runs/37314110213)、[Docker 三作业](https://github.com/ForceMind/MyAPI/actions/runs/37314110258)及[静态官网检查](https://github.com/ForceMind/MyAPI/actions/runs/37314110295)全部成功。PR 测试合并 `32149da39c34307b55a052b04bc0d80611993c58` 的 tree 与该源码一致。

- 根 Go vet/build/完整模块测试、全部既有 race 作业与独立 `GOWORK=off` relaykit build/test/vet 通过
- SQLite、MySQL 5.7、PostgreSQL 9.6 实库的 RelayAccountHold 持久化/重开/并发/清理、TokenBudget、双 writer R1 派送及恢复、额度 schema、模型发现/路由合同通过，没有用 SQLite 代替其余引擎
- 前端 127 文件/645 测试、typecheck、生产构建、旧 quota Chromium 与完整 320/1280 路由/有序尝试日志 Chromium 通过；[当前路由截图](https://github.com/ForceMind/MyAPI/actions/runs/37314110213/artifacts/11348060615)使用合成数据，不能冒充真实账号验收
- S1/S2-A 数据库、ClickHouse、Redis 两作业、Desktop 与发行合同通过；Docker 的 LAN/fresh/handoff 合成安装检查通过，未执行发行或部署

失败与修正保留：首个组合源码 `018e453` 的前端/Chromium及 Docker 已通过，但 [B2 作业](https://github.com/ForceMind/MyAPI/actions/runs/37312853364/job/111772264022)在 PostgreSQL fixture 清理时出现 `sql: database is closed`。原因是测试复用带有自有连接池的 PostgreSQL dialector，所谓重开连接实际共享原池，关闭后令原连接无法清表。`2720806` 只修正 `model/relay_account_hold_test.go`：每次 SQLite/MySQL/PostgreSQL 打开均创建新 dialector，并证明关闭重开池后原池仍可用；原断言全部保留、生产代码未改。独立复核和本地定向 race/SQLite/vet通过，随后上述准确源码的三库与完整 CI 全部通过。首轮旧 backend 因新提交被取消，不把取消步骤计作成功。

接续时先读 [beta.5 交付卡](BETA_5_DELIVERY_CARD.md)与 [主计划](MYAPI_MASTER_PLAN.md)，保护任何并发 WIP，核对当前分支/远端。不要重做 beta.4/5 已验证功能，也不要把下方历史未提交或等待授权状态当作当前事实。

本次交付是 beta.5 限定源码候选，未合并 main、未创建 tag、未发布新制品、未部署。VERSION/默认镜像仍为已发布 beta.3；真实 OAuth、账户窗口/429、账单与目标环境、生产升级/回退仍未验。保留 MyAPI 命名、既有 README 和来源/法律说明；品牌或界面调整不证明完整原始源码替换。本版停止扩展，后续按主计划逐版冻结范围，不混入 beta.6 费用资格或完整 UI 重构。

本节覆盖下方历史“本地未提交/待同步/待三库与 CI”的状态。当前文档提交仅同步已验证源码的结果，不改变运行时、测试、工作流或依赖；文档 HEAD 自己的检查状态另在 PR 回读，不把源码绿灯冒充文档 HEAD。

## 以下为保留的交接历史

# 当前接续：beta.5 组合候选待准确HEAD验收（2026-10-05）

本节优先于下方历史交接。第一批重试安全与第二批有界期限、持久账户冷却、
成功账户绑定及预览/日志解释已实现，原工作区已有下述本地验证记录。恢复核对确认
源工作区HEAD仍为`8252fe9`，51文件组合暂存树为`dc5e8a2`；`8a467e0`是保留的第一批历史树。
本次仅以独立索引整合远端`4847386`的命名/来源状态，并补齐第二批race筛选；
源工作区/default index不变，beta.5未提交/推送。原待审批外发仍未由本次操作解决，
不得替换或绕过该工具调用。新准确HEAD的三库、Chromium与完整CI仍待；不能reset/stash/clean。

先读[beta.5交付卡](BETA_5_DELIVERY_CARD.md)第二批节的完整合同与验证边界：

- 共享尝试期限0–3600秒与失败冷却0–300秒，均默认0关闭；不重定义
  `RELAY_TIMEOUT`。期限取消不代表持久化清理/结算必在同一秒数内返回。
- 冷却按渠道、稳定凭据与派送配置隔离，展示名/计数不改变身份。到期只撤销临时
  门禁，不能覆盖可靠Codex耗尽；当前凭据、权限、预算和窗口继续约束派送。
- 候选中的冷却到期允许同渠道健康账户继续参与；冷却排除所有其余合格账户时才列排除。账户绑定仅
  确认成功后保存；无session输入的预览始终未评估绑定，不可展示为当前命中。
- 管理员日志的冷却为后续请求证据，非耗尽或重试延长；旧载荷和普通用户隔离保留。

排除提示已明确限定“其余符合条件的账户”，不把耗尽/禁用凭据称为冷却。
文案修正后定向63测试10.43秒、前端生产构建8.80秒通过；七语言实际渲染、
本批18项文案/占位符、完整typecheck及涉及文件lint均通过。全源码提取发现8项
未改动页面的既有English缺键，第一批暂存基线同样缺失；不宣称全站翻译完整。
最终根Go测试/vet及独立relaykit的`GOWORK=off` build/test/vet通过；实际HTTP
21例（含OtherSettings变化拒绝、完整成功账户绑定链）、SQLite重开/并发单调冷却/
纯摘要及Redis配套绑定TTL/重建/清除均通过。旧仅整数ID且无真实渠道的夹具改为
合格渠道及账户配套绑定后，完整service和最终整体验证通过；定向race及最终修正后的四包复验均已通过。
新远端CI、MySQL/PostgreSQL及
Chromium仍未取得；不复用beta.4或第一批绿灯冒充本批，不尝试先前已拒绝的本地
浏览器执行。真实账户/账单/目标部署继续未验；VERSION仍为已发布beta.3。

最终本地根测试/vet与独立relaykit构建/测试/vet、定向race均通过；最后诊断修正
后完整根测试/vet及四个受影响包的race再次通过。空凭据且无hold不再误报冷却。
以上为原第二批冻结时的验证记录。本次恢复后的源工作区保持`8252fe9`/组合暂存树`dc5e8a2`，
只交独立整合树与核验清单；未提交或外发，不替换待审批工具调用。恢复环境已补齐Go1.25.1及锁定依赖，
组合候选定向race、资金/权限/恢复、SQLite合同、受影响包vet与独立relaykit通过。完整根测试、前端套件
与浏览器本次未复验，MySQL/PostgreSQL明确跳过；不能用历史或命名分支CI替代beta.5新HEAD验收。

2026-10-05 独立复核另修正两个具体缺口：非阶梯计费跨组后须以冻结输入先建立/补足预留再派送，
免费转收费及较低收费转较高收费均不能绕过资金门禁；禁用跨组重试后须保持实际已尝试分组，
仍允许同组健康账户和首次选择时扫描空组。新增28个双writer/Token及按次实际HTTP合成场景，
另有五个分组边界场景与冻结输入/溢出测试，均经红例、修复及独立复核；未扩大计价资格。

继续当前beta.5闭环，不扩beta.6费用资格、不做全站UI重构；必要证据与准确状态
齐备后交付本版候选，发布/部署仍须独立授权。

# 历史接续：beta.5 正在实施（2026-10-04 北京时间17:56）

负责人已明确同意继续beta.5及既有计划，旧“beta.5等待授权”不再适用。
实施基线为同名开发分支 `8252fe97b10311693c141538f4277ab739bd1ad4`；
该HEAD准确CI全部通过，beta.4源码与证据保留。当前未提交WIP须先核对归属和状态，
不得reset/stash/clean或用旧HEAD覆盖。新HEAD/远端/工作树以实时核对为准。

先读 [beta.5交付卡](BETA_5_DELIVERY_CARD.md) 与主计划，继续当前有界批次。
本批先修复重试上限、逐凭据排除、派送未知/部分输出防重放及管理员尝试日志；
账户级会话保持、冷却、撤销和恢复仍属于beta.5后续闭环，不可提前宣称完成。
前后端按独立文件分工，文档和发布核对统一协调；不重复实现beta.4。
尚未发布/部署或真实账号验收；VERSION仍保留已发布beta.3。

以下为历史交接和已完成beta.4记录，状态冲突时本节优先。

# 当前接续：beta.4 限定源码候选已通过（2026-10-04）

2026-10-05 状态澄清：产品名称为 `MyAPI`（无空格），六份 README 的品牌说明与文档重写已完成。品牌调整或界面重构不证明完整原始源码替换；当前仍保留继承实现，完整替换尚未完成，也未取得逐项来源验证。历史独立实现目标与当前复用既有 API/组件的范围仍需核对，本批不启动新的重写计划。beta.4–7 既有执行顺序及许可证、NOTICE 和法定界面署名保持不变。

本轮实施基线d0a1caa；最终实现/脚本源码 `5ac05502606fb63c3032fa9767d2c5b69256f406`。
CI `37190313361` 十作业、Docker `37190313249` 三作业全部成功；604前端测试及
320/1280完整路由Chromium通过，测试合并树与源码相同。证据、支持矩阵、查看入口、
安全恢复及失败历史统一见 [本版交付卡](BETA_4_DELIVERY_CARD.md)。

开发分支仍为 `codex/r1-usage-review-20261002`，PR #2保持Draft。后续只有本轮
收口文档，最新HEAD及其CI应实时回读，不能把源码绿灯冒充新文档HEAD结果。
保留工作树/并发修改，不reset/stash/clean，不先pull/rebase或合并main。

2026-10-04 北京时间17:14，负责人另行确认将完整UI重构补为核心功能齐备后、Desktop前的
独立交付节点。当前只更新主计划，不代表UI或beta.5实施已授权；原型/视觉/全站迁移
范围及停止条件见主计划新增节。后续汇报保留项目/编号/北京时间，不重复无法核验的
模型/思考强度/Token占位。

**停止beta.4扩展，不重做本轮。下一版beta.5仅报告范围，等待负责人继续授权。**
beta.5为多账户/冷却/可靠耗尽与有界切换/会话及解释日志，不扩智能选模、媒体、
商业、学习或beta.6计价资格。负责人允许本轮子任务加速不等于后续无限并行开发。

未合并main、未tag/发布/部署，未操作真实账号/付费/生产数据。VERSION/默认镜像
保留已发布beta.3。真实账号/账单/目标环境和生产恢复仍未验/暂缓。当前说明优先
于下方原规划交接的“下一版beta.4”和历史R1模板。

# My API 当前开发交接（2026-10-04）

## 本次分版本计划交接（优先入口）

负责人要求把新的完整分版本计划与项目参考取舍同步仓库，暂交其他 agent 完成。本轮只更新文档，不开发、不启动其他 agent、不新建Goal/自动化、不合并/发布/部署。

- 最新总体入口：[MYAPI_MASTER_PLAN.md](MYAPI_MASTER_PLAN.md)前半部，含beta.4–7、自用0.2.0、后续Task/安装/Desktop/学习和可选商业目标；sub2api等十个参考项目的采用/不采用规则与所属版本同处记录。
- 可复制提示词：[CODEX_HANDOFF_PROMPT.md](CODEX_HANDOFF_PROMPT.md)开头当前模板，不用下方旧设备/R1模板。
- 开发与文档分支：`codex/r1-usage-review-20261002`；交接前HEAD/远端均为`feef999f961395ffbc94b9ed17251b68429bbc16`。之后新增本批文档提交，准确HEAD通过`git rev-parse HEAD`和同名远端分支回读；不填自引用提交号。
- 已发布beta.3源码仍为`2f2717bffe580baf23820480a7d40d54e676199e`，tag/制品不变；发布与digest见[RELEASE_BETA_3.md](RELEASE_BETA_3.md)。文档提交不重新证明运行制品或真实业务。
- 本轮开始仅`docs/MYAPI_MASTER_PLAN.md`有前几轮本任务规划WIP，未发现其他修改。只提交明确的计划/提示词/索引文档，不混入新业务源码；其他工作树保留。
- 用户在新环境接手时使用当前实际目录，不依赖任何历史绝对路径。不要回到旧`main`或`codex/r1-handoff-20261002`寻找新计划，不覆盖未提交工作；远端并发前进时先核对归属，不强推。
- 下一实施版是beta.4模型发现/识别/基础路由。先核对实际caller，冻结“两个上游、一个公共模型、预览测试→调用→日志”的交付卡；按新对话具体授权推进，不同时扩调度、费用协议、商业或学习。
- 真实OAuth/账户/账单/目标环境验收仍未验或暂缓，不抹掉也不反复催问。旧missing/estimated红灯已在R1后续修正，不能重做旧失败。现有支持资格见[NEXT_USABLE_VERSION.md](NEXT_USABLE_VERSION.md)。

恢复路径：在干净或明确归属的检出中读取上述分支及相对路径文档，先保护WIP；不reset/stash/clean、不移动tag。旧技术合同和逐批证据可查，下方旧“当前分支/下一步/停止扩范围”按日期保留，不覆盖本次交接。

## 历史记录：R1开发和beta.3发行

2026-10-04：已获负责人明确批准并发布 `v0.2.0-beta.3`（源码 `2f2717b`）。
准确候选 CI 十项、Docker 三项、GitHub 制品及 Full/LAN 双架构 GHCR 发布均成功；
匿名镜像回读和精确工作流身份的独立签名验证通过。部署入口及 digest 见
[beta.3 发布记录](RELEASE_BETA_3.md)。真实 OAuth、账户/账单与目标 HTTPS 验收仍暂缓/未验，
未合并分支、未部署真实服务，未扩大 R1 支持资格。下文按时间保留旧状态。

2026-10-04 00:01 北京时间：六项代码入口/权限/正常异常合同与自动化证据核对完成，索引及支持边界统一在[NEXT_USABLE_VERSION](NEXT_USABLE_VERSION.md)首节。本轮未发现新的已证实代码缺口，不把核对完成说成无Bug或真实验收通过。运行源码3337553110c423d70f3e9133f664f729323015f2的CI37133546782十项、Docker37133546820三项全部成功；580前端与合成Chromium通过，末model race23.892s，三库R1 .47/.95/3.44s。源码基线已知缺陷均有修复批和证据，真实验收按负责人要求暂缓，R1仍未封版/发布/部署。公开官网只读复查原15秒限制首次超时、后同函数5.980s成功；准确SHA0d9fb6b2原文40型号中7个符合既有候选发布合同，来源未发布也未接入任何新预算。本批只同步代码侧状态/合同索引，保留原支持资格和失败历史，不扩价格、支付或测试框架。

2026-10-03 23:29 北京时间：ea56fc2重定向批已通过CI37132233162十项和Docker37132233246三项，合并c76f4aa5树68cd35ed与HEAD一致；原三库R1 1.35/3.79/6.14s、schema .05/1.33/14.05s、末model race15.686s，无失败作业重跑。用户明确暂缓真实验收，不再等待测试实例，继续既定R1独立查错。

本批仅修OAuth有效期整数换算：两个入口原先接受超出time.Duration可表示秒数的expires_in，出现1734年或已经过期的成功结果；新增单/多凭据集成红例0.067s还证明该异常结果会替换本地记录。现统一用int64读取秒数，在纳秒换算前拒绝非正或不可表示值，不饱和、不猜默认有效期，也不设猜测的供应商最大期限。缺失/null/0/负数、上界及越界、正常有效期、单/多账户不覆盖、原取消/并发旋转保持，定向service/controller0.127/0.129s通过，最终service/controller相关race2.317/2.819s通过，完整根Go/vet通过（service14.174/controller3.146s，未变包复用缓存）。若提供方已经旋转，保留本地原记录不代表旧凭据仍可用或已自动恢复。无真实凭据/请求、UI、DB结构、TLS/代理、预算/计价或relaykit变更；准确新SHA CI待验，不等同R1封版或部署。

2026-10-03 23:05 北京时间：用户明确暂缓真实验收，继续既定R1独立开发和自主查错；不再等待测试实例，真实OAuth/账户/账单仍标未验。当前云临时工作区重建后已从远端7964cf7恢复，前批无未提交损失。7964cf7的CI37128917386十项和Docker37128917371三项全通过；合并d62a31ab树b9359b76与HEAD一致，三库R1 .52/1.54/5.86s、schema .06/.49/4.99s、最后model race24.188s，PR仍草稿未合并。

本批仅修Codex授权码交换/refresh-token刷新客户端的重定向边界：0.043s有效红例证明两个正式入口会跟随3xx到第二地址，307/308还会重放含合成授权码/verifier或refresh token的POST，并接受新地址结果为成功。现只在既有OAuth客户端副本拒绝全部3xx，保留Transport、代理、PKCE、超时及旋转后条件持久化，不改变共享relay client策略。301/302/303/307/308跨源及同源其他端点、正常成功/共享client不污染和既有刷新合同定向0.053s通过；最终根Go完整测试/vet通过（model41.631/service14.202/relay0.587s），相关OAuth/刷新race1.554s通过；重建所需原前端生产构建6.17s通过，无前端源码变化。未发送真实凭据、不登录账号，不改预算、数据库结构、TLS策略、前端、relaykit或授权框架；新准确SHA CI仍待验，不能据此宣称真实OAuth已验或R1可部署。

2026-10-03 22:11 北京时间：本批持久保存 Realtime 完整 reported 分段的有界累计父项/模态 Token 数和冻结报价下限；仍用既有 ReviewMetadata、known_realtime_quota 和生命周期锁/CAS，不新建账本或扩大严格预算资格。在 response.done 交付客户端前写入，同序号相同内容幂等，跳序/回退/不一致或人工接管后拒绝覆盖。普通报价对累计值沿原规则取整，阶梯表达式复用每个 response 的 len/缓存规范化与费用，不能把整连接算成一个长上下文；未知/估算/无有效图片价格仍待核对，缓存/reasoning不额外叠加。检查点只是已知前缀及费用下限，不能当完整最终账单或真实订阅扣费。

实际子进程先交付8单位、再发下一请求后强杀，重开SQLite的两个writer原红例均接受Root actual0并退回100；现拒绝0，可靠最终8的授权恢复幂等到992，原纯崩溃留100和正常8+12结算20均保留。检查点写失败时保留已知8在会话内，并通过原待核对重试持久保存，不猜0/自动退款；进程在写入结果尚不明确时消失仍只承诺保留预留，不能声称丢失证据已重建。新合同复用既有三库回归入口。最终根模块完整Go/vet通过（model39.453/service11.716/relay0.583s），定向model/service/relay/OpenAI通过，race2.609/17.444/7.273/1.112s；新SHA三库/CI待验。初次新增小数夹具误按截断断言，改用0.4+0.4检验原有四舍五入一次，不改计价函数；首轮race的正常连接控制曾出现client reset并只保留8，夹具补齐客户端显式存活/关闭所有权，原20及双writer断言不改，最终独立子进程race复验7.044s通过。

前置f77c10d的CI37125999607十项/Docker37125999689三项已全部成功；三库R1 0.94/1.61/4.73s、schema0.05/0.58/19.77s，最终model race24.166s。合并测试d6ca6e53树06acc839与HEAD一致，PR仍草稿未合并。原始missing/estimated测试不变，relaykit/前端不改；真实OAuth、账户、账单及目标部署仍未验，不据此封版或宣称可部署。

2026-10-03 21:19 北京时间：WebSocket持久派送候选本地完整验证通过，尚待同批同步/新SHA CI。仅在按Token的既有Realtime连接成功后、任何帧转发前写协议限定的派送证据；没有清除标记来扩预留。legacy和authoritative实际子进程发出response.create后被强制结束，重开SQLite仍保留100且原退款入口拒绝无证据退回；正常连续8+12从初始5扩预留后只结算20。未知断线、写入失败后保留/重试、明确0、Root拒绝普通用户/幂等恢复、旧会话后续追加阻断均有合同。普通HTTP不获Realtime例外，错误沿既有翻译/禁止重试通路处理。定向model/service/relay0.481/1.781/0.281s，race5.041/34.539/5.549s，完整根Go/vet通过（model39.414/service11.285/relay0.440s）；controller定向无匹配测试不作独立覆盖，完整包已运行。三库协议标记/追加/退款门禁接入既有R1合同，新SHA实库待验；不改relaykit/前端/预算资格或价格，不把当前保护称逐段用量/费用全部落盘或真实账号账单验收。

前置0b391bc最终十项CI/三项Docker均成功；Backend首轮原SQLite并发重放的SQLITE_BUSY及一次失败作业重跑记录保留，原测试未改且本地count3通过。最终Backend所有race实际执行，最后model24.060s；三库六类追加合同均通过（R1 0.55/1.62/5.64s，schema0.07/0.55/7.80s）。合并0fff740b树9d281024与该HEAD一致，PR仍草稿未合并。

2026-10-03 20:23 北京时间：legacy追加预留候选本地完整验证通过，尚待同批同步/新SHA实库验收。两个初始100记录而实际扣至200的红例已通过同事务扣减/累计记录修正；复用缓存主体锁/排空，钱包欠费、有限Key、订阅上限和无余额自用语义保留。同一累计目标重放不重复扣减，持久待核对/终态意图阻断追加，提交确认丢失按原身份读回，不盲重扣/退款。追加订阅的退款合同另先红（遗留100），现同步既有pre-consume累计记录与退款事实200，精确结算150或取消回0且重复调用一致。最终定向model/service0.358/0.404s，model race2.726s、最终service race7.294s，完整根Go/vet通过（model36.134/service10.523s）；原missing/estimated文件不变。Redis批量合同使用隔离miniredis，三库同一追加合同已接既有R1 CI，但本地MySQL/PG未运行，新SHA仍待远端证明。不改relaykit/前端/新预算或价格，不声称WebSocket持久派送已完成。

2026-10-03 20:09 北京时间：e7db850的CI37121222918十项与Docker37121223017三项全部通过；合并4b26d913树d7385dd9与HEAD一致。三库恢复0.52/1.52/5.45s、schema0.07/0.56/8.20s，最后model race23.640s。继续开发而非等待外部实例：legacy追加预留的记录失败/外部待核对两个红例已定位，当前只修扣减与预留记录同事务、终态/待核对拒绝和重复目标幂等。新增正常结算/退款合同又证实订阅追加100未包含在原退款记录中；现将既有订阅pre-consume记录随同事务抬到累计值，并让现有退款事实使用该累计值，保留原身份验证，不放宽退款断言。候选仍待最终完整验证/同批同步及新SHA三库，不能拿e7db850绿色代替；WebSocket持久派送接线尚属后续批。

2026-10-03 19:46 北京时间：WebSocket握手取消候选本地完整验证通过，尚待同批同步/新SHA CI。预先取消不再进入URL/请求头准备或联系上游；克隆现有拨号器保留代理/TLS/自定义拨号，直接Upgrade、HTTP CONNECT、TLS和自定义TLS的握手中取消均关闭临时连接。成功交付后取消钩子移除；新增确定性合同发现“101已确认后取消”曾被候选改写为未派送错误，现保留已接受连接交给原handler收尾，先红后绿，不触发错误的发送前路径。最终channel全包0.206s、定向race1.108s、完整根Go/vet通过；无relaykit/前端/账务修改，复用c00bfe1已验独立模块/580前端证据。原用量/权限/恢复规则不变，WebSocket跨进程派送与扩预留安全仍独立待核查，不能把握手修正称完整崩溃恢复。

2026-10-03 19:13 北京时间：d4ec4f5十项CI/三项Docker已全过；下一Realtime在途/断线候选本地验证完成，尚待同批同步与新SHA验收。两类先报告8再发起/开始新回复而无终态的真实隔离WS合同先红后绿，结束时保留已知计数并进入既有待核对；created/done按活动ID匹配，取消请求不能替代终态，正常及并发乱序完成保留。待接受请求和活动回复各最多128、ID512字节，完成即删除活动记录；自动音频与手动请求无法关联时不猜归属，保持未知，服务端明确关闭自动创建的手动音频可正常完成。适配器定向0.033s、adapter/service race1.124/5.369s、独立DTO race1.017s、完整根Go/vet与独立relaykit全测/build/vet全部通过；正常原帧不重写，无DB/UI/新预算或费用规则。仍不是完整进程崩溃恢复或真实供应商账单验收，持续核查R1独立缺口，不把剩余都归为等待外部实例。

2026-10-03 18:39 北京时间：Realtime原始证据候选已完成本地验证，尚待同批推送/新SHA CI。四个缺失/矛盾JSON合同先红后绿，不确定用量保留预留；新增双writer实际900余额/100预留及拒绝退款/结算证明。真实隔离WS已知段后空usage、类型错、坏JSON、缺response均保留原计数并标待核对；累计先整体验证再提交，溢出不部分写。明确0/完整文本与逐Response计价保持，表达式使用cr才要求缓存总数明确。最终完整根Go/vet、独立relaykit全测/build/vet通过；service/OpenAI定向0.291/0.030s、race5.152/1.087s，DTO race1.039s。旧冻结报价夹具通知快照1000触发异步通知和全局单位恢复竞争，仅将通知快照置高，实际数据库1000、200收费/日志/零退款断言全保留，复验通过。原missing/estimated文件不变，不扩新预算资格、DB/UI、支付或价格框架。

2026-10-03 18:34 北京时间：负责人要求持续推进，每批落实代码/文档/仓库/计划。独立复核新增四合同证实Realtime原始空/缺字段/矛盾usage会确定结算并退预留，已重新打开R1证据收口；此前b136b3f全绿不能替代新边界。当前候选保留原始字段存在性、异常/溢出累计进入既有待核对、正常完整0保留；缓存计价仅在表达式使用cr时要求已报告缓存总数。定向服务/真实隔离WS已绿，完整Go/独立relaykit/race进行中；首轮宽选race暴露旧冻结报价测试的异步通知与全局单位恢复竞争，待按隔离夹具修正，不能跳过。无新预算资格、DB/UI或价格规则，真实账号验收仍单列。

2026-10-03 17:28 北京时间：13add0f源码批的CI37112438675十项及Docker37112438620三项全部SUCCESS。Full handoff真实演练从固定71277bf创建旧数据，当前镜像迁移和原SQLite/WAL备份恢复旧镜像均通过；原用户/有限Key/15单位用量/单日志身份相同，旧用户策略false/revision0且假上游仍仅1次。LAN原安装、Full fresh/同镜像恢复、580前端/Chromium、三库与既有race通过；最后模型race21.646s。合并0eda2dfa树be5fccde与HEAD一致。下一步为真实OAuth、账户窗口/429、实际用量费用和目标环境验收，当前没有真实实例/账户证据，不封版或发布。

2026-10-03 17:11 北京时间：e14fbb4的CI37110932462十项及Docker37110932432两项全成功；原450、新201条续跑/跨页和空白指纹race联合24.291s通过。新旧schema三库实际通过：SQLite0.07s、MySQL0.55s、PostgreSQL6.63s；PG日志明确旧CHAR值64字节、trim后0，另两库0字节。原恢复三库0.50/1.48/5.53s；合并树5ec3950与HEAD一致。 当前按既定队列进入隔离交接源码升级/备份恢复：固定71277bf旧镜像创建合成用户/有限Key/一次15单位请求，原SQLite/WAL备份分别复制给当前镜像及旧镜像，核对身份/限额/日志且不自动启用旧用户零余额策略。复用既有Docker smoke增加Full handoff场景；本地34运行探针及2工作流回归、32静态发行合同通过，新SHA真实双镜像待验。不是已发布版本升级或生产回滚，不宣称R1完成。

2026-10-03 16:44 北京时间：81db217的主CI37109685228九项成功，B2新增旧迁移schema合同在PostgreSQL9.6的CacheApplied断言失败，SQLite/MySQL通过；原450及201条恢复/跨页race已在Backend通过，Docker37109685169两项全过。当前补修识别旧schema的全空白定长指纹，保持原旧缓存已应用约定；当前schema不凭空白推断已应用。64空格夹具先红后绿，增加精确payload指纹断言，输出合成旧列长度以核对实库差异。定向0.423s/race3.229s、完整根Go/vet及workflow检查通过，新SHA实库待验，不删除原失败或标整版完成。

2026-10-03 16:21 北京时间：9610234的CI37107566979十项和Docker37107566732两项全绿，580前端及320/1280未知状态截图已核看。本批按既定队列收口有界迁移：外层只读id/request_id、收据链短页不再查额外空页；生产5秒、200条批次、1000条轮次上限和原450条测试全文均未改。旧race先失败，优化后原用例连续3轮通过；新增半完成head处取消、关闭重开SQLite补齐且重放不重建，以及201条链跨满页终态保护通过。完整根Go/vet与相关race通过，既有三库CI末尾追加原迁移schema合同，新SHA实库/CI待验。不改变费用、结算、未知恢复授权或表达式规则。

2026-10-03 15:36 北京时间：84b1c5b的CI37105753537十项和Docker37105753570两项全绿，LAN原脚本实际构建/健康/初始化登录及Full/SQLite-WAL恢复均通过。本批按R1额度展示收口：Codex当前窗口缺失/null/非数值/越界用量明确显示未知，不显示0%或数值进度条，实际0保留；仅前端展示，不改账户门禁或结算。六个新增回归先红后绿，前端122文件580测试/类型/lint/构建通过，新增320/1280px未知→真实0浏览器检查待新SHA CI。NEXT_USABLE_VERSION开头已整理当前六项执行表；继续既定R1，不等待用户选择环境，不扩历史F1–F8。

2026-10-03 15:12 北京时间：df1ddbe的真实LAN安装在源码镜像构建完成、容器启动后健康失败（Docker37105072138/job111151865218）；原Full/SQLite恢复通过。发现Compose无条件传入PUBLIC_URL作为Secure Cookie可信来源，与LAN HTTP/非Secure模式冲突。当前仅补安装接线：CLI/脚本从最终模式派生可信来源，LAN HTTP明确为空、HTTPS Secure保留准确来源，Full拒绝关闭Secure；Compose保留显式空值，LAN初始化配置同样写空。不放宽应用Cookie/Origin校验或健康等待。四项合同先红后绿，CLI81、运行27、LAN、升级19/发行32及Bash通过；新SHA实际安装与CI待验，详见R1_INSTALLATION_CHECK.md。

2026-10-03 14:59 北京时间：负责人将自行经GitHub安装，并要求先在我们的环境试验。f6ae1d1的CI37099811366十项及Docker37099811371 Full/同镜像恢复已全过；但默认安装仍拉旧v0.2.0-beta.1，R1草稿未发布。当前只增既有Docker smoke内的真实LAN源码安装job：临时私有配置、准确SHA本地构建、原install.sh、健康/镜像/loopback/资源限制和全新初始化认证；保留原Full探针。工作机无Docker，不能把本地静态检查当安装成功；CLI77、运行27、发行workflow10及32合同、LAN和Bash/YAML通过，新SHA真实脚本待验。安装入口及整版剩余边界见R1_INSTALLATION_CHECK.md，不新增系统安装器或发布。

2026-10-03 13:23 北京时间：100429f的CI37098159180十项及Docker37098159164 Full/SQLite-WAL恢复全部成功，此前跳过的backend race均已执行通过，三库恢复0.47/1.00/4.18s。本批仅阻止Gemini原生通配入口将countTokens或未知action当生成请求：在读体、估算、预留和发送前校验显式操作；既有生成/流式/embedding/predict及无后缀默认、内部诊断保持。正确JSON请求头的回归先红后绿，拒绝路径读体次数为0；定向helper/Gemini及race通过，完整根Go/vet通过。无新增计数端点、价格/预算资格、数据库或前端变化，新SHA CI及Full镜像待验。详见R1_CURRENT_ITERATION最新节，真实账户和跨版本验收边界保留。

2026-10-03 12:53 北京时间：ebc8e41的Docker37096798991 Full/SQLite-WAL恢复成功，主CI37096798994九项成功、Backend在旧TestRefundMidjourneyQuotaLegacyFactConcurrent失败（期望4个成功返回、实际3），其后race跳过。原用例本地30次和完整service5次未重现；加入300ms受控慢写稳定复现同类失败（仅1个确认，金额/日志/幂等断言仍通过）：等待者先耗尽50轮，真正持有者仍在执行。本补修只调整测试确认顺序，首轮4调用仍并发，全部返回后重放pending；每调用总上限仍50、间隔仍2ms，保留4确认和全部原金额/单事实/单日志断言，增加慢处理者确实产生pending的断言。普通与慢写各30轮、race3轮、完整service/vet通过；无退款生产逻辑改动，新SHA CI待验。

2026-10-03 12:28 北京时间：ddf60f9的CI37095413909十项及Docker37095413817真实Full/SQLite-WAL恢复全部成功，三库恢复0.50/1.48/5.50s，新Gemini原始用量合同已通过。本批仅将现有发送不确定性保护接到已有/v1/messages、/v1/completions和已验证Gemini生成请求的/v1beta/models/、/v1/models/入口；Gemini适配器自己构造上游action，因此不能仅凭URL后缀决定是否保护。复用现有持久派送、未知保留预留、Root恢复，不改变路由权限、无余额自用或严格预算资格。新作用域/双writer幂等恢复先红后绿，真实HTTP假上游接收断连仅1次、不退款；service/channel定向race6.153/2.047s通过，完整根Go/vet复验通过，新SHA CI/镜像待验。

2026-10-03 12:00 北京时间：7110b9f的CI37092951200十项及Docker37092951223真实Full/SQLite-WAL同镜像恢复全部成功，三库恢复0.40/1.11/5.00s、574前端与Chromium通过。本批只补Gemini有效JSON原始用量/流终态证据：缺失/null/矛盾计数和分类进入既有待核对，明确0不替成本地估算；清除上游注入billing_usage；流要求原始候选结束及对应最终用量，倒退/坏尾部/迟到未完候选拒绝确定计费。缓存与thought按既有包含关系只算一次，不扩大严格Token/USD价格资格或支付。新增DTO/适配器/service合同先红后绿；定向race通过，独立relaykit全测/build/vet通过，最后迟到候选边界补修后完整根Go/vet再次通过。原missing/estimated文件未改，源码文档尚待同批同步，新SHA CI/Full镜像待验。

2026-10-03 11:20 北京时间：18eb5cb的Docker配置在调度前失败（37092770363，无执行job），原因是job级env不能引用runner.temp。改为run步骤用RUNNER_TEMP建立准确0700目录，成功创建后通过GITHUB_ENV传给后续步骤；新增回归防止再放回job env。26探针与workflow32合同通过，恢复尚未真正重跑，新SHA运行证据待同步后核验，不把调度失败称容器启动。

2026-10-03 11:16 北京时间：9269139的Docker37092219301在副本检查SMOKE_RESTORE_DATABASE_MISSING失败，未启动恢复实例，源实例恢复运行。Docker官方说明docker cp不能复制tmpfs，本轮原/data正是tmpfs；空副本被拒绝。本补修将仅本次CI合成实例的数据放入RUNNER_TEMP下准确run/edition/SHA专属0700目录，源与恢复容器以runner UID/GID运行，保留loopback、CPU/内存和日志上限。复制前额外核验实际/data为准确bind源，拒绝tmpfs/其他目录；清理先移除所有权匹配容器，再按准确路径删除自己创建的数据目录。25探针（增加tmpfs拒绝）及workflow32合同通过，真实恢复仍待新SHA复验。

2026-10-03 11:05 北京时间：033c8dc的CI37090904483十项及Docker37090904496真实Full隔离验收全部成功，恢复三库0.48/1.05/4.34s、574前端/Chromium与两条真实请求计量通过；合并树与HEAD树552e870519bb97e72b9c82edbfc37294f051f550相同。本批仅加同镜像SQLite副本恢复：暂停合成实例所有写进程，复制完整/data含WAL，恢复源实例；保留备份校验，再复制到独立临时目录启动同镜像，重登并核对原Key身份、钱包/Key/统计和请求日志。凭据只在内存，无生产数据或新上游请求。此为暂停点一致快照恢复，不是优雅停机、跨版本升级或生产回滚。当前本地25探针测试及13/32/19静态合同通过，新SHA实际恢复与CI待同步后验证。

2026-10-03 10:43 北京时间：7f3650a的Docker37090461731在旧余额夹具立即读统计处SMOKE_FIXTURE_USAGE_MISMATCH；该路径前次通过、本次早于Root检查失败，不能报Root方案已验证。旧夹具同样在响应后结算，本小批把两条路径统一为最多50次/100ms只读等当前请求消耗日志再查精确账务；仅有效空页可等，畸形/重复/不匹配继续失败，不重发请求。旧路径先红后绿，19探针、runtime13、workflow32通过；源码文档同步后继续新SHA实际容器验证。未修改业务结算或验收值。

2026-10-03 10:35 北京时间：f6e64d0的Docker37089960060旧余额用户完整夹具已通过，新Root请求已成功返回实际usage，但立即读取统计/Key时SMOKE_SELF_USE_USAGE_MISMATCH。核对后处理在响应写出之后结算，客户端收到正文不保证结算完成；本补修先有界只读等该request_id唯一self_use消耗日志，再核验原精确余额/Key/统计断言，不重发请求、不接受部分/估算、无日志仍失败。增加先无日志再完成的夹具，确认仅一条上游请求；19项测试、runtime13/workflow32通过。固定错误进一步区分钱包、统计、Key，仍无敏感正文输出。该时序解释尚需新SHA实际容器验证，不能宣称账务已通过。

2026-10-03 10:27 北京时间：1635cd7已同步，Docker37089544065真实Full镜像构建、启动和健康检查成功，但旧余额夹具在额度设置失败，尚未到新Root探针或前端。定位为旧脚本漏传ManageUser必填request_id；本小批只补按构建SHA/测试用户固定的幂等编号，保持余额/Key/日志断言，不改生产接口或开启商业模式。回归先红后绿，19项探针、runtime13及workflow32合同通过；源码文档同批同步后重跑实际容器，不宣称1635cd7全绿。

2026-10-03 10:18 北京时间：006bb7e的CI37087751202十项全过，CLI77、升级19项合同、574前端/Chromium及三库已核验；提交后clean-tree源码打包2719文件通过。当前有界候选复用既有docker-smoke隔离工作流，为当前同仓R1分支PR自动运行Full（手动Full/LAN保持），只构建load、push:false、不登录或发布镜像。新增新初始化Root零余额请求探针，与旧有限余额用户探针并存，检查商业关闭、有限Key实际扣15、用户余额仍0、单次上游与self_use日志。本地19项探针测试、运行及发行工作流合同通过；新SHA CI和真实隔离容器均待同步后验收，不能以模拟响应替代。实际账号/OAuth/账单、升级备份恢复仍未验；详见R1_CURRENT_ITERATION最新节。

2026-10-03 09:49 北京时间：eba51bc十项CI及574前端/三库/Root恢复Chromium已全过，320/1280截图已核看。最新有界候选修CLI升级失败回退风险：目标启动前失败沿用旧恢复；目标启动一旦尝试则保留目标配置/现场，不自动重启旧镜像，因为数据库可能已迁移。dry-run/错误消息明确.env备份不是DB备份，新增故障合同先红后绿，定向9及CLI77测试、升级19项合同等已过；pack依原clean-tree门禁等待提交后复验，同SHA CI待验。无Go/前端/计费变更，详见R1_CURRENT_ITERATION与UPGRADE_REHEARSAL最新节；真实容器和账号、升级/生产回滚仍待验。

2026-10-03 09:16 北京时间：7072dbb已同步，新人工恢复三库实际PASS（联合SQLite0.43s/MySQL1.26s/PostgreSQL5.56s）。573前端测试/构建通过，Chromium发现中文窗口缺少本地化关闭入口（共用图标名是英文Close）。本小批仅在新窗口提供既有翻译Close按钮、保留Escape并补不写入回归；不改英文选择器掩盖问题，不改后端/数据库。前端最终122文件574测试/类型/lint/构建通过，新SHA浏览器待复验，不称前置十项全过。详见R1_CURRENT_ITERATION首节，真实验收仍未完成。

2026-10-03 08:58 北京时间：c7ab856 的 CI37081017788 十项全过，新增双writer派送三库和568前端/Chromium及精确树已核验。本批接用量日志Root待核对列表（有界keyset、含无完成日志的持久派送）与确认请求结束后的人工恢复。现有确定结算不得被覆盖，迟到自动意图/事实在事务内受生命周期锁保护；接管先保存首次金额/操作者/证据摘要，失败后相同命令重试。无新表/支付/协议扩展，旧无证据open/prepared不猜零退款。新增模型/路由race通过，前端122文件573测试/类型/lint/构建通过；补修人工终态后仍可创建迟到新结算工作的缺口，最终Go/vet与模型race6.877s复验通过，秘密扫描无发现；新SHA三库/320和1280px Chromium待核验；详情以R1_CURRENT_ITERATION首节为准，真实验收与未覆盖供应商边界保留。

2026-10-03 08:09 北京时间：a299d6a 的 CI37078122556 十项全过，三库/568前端/Chromium与合并树一致已核验。下一有界候选把普通HTTP文本的发送标记及既有计价证据保存到现有双writer行，保存失败不发上游；重启后的直接退款、后台退款和扩展也需遵守标记，已知拒绝经CAS才清除。无新表或UI，原missing/estimated不变；本地定向及相关race、最终完整Go/vet通过，畸形标记模型race复验4.942s通过，秘密扫描无发现；新SHA三库CI待验。完整崩溃记录发现和Root接管入口下一批再接，旧无证据open/prepared不猜零、不退款，所有实例须支持新语义，真实环境仍未验。详见R1_CURRENT_ITERATION首节。

2026-10-03 07:29 北京时间：3697c89 的 CI37072061927 十项全过，既有三库恢复、568前端测试和Chromium已核验。本批仅补普通Key标准HTTP Chat/Responses按Token计量的发送不确定性：发送后解析/网络等不确定失败保留预留，进入既有待核对并禁止自动重试；发送前取消与原始明确拒绝（含429）保持原语义。双writer/自用/旧限额、Root幂等恢复、写入失败保护及真实HTTP断连单次发送测试通过；本地错误防伪先红后绿，最终根Go全测/vet和相关race通过，原missing/estimated文件未改。无新表/UI/预算资格扩展；源码文档同批同步后必须核验新SHA CI。本轮不是普通Key持久派送或完整崩溃恢复，prepared/open恢复及真实环境边界仍未闭合。详见R1_CURRENT_ITERATION最新节。

2026-10-03 06:17 北京时间：8121b9e的CI37066909385十项全过，三库恢复与响应式用户策略页面已核验。本批继续同一R1，新增Claude有效JSON字段存在性/流终态证据，缺失/null/估算/累计输入输出倒退进入既有待核对；明确0不估算，缓存-only不再误作零消耗，TTL超界不饱和为确定扣费。适配器与结算合同先红后绿，原始missing/estimated文件未改；最终根Go/vet、独立relaykit全测/build/vet与定向race通过，无UI/数据库结构/费用预算资格扩展。源码文档待同批同步，新SHA CI待验；详情见R1_CURRENT_ITERATION首节，其他供应商与真实环境边界仍保留。

2026-10-03 05:24 北京时间：2694048 已同步，CI37065906993 的新增自用/Token/USD/旧限额/审计恢复三库实际通过（SQLite0.35s、MySQL0.94s、PostgreSQL5.31s）。Chromium发现用户策略弹窗随表格从桌面切到移动卡片时卸载，导致未保存状态丢失；本补修将弹窗移到稳定页面层，沿用既有Provider目标用户/弹窗状态，新增跨320/1280px保留勾选的浏览器断言。原失败保留，不降低超时或跳过断言；新SHA仍须浏览器复验。预算/结算/数据库规则不变，真实验收与独立迁移race限制保留。

2026-10-03 05:07 北京时间：前置0b60819的十项CI、阈值三库与Chromium已通过。当前新增非充值自用准入候选：Root独立审计策略、新安装Root零余额、冻结self_use来源、双writer/Key限额/严格预算/未知恢复兼容及用户管理页。现有用户不自动取消限额，新注册普通用户不自动无限；首批仅Chat/Responses，其他入口对新策略拒绝。前端568测试/类型/lint/构建与最终完整Go/vet通过，审计详情及冻结来源路径二次门禁先红后修、定向race复验通过；另删除全仓无调用的历史固定默认口令初始化函数，保留正式Setup流程；新SHA三库与浏览器尚待同步后验收。详见R1_CURRENT_ITERATION首节，不重做前批，不称R1完成或可部署。

2026-10-03 03:18 北京时间：44c0ca1 的 CI37051741438 十项全过，新增阈值三库/Chromium通过。核看截图发现取消冲突开关后旧提示残留，本小批修立即校验并新增组件/浏览器回归；不改后端，待新SHA验证。详见 R1_CURRENT_ITERATION 最新节，真实验收与批迁移race限制不变。

2026-10-03 02:28 北京时间：本批账户/窗口安全阈值已接原生 Codex 可信新采样、路由/固定账户/发送前门禁，以及 Root 设置与所有者只读页；不与当前直连 Token/USD 模式混开。旧样本不回填资格，缺失百分比不再默认满额。根模块测试/vet与前端562测试/类型/lint/构建已过；阈值定向 race 通过，但既有批迁移测试单独 race 仍超时，已如实登记。源码/文档待同批同步，新 SHA 三库与浏览器待验。最新边界与证据以 R1_CURRENT_ITERATION 首节为准；真实环境和最终自用验收仍未完成。

2026-10-03 01:26 北京时间：`b0b7a3f` 费用候选的 CI `37037635553` 十项全过，三库与费用 Chromium 已核验。下一有界迭代为逐账户/窗口剩余百分比避让，限定可信 Codex 采样；不分摊逐 Key 百分比，不与当前直连 API 严格 Token/USD 模式混开，不将旧不完整样本当新证据。具体范围、默认新鲜度与验收见 R1_CURRENT_ITERATION 最新节。真实环境及最终自用验收仍未完成。

2026-10-03 00:52 北京时间：基于 `afaf250` 的费用预算候选已接精确 USD 预留/结算、Root 恢复和 Key 页；费用与 Token 独立开关，复用同一未决请求和 writer。当前本地验证已过，新 SHA 三库/Chromium 待同步后验收。费用模式必须显式 service_tier=default、匹配模型和完整缓存分类证据，只计冻结官网价格下的实际 Token 成本，不等于上游发票。先读 R1_CURRENT_ITERATION 最新节，随后推进账户/窗口安全阈值；真实环境与最终自用模式验收未完成。

23:53 北京时间：`afaf250` 的 CI `37028335783` 十项全过，新增 Token/双账本恢复三库及 Key 页 Chromium 合成操作已通过。下一有界迭代为精确 USD 费用预算，复用现有预留/恢复与冻结官方价格，当前费用预算尚未实现；先读 R1_CURRENT_ITERATION 最新节。账户百分比与真实部署验收仍未完成，不把上一批候选验收等同 R1 封版。

23:12 北京时间更新：前置 `36e72a2` 的 CI `37011043799` 十项全部成功，独立 Token 模型合同已在 SQLite/MySQL/PostgreSQL 实库通过。本批继续同一严格 Token 迭代，新增原生官方 Responses 的精确输入计数/发送上限/持久准入、正常或未知结算、两种 quota writer 的审计恢复，以及 Key 管理页设置/恢复入口。当前代码的限制与验证见 R1_CURRENT_ITERATION 最新节；本批 exact-head CI/新增三库耦合与 Chromium 页面回归须同步后核验，不得复用前批绿色。实际费用预算与账户百分比仍未接，不称 R1 完成或可部署。

## 2026-10-02 后续接管状态（本节优先）

21:00 北京时间更新：`7295ce1` 的 CI `37006732955` 十项全部成功，Chat 零值/缺字段证据已收口；前置价格发布检查点也已通过。当前同一 Token 预算迭代新增独立整数预算、持久请求预留/待核对/Root 审计恢复的模型基础、迁移及隔离合同，尚未接运行时准入、公开 API/UI，不开放配置开关。SQLite 定向与 race 通过，新三库合同待本批 SHA 的 CI；不能说 Token 预算已可用。完整范围与证据见 R1_CURRENT_ITERATION 首节，下文旧待验状态为历史。

19:15 北京时间更新：前置 `3094e5e` 的 CI `36994474226` 十项全过，新增发布合同三库实际 PASS。其后本批已接 Root 预览/确认 API、来源页发布/锁/回滚、请求版本冻结及日志参考成本标识，本地必要验证通过；本批 exact-head CI 与新增 Chromium 操作尚待同步后验证。当前仅支持受限 Standard 纯文本参考计价，费用/Token 预算未接，不能冒充上游账单或 R1 完成。详见 [当前迭代最新节](R1_CURRENT_ITERATION.md)，优先于紧随其后的基础检查点描述。

已验证绿色检查点为 `9410b02`，位于 `codex/r1-usage-review-20261002`；[草稿 PR #2](https://github.com/ForceMind/MyAPI/pull/2) 的 CI `36989602580` 10 项全部成功，包含恢复表单的新增 Chromium 合成操作及三库合同。当前继续有效价格发布迭代，已加入来源候选表达式、数据库原子发布/锁/回滚基础与回归；尚未接通公开 API、页面和请求冻结，不可当成可操作发布功能。本批 exact-head CI 待同步后验证。下文“未推送”和旧红灯是早期记录，最新边界见 [R1_CURRENT_ITERATION.md](R1_CURRENT_ITERATION.md)。R1 新预算和真实部署验收尚未完成。

负责人已恢复 R1 范围内的实现、隔离验证与文档更新，并授权选择安全可落地的业务规则。已明确：严格费用/Token 预算的未知用量暂停后续请求、保留预留等待证据或授权核对；百分比采用逐账户/窗口的剩余安全阈值，不伪造逐 Key 百分比消耗。**这是确定的规则，实际费用/Token 预算及准入联锁仍未完成。**

当前候选从远端 `codex/r1-handoff-20261002` 的 `71277bf6055ce68b8cd1d11f1d10f1907d7696fd` 接管，在当前实际 checkout 开发，不使用下文旧设备路径。原 missing/estimated 合同已经在本地候选转绿；原断言保持。增加持久待核对、授权恢复、幂等预留准入、统计/日志投影与界面，并继续第一轮验收；详见 [当前迭代与证据](R1_CURRENT_ITERATION.md)。

负责人于 2026-10-02 07:03 UTC 授权自主推进既有 R1 开发、源码同步、草稿 PR 和 CI 修复；开发分支为 `codex/r1-usage-review-20261002`，基线为交接分支。远端实际同步和 CI 以回读为准。`9410b02` 的 MySQL/PostgreSQL 隔离实库合同与 CI 已验收；新增价格发布代码需重新验证自己的 exact head。真实浏览器/容器/OAuth/账单及升级回滚仍未验收。正式上线与真实账户/凭据操作另行确认。每次源码与文档同步后主动汇报，附北京时间及距上次汇报间隔；不得用旧快照红灯或局部绿色代替当前事实。

本节是当前入口；下方旧 Linux 模板只保留历史，不作为当前目录、授权或完成状态。启动提示词见 [CODEX_HANDOFF_PROMPT.md](CODEX_HANDOFF_PROMPT.md)，R1 需求及逐批证据见 [NEXT_USABLE_VERSION.md](NEXT_USABLE_VERSION.md)。不重新规划 F1–F8，不重做已完成批次。

**同步结果（2026-10-02）：** 源码快照 `f93695873c4413d42a48fe1af302bee3bc70be98` 共 232 个文件（含继承商业支付 WIP），随后交接文档提交 `17ba246466e285e73b914f9a80fc58af9b347aec`。首次公开推送被安全审核拒绝；向负责人披露范围后，负责人再次明确要求推送，原命令重试成功，没有绕过审核。[GitHub 交接分支](https://github.com/ForceMind/MyAPI/tree/codex/r1-handoff-20261002) 已存在；后续同步状态文档提交以远端同名分支回读为准。不写 main、不创建 PR/tag、不发布或部署，CI 和可部署资格仍未验收。

## Git 与工作树

- 仓库 `https://github.com/ForceMind/MyAPI`；交接分支 `codex/r1-handoff-20261002`，未封版源码快照。提交通过 `git rev-parse HEAD` 与远端同名分支回读，本文不填写自引用 SHA。
- 基线 `57ec31a58fc737ba2be0601e4102123436c3d938`；交接前远端 main 是 `f6536ca96126415f74d239165fd688589bd54af9`，不是该基线。不要先 pull/rebase main，也不要强推或直接覆盖 main；集成与合并另行授权。
- 候选 `/Users/wxx110/.codex/worktrees/next-usable-candidate/MyAPI`：交接开始时 detached HEAD，229 个修改/新增文件条目。快照包含从原工作树继承的商业支付 WIP 及候选 R1 改动，不把全部差异称为本次新增或已验收。
- 主目录 `/Users/wxx110/工作/Prive/MyAPI` 当时干净，分支 `codex/mac-durable-accounting`，仍在基线，没有被候选覆盖。
- 原 `/Users/wxx110/.codex/worktrees/2daa/MyAPI` 保留全部 WIP，只读保护，不 reset/stash/clean、归档或整包搬入。候选不等于原工作树全部成果。
- `VERSION` 仍为 `0.2.0-beta.1`；历史 tag 不代表本候选发布。此次只提交/推送交接源码与文档，不创建 tag、PR、合并、镜像或部署；不包含 `.env`、数据库、真实日志、凭据、依赖缓存和构建产物。
- 本轮读取 Goal 返回 null，没有创建自动化或子智能体；下一模型先查目标，不声称旧 Goal 仍持续运行。

## 已有成果与边界

|流程|已有候选成果|未完成/待验|
|---|---|---|
|自用 UI|新安装商业默认 disabled；关闭模式隐藏购买推广/当前余额；首页概览简化，用户/Key/历史恢复入口保留|旧模式不强改；三种新预算未实现；商业启用资格未验收|
|额度及渠道|同图细线与系列选择，多 Key 采样/身份隔离，Codex 历史按 series_id，测试复用成功 model/endpoint/stream，确切耗尽 429 避让|专用历史无新 UI 选择入口；真实账户、容器 OAuth/刷新、上游测试/路由计数待验|
|usage/实时|Responses 明确零值不覆盖，冻结 usage/reasoning/cache 明细；日志来源标签；冻结单位/模型/工具倍率；实时累计预留及重复扣费修正|未知/部分/畸形 usage 未收口；reported 不等于账单核对；实时有界汇总不是持久逐 response 证据|
|官网价格|固定官网抓取/严格十进制解析；Root 保存不可变 SHA256 来源版本及按摘要读取；独立参考成本计算和 Root 页面|保存不等于发布；未应用有效价格或 Key 预算；差异确认、管理员锁、原子发布/回滚、档位/上下文/地区/工具/多模态资格未完成|

商业支付代码仅保留默认关闭模块及历史恢复，不继续扩展支付商，不宣称可以安全收款，不建设动态插件框架。

## 红灯和规则确认

`go test -p 1 ./service -run '^TestPerTokenSettlementRequiresReportedUsage$' -count=1` 中缺 usage、Estimated usage 合同失败：预留 100 后仍确定扣费 100/120；reported 对照正常扣费 120。service 全包不能宣称通过，历史绿色已被新合同推翻。禁止删测试/放宽断言、猜零价或自动退款制造绿色。

先集中确认两条规则，未确认前不实施关键副作用：

1. 严格费用/Token 预算 Key 遇到未知 usage，是否暂停后续请求、保留预留，等待可靠 usage 或授权人工恢复？少量预留或继续估算不保证硬限额；固定按次/独立工具费用分别处理。
2. 百分比是账户剩余安全阈值，还是每个 Key 在各账户窗口内独立消耗预算？共享账户差值不能可靠归属 Key，不伪造逐请求百分比。

原 2daa 的 usage_unknown 冻结/审计/恢复未纳入候选，legacy 保留仍有进程内边界；不得整包复制来冒充持久保证。改结算前读 `pkg/billingexpr/expr.md`，核对预留生命周期、writer 模式与权限。

## 验证与下一迭代

- 2026-10-02 交接复核：Gitleaks 8.30.1 官方校验和通过，扫描暂存补丁约 1.27MB 未发现秘密（只覆盖本次差异，不是全历史无秘密保证）；暂存 diff 检查通过。relaykit 使用 `GOWORK=off GOMAXPROCS=1 GOMEMLIMIT=768MiB go build ./...` 独立构建退出 0。
- GitHub API 确认基线 `57ec31a58fc737ba2be0601e4102123436c3d938` 已存在于公开仓库；源码快照仅在其上新增一个提交，另有交接文档提交，不夹带额外未核查的祖先历史。未因推送拒绝改变仓库可见性、换工具外发或隐藏商业 WIP。
- 同日重跑上述未知结算合同，1.770s 退出 1，missing/estimated 两项仍失败、reported 对照通过；未修改生产行为或隐藏失败。GitHub API 回读仓库为 PUBLIC，快照范围为候选源码与交接文档，不发布制品。
- 逐批命令/日期见 R1 卡，按变化范围复用，不是当前全部仓库绿色或独立审查。最后 controller/router 全包 2.048s/1.017s、相关 race/vet 通过；最后 service 全包 6.977s 退出 1，仅上述两项红灯，后续该包源码未变。
- 最后官网来源 UI 14/14 合同、typecheck/lint、构建 2.06s 通过；新增七语言键 missingCount=0，历史未翻译项未整体补完。沿用既有 UI 组件、会话隔离与指定 i18n 脚本，未增加项目依赖。
- 320/390/768/1440 合成浏览器稳定布局、保存反馈、失败隐藏旧表、按摘要读取及键盘滚动已观察。`http://127.0.0.1:8769/system-settings/models/openai-pricing-source` 只是本机静态构建+合成 API，进程可能结束，不是后端持久化证明。
- SQLite 和跨库声明/合同代码有部分证据；MySQL 5.7/PostgreSQL 9.6 实库、当前交接 SHA 的 CI、真实容器/OAuth/上游账单、升级/恢复/回滚未验收。relaykit 独立构建历史证据在 R1 卡，新改其 API 后须 `GOWORK=off go build ./...`。
- 官网曾实取不等于当前价格相同。截图留在本机会话可视化目录，未入 Git：`r1-saved-price-source.jpg` / `r1-saved-price-source-mobile.jpg`。

接管先执行 `pwd`、`git status --short --branch`、`git remote -v`、`git log -5 --oneline`、`git rev-parse HEAD`；确认远端交接分支，不切 main 丢失候选。读取本节、R1 卡和 AGENTS 后只定位本轮必需代码。

下一迭代优先“正常结算 → 未知待核对 → 权限恢复 → 幂等限额”的完整可操作闭环，依赖上述规则确认。缺确认先安全诊断，不继续无限增加价格辅助能力/支付功能。随后接价格发布和费用/Token 预算，百分比按确认口径最后接入。每版固定范围、查看入口、验收和停止条件，额外需求放下一版。

## 历史归档：旧 Linux 模板（不用于当前接管）

# My API 新对话启动提示词（Linux 源码开发）

编制日期：2026-09-05；2026-09-06 增补产品、发行与 S5-P 交接事项。将下方文本作为负责人指令复制到在 `/root/myapi` 打开的新对话。
这份模板只有在负责人实际发送后才构成其中描述的授权；文档本身不授予额外权限。
完整任务拆分见 [全项目执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md)。

本提示词用于接管已本地提交的 B2-1 候选，不使用旧 Mac 交接提示词，不重新要求 HEAD 必须停在旧 main。

## 2026-09-06 使用前补充

模板中 B2-1 的历史状态仅用于说明当时授权，实时 Git、[全项目完成执行计划](PROJECT_COMPLETION_EXECUTION_PLAN.md) 和 [完成度审计](COMPLETION_AUDIT.md) 优先。当前已知还有未提交的本任务代码/文档；新对话必须先按模板核对工作树，不得覆盖或假定归属。新对话还必须完整读取：

- [提示词学习与版本中心（S5-P）](PROMPT_LEARNING.md)：默认关闭、三层授权、合成样本/预算/版本/Codex 文件边界；不得把模板当作读取真实日志、调用模型或覆盖宿主指令的授权。
- [发行制品、安装与更新合同](RELEASE_MANIFEST.md)：Full/Lite/Desktop、功能版/安装形态/访问模式分离、Legacy LAN 映射、统一入口、更新/切换/恢复边界；不得把合同当作发布、tag、生产升级、防火墙或公网开放的授权。
- [Lite 与 Legacy LAN 迁移](LAN_LITE.md)：旧 `MYAPI_EDITION=lan` 只映射 Lite/local 或 Lite/lan，升级不能自动 public。

当前局部证据也应按范围读取：Ali、Doubao、Gemini、Hailuo、Jimeng、Kling、Sora、Suno、Vertex、Vidu 的 B2-2A parser 在 HTTP 200 时已由 legacy `DoResponse` 调用，gate-off 非 200 安全兼容 bridge 在 parser 前处理；Task 单个 outbound attempt 的 one-shot body、3xx 不跟随、客户幂等头隔离和 Vertex OAuth JWT 换取无重定向均已有受限制最终测试与独立审查，但没有 durable 提交接线。B2-2B0 的设计确认不能包裹旧 retry/failover，真正 POST 必须等 D02 与 B3-A；当前另有无 caller 的严格 protocol、T0 owner/replay 原子基元、Full Content 字段级幂等脱敏，以及 owner-scoped、no-store 的只读 `GET /v1/task-operations/:id`（DTO/router/controller 已接线），B1a/B1b 另有 JSON 与仅 video form/multipart canonical request fingerprint，仍无 POST、账务或上游。`setting/system_setting/legal.go` 与 `setting/perf_metrics_setting/config.go` 是 C09-N1 的私有 immutable generation：分别保持 legal 原键/默认/controller 输出，以及 perf metrics 四键、整值浮点解析和读时 fallback，仅返回 detached snapshot；另有 `setting/payment_runtime.go` 的 C09-N2a immutable payment generation，它涵盖现有支付 option 与 `TopupGroupRatio`，三者都不读写 legacy global、OptionMap、DB、SDK 或网络，不能被当作付款/回调/账务已迁移。C09-N5a 的 `/api/option/diagnostics` 是 RootAuth/no-store 的有界主库只读诊断，业务层不触碰 OptionMap/runtime/LOG_DB/Redis，但平台鉴权/限流仍可使用自己的 cache；不回显 values 或不可信 key，MySQL/PG/CI 仍待。`service/promptlearning` 是未接线的 P1a/P1b 纯准入/脱敏/指纹内核：仅 package-private 完整 server-observed turn 可提取一个可信新增用户段，排除段/客户端来源声明不进入样本或指纹，仍不能读取真实日志或计数。`service/accesspolicy` 是 S3-PRE1 无 caller 的 detached snapshot/diagnostic，所有 mode 都不应用策略、不触碰路由/价格/账务/缓存；D04、audit ingress 和 enforce 仍待。`cli/lib/release-manifest.mjs` 与 `installation-state.mjs` 分别是 schema-1 结构选择和内存安装状态/cleanup plan，且拒绝 Proxy/访问器等 hostile JS 输入；它们不能获取/验证资产、读写文件、安装、更新、切换、回退。上述子范围均不改变 C03b gate、生产、真实模型或宿主文件边界。

本轮的其他有限本地证据如下：B2-2B1a/B1b 已补严格 JSON 与仅 video form/multipart canonical digest/版本化请求 HMAC，仍未接入 POST/账务/上游；C09-N1 现有 22/24 个注册族采用受控快照/复制，新增 `gemini`、`billing_setting`、`payment_setting` 与 `global` 后，仅 `performance_setting`/`channel_affinity_setting` 分别等待 D14/D15。Gemini 的 `ConvOptions` callbacks 与 request snapshot 同代；billing 的 mode/expression map 同代深复制，价格、定价与同步读取各自单次捕获；payment 保持七个既有持久键及金额 map 的 detached snapshot，不接 PaymentRuntime、SDK、订单或回调；global 深复制黑名单/策略图，缓存中旧 `ConvOptions` 保持旧代、重试才按既有逻辑重建。连续单 key 写入仍不构成跨 option 原子性。既有 token/quota/Grok/Qwen/fetch/OAuth 的兼容边界保持不变；S5-Q P2A 仍仅生成 fail-closed occurrence identity；`release-manifest-evidence.mjs` 仍仅处理未受信 raw bytes；`legacy-installation-profile.mjs` 仅把显式 Legacy Docker 配置 fail-closed 映射为 Full/Lite 与 local/LAN/needs_manual，永不推断 public、不读文件或接线 CLI。适用的单核/768MiB test/vet 和未参与实现者独立审查均已完成（manifest/installation state Node 28/28，evidence 9/9、legacy profile 7/7）；没有 MySQL/PG 当前 SHA、CI、真实日志/模型、文件安装或外部网络验收。

`performance_setting` 是余下两个 C09 热读族之一：只读盘点已发现它分开发布 disk/monitor 投影，磁盘缓存消费者可在一次操作中多次读取配置，不能以 source 指针原子化冒充全链路同代。D14 必须先决定 `DiskCachePath` 热切换及跨投影一致性；此项没有代码、测试或运行行为变更。

`billing_setting` 已完成最低快照化：`billing_mode` 与 `billing_expr` 同 generation 深复制，`relay/helper/price.go`、`model/pricing.go` 与同步数据各只读一次 snapshot；不改 `pkg/billingexpr/expr.md` 的表达式/额度合同。兼容单 key 更新不能宣称跨 key 原子，统一提交仍留给 C09-N3 typed bulk；本项有单核/768MiB test/vet 与独立审查证据，但不改变运行中的账务语义。

`channel_affinity_setting` 是余下热读族中的 routing/cache 高风险项：规则/嵌套模板影响渠道、retry、上游参数和账务归属，capacity/TTL 却只在 HybridCache 首建读取。D15 必须先决定重启或受控 drain/rebuild/epoch；不得在普通快照化中静默清 cache、迁移 Redis、改变规则优先级、retry 或账务归属。本项没有代码、测试或运行行为变更。

本轮 P0 文档整合及上述纯子范围不表示 S4-D、完整 S5-P、S6 或 S7 已实现。实施前仍按 AGENTS 的“先方案、明确批准、再执行”规则，且保持 B2/B3/C03b 主链优先与共享文件单写者。

```text
你接管 My API 在 Linux 开发目录 /root/myapi 的全项目后续开发。

请完整读取并执行 docs/PROJECT_COMPLETION_EXECUTION_PLAN.md，按其中完整目标推进到交付就绪，
不是只完成 B2，也不是重做已经验收的功能。我批准已明确合同范围内的实现、测试、审查修复和文档收口；
不要为同一范围的常规修复反复请求确认。计划中“建议待决定”的核心业务选择仍按阶段集中说明，
没有答复不得假定已批准；同时继续不依赖该选择的独立工作。

绝对边界：
- 只操作 /root/myapi 源码及本任务隔离临时目录；不访问或操作 /root/new-api 生产运行目录。
- 不改生产 compose、数据库、日志、配置、容器；不重启生产、不部署。
- 不发布 GHCR/NPM，不登录 GHCR，不创建/移动/删除 tag，不强推，不合并受保护分支。
- 不 git reset、git clean、强制 checkout，不覆盖或回退未提交工作，不自行 stash。
- 不全机清理或安装，不读取真实 Codex/Claude/浏览器/Keychain 凭据做测试。

现有成果：
- 仓库 https://github.com/ForceMind/MyAPI.git。
- 交接时分支 codex/b2-durable-submissions。
- B2-1源码候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457，随后可能有纯文档审计提交，实时HEAD以Git核对。
- 前置B2-0合同提交513ea6d6e863883aa3415a785a4d858bcf7210b0不是当前源码候选。
- 前一 R4 tip 83f24c5，功能 c5cf662、审计 b77ae6b；不要重做已有 R4。
- B2 尚未完成；B2-1最终DB时钟候选受硬限制定向通过，独立代码审查无P1/P2；实库CI仍未运行。
- GitHub CLI 曾确认登录，但B2-0/B2-1均未同步；最新push被自动审批拒绝，要求明确该批内容和目标的外发授权。
  不把登录状态当成推送成功，不绕过自动审批。本地工作与同步阻塞分别处理。

先核对当前内容、Git差异和已记录提交；第2.1节是修复前历史指纹，不得用它回退修复后的文件。
源码候选已经提交；如果工作树不干净，立即停止并报告具体文件，不自行stash、回退或推测归属。
历史草稿清单不是忽略当前未提交修改的许可；如确需接管，应先取得负责人对实际修改的单独明确授权。

第一步只做只读核对：
pwd
git status --short --branch
git remote -v
git log --oneline --decorate -5
git rev-parse HEAD

然后完整读取：
AGENTS.md
/root/.codex/AGENTS.md（存在时）
docs/PROJECT_COMPLETION_EXECUTION_PLAN.md
docs/MYAPI_MASTER_PLAN.md
docs/DEVELOPMENT_EXECUTION_PLAN.md
docs/COMPLETION_AUDIT.md
相关专题及目录内 AGENTS.md。docs/DEVELOPMENT_ON_MACOS.md 是后续 Mac 验收资料，
不是本机环境事实。保留当前功能分支，不为对齐旧 main 而先 switch main / pull。

持续目标：
完成 S2-B2/B3、C03b 全账务与缓存恢复、C09 剩余配置/限额、S3 账户和 Key 真正策略、
S4 完整运行/升级/备份恢复、S5 额度闭环及确认的扩展、S6 完整独立 UI/官网、S7 交付就绪。
保留原协议、B1 计费快照、导入、日志脱敏、额度分析、Full/Lite/Desktop 与 relaykit 独立性；Legacy LAN 的安全边界按迁移合同保留，不自动开放公网。
读取已有目标后使用产品提供的方式维护或恢复，不重复建目标，不把未完成目标标成 complete。

顺序：
R0 接管与当前表 → B2-1 安全模型修复 → C03b-0 共同合同 → B3-A 原子账务核心 →
B2-2 完整提交/查询 → B3 恢复/outbox → C03b 全写入迁移/切换演练 → S3/相关 C09 收口 →
S4/S5/S6 分依赖推进 → S7 全项目验收。
Adapter 纯解析、CI fixture、S3 设计、S4 设施等独立任务按计划并行，不制造循环依赖。

已确认 B2-0：
- submission_unknown / outcome_unknown 不自动重发或退款；仅 Provider 可验证或审计人工处置。
- DISPATCHING 后 v1 禁止可能已送达请求的重试/failover，包含 Provider/Transport 隐含重试。
- 幂等 scope token + HTTP method + operation kind；同 key/同指纹重放，同 key/异指纹 409。
- 活动 operation 不过期，终态保留 180 天；先持久化，再统一 202，所有状态可查询。
- 主库 Task 账务事件唯一权威，分库/ClickHouse 日志至少一次投影，查询/导出/统计去重。
- 现代 Task 首批；Midjourney 后续单列；C03b 完成前 gate 关闭；旧 writer/poller 排空升级后才可启用。
- 上述启用仅在隔离测试中验收，不操作生产。

B2-1 首先修复：
专用且跨节点/重启稳定的幂等秘密与误换检测；规范业务事件键；v1 单 attempt；
GORM 不可变载荷保护；outbox 权威一致性及数值边界；补旧表迁移和 outcome_unknown 测试。
不要用现有全局 DB/异步 Redis/内存 batch helper 冒充原子账务事务。

多智能体：
启用最多四个活跃角色（含主代理），明确文件所有权；主代理整合与唯一测试调度，
两个独立模块实施，按阶段安排未参与实现的独立审查者。模型/强度依可用 AGENTS 与实际工具能力选择，
不虚构切换。接口和共享迁移文件单写者，不并行争写或重复测试。

资源限制：
- 约 2 CPU / 3.5GB。Go GOMAXPROCS=1、GOMEMLIMIT=768MiB、go test -p 1。
- 本机必要测试使用进程组 CPUQuota=100%、MemoryMax=768M 等有效硬限制。
- Node/Bun 单 worker、进程组内存不超过 768MiB。
- 不并行运行 Go、前端、浏览器、Docker 构建。
- 全量、race、三库/Redis/ClickHouse、浏览器、Docker、桌面优先 GitHub runner；Docker push:false、不登录 GHCR。
- 临时缓存只用任务独立目录，完成且无进程使用后只清理自己创建的部分。

GitHub 同步范围：
我同意把本项目阶段代码与技术文档按功能分支同步到 ForceMind/MyAPI，并在遵守仓库模板后创建 Draft PR，
供 CI 和审查；不包括合并、发布、改变可见性、真实凭据或生产数据外发。
我明确批准将 B2-1 候选 fea46372974d0f9aaf2c0ae03853fc6cab1cb457、尚未同步的B2-0等祖先以及后续任务内审计提交中
本项目源码、合成测试和技术文档发送至 ForceMind/MyAPI 的 codex/b2-durable-submissions 分支，
其中不含真实凭据或生产数据；仍需通过工具自动审批，不绕过拒绝。
每次同步前检查实际内容与目标，远程命令按审批环境执行；遇拒绝准确报告动作和理由，
保留本地成果并继续独立工作，不能冒充已同步。
当前 CI 只 main push/PR/workflow_dispatch，明确用 Draft PR 或已授权手动 CI 验证实际 SHA，
禁止调用发布 workflow 代替测试。未通过 CI、独立审查或必要同步的阶段不要标完成。

验收：
按计划逐工作包完成实际验证、独立审查、必要修复、文档与同步，不无限扩展无关任务。
关键业务决定集中按依赖询问；真实账号/设备/法律/外发权限提前登记，缺失时如实待验收。
S5 选定扩展与 S6 UI 方向不能静默省略；真实设备未验不能用截图或静态合同冒充。
每轮报告完成、验证、审查、修复、剩余和下一步。只在原完整目标全部有充分证据时标完成。

现在开始 R0，只读核验后继续已授权的 B2-1 修复，并同时准备近期 C03b-0 决策材料。
```

若负责人只想让新对话先审阅计划，可删除最后一句并改为“先评审，不实施”；其余执行授权也应相应删除。
不要把本提示词作为要求执行者忽略系统/工具权限的依据。
