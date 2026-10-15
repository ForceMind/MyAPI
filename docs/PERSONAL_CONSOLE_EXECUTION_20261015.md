# 个人控制台第一批实施记录

已批准依据：`PERSONAL_CONSOLE_IMPLEMENTATION_HANDOFF_20261015.md`、prototype-v0.1、visual-v0.2。七张最终 PNG 已由主智能体实际打开检查。2026-10-15 开工基线：`codex/subscription-key-share-20261014` / `5d8cb5ac25942aeb9b27b6d4460e22a7c40a8dc5` / beta.14，远端同 SHA；既有 CI 38570694456 success。保留开工时两份已有文档差异及未跟踪设计/原型。

## 范围与实施顺序

1. 主代理：共享个人外壳、原 M logo、224px 深色侧栏、四项移动底栏及设置次入口；仅由已确认服务端 disabled 模式启用，保留原商业外壳及权限。新增 `/upstream-accounts`，复用渠道管理与登录。
2. overview_upstream：概览和上游账号，复用窗口观测、本人日志、现有 Keys 与 subscription-share API。未知/过期/失败独立显示，真实窗口分别表达。只读池视图补 Root-only channel_id，避免同名渠道误关联；不新增数据库字段/迁移。
3. profile_logs：身份、仅本人只读范围、偏好和安全入口；个人日志列表/详情。成功结果与缺 usage 分开展示，Token 未知；失败可重试且导航可用。
4. keys：已有 Key 列表及分额表单/预览，沿用 CAS、操作幂等、未知保存保护、暂停/恢复。调整保留已用；20+50 可存、60+50 拒绝、0 停后续请求、100 合法，不预留容量。
5. 主代理集成翻译、候选 beta.15、测试和普通 Actions；非实现者独立审查代码及实际浏览器截图，修复范围内问题后同步同一功能分支和 Draft PR #17。

## 文件所有权与资源

最多四个活跃角色，无递归。当前代理继承主会话模型/强度；工具未返回实际模型标识，不声称切换至 Terra/Sol。主代理持有 components/layout、styles、routes、locales、VERSION、只读 Go 补齐及综合浏览器/CI；overview_upstream 独占 personal-workbench、personal-upstream、对应浏览器脚本；profile_logs 独占 profile、usage-logs 及 profile/personal-status 浏览器脚本；keys 独占 keys 及 subscription-share 浏览器脚本。同一文件单写者。

保留beta.14后端语义、渠道登录、Key身份、权限、商业模式及所有历史账务。禁止生产数据/凭据、历史副本、恢复worker、生产启动/迁移、main合并、标签、发布、部署。仅普通验证和已授权功能分支 Git 同步；不新建远程仓库。

## 验证与交付条件

先相关行为测试、TypeScript、涉及文件lint、七语言i18n，再复用普通CI的构建/Chromium/后端兼容回归。本机未安装前端依赖和浏览器，优先Actions，不重复安装重型缓存。浏览器以隔离合成API运行真实编译页面，覆盖1280/1440和320/390、长文、导航、焦点/键盘、资料失败重试、成功缺usage、分额边界；保存实图并人工查看。合成浏览器不能证明真实上游或生产状态。

交付包含可运行页面构建、真实桌面/手机截图、准确源码/版本、测试范围、独立审查、剩余项。beta.15远端标签查询尚未占用，本批只更新候选元数据，不打标签。风险：旧浏览器驱动绑定旧UI，需更新定位并保留行为合同；后端未提供的HTTP/usage字段必须显示未知，不能填设计示例。

## 阶段状态

- 基线与设计核对：已完成。
- 外壳、页面及只读关联字段：实现完成，验收修正进行中。
- 翻译/集成/验证：七语言同步完成，类型/构建/单元及浏览器分阶段验证中。
- 独立审查/截图/分支同步：功能分支已同步，独立实图审查进行中，尚未完成全部验收。

持久 Goal 已创建；阶段结果和CI/源码证据在此追加。只有全部交付条件满足才标记完成。

### 第一轮实现集成

外壳及五个页面已写入；新增131条英文源文案并经规定脚本填写七语言，i18n sync七语言missingCount=0。个人Keys移除钱包策略入口；旧商业界面的策略/预算回归改为明确commercialShell场景，新个人浏览器覆盖无金额/成功缺usage/分额及恢复。

独立静态审查已修P2：日志跳转Keys的filter未承接、手机版本不可见、未知基线误显示0、外壳缺旧维护页面所需上下文；头像与页面动作预留间距待实图核对。调用记录按user/role/session隔离并使用Intl合法locale，金额不展示，缺可靠HTTP字段显示未知。

