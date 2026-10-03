# My API 新对话/新模型提示词（2026-10-02）

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

## 本轮续作提示（优先于下面的交接时快照）

21:00 北京时间续作：`7295ce1` 的 CI `37006732955` 十项全过，Chat 用量证据与前置价格发布不再重做。现在同一 Token 预算迭代已有持久模型/迁移/Root 恢复基础和 SQLite/race 候选证据，先核验本批三库 CI，再接可靠发送上限、结算与失败恢复、Key 页面及权限入口。尚无运行时/API/UI 接线，不开放未保护的开关，也不称 Token 预算完成。精确状态见 R1_CURRENT_ITERATION 首节。

19:15 北京时间续作：不要重复后端价格发布基础（`3094e5e` 的十项 CI/三库已过）。本批又接通 Root 预览确认接口、来源页发布/锁/回滚、请求冻结和日志参考成本；先验当前 SHA 的三库/Chromium CI，再推进实际费用/Token 预算，别把旧绿色套给新提交。Standard 纯文本以外的来源适用性仍保守拒绝，不把参考价当上游实际账单。当前完整实现/局部验证/残余边界见 R1_CURRENT_ITERATION 首节。

同步更新：`codex/r1-usage-review-20261002` 的 `9410b02` 检查点已通过 CI `36989602580` 全部 10 项，含 R1 待核对三库合同及恢复表单 Chromium 操作。后续价格发布基础代码已进入本轮工作树，但尚无公开发布 API、页面或请求冻结接线；先核验当前 HEAD 的 CI，再继续该有界迭代，不重建候选、不把旧绿色复用给新增数据库代码。每次源码/文档同步后主动汇报，带北京时间和距上次汇报间隔。整个 R1 未完成。

先读 [R1_CURRENT_ITERATION.md](R1_CURRENT_ITERATION.md) 及 NEXT_SESSION_HANDOFF 的最新节，并按当前 Git/工作树事实接管。负责人已批准继续 R1，且已确定严格未知用量暂停并保留预留、百分比采用账户剩余安全阈值；不要再次把这两条视为待选。实际预算与准入联锁尚未完成，不把业务选择当实现证据。

当前候选基于 `71277bf`，包含待核对/恢复及其验证记录；不得覆盖、重做或因为旧模板而回退。原 missing/estimated 失败已在候选修正，检查当前测试而非照抄旧红灯。2026-10-02 07:03 UTC 负责人已批准自主推进既有 R1 开发、新分支/草稿 PR/CI 源码同步与修复，无须重新询问是否继续。远端同步以实时回读为准；正式上线、真实账户与凭据操作另行确认。保持单智能体、一个有界迭代，不扩展 F1–F8、支付或价格辅助工程。

复制下段接管当前 R1。详细事实见 [NEXT_SESSION_HANDOFF.md](NEXT_SESSION_HANDOFF.md)。下方旧设备模板只保留历史，不执行其中冲突的范围和状态声明。

```text
你接管 My API 的 R1 自用版。先读 AGENTS.md、docs/NEXT_SESSION_HANDOFF.md、docs/NEXT_USABLE_VERSION.md，再核对 pwd、Git 分支/HEAD/工作树/远端，不重做已完成批次，不重新规划 F1–F8。

GitHub：https://github.com/ForceMind/MyAPI
交接分支：codex/r1-handoff-20261002（未封版快照，不可当作可部署版本）。
同步状态：负责人再次明确要求推送后，交接分支已成功同步 GitHub。源码快照 f93695873c4413d42a48fe1af302bee3bc70be98 共 232 个文件，含继承商业 WIP；另有交接文档提交，最新 HEAD 以远端同名分支回读为准。未合并、发布、部署，不能把推送成功当作 CI/真实业务验收。
本机候选：/Users/wxx110/.codex/worktrees/next-usable-candidate/MyAPI。
基线：57ec31a58fc737ba2be0601e4102123436c3d938；实际提交以分支及远端回读为准。
主目录未被候选覆盖；原 /Users/wxx110/.codex/worktrees/2daa/MyAPI 只读保护全部 WIP。不要 reset/stash/clean/强制 checkout，不先 pull/rebase main。

主体自用：保留用户归属/权限/汇总及每个 API Key 的额度管理。商业默认关闭，无充值余额概念；已有商店/充值/订阅仅保留可选模块及历史恢复，不扩展支付商/动态插件框架。三种新预算是实际 API 费用、实际 Token 总量、账户百分比。缓存与 reasoning 是包含子项不重复计数；费用按实际分类和冻结版本，OpenAI 来源须为官网；订阅渠道 API 等价成本只标参考。

保留已有首页/概览、细线账户图、成功测试选项复用、确切耗尽 429 避让、OAuth 接线、usage 来源展示、冻结价格和实时重复扣费修正、官网来源获取/保存/读取页面。来源保存不是有效价格发布，三种预算尚未实现。

先收口 service/usage_settlement_contract_test.go 中 TestPerTokenSettlementRequiresReportedUsage 的 missing/estimated 两项红灯，不能删测试、放宽断言、把未知计零/退款或估算成精确。先集中问我：严格费用/Token 预算 Key 遇到未知 usage 是否暂停后续请求并保留预留等待可靠证据/授权恢复？账户百分比是剩余安全阈值还是各 Key 的窗口预算？共享账户差值不能归属 Key。未确认前不实施关键副作用。改结算前读 pkg/billingexpr/expr.md，不整包复制原 WIP。

一次一个有界版本，先报告本轮范围、可查看入口、依赖、验收及停止条件；优先正常结算—未知待核对—权限恢复—幂等限额闭环，再接价格发布和费用/Token 预算，百分比按确认规则接入。默认单智能体，查已有 Goal 后按 AGENTS 承接；不创建自动化/子任务。最小有意义验证，复用适用证据，避免无限扩大支付、价格辅助功能或测试工程。

当前 service 不全绿；MySQL/PostgreSQL 实库、当前 SHA 的 CI、真实容器/OAuth/账户/账单、升级回滚未验收。8769 预览只是合成 API。不得宣称 R1 完成或可部署。沿既有批准范围实现/验证/更新文档；新批次推送、合并、发布、部署及真实账户操作取得对应授权。先完成只读接管核对，给出下一迭代，不立即开始大范围扩展。
```

