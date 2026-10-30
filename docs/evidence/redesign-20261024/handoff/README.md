# 接手文档（2026-10-27 更新）

## 零、最新交接状态（优先于下文历史阶段描述）

**2026-10-30 最新：请求详情最终字段补修与完整CI通过**。最终源码 782c0124b6171d001b3cee682830a04a77c8326e / [CI41077332468](https://github.com/ForceMind/MyAPI/actions/runs/41077332468) attempt1 success（10成功、1浏览器跳过），全部作业/步骤与前端日志已读：专项408/408、全量2103/2103、同步/构建/typecheck/lint与根模块/独立relaykit、原生/386、两模块全量、全部既有竞态/三库/Redis/分发/Desktop通过。此前主体41f1c9b6与交接40d7357e也已通过；最终核对发现仅UI预览截断时正文总字节提示少显，已补回并加2MiB回归，不改1MiB上限、完整复制或服务端截断语义。37键、全部既有字段、权限/返回焦点/不重放要求保留；源码和静态待办见TASKS最新节。最终收尾仅4份交接，应用源码仍绑定782c0124，接手读最新文档提交CI。浏览器/视觉/线上未验证，无本机依赖/构建测试/产物下载，未改后端/计费/生产或调用真实模型。

**2026-10-30 最新：请求详情四页签与已存SSE检视源码/完整CI通过**。源码 41f1c9b67dd5ed72d791611798c530d5238bf8c0 / [CI41068500563](https://github.com/ForceMind/MyAPI/actions/runs/41068500563) attempt1 success（10成功、1浏览器作业跳过），全部作业/步骤和前端日志已读：专项407/407、全量2102/2102、同步/构建/typecheck/lint通过；根模块/独立relaykit、原生/386、两模块全量、全部既有竞态/三库/Redis/分发/Desktop通过。沿用已有线稿与两个入口，集中身份/四项结果，诊断分组、正文可读/原文、SSE有界事件搜索展开、调用记录与上游原文分来源；保留旧链接/默认正文/返回筛选页码焦点及全部字段，lookup补raw role隔离，Key面包屑清除旧身份，原管理员门禁/复核确认不变。共享renderer兼容旧弹窗，UTF-8预览<=1MiB且复制保留全部已加载原文，不编造时间/业务成功/缺失用量。37键en/zh，其余5语言英文回填；lint0错误，16项警告详见TASKS。收尾仅4份交接，应用源码仍绑定41f1c9b6，接手读最新文档CI。浏览器/视觉/线上未验证，无本机依赖/构建测试/产物下载，未改后端/计费/生产或调用真实模型。原预算/订阅金额展示口径与旧MIME helpers差异只记静态待办，不自动扩改。

**2026-10-30 最新：UI第二批三个日志列表源码与完整CI通过**。源码 b9a5d85519a1d8be2164d78cfe31c109fc9f4e56 / [CI41041830295](https://github.com/ForceMind/MyAPI/actions/runs/41041830295) attempt1 success（10成功、1浏览器跳过），全部作业/步骤与前端日志已读：专项315/315、全量2046/2046、同步/构建/typecheck/lint通过，根模块/独立relaykit、原生/386、两模块全量、全部既有竞态/三库/Redis/分发/Desktop通过。沿用requests/drawing/tasks线稿，复用单日历/分页，统一草稿后查询、首尾/数字/跳页/20-50-100、手机完整字段与只读入口，事实/HTTP/用量/结算分别呈现。前端allTime布尔状态区分双端清空与首次默认日期，保留日期0、ms/sec、未知/错误与权限隔离；新增9键en/zh，其余5语言英文回填。前两轮专项/fixture/lint问题已修，精确断言保留，红绿记录见TASKS最新节。收尾只改4份交接，应用源码仍绑定b9a5d855；接手读取最新文档提交CI。浏览器未验证，未改后端/账务/生产/发布或真实供应商，无本机依赖/构建测试/产物下载，不称全站UI或视觉验收完成。

**2026-10-30 UI收尾文案修正**：最终自查发现旧键 `This month` 的中文为“本月获得”，新日期预设改用独立 `Current month`（中文“本月”），补齐七语言键，保持四项预设行为断言；本批新增键合计22个。此前源码3e31517e的完整资格验证仍有效；交接提交060c4acd的[完整CI41017482486](https://github.com/ForceMind/MyAPI/actions/runs/41017482486)也已成功（10成功、1浏览器作业跳过）。本次收尾提交还需回读其完整CI，未改其他页面、后端或生产，浏览器未验证。

**2026-10-30 最新：UI首批正文/SSE分页与单日历范围已实现并通过完整CI**。源码 `3e31517e243ac92ab95bbdba3b17eb2c59e404a9` / [手动CI41013409921](https://github.com/ForceMind/MyAPI/actions/runs/41013409921)，run_browser=false、body_ui_baseline=true。固定旧源37450c2a的两项缺陷先按预期失败，恢复候选后分页/日期/返回状态专项78/78、前端全量1974/1974、同步/构建/typecheck/lint通过；完整CI首次success（10成功、1浏览器作业跳过），全部作业/步骤已回读，含根模块/独立relaykit/原生与386构建、两模块全量、既有竞态/三库/Redis/分发等。收尾只改交接，应用源码仍绑定3e31517e，接手读取最新文档CI。提供首尾/数字页/直接跳页、20/50/100条；按history entry恢复已应用筛选/page/pageSize，身份/权限失效清除，有界内存不写URL/storage；同一日历选起止、跨月/预设/秒级时间/取消应用/开放端点与非法时间校验。新增21键en/zh、其余5语言英文回填；必要补齐原Calendar焦点ref。既有线稿/方向3/Logo不重做，未改后端/计费/生产或诊断控制器，不打包部署新镜像；生产上次已核对源码3e6b8cca，不因dev新提交自动更新。浏览器未验证；准确进度见[TASKS](../TASKS.md)及[线稿实现记录](../ui-design-v2/wireframes/README.md)。

**2026-10-30 最新：3e6b8cca已部署；本轮只读取证结束，控制器修复待确认，随后回UI**。生产实际app为已验收源码3e6b8cca（目标config 38b61c5b…，服务器已校验等价的loaded ID c303b3e8…），最终容器healthy、缓存会话开关true、诊断false；本轮只读复核保持该状态。部署当时三阶段9479条消费/9680条日志及各自累计扣费一致、完整性/FK检查通过，使用最新数据、保留旧资源和备份，未恢复旧快照。单次窗口02:51:17—02:51:19约2.5秒、保存挂载日志327字节、自然样本0，不能称缓存命中验收通过。已证控制器以max(累计日志增长,进程自启动wchar)>=80MiB提前关窗；该口径混入非日志写入。退出2与初始化时收到停止信号的运行时路径吻合，但历史信号事件/调用栈及当时计数/PID未保存，仍属推断。详细证据、边界和待确认最小方案见[缓存专项第0节第六轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。本轮只改4份交接，不改产品/服务器文件/生产配置，不重开诊断、不发模型请求；推送后读取本次文档CI，镜像仍绑定3e6b8cca。下一批回到已有UI线稿的正文/SSE分页和单日历范围选择，不重画全站，不自动实施或跑浏览器。

**2026-10-30 最新：缓存修复新镜像已在Actions打包验收，生产方案待确认**。源码 `3e6b8cca333194644a8bb2902f662095b407e090` / [完整CI40911707379](https://github.com/ForceMind/MyAPI/actions/runs/40911707379)通过；[Full+SQLite 40924927031](https://github.com/ForceMind/MyAPI/actions/runs/40924927031) attempt1 success，构建/独立加载2作业成功、LAN安装跳过，20项检查全true，browser=not_run。新[镜像包](https://github.com/ForceMind/MyAPI/actions/runs/40924927031/artifacts/12919793136)与[报告](https://github.com/ForceMind/MyAPI/actions/runs/40924927031/artifacts/12919559223)至北京时间11月6日01:51/01:53；准确manifest/app身份在[缓存专项第0节第五轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。包内缓存会话开关默认false；将交接服务器任务先提出更新镜像及开启开关的最小方案，维护者针对该方案确认前不得动生产，只拟观察自然流量，不主动模型/采样/凭据刷新。本机未下载构件/安装依赖/构建测试，未发镜像注册表或tag，未部署。收尾只改交接，镜像仍绑定本段源码，接手读取最新文档CI。

**2026-10-30 最新：A/B/C最小修复已通过产品完整CI**。候选 `42a258dfecc8e584a231f6b81955440091a969d1` / [CI40905811752](https://github.com/ForceMind/MyAPI/actions/runs/40905811752) 首次success（10成功、1浏览器作业跳过）；已读全部作业/步骤，两模块测试、独立relaykit、原生/386构建、全部竞态、三库/Redis/分发等通过；前端24/24专项、1917/1917全量、同步/构建/typecheck/lint通过。收尾只改交接文档，应用仍绑定42a258df，接手读取最新文档CI。产品/探针源码 `9fbf640e` 的[对照40905060265](https://github.com/ForceMind/MyAPI/actions/runs/40905060265)成功，后续42a仅工作流/locale顺序/合成夹具修改，后端与对照工具等价。A补usage形状/四态投影及内部计费信封净化，B默认关闭 `MYAPI_CODEX_CHAT_DEFAULT_CACHE_IDENTITY_ENABLED`，C只读保存窗口刷新并区分旧错误/WHAM失败/转发限额。全部验证在Actions；浏览器未验证，未部署/构建新Docker包/改生产/发真实供应商请求，不承诺真实缓存命中。准确证据与边界集中在[缓存专项第0节第四轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)和[工具说明](../../../../tools/cache-compat-compare/README.md)。

**2026-10-29 最新：缓存身份/usage合成对照已通过，产品方案待确认**。测试/工具源码8f9e533c的[对照40888007112](https://github.com/ForceMind/MyAPI/actions/runs/40888007112)与[完整CI40888007450](https://github.com/ForceMind/MyAPI/actions/runs/40888007450)首次success（常规10成功、1浏览器混合作业跳过）。固定sub2api a3298b46只在runner检出，双方实际实现共用合成输入：MyAPI缺省不补key/session，sub2api稳定补齐且按Key/账号隔离；2种兼容usage形状缓存7在MyAPI变0、Sub2保留7，明确零/未知字段差异也已记录。**本批只测试/工具，不改产品**；另一会话0d9b794a/f0ae4505已保留，生产cd35不代表已含开发分支修复。采样反馈只读发现后台近期正常，连接页把旧限额/error系列统一标失败且缺少自动刷新，未验证浏览器实例。矩阵、报告与待确认最小方案见[缓存专项第0节第三轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)及[工具说明](../../../../tools/cache-compat-compare/README.md)。收尾只改文档，仍读取最新收尾CI；后续产品修复/生产动作另行确认。

**2026-10-29 空Content-Type采集修正已通过代码CI与镜像验证**：源码 `cd35f867d43b0a42d61500221bb9f99eb457e36a` / [完整CI40846159247](https://github.com/ForceMind/MyAPI/actions/runs/40846159247) 首次success（10成功、1浏览器混合作业跳过），定向13族及空头流式/非流式Chat的8个四状态用例全PASS，无FAIL或DATA RACE。[Full+SQLite40846469827](https://github.com/ForceMind/MyAPI/actions/runs/40846469827)首次success，通用20项全true，browser=not_run；[新镜像包](https://github.com/ForceMind/MyAPI/actions/runs/40846469827/artifacts/12885307002)/[报告](https://github.com/ForceMind/MyAPI/actions/runs/40846469827/artifacts/12884514813)至2026-11-05。产品仅日志观察器16行：空头按已传输完整JSON的显式stream选SSE/JSON，不依赖客户端模式；非空头、未知协议、默认关闭和原断言保留。准确身份与边界见[缓存专项第0节第二轮](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。旧d2b1f664生产窗口的12条原始usage未采集，不是供应商零命中；旧镜像不含修正。本轮只在Actions构建测试，未动生产、未开诊断、未发模型请求，后续部署/窗口另行确认。收尾只改4份文档，镜像继续绑定cd35f867，接手读取最新文档CI，不沿用下方旧包。

**2026-10-29 上游日志取证插单已通过代码CI和镜像验证**：代码 `d2b1f66450364a9299f676bfa519aedc692dc367` / [CI40828494736](https://github.com/ForceMind/MyAPI/actions/runs/40828494736) 首次success（10成功、1浏览器混合作业跳过），新增诊断8族与原会话头4族定向竞态测试全部PASS，无失败或竞态报告。[Full+SQLite镜像验证40828937555](https://github.com/ForceMind/MyAPI/actions/runs/40828937555) 首次success，20项全true，浏览器未验证。[镜像包](https://github.com/ForceMind/MyAPI/actions/runs/40828937555/artifacts/12877962270)与[报告](https://github.com/ForceMind/MyAPI/actions/runs/40828937555/artifacts/12878195602)保留至2026-11-05。新增 `MYAPI_UPSTREAM_DIAGNOSTIC_ENABLED=false` 默认关闭，还须正文日志开启才生效；复用JSONL脱敏/轮转，新增 `upstream_request` 与 `upstream_usage`，按 `request_id + upstream_attempt + channel_id` 关联。新阶段需管理员直接读取JSONL，现有页面/API仍只展示原入口/出口，未改UI。准确镜像身份、字段语义与部署交接见 [缓存专项第0节](CACHE_REVIEW_20261027.md#0-2026-10-29-默认关闭的上游诊断日志)。后续收尾只改交接文档，镜像仍绑定d2b1f664，不重标成文档SHA；接手仍读取最新文档提交CI。本会话未部署、未开启生产开关、未发真实模型请求；本批只补取证，不宣称缓存根因已修。

**2026-10-28 当前转回UI设计，产品实现尚未开始**：维护者要求全站页面/按钮/组件统一设计与合理精简，**每页先画线稿**。当前设计提案在[全站线稿目录](../ui-design-v2/wireframes/index.html)与[说明](../ui-design-v2/wireframes/README.md)：326个页面/分区/交互视图、652张桌面/手机SVG，覆盖66条路由、42个设置分区和54个内部页签/模式映射，含详情页签、安装步骤、弹层与状态，不把视图数写成实际路由数或已实现页数。首份设计提交`88165830`的[CI40682248336](https://github.com/ForceMind/MyAPI/actions/runs/40682248336)首次success（10成功、1个浏览器混合作业跳过），实际只触发CI；后续收尾补充仍须读取自己的完整CI。优先设计正文与SSE直接跳页、每页条数和同一日历选择时间范围；产品代码未改，浏览器未验证，待维护者审阅后分批实施。CLI来源规则后移，UI方向3与Logo不重做。

维护者已单独授权将dev快进合入main，当前两分支基线均为`c916051e`；[main CI40664697561](https://github.com/ForceMind/MyAPI/actions/runs/40664697561)首次success（10成功、1个混合浏览器作业跳过）。**执行偏差必须保留**：main推送还触发了既有[网站浏览器40664697462](https://github.com/ForceMind/MyAPI/actions/runs/40664697462)，Chromium在GitHub执行成功，发现并请求取消时已结束；不能声称该轮完全未跑浏览器。网站显式授权开关尚未落实；此线稿批次只改docs，仍须读取真实CI。后续不因此获得自动合main、发布、部署或真实供应商授权。

**2026-10-28 临时测试部署已验收**：测试源码 `cf6773fdf7c5c1365291cdee06d7b25897fa7e4f` 的 [主CI40640242136](https://github.com/ForceMind/MyAPI/actions/runs/40640242136) 为success（10个作业成功、1个混合浏览器作业按规则跳过）；[Docker smoke 40640591332](https://github.com/ForceMind/MyAPI/actions/runs/40640591332) 为success（Full打包、独立加载验收成功，LAN安装跳过）。Full + SQLite、linux/amd64 报告20项全true，含7个静态资源的HTTP读取、鉴权、合成上游精确用量、原容器重启和SQLite/WAL同版本恢复，`browser=not_run` / 浏览器未验证。镜像ID、manifest SHA-256、镜像包与报告链接在TASKS同名条目，交付物有效期至2026-11-04。其后收尾提交只改文档，不重建镜像；镜像继续绑定cf6773fd，不能重标记成文档提交SHA。全程标准GitHub-hosted Ubuntu runner，本机仅编辑文本，不安装依赖或下载产物，不触碰生产或真实供应商；公开仓库、10GiB缓存上限，不启用付费资源。临时容器清理已成功，不提供常驻测试网址，不代表生产迁移、真实供应商或跨版本回滚验收。原会话头修复基线及历史全绿证据保留；当前不得沿用11/11全执行口径。

维护者已明确批准 **Codex 会话头最小修复**，要求不放宽断言、提交推送 `origin/dev` 后读取完整 CI，构建与测试全部在 GitHub Actions。核心修复 `069f67d7` / [CI 40500636411](https://github.com/ForceMind/MyAPI/actions/runs/40500636411) 与显式头值映射补充 `9795c0f5d4c3cc52beffcebebee1757364f7a3cf` / [CI 40504125721](https://github.com/ForceMind/MyAPI/actions/runs/40504125721) 均已11/11成功，远端已回读。其后的交接提交仅更新文档，接手时仍须核对最新Git与Actions。缓存专项细节和日志节选见 [CACHE_REVIEW_20261027.md](CACHE_REVIEW_20261027.md)。

- **工作区已整理成功**：主检出 `/Volumes/Work/Prive/MyAPI` 本身就是 `dev`。`dev-work`、`fetch-latest-repo-2c038a` 两个嵌套工作区已移除，未删除任何分支、仓库 `.git`，未执行 prune。**不要重跑仓库里的旧整理脚本**；它不是本轮获批加固后执行的临时副本。
- 内置磁盘的两个 Codex 工作区仍登记在仓库中，未操作。维护者删除旧项目后若报工作区错误，由维护者执行 `git -c core.fileMode=false worktree prune`，接手者不要擅自清理。
- **最近产品全绿基线**：`9795c0f5d4c3cc52beffcebebee1757364f7a3cf` / [CI 40504125721](https://github.com/ForceMind/MyAPI/actions/runs/40504125721)，11/11成功，含会话头核心修复与显式值映射补充。`069f67d7` / CI40500636411、`e7eef352` / CI40447186866、`d3beeeef` / CI40284651016 也全绿，下文“5b、5c 未经过 CI”已是历史。
- **修复前红灯基线**：`e8652185be292ad676417a67cee0dee041df5af1`，仅新增 `router/codex_session_headers_test.go`；[CI 40462005121](https://github.com/ForceMind/MyAPI/actions/runs/40462005121) 10/11成功。随后仅改6份文档的 `50bad4db` / [CI 40474459858](https://github.com/ForceMind/MyAPI/actions/runs/40474459858) 仍仅同一测试族的5个子场景失败。
- **当前产品/测试提交**：`9795c0f5`，产品只改 `relay/common/override.go`，测试只补两处现有文件。三个 Codex 会话头默认引用在参数操作前建立，运行时最终映射不补回缺失项；显式覆盖、keep_origin、delete_header 保留，不修改冻结渠道配置。参数操作读取真实客户端值，显式值映射对照已随完整CI通过；原取证断言未改。
- **已修并全绿**：`6392caf9` 修复 Chat → Responses 漏传 `prompt_cache_retention`；[CI 40443871659](https://github.com/ForceMind/MyAPI/actions/runs/40443871659) 11/11 成功，根模块与 relaykit 测试、relaykit 独立构建均通过。先红证据为 `92e81914` / CI 40441341548，合成 Codex 出站预期 `24h`、实际空。
- **修复前已实测**：会话头取证在 5 条路径中收到空的 `Session_id`、`Thread_id`、`X-Codex-Turn-State`：缺缓存键、模型名不匹配默认亲和规则、Chat → Responses、渠道正文直通、全局正文直通。原取证断言及覆盖/删除、隔离、严格门禁、不重放、显式值映射保护已在当前产品提交的完整CI通过。证据是 Actions runner 的回环合成上游，**不是实际缓存命中验证**。
- **重要授权边界**：本次会话头最小修复已获明确批准，无需再次询问同一方案；其他缓存统计疑点、UI返工、部署与真实供应商验证不因此获得授权。不得为绿灯删测试、改为空值期望或放宽门禁。
- **UI 只抽查**：已查看基线构件中的 17 张英文亮色 320/1280 原图，看完已清理；不等于 5b/5c 全量视觉合格。模型/用户表格固定操作列边缘文字和安装向导滚动后品牌有裁切现象，尚需验证横向/纵向滚动可达性，不能直接认定所有现象是产品回归。其余 P1/P2 清单保留。
- 本轮全程未在本机安装依赖、构建、测试、跑浏览器或 Docker；未调用真实供应商、部署、发布、改生产配置。没有后台构建/服务需要接手；临时日志不作为长期证据，证据在仓库摘要与 Actions。

### 接手后先做什么

1. 只读核对主检出为 `dev`、工作区状态、最新提交和 CI；读本文件、缓存专项、OPERATIONS、DIRECTORY_GUIDE、TASKS、PLAN_PHASE2、根与前端 AGENTS。
2. `9795c0f5` 的完整CI已全绿；读取其后文档提交的真实CI结果，若有失败先定位实际断言。不要重新排查已通过的5b/5c编译或重搭工作区。
3. 会话头最小修复已验证，不重做；继续保护显式覆盖和删除、鉴权隔离、严格门禁、不重放。详细实现与验证状态见缓存专项。
4. 其后恢复 UI 全量截图验收及原 P1/P2 工作，每项按原规则先给方案、等确认；不要因缓存插单丢掉原待办。


本文件是接手入口。同目录另有：[PROMPT.md](PROMPT.md)（可直接复制给接手 AI 的提示词）、[OPERATIONS.md](OPERATIONS.md)（操作引导）、[DIRECTORY_GUIDE.md](DIRECTORY_GUIDE.md)（目录引导）。

证据标记：**[CI]** 在 GitHub Actions 上实际通过或失败过；**[读码]** 只读了代码或报告，未运行；**[未验证]** 已写入但没有任何运行结果；**[维护者]** 维护者在对话中明确说过。

## 一、项目与目标

- 仓库：`ForceMind/MyAPI`，开发分支 `dev`，稳定分支 `main`。历史“main仍是92fa1182”已由2026-10-28获单独授权的快进合并更新，见零节。只用这两个分支；后续开发仍只推dev，不新建分支、不自动合main、不发布、不部署生产。
- 项目：Go（Gin + GORM）与 React 19（Bun、Rsbuild、Base UI、Tailwind）的 AI API 网关。三个版本：Full 完整版、Lite 个人版、Desktop 本地版（Lite 的桌面形态，默认只开放本机）。定义见 `docs/MYAPI_MASTER_PLAN.md` 第 1、8 节与 `docs/RELEASE_MANIFEST.md` 第 2 节。
- 维护者本批需求原话 **[维护者]**：遵循第一性原理评估整个项目；审查代码结构，精简代码；按项目计划制作完整版、个人版、本地版；保留现有 Logo，整个 UI 重新设计，要易用，PC 与手机都要适配度高；使用设计 skill；部署脚本也要完善。
- 本批起点：维护者给定的已验证基线 `46a39080`（`0.2.0-beta.26` 候选，CI 运行 39943576911 全部通过）。

## 二、维护者的硬性要求（接手者必须遵守）

1. 与维护者的全部对话用简体中文；不用华丽措辞；结论先行。
2. 方案经维护者明确确认后才执行；每一步先说明成功标准与验证方法。
3. 未验证的内容不得写成无标记的断言；未经验证不称"完成"。
4. **构建、测试、浏览器验收全部在 GitHub Actions 上进行，不要占用本机空间** **[维护者]**：不在本机安装依赖、构建、跑浏览器、跑 Docker。本机只读写代码和做不占磁盘的轻量检查。
5. 每一步完成后提交并推送到 `origin/dev` **[维护者]**，推送后读取该提交的 CI 结果。
6. 不合入 `main`；不发布 NPM / GHCR / tag；不部署生产；不改生产配置；不调用真实供应商；不重新压测。这些都需要维护者单独授权。
7. 界面语言暂时只要求中文与英文 **[维护者]**。新增文案提供 en 与 zh 译文，其余五种语言（zh-TW、fr、ru、ja、vi）用英文回填并记入待办。这是对 `AGENTS.md` 七语言要求的临时放宽。
8. 子代理默认不用 Fable 模型，只有"生成方案、咨询、审查方案"可以用。一个角色最多一个子代理。子代理的汇报不是已验证事实，要亲自核对。
9. 用户可见的结论格式：每次实质性汇报按"当前完成 / 验证结果 / 审查发现 / 已修复问题 / 剩余任务 / 下一步"六段，小事一段话。

## 三、已完成的工作（按时间顺序，均已推送，除非标注）

### 3.1 评估与精简
- 架构评估报告：`docs/evidence/redesign-20261024/architecture-review.md`（`61e0f877`）。**[读码]** 由 Opus 子代理只读完成，主会话抽查了关键结论；它自己列出的覆盖缺口见第五节。
- 删除 74 个未引用的前端文件与 5 个依赖（`f4d0026c`）、158 个不可达 Go 函数（`ba6a3c41`）**[CI]**。计费相邻的 37 个不可达函数按约定保留。

### 3.2 版本能力与后端
- 后端识别 `lite`（`lan` 为 Legacy 别名），`/api/status` 新增 `product_edition` 与 `capabilities`，Electron 改传 `lite`（`e99d66a9`）**[CI]**。
- 维护者批准的三项门禁变更（`07207713`）**[CI]**：未知 `MYAPI_EDITION` 值拒绝启动（合法值为空、`full`、`lite`、`lan`）；Lite 下补封 `/api/redemption`、`/api/deployments`、Midjourney（`/api/mj`、`/mj`、`/:mode/mj`，后两者由主会话扩展，已告知维护者）；Lite 下只放开 `GET /api/subscription/self`。
- 日志新增可选参数 `result=failed`（`eb08c3f3`）**[CI]**，MySQL 与 PostgreSQL 的测试接入了 CI 的 S1 作业且通过。
- 维护者确认：管理员概览继续读取全部用户的记录（与调用记录页一致）；亮色青色对比度保持不变（遥测填充必须同时显示数值）。

### 3.3 UI 重新设计（方向 3「流向图」）
- 第一版设计（方向 A，工作台加检视器）被维护者否决，其外壳、令牌值、页面布局已被取代。第二版方案与三个可点击样稿：`docs/evidence/redesign-20261024/ui-design-v2/`，维护者选定**方向 3**（`00763936`）。
- 第 1 步 令牌与顶部导航外壳：`b28cf752`、`e59adcdd`、`d57a4d7b`、`f5770baa`，CI 运行 40166671819 **[CI]** 通过。
- 第 2 步 首页流向板：`edf7ae1d`、`cceb4aa5`、`ed45ec3e`，CI 运行 40175849784 **[CI]** 通过。数据口径：24 小时内最近 100 条日志在前端聚合，核心卡总数精确读取；未新增后端接口。
- 第 3 步 Key 列表、Key 对象页、请求页：`67be4186`、`b2607555`、`f3b29ffe`、`20ec63f1`，CI 运行 40187649824 **[CI]** 通过。新地址 `/keys/<id>`、`/keys/<id>/requests/<requestId>`、`/usage-logs/request/<requestId>`，旧 `?inspect=` 重定向。
- 第 4 步 上游卡片网格、连接对象页，加三个收尾项（概览"失败 N"用 `result=failed`、分页按钮手机 44px、Lite 个人资料页"我的订阅"只读卡片）：`1a025f6b`、`ae4a9f63`、`58be988e`，加两个 lint 修复 `bef61f33`、`ead5041f`。CI 运行 40268212418 **[CI]** 通过。
- 第 5a 步 请求区（调用记录、任务、绘图、正文与 SSE、分析）：`1e91109f`、`4605684d`、`8ee0ada6`、`045127e1`、`7cd12762`、`a53973d4`。CI 运行 40271665685：首次失败于"高级维护"浏览器步骤（`window-maintenance` 的"关闭订阅池"对话框 15 秒未隐藏），**只重跑失败作业后通过** **[CI]**。按现有证据判断为偶发，但没有找到根因，见第六节。

### 3.4 部署脚本（第二阶段 B1、B2）
- B1 版本值贯通（`80f9185b`）：`Dockerfile`、`cli/myapi.mjs`、`deploy/install.sh`、`tools/lan/check.mjs` 接受 `lite`（`lan` 为别名），镜像坐标 `ghcr.io/forcemind/myapi-lan` 不变 **[CI]**。
- B2 配置与引导（`cddc648d`）**[CI]**：刷新 `deploy/.env.example`；`myapi init --public-url` 一步生成 `.env`；`myapi doctor` 新增版本值、端口、目录权限、磁盘空间、Docker 与 Compose 版本、访问范围一致性检查。
- 第二阶段计划：`docs/evidence/redesign-20261024/PLAN_PHASE2.md`（`cc2e191d`）。

### 3.5 第 5b、5c 步（交接时未验证，现已通过 CI；视觉未收口）
- 第 5b 步 设置、用户、系统信息、个人资料：`df2b96ba`、`acd53ee5`、`403cca15`、`470d0860`、`6e89abd3` **[未验证]**。
- 第 5c 步 商业（钱包、订阅、兑换码）、认证页、安装向导、公共页与错误页、Playground / 聊天 / 提示词学习、模型目录与账号窗口：`496dafc5`、`5844e5b8`、`107daf1b`、`58d8ce2c`、`5bbf6cb1`、`3ca80983`、`e44f4056`、`c531c0a8`、`e1aad933`、`ad33f977` **[未验证]**。
- 这两批代码当时是在本机没有依赖、不能编译的条件下写的。**2026-10-27 已核对**：`d3beeeef` / CI 40284651016、`6392caf9` / CI 40443871659、`e7eef352` / CI 40447186866 全绿；以上条目的“未验证”是历史写入时状态。当前红灯来自之后新增的会话头取证测试，见零节。

## 四、未完成清单（按优先级）

### P0 立即
1. 已获批准的会话头修复与映射补充 `9795c0f5` 已完整CI全绿，见零节与缓存专项；不重做。不要把历史取证红灯当作5b/5c编译失败，也不要放宽测试。
2. 5b、5c 构建与现有浏览器自动验收已通过；继续用 CI 截图做完整视觉检查（当前只抽查 17 张），核对裁切现象是否可通过正确滚动完整访问，确认真实缺陷后按授权返工。

### P1 UI 收尾
3. 剩余的 lucide 图标迁移到 Hugeicons（维护者已确认统一 Hugeicons，随页面批次迁移；此前统计 279 个文件用 lucide、32 个用 Hugeicons）。必须在 CI 上确认导出名存在。
4. 5b、5c 子代理记录的未做项：
   - `profile/components/personal-profile.tsx` 是生产代码里无引用的死组件，但有测试覆盖，需先迁移测试再删除。
   - 设置页没有共享"危险操作区"组件。
   - 模型目录与账号窗口页没有统计块。
   - 订阅与兑换码的手机记录没有单测；5c 没有写 README 与截图证据。
5. 已知但没修的界面问题：Lite 版本行在 1280 宽度下于"用量未提供"中间折行；英文计数无单复数；1024 宽度上游节点名被截断；`/channels` 视图切换按钮没有可访问名称；`models-390` 的"更多"按钮 40px 的修复未验证；Key 节点只显示 `#ID` 而非掩码尾号、上游节点没有"测试失败 401"（现有数据不提供）；首页没有"今天 / 7 天"切换；Full 首页核心卡没有"用户 / 分组"摘要；channels 的 toast 文案仍写 "Track progress in System Info"；设置里"Header navigation"的中文"顶部导航"易与控制台顶栏混淆；`personal-console.css` 的 `.pc-card` 依赖运行时可能不存在的 `--radius-card`。
6. 七语言：补 zh-TW、fr、ru、ja、vi 的译文；验证俄语、法语下顶栏导航（1024–1279 宽度需把一项收进"更多"，样稿未验证）；恢复七语言截图验收。`web/src/i18n/locales/_reports` 一直没有重新生成（子代理不能运行同步脚本），在能运行 `bun run i18n:sync` 的环境（注意 exFAT 上要过滤 `._*.json`）重新生成。
7. Desktop 专属界面：首页"本机接入"卡；托盘文案接 i18n（见部署 B5）。
9. 命令行与安装脚本公网 HTTPS 来源规则对齐（见上方"已确认的小决定"第二条），小而独立，可与 CI 修复穿插。
8. `tools/` 下不在 CI 里的浏览器脚本（`console-layout-browser.mjs`、`input-policy-browser.mjs`、`tools/quota/token-budget-browser.mjs`）可能依赖旧外壳，需检查；`tools/runtime/personal-browser.test.mjs` 有 5 个基线就存在的失败，不在 CI 里。

### P2 第二阶段（`PLAN_PHASE2.md`，B1、B2 已完成，其余未开始）
9. A1 补全结构审查（账务与任务恢复逐文件读；支付回调在 Lite 下对未完成订单的处理；只用外部登录、没有密码的账户在 Lite 下能否登录；`tools/`、`website/`、`deploy/`；790 个单调用点函数；637 个疑似未用翻译键）。
10. A2 Lite 降级影响修复（依 A1）；A3 审查并删除 37 个计费相邻不可达函数；A4 分层整理（`controller/channel-billing.go` 约 1.2k 行业务逻辑下沉 `service/`）；A5 内联单调用点短函数；A6 清理前端未用导出（140 + 68）、未引用 UI 原语（约 1,448 行）、未用翻译键。
11. B3 Linux 原生服务（修正 `my-api.service`：行内注释从 systemd 语法看不被支持，未实测；加固；`deploy/install-native.sh`；user service）；B4 `myapi backup` / `myapi restore`；B5 Desktop（托盘 i18n、窗口关闭 / 退出 / 停止服务区分）；B6 容器加固评估（非 root 运行）；B7 中英文部署教程。
12. C1 七语言补译与验收；C2 独立质量审查（Fable，只审查）；C3 全量验收。

### 维护者已确认的小决定（2026-10-26，原话"都按你推荐的"）**[维护者]**
- Docker 的 `docker-build.yml`、`docker-smoke.yml` **暂不改用 `lite` 构建**，保持 `lan` 构建同一个 `myapi-lan` 镜像，等 Lite 镜像坐标落地再一起做（这两个工作流的版本值与 `tools/runtime` 里的镜像包合同绑定，改动会连带改浏览器脚本）。
- 命令行与安装脚本**对齐到安装脚本的更严格规则**：`lan` / `lite` 版本只接受本地或私网来源，拒绝公网 HTTPS 来源。**这项还没有实现**，列为待办 P1-9：改 `cli/myapi.mjs` 与 `cli/test/edition-contract.test.mjs`（该测试目前为规避这个不一致用的是 http 公网来源），并检查 `tools/lan/check.mjs`；先让新测试失败再通过；不得改变已有部署的现有配置。
- 分析页图表前 5 个系列用设计色、其余统一中性灰，**已接受**（用户图、模型图在系列很多时靠图例与悬停提示区分）。
- 另有 5 项原文档遗留决定已按推荐确认（见 `PLAN_PHASE2.md` 第二节）：Lite 镜像坐标保持不变、签名信任根采用 GitHub OIDC / Sigstore 但本阶段不接线、原生安装路径、Desktop 先 macOS 再 Windows、自动更新全部默认关闭。
- 上述既有决定不要重问；**2026-10-27 后续已明确批准 Codex 会话头最小修复**，当前实现与CI状态见零节，不再重复询问同一授权。

## 五、已知覆盖缺口与风险

- 结构评估的缺口（评估报告第 8 节）：账务子系统与任务恢复只读到文件级；三种数据库的实际迁移没有运行验证（CI 的数据库作业覆盖其中一部分）；`tools/` 约 4.2 万行脚本、`website/`、`deploy/` 未评估。
- 全量前端测试在本机（macOS、Node 26）有环境性失败：两个 `credential-*` 存储类用例基线就失败，高负载下 `strict-parameter-selection`、`account-switch`、`handoff-lifecycle` 偶发失败。CI（Linux）上这些都通过，以 CI 为准。
- 子代理的汇报都不是已验证事实。本批里主会话亲自重跑过的验证已在对应 CI 记录中。

## 六、接手后第一轮 CI 变红时的优先排查点

子代理自己列出的最担心处（**[读码]**，未验证）：
- 5b：吸顶保存栏的浏览器断言（"Qualify shared UI" 步骤）；`system-status.test` 里带参数的路由 Link 的 href 断言；设置卡片统一字号覆盖所有分区后的视觉；用户手机记录依赖用量单元格渲染，`personal-status` 的精确文本断言不能出现第二处匹配；个人资料卡片改用默认间距与 h2 后的测试依赖。
- 5c：公共顶栏从 fixed 改为 sticky 并去掉各页 `pt-16`/`pt-20`，320 宽度可能溢出或遮挡；`rounded-card` 与组件自带圆角类经过 `cn()`（tailwind-merge）合并时两者共存的冲突（已修 Skeleton 与 InputGroup 两处）；`--radius-card` 运行时是否存在；订阅与兑换码在 640–1023 宽度的分页与固定高度；充值卡在 320 宽度下标题与按钮并排；提示词学习页改用 `SectionPageLayout` 后的单测。
- 偶发项：`window-maintenance` 的"关闭订阅池"对话框隐藏超时一次，重跑通过。如果再出现，先判断是否为对话框退出动画（`data-ending-style`）未结束，再决定是否改脚本等待方式；不得放宽断言。

## 七、重要约定（避免重复踩坑）

- oxlint 规则 `curly: multi-line`：`if` 的语句体换行时必须加花括号（同一行单语句允许）。这条规则让 CI 红过两次。测试里用 `.querySelector()` 而不是 `.getElementById()`。
- GitHub CI 对同一分支的新推送会取消还没跑完的旧运行，所以攒好几个提交再一起推送；一轮约 30 分钟。
- 修复某个失败前先从 CI 日志找到具体断言和文件，不要凭推断改（本批里一次凭推断改错了位置）。
- 浏览器验收脚本里，"遮挡 / 裁切 / 不可读 / 功能不可达 / 用量未知不补零 / 不重放写入"类断言不得放宽；只有需求确实变了的断言才改，并写明依据。
- 每个提交只含自己的文件（路径限定提交），避免并发会话互相带入暂存内容。