本机已执行Node fixture测试和浏览器驱动语法检查、Bun轻量TSX解析、diff检查；本机Go定向测试因只读默认GOCACHE未执行，未额外建立重缓存，交CI。无node_modules，Vitest/tsgo/lint/生产构建/Chromium尚待CI。CI把同一生产构建提前至typecheck之前，由TanStack插件生成新增路由类型；不重复构建，保留构建+路由产物供运行预览和回读生成文件。

首轮源码 `80594a49216400afa215f468b9363ac2d9c24e20` 已同步 Draft PR #17，普通CI `38743485122` 启动。生产构建成功；typecheck发现新外壳固定URL未按TanStack动态路由声明、只读数组传入可变导航类型，以及3个fixture/测试API类型问题，均按错误逐项修正。保留失败日志 `/tmp/myapi-console-80594a4-frontend.log`，不以语法解析代替类型检查。分发合同亦失败，正在定位。历史数据与生产未操作。

修订验证：使用项目固定oxfmt 0.57.0和oxlint 1.72.0轻量工具整理涉及文件（保留原版权头），未安装完整前端/浏览器依赖。40个涉及TS/TSX文件lint通过（另2个CSS已格式化）；模板候选版本定向CLI测试在正常子进程权限下通过。第一次本机完整CLI测试在受限沙箱仅返回文件级失败、无具体用例证据，保留 `/tmp/myapi-console-cli-tests.tap`，不作通过结论。部署模板已统一beta.15且保留未发布说明；新增RELEASE_BETA_15.md。综合浏览器独审补强输入/输出Token各自Unknown、fixture时钟及准确CI SHA，分工明确覆盖五个页面。

第二轮e91fbc9：普通CI `38744477080` 的构建、typecheck、全局lint通过，Vitest运行中。已回读CI生成路由，唯一语义新增为/upstream-accounts，复制生成产物供仓库同步。分发检查已通过先前CLI候选版本合同，后在npm包体积门槛失败（55,325,291 > 52,428,800字节），原因是本批纳入Git的设计PNG/PDF被源代码包docs规则一起打包；准备排除仅设计/原型二进制审阅资产，仍保留Git设计依据，不放宽体积上限。

第二轮Vitest实际结果：219文件中216通过、3失败；1363项中1359通过、4失败，并有一项新外壳测试缺少jsdom动画API的未处理异常。四失败均为新增测试定位（BaseUI实际button角色、预览重复百分比文本），按用户可见角色及有业务含义的dl行定向修订；动画API仅在该测试fixture补齐并恢复。未改生产行为制造通过，等待新CI。

npm真实dry-run回读：新增docs/.npmignore仅排除design/prototypes的PNG/PDF，包体积33,066,970字节、审阅二进制0项，原50MiB上限保持。设计资产仍保留Git。已下载CI可运行构建；CI合并引用db37cac与e91文件树同为462a8b5，未合并main。loopback4173已启动只读合成预览（/tmp/myapi-console-preview.mjs），GET状态回读disabled；不运行Go、不触及数据库/凭据。

第三轮d822001：CI `38745544439` 构建/typecheck/lint通过，219文件/1363项Vitest全部通过，分发合同通过。真实Chromium开始运行，旧quota/routing/access驱动仍按旧日志表格/Key菜单定位导致失败；另普通owner profile fixture缺新增本人subscription-share GET的允许响应。按实际失败修驱动：保留商业后台原回归，个人新页面由专属五页面journeys验收；只允许本人已有Key的share空响应，不放宽其它权限。仍未完成实际新页面截图验收。

集成补查修P2：新Key创建后原provider刷新信号未作用于新卡片查询，已纳入queryKey；原高级Edit/Assigned access/budget/CC Switch/delete通过每卡折叠入口复用，保留确认与安全，不静默删除旧功能。Provider按用户/角色/session重挂，避免旧dialog/已解析Key跨会话残留。新增4项维护入口行为回归。旧真实应用Driver同步适配中，Node doubles仅为driver合同，尚未运行真实双writer应用。

已实际打开d822的CI失败实图（个人日志桌面、渠道320）检查：外壳/原M/柔和蓝已按新图渲染；失败来自旧表格定位，不是页面未运行。补个人头像中性灰蓝、桌面版本简短可读并保留完整revision的title；窄屏选中导航暗色对比度及长标识符换行补强，专属浏览器加入真实颜色对比和卡片溢出检查。截图属于隔离合成API，不能证明生产。

本机正常权限下，两份runtime Driver合同测试155/155通过（其中personal double128项），固定安全报告边界未放宽；受限沙箱首轮docker-smoke.test仅文件级失败的记录仍保留。真实MyAPI/Chromium双writer仍待远端工作流，Node double不是其替代。