## 历史归档：旧设备提示词（不用于当前接管）

# 新设备 Codex 启动提示词

将下面整段复制到新 Mac 的 Codex 新对话中。它只描述工作范围和仓库事实，不包含任何
密钥、服务器 `.env`、数据库或日志内容。

```text
你现在接手 MyAPI 项目在 macOS 上的开发工作。

目标：继续完成 MyAPI 的开发计划，但不从头重做已经完成的功能，不擅自发布、推送镜像、
重启服务器或修改生产数据。当前开发主机是 Mac；Linux 服务器只作为远程仓库、CI/GHCR
或经明确批准的部署环境。

仓库：
- GitHub：私有仓库 https://github.com/ForceMind/MyAPI
- 本机目录建议：~/src/MyAPI（不要放在 iCloud Drive、Dropbox、OneDrive 或网络盘）
- 首先执行：git status --short --branch、git remote -v、git log --oneline --decorate -5
- 然后读取仓库根目录 AGENTS.md、docs/MYAPI_MASTER_PLAN.md、docs/COMPLETION_AUDIT.md、
  docs/DEVELOPMENT_ON_MACOS.md；如果当前环境有 `~/.codex/AGENTS.md`，也读取它（当前 Linux
  机器上的对应路径是 `/root/.codex/AGENTS.md`）。

开发环境：
- macOS 原生开发；安装 Xcode Command Line Tools、Homebrew、Docker Desktop、Go、Bun、Node。
- Go 版本以 go.mod 为准（当前 1.25.1）；Bun 以 lockfile/CI 为准（当前 1.3.14）；Node 22.x。
- Docker Desktop 使用 Compose v2.17+，构建使用低并行度；不要长时间占满 CPU/内存。
- Electron macOS 可以在本机打包；Windows 安装包交给 Windows runner/设备，不能由 Mac 验收。

已完成并必须保留：
- Codex/Chat Completions/Responses、SSE、附件转换和参数兼容；
- 完整请求/响应/分片日志、脱敏、查询和移动端日志展示；
- 普通渠道与 Codex OAuth 额度快照、每分钟/小时/天/周趋势、时区、失败状态、可选采样
  和只读告警状态；概览同时显示每分钟变化列表和 Codex 账户可用额度折线图，管理员渠道
  页面提供完整筛选与历史图表；
- Account Tier 与 Key Access Profile 兼容领域模型、注册表和清晰 Key 表单；
- MyAPI 独立品牌、UI、静态官网、LAN Lite 和 Electron macOS/Windows 合同；
- GHCR tag 自动构建 Full/LAN 镜像、不可变 digest、CLI 健康检查/备份/回滚；
- Claude Messages/Responses 兼容和官方边界；Google Antigravity transport 的官方边界；
- SQLite 迁移修复、脱敏副本升级/回滚、运行时认证探针和现有文档。

当前事实：
- main 与 origin/main 必须先重新核对；不要假定旧聊天记录中的提交仍是 HEAD；
- v0.1.0/v0.1.1 是受保护历史 tag，不得移动或覆盖；正式发布必须先确认新版本号；
- NPM、正式 GHCR 发布、生产升级、法律/NOTICE 审查仍需负责人确认；
- 真实手机、macOS/Windows 安装和 PostgreSQL 恢复是外部验收，不得用静态测试冒充；
- GitHub Actions 如果 job 在 steps: []、runner 启动前失败，应记录为 Billing/runner 外部阻塞，
  不要为了“修绿”修改无关源码；
- LAN Lite 不读取本机 Codex/Claude/Antigravity/Keychain 凭据文件，不实现凭据文件导入；
  上游凭据只在 MyAPI 管理界面显式配置。

工作规则：
1. 修改前先检查工作树和相关文件，说明本轮目标、范围、验证和风险；保护其他改动。
2. 使用 apply_patch 编辑文件；不要把 .env、数据库、日志、node_modules、dist 或密钥加入 Git。
3. 遇到核心架构问题先提出方案，不整体重写稳定模块；数据库改动兼容 SQLite/MySQL/PostgreSQL。
4. JSON 业务序列化使用 common/json.go wrapper；relaykit 必须独立构建。
5. 完成修改后运行受影响测试、相关合同检查和必要构建；记录实际命令、结果和未验证范围。
6. 不要执行 npm publish、创建/移动 tag、GHCR publish、生产重启或服务器数据操作，除非我在
   当前对话中明确授权。
7. 每轮结束报告：当前完成、验证结果、审查发现、已修复问题、剩余任务、下一步。

开始动作：
- 先只读检查仓库状态和上面列出的文档；
- 建立动态计划，标记已完成、进行中、待验证和外部阻塞；
- 优先处理真实代码缺陷或 macOS/Docker Desktop 开发迁移问题，不重复实现已验证功能；
- 没有真实设备或外部权限时，明确记录阻塞，不伪造验收。
```