实图/源码继续核查修正：旧日志仅含usage_accuracy=estimated/unknown时曾被个人页误称上游已返回Token，已先复现后最小修正并新增5个回归；明确reported有效零值可显示，完全缺metadata的历史正Token兼容保留。手机调用详情新增滚动至usage卡的独立截图及Token两行/查看Key按钮不被底栏遮挡的几何检查，首屏不冒充全页验收。渠道维护页归属上游账号导航并补选中状态回归。

0973fbf普通CI `38747705218`：构建、类型、lint、单元及quota/routing/access三个Chromium回归通过，其余兼容/分发作业通过；后端完整回归仍在运行。shared UI在profile最后stalled-bootstrap场景失败（self读2次，预期1），前8个资料journey含失败重试及认证refresh恢复已通过；不放宽读取断言。实际打开healthy-320/1280并独立复核，原M、柔和蓝、中性头像及短版本可读。截图为滚动后的展开状态，默认完整页未验；发现旧Settings泄漏余额提醒阈值，正在限定个人模式隐藏，商业保留。

shared UI执行调整为逐脚本收集全部结果并保留任一失败的非零退出码，避免第一个失败阻止五页面专属截图。隔离真实应用工作流 `38749705760` 已手动派发，准确源码0973fbf；matrix包含full/fresh legacy、full/fresh authoritative、lan/fresh和合成旧源码handoff，另有临时安装器。全部为CI新SQLite与临时凭据，测试上游仅回环；不访问真实历史副本，不推送镜像、不发布、不部署。该运行尚未验收。

资料重复读取根因已确认：初始funding状态确认后旧临时外壳切personal，Outlet重挂再次读self。现在等待status.confirmed或明确status失败再读；placeholder不算确认，等待15秒有失败终态且手动Retry独立可用。补hook超时/恢复和placeholder→确认单次读取回归，原浏览器计数保持严格。个人通知设置显式隐藏余额阈值及说明，商业默认保留，设置保存沿用原阈值以满足既有API合同。非实现者已独审usage证据逻辑及阈值传递，无新增确认缺陷；新版行为/截图仍待CI。

0973fbf普通CI后端已完整通过：root/relaykit tests、独立build、386、JSON policy、三库作业与全部race检查success。c2e527d修订独审另发现状态仍pending时手动Retry后卸载未及时abort，已补取消和真实signal回归。仅前端及隔离测试配置有后继，不修改账务逻辑。

真实应用38749705760：LAN安装器、LAN fresh、合成handoff通过；full/fresh两writer均在reload-check收到静态asset429，未全程通过。已产出13张/每writer遮敏实图，主实际查看桌面Keys与手机成功日志，只证明这些状态已运行。根因是Playwright route外网阻断禁用HTTP缓存，7次document冷加载共享GW默认120/180s；GW仅作用于Web资源，不是API/relay桶。仅personal+full+isolated+loopback+固定SHA的CI容器配置1120/180s，按7文档×完整160项产物目录保守估计（非逐页实测）；生产120/180s、API/认证/搜索/relay保护保持。配置拒绝越界及driver合同157/157通过，真实应用重验待新源码工作流。

门控独审补P2：status失败允许非金融资料恢复时回退旧外壳，其默认通知设置曾重新露出余额阈值。现在仅明确enabled/retirement显示，unknown与disabled隐藏；保留两种已确认商业模式原行为，并调整对应状态回归。

7cc23ee普通CI `38752321998`：build/type/lint通过，220文件1379项Vitest中1378通过、1失败。失败揭示真实门控循环：self完成后Settings新status observer重试失败查询，error→pending→error使useProfile反复取消/再读并卸载身份。Profile现在仅初始等待模式解析，一旦confirmed/error获准便保持至离页；独审通过，四mode原身份/通知断言保留并增加读取1次及身份持续存在检查。浏览器依赖也改为只要production build成功即执行，单元失败仍保留失败结论，但不再阻止后续截图证据。

7cc23ee真实应用 `38752335168`：authoritative全程success，准确revision/Root-owner角色、14张遮敏截图、5次真实UI请求及拒绝无额外上游、精确日志/计数/刷新均完成；主已打开桌面日志和手机Keys实图。legacy首次ledger-check的LOG_MISMATCH未有字段细分，不称已确认根因。源码确认日志Count/Find分读可出现total0/items1，旧Driver已用Node稳定复现失败；现在只允许精确request/owner/Key/model/charge/Token/source/遮敏匹配的唯一行在原10秒内只读重读，最终仍必须1/1。其他错配立即失败，新固定分类/数值掩码不输出原始数据。含越界/错误/截止时间的Node合同162/162通过；真实legacy重验待新源码。LAN与合成handoff本轮均通过，仍无生产操作。
