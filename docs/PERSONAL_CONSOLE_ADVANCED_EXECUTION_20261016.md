# 个人控制台高级维护实施记录

## 目标与批准边界

维护者于 2026-10-16 授权继续完成窗口重选、安全设置与模型范围，沿用已确认 visual-v0.2。凭据轮换与跨账号迁移仅核对现有能力并列必要决策，不实施凭据替换或迁移。此前 beta.16 `630f905` 已验收，归档 HEAD `dd5d54dd4b7c106d80926dcf63a367449447cabc`；沿用 `codex/subscription-key-share-20261014` 和 Draft PR #17，不重置旧 main。

保留原 M、柔和蓝、五页面外壳、现有身份/Key、近似原生窗口与未知用量语义。历史账务、生产数据、凭据、备份继续冻结；不合并 main、不打标签、不发布、不部署。本批自动化仅使用合成浏览器 API 与 CI 临时数据库。用户现有两份计划文档差异及 v0.1/drafts 目录保持原状，不纳入提交。

## 现状与选择

- 窗口重选/恢复已有 Root、CAS revision、操作 ID、同主体/同 slot、显式确认和未知保存合同；高级入口沿用旧通用列表，须补清楚的当前/目标窗口、全池影响与动作反馈，不能暗示跨账号迁移或准点重置。
- 个人资料已保留折叠安全入口，但展开混合通知/绑定、密码、控制台访问 Token、删除账号、Passkey、2FA 与会话卡。按实际能力整理可完成流程、状态与确认，限制迟到回调和认证重放，不自动生成/轮换凭据。
- Key 模型范围藏在完整编辑抽屉的高级区域。现有 `PUT /api/token/` 会更新全部 metadata，不能发送两字段假装部分更新；单独范围编辑需要保持其它字段的窄更新接口及并发保护，或明确使用完整编辑。主线程先核对后确定最小接口，禁止把范围维护转为配额修改。
- 当前 shadcn CLI 未安装；`bun x --no-install shadcn info --json` 已核对不可运行。以实际 components.json（base-nova/Base UI/Hugeicons）和维护中的组件 API 为依据，不为此安装应用依赖。云环境状态工具/策略文件当前不可用，不据此推断网络授权；远端命令依既有权限流程执行。

## 实施顺序与文件所有权

1. 只读规划角色 `maintenance_plan`（工具支持的 gpt-5.6-sol/ultra）核对三流程合同、现有轮换/迁移能力、风险与测试。主线程自行复核步骤的目标作用、依赖、保留内容和可复用流程。
2. 三个实现角色在清楚的所有权内并行：窗口组件/API和对应测试；profile 安全组件/hooks/API和对应测试；Key 模型范围组件/入口/测试。不得递归派工，不争写共享文件。实际派工清单在后续阶段记录补齐。Terra 配置不可用，采用 AGENTS 已列的 Sol 组合承担有安全状态约束的局部实现。
3. 主线程持有必要 Go/controller/router 合同、共享样式/组件、七语言、候选版本、CI/浏览器驱动、文档与集成。预计候选 beta.17，更新统一元数据，不创建标签。新增文案由各实现者列键，主线程用规定脚本同步全部七语言。
4. 必要测试包括窗口影响/双确认/CAS/未知结果/关闭池/会话切换；模型限制 enabled 与空集合的真实语义、原有/已不可用模型保留、窄更新与并发、权限；安全读取失败/禁用能力/表单校验/显式确认/敏感结果不跨会话泄漏。成功、错误、加载、未知、空态与手机长文/键盘动作均覆盖实际改动。
5. 使用既有 GitHub Actions 执行类型/lint/build/单元/必要后端与三库、实际 Chromium 页面、桌面/手机原图；非实现者独立审代码和实图。更新本地只读 CI 产物预览，归档哈希与报告，按批提交并回读远端、更新已有 Draft PR。

## 验收、资源与停止条件

一个新持久 Goal，最多 4 个活跃角色，不递归。主线程负责接口与集成，重型依赖/构建/浏览器优先 Actions；本机仅缓存格式/lint、脚本和必要定向 Go。未指定时间/token 预算，不设置工具预算参数。

交付须是可运行的现有高级能力整理与必要修正，包含真正操作/失败恢复的测试和实际桌面/手机图；没有未解决 P1/P2、必需检查通过、源码/候选版本/证据/功能分支同步可追溯。轮换/迁移交付只读能力表、约束、必要决策，不能以未决定的新流程冒充完成。只有真实验收完成、用户暂停或必需权限/外部条件阻断才停止；一般实现细节与范围内修正连续执行。

## 阶段状态

已完成：beta.17三组高级维护、窄字段API、七语言、完整CI/Docker运行及原图补验通过，最终独立复核和交付归档见文末及交付记录。旧候选失败结果保留历史，不作为验收通过证据。

## 实际派工与接口决定

- `maintenance_plan` 先按 Sol/ultra 只读规划，随后沿用同角色实现窗口组件/API/测试及独占 `tools/ui/window-maintenance-browser.mjs`；不触碰 profile/模型入口。
- `security` 按 Sol/high 持有 profile 安全组件、专属 API/hooks/测试及 `tools/ui/security-maintenance-browser.mjs`。必要的资料读取身份隔离保持原超时/重试合同；不改全局 HTTP/认证实现。
- `model_scope` 按 Sol/high 持有独立模型 dialog/API/helper/测试、Key 入口/types/cells/通用编辑保护与 `tools/ui/model-scope-browser.mjs`。主线程持有 Go、CI/版本、共享样式、翻译和文档。所有角色无递归派工。
- 自行复核后采用最小安全新端点 GET/PUT `/api/token/:id/model-limits`。DTO 只含 Token ID/名称、enabled/list 和 opaque revision；复用完整元数据 fingerprint，排除额度消耗。PUT 在用户/Token 行锁内验证启用 owner 和 fingerprint，仅更新两列，经现有 cache fence。普通元数据编辑省略模型字段时从 SQL 更新列排除它们，避免旧表单覆盖新范围；额度 CAS 编辑保留原 enabled/CSV 和完整 fingerprint。无新增表/列/账务算法。
- 只读规划中“true + 空集合未定义”的建议未采用：实际 relay 把它解释为拒绝全部。UI 明确三状态，拒绝全部需显式选择及确认；候选目录仅为建议，不验证成实际路由可用性、不删除未列出的旧模型，也不扩大访问策略。
- 本机缓存定向 Go/model/controller 及相关原额度元数据测试通过。受限环境的 miniredis 回环 socket 首次被禁止；同一合成测试在已批准的沙箱外重跑通过，不将首次失败归为业务缺陷。三库实际结果待 Actions，SQLite 本机成功不外推其他库。
- 已核对凭据能力并形成 [必要决策](PERSONAL_CONSOLE_CREDENTIAL_DECISIONS_20261016.md)，未调用任何真实生成/刷新/替换/迁移接口。

## 首次集成检查

- 模型独立复核发现普通编辑和 status-only 可能覆盖并发模型范围，已改为按字段是否出现选择更新列，以及仅写 status；补充确定性 handler 交错测试。保留完整额度 CAS。
- 安全独立复核发现 OAuth 旧 callback 可反馈新流程，以及一次性凭据生成中关闭会丢失返回；已加入流程身份守卫、提交中关闭锁与 deferred 响应测试。未知结果明确提示旧凭据可能已失效，不声称回滚。
- 模型未知保存回读失败后必须保持写锁，修改表单不能解锁；只有成功读取当前服务器状态才能恢复提交。
- 七语言新增 83 项经受控 add-missing-keys 脚本与 i18n:sync 同步。
- 最新本机定向命令使用 ^Test(TokenModelLimits.*|TokenMetadataPreservesWideQuota|TokenQuotaVersionIsExactAndStaleEditsWriteNothing)$，model 0.785s/controller 1.863s 通过，覆盖新字段窄写、交错更新、缓存未知和旧标识符。
- 真实 Docker personal 旅程新增 opt-in advancedModelScope：通过真实 UI 和 Go 窄端点限制后恢复模型范围，核对 Root 跨 owner 404、元数据/用量/上游调用不变；结果待 CI，不以脚本检查冒充运行通过。

首次集成缓存格式/lint通过（通用额度编辑测试中原有 query-selector warnings 保留，0 error）；三个新浏览器驱动 node --check 通过。完整隔离驱动 Node 合同 166/166 通过；首次受限回环失败后仅在批准环境重跑。窗口、模型、profile owned 文件已冻结准备新源码 CI。

## 首轮候选补充修正

源码 b85779b 已同步并触发 CI 38845560151、隔离 Docker 38845571447；CI 类型检查观察到失败，日志须等前端作业结束读取，尚不能称通过。独立审查补齐两个范围内问题：security subtree key 纳入 role，避免角色切换后旧请求留下 loading；窗口脚本保留16矩阵和owner场景，en1280/vi320改为dark并实测根主题，vi320增加完整恢复/未知回读功能流。密码/删号未知反馈、显式重试和关闭清理补确定性RTL；没有引入新的凭据恢复协议。

主线程反例检查补拒绝 models 数组 null 元素：Go JSON默认会将它转换为空字符串，不能冒充保留的旧空CSV段。新增回归同时验证 null拒绝不写入、原样旧空字符串可保留。最终定向model/controller再次通过（1.002s/2.069s）。

## 首轮实际失败与定向修正

- 前端单元1490通过/3失败。模型保留标签有两个，改为逐项准确计数并修复未定义get spy；关闭池测试等待异步region就绪再操作，保持全部确认/POST/回读断言；OAuth测试原全局第一个Bind实际是Email，改用具名Bind GitHub/Discord，并核验provider/state/code/signal。为绑定按钮补动作与provider组合的可访问名称，无新文案。
- 首轮Go模型范围配置合同已在SQLite/MySQL/PostgreSQL分别实际通过。
- 隔离Docker legacy首轮在model-scope-restrict阶段20s超时，读/open/owner边界已通过。该阶段包含模式、搜索和候选选择，根因尚未最终确认；将候选定位限定在建议容器，增加真实目录读取断言和安全细分stage、失败遮敏原图供下一轮识别。不改变模型访问或relay语义，不将推断记为验证结论。
- CI补保存类型检查日志，与单元日志一同上传，使失败时可在作业结束前读取具体错误。

## 首轮浏览器与跨平台修正

首次advanced三脚本均失败：模型手机导航被modal正确aria-hidden（改取唯一实际DOM几何，保持遮挡断言）；安全中文“两步验证”标题/摘要同名（按安全区card-title精确定位，panel截图滚动实际安全卡入视口）；窗口未确认按钮disabled:pointer-events-none导致中心命中跳过目标（仅hit-test瞬间恢复目标pointer-events再还原，真实覆盖层仍会失败），同时先保存原图再断言。首轮无窗口原图，不虚构实际查看。

主线程实际查看已产出的320模型与安全图，发现长旧ID被徽标挤窄，已将手机ID改为全宽首行、操作次行，增加原样字符串与几何断言；安全panel原图只见入口，已要求展开区域实际进入视口。

386检查发现controller新测试map内宽常量默认int溢出，改为显式int64保留数值，定向controller通过1.949s。驱动修订完整Node166/166再次通过。首轮CI已收集必要失败、三库、核心和advanced报告后取消剩余共享兼容步骤以节约资源；最终修订须完整重跑，不复用该取消状态当通过。

首轮完整ts日志准确只有三项：未定义get、nullable model_limits赋给string、2FA setup unknown赋给业务类型。分别修正spy、原始null CSV映射为空串、逐字段校验真实2FA响应三字段后显式构造typed DTO；新增合法与错误备份码类型API合同。所有已知首轮失败修正集成，准备完整第二轮CI/隔离Docker，不把语法或本机缓存检查替代必需资格。

第二轮资格源码5ba90cfee023d977a6aa8e7559576f6bd44b7c72，完整CI 38847248448、隔离Docker personal 38847256100均已启动；远端分支和Draft PR #17 head已回读一致。待实际结束，不预先填写成功。

## 第二轮阶段结果与末次收口

第二轮生产构建/type/lint通过，233文件/1496单元全部通过；三库作业完成，后端进入完整root/relaykit测试。实际advanced/隔离runtime仍进行中，不预判结果。

独立审查发现同SID角色变Root时旧profile.role可误显示删除可用（后端始终拒绝），已使展示优先采用当前认证角色，并让资料读取identity含role；补重开安全区后禁用与Root说明回归。该新增回归尚需下一轮CI。另通过规定脚本修正vi既有Bind/Bound/Not bound三值为Liên kết/Đã liên kết/Chưa liên kết；仍83新增键，3个既有值修订，其它六语值不变。只读复核已关闭这两项；没有扩大历史assertion cast旧债清理范围。

## 第二轮高级流程结果与修正

- 第二轮所有1496单元、type/lint/build通过。advanced仍失败：模型320动作进入底栏区域（实际原图确认），现将mobilemaxheight收紧为calc(100dvh - 10rem - env(safe-area-inset-bottom))，桌面90dvh和内部滚动保持，严格动作在底栏上方断言不放宽。
- 窗口确认#id解析为aria-hidden的Base UI原生checkbox，Playwright报outsideviewport；改用可访问checkbox名称，保持真实点击/双确认/Enter，无force。第二轮窗口replacement/recovery桌面dark原图已由主线程与非实现者实际查看，确认控件在viewport且无遮挡。
- 模型/真实runtime的radio同类#id问题，改为具名真实role radio，保留checked断言；真实目录合同已通过，第二轮失败图显示仍unlimited，准确停在restrict-mode。新搜索stage进一步区分失败。
- 安全18个布局旅程后错误恢复失败；实际failed-statuses图显示Passkey已恢复，固定失败次数被PasskeyCard与SecureVerification并发读取耗尽。fixture改明确失败phase，三卡真实错误出现并截图后才切恢复、逐个Retry，不跳过失败或恢复断言。
- 第二轮失败用于定向修正；第三轮须完整qualify。Go生产逻辑与5ba90cf一致，后续变化仅profile角色展示/vi词义、模型弹窗几何和浏览器/运行驱动，历史账务仍冻结。

## 第三轮资格与末次驱动修正

第三轮type/lint/build通过；1496单元通过，新增Root角色回归1项因测试漏写现有and all data文案失败，已仅修测试完整可访问名，角色/禁用断言保持。模型17个真实浏览器旅程全部通过，含三模式、未知回读、手机长ID和严格底栏几何。

独立实际审图又发现model vi320dark portal仍白色：共享CSS浅色变量直接覆盖body portal而暗色只在console子树。主线程把dark变量作用于个人dialog/sheet并补popover，模型driver增加实际computed背景/文字暗色断言；安全driver增加vi320dark密码原图与同类颜色断言。已通过的security19个布局/错误恢复之后，功能密码字段非exact标签匹配New与ConfirmNew，改全部exact并核对Token/VerificationCode同类歧义。窗口unknown401错误把初始bootstrap刷新计数1与0比较，改提交前基线不增长且PUT绝不重放；只精准排除该fixture接口已知401资源日志，仍要求JS错误/未配置请求为空。

第三轮真实runtime已完成首次真实窄PUT/GET，并进入恢复保存后失败；未获得HTTPstatus，限流429不是已证事实。静态源流程9次CT、personal显式7次及硬加载5次估计共21/20，可解释最后失败；去除两次advanced open重复forceLoad后估计19/20。原reload-check真实重载与每次dialog真实GET/PUT/GET、Root越权404、两写/额度/元数据/上游不变量全部保留，不改变任何production或临时限流数。restore细分阶段并输出固定event+数字status待验证。最终Node合同166/166再次通过。

## 第四轮实际运行与新鲜读取修正

源码e01b31e的1497项单元全部通过，type/lint/build通过；窗口17、模型17个浏览器旅程通过，实际暗色portal背景/文字和手机底栏几何符合断言。安全19个布局及错误恢复通过后，密码Cancel重开与关闭动画中的同名submit冲突；仅将入口限定安全卡并等待弹窗detached，保留清空和Enter提交断言。

Docker实际首次模型PUT及GET均200，第二次打开未发GET。核对main.tsx发现全局staleTime=10000ms：弹窗重开采用新鲜缓存，不能满足每次打开读取当前CAS的合同。现显式staleTime=0/refetchOnMount=always，并仅接受isFetchedAfterMount的成功结果作为表单和命令基准；待新GET时不展示旧可编辑内容，失败可手动Retry。新增生产staleTime下同QueryClient重开、延迟响应、最新revision提交的确定性测试；安全代理独立复核通过。焦点/重连不自动回读，未知写锁和身份隔离保持。最终完整资格待第五轮，不将第四轮失败候选归为完成。

## 第五轮产品资格与安全驱动收口

源码acba3f4在production构建/type/lint、233文件1498单元、71核心浏览器、窗口17与模型17旅程通过。ConfiguredDatabases在SQLite/MySQL5.7/PostgreSQL9.6实际通过。Docker38851589638六项作业全部成功，legacy/authoritative各6检查/15原图、运行错误0，实际模型范围两次PUT及GET均200、每次重开GET、其它元数据/额度/上游计数保持。新鲜读取缺陷已由真实Go运行关闭。

安全19布局/错误恢复通过，密码验证、Cancel清空与Enter/authrotation已继续通过；AccessToken显示/遮敏后，Close同时匹配footer与X使driver严格模式失败。仅排除data-slot=dialog-close并匹配精确文本，关闭后等待detached再重开；保留零自动生成、双确认、一次性显示与清理断言。其余Passkey/2FA/备份码/会话locator逐项对照实际组件核对。产品实现与acba3f4无变化，最终CI仍须完整通过；已通过Docker按准确不变输入复用，不为仅tools/ui脚本和文档变化重复构建镜像。

独立文档复核指出密码/删号未知不能承诺安全状态回读，已改为先人工核验、显式决定重试；该P2已由非作者回读关闭。Dockerreported usage与专属缺usage语义测试边界也明确区分。

## 第六轮安全WebAuthn夹具前提修正

源码83a23c9的1498单元、三库、71核心旅程、窗口17与模型17继续通过。安全密码及AccessToken完整流程已越过旧定位问题；随后Enable Passkey后等待Remove Passkey超时。旧报告没有begin/finish计数，无法确认具体浏览器错误，不能将推断写成已测根因。

实际代码合同核对：2FA=false时无SecureVerification前置，options.publicKey与helper一致，finish/authrotation/status形状匹配；明确发现HTTP127.0.0.1 origin及RP ID不符合WebAuthn域名要求。[Chromium origins规则](https://chromium.googlesource.com/chromium/src/+/refs/heads/main/content/browser/webauth/origins.md)与[W3C WebAuthn3](https://www.w3.org/TR/2026/REC-webauthn-3-20260825/)要求有效域名RP。现仅将合成driver改为localhost origin/RP ID，服务仍只bind127回环，不启用安全绕过flag。新增固定stage、安全数值计数、合法authrotation布尔、Passkey卡失败原图及真实WebAuthn序列化字段检查；不记录secret/header/原始token。剩余2FA/backup/sessions前置及响应也逐项对照实际API。

主线程把CI既有advanced执行与上传block前移至browser-dependencies成功之后，较长旧矩阵之前先取得本批功能反馈；全部其它套件、条件、超时和断言保留，YAML解析及依赖顺序通过。产品/runtime输入仍与acba3f4一致，完整第七轮CI和实际Chromium结果待核对。

## 第七轮功能通过后的实图P2修正

48a1d89的1498单元/三库继续通过，高级窗口17/模型17/安全20全部通过。安全实际完成密码、Token、CDP Passkey、2FA、备份码、会话撤销，认证轮换及秘密关闭清理断言通过；有效localhost夹具已在真实Chromium中完成注册。

非安全实现者实际像素审查发现已启用Passkey的Remove按钮淡红目标20px高，对比Disable2FA42px；AlertDialogTrigger覆盖data-slot且flex-1在column布局收缩，使个人Button44px选择器不命中。此为产品P2，功能passed不能代替视觉通过。仅该Button加min-h-11，driver真实boundingBox断言height>=44并将高度写入报告，不改全局组件。独立审查还指出手机failed-statuses原帧只见警告图标；现在失败阶段额外拍Passkey/2FA/sessions三张完整卡片原图，再恢复phase逐Retry。缓存格式/lint、nodecheck/diffcheck通过。

最终第八轮CI验证按钮高度、三卡失败图和完整行为。Go/API/Key流程/runtime driver/依赖/版本与已成功Dockeracba3f4不变；新增profile按钮高度样式由最终production-build浏览器验证。Docker复用仅限其实际Go/Key旅程，不声称最终profile样式或二进制字节已由该旧运行覆盖，源码差异另列。

## 最终候选运行

最终源码0a1f6c4cfe4d227612791d4ad32e138f23a0ed3a已同步，完整CI38855315874，PR checkoutc88c8089660e064b31920a02e057e3df34508229，tree684782c16074ad62cda6bdb2816a9691342d4e39与源码相同。第八轮除driver外新增真实profile按钮样式修正，故再运行Docker38855486670以取得最终完整构建的准确源码证据；旧acba3f4成功仅保留历史记录，最终交付不依赖其复用。状态待最终实际结果、原图P2关闭及归档同步。

## 应用资格完成与截图单独补验

CI38855315874已实际完成，10项作业全部success：1498/233单元、type/lint/build、三库、71核心、54高级及六个兼容脚本，root/relaykit/386/race均通过。Docker38855486670六项全部success，两个writer各6checks/15PNG/0errors，准确source0a1f6c4；原Passkey按钮P2已实图及height44关闭。非实现者实际检查最终window/model差异图和双writer代表图，未发现产品P1/P2。

TwoFA失败卡单图只绘出上段，未完整展示Retry，功能20旅程及恢复均通过但证据不完整。仅driver在截图前居中整卡，等待settled，验证整卡/Retry视口与命中；产品不改。使用已注册website-browser workflow的仅手动security_evidence模式复用上述完整通过CI构建，避免为截图驱动改动重建应用。守卫先核对CI身份/成功状态、producer与artifactcheckout树相同，compare只允许docs/securitydriver/该workflow且拒绝300文件潜在截断和越界重命名；再设显式buildHead，报告分列build/driver/workflow身份。默认网站步骤完整保留，contents/actions仅read，主CI不改。新workflow文件未采用，因为[GitHub手动事件规则](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)要求默认分支已有工作流，本批不合并main。

独立maintenance_plan只读审查通过，无P1/P2；YAML/bash/node/diff检查通过。下一步：提交仅证据driver/既有workflow/记录，手动补验准确c88构建，实际看完整TwoFA失败原图，再归档和同步Draft PR。

## 最终验收与归档

应用源码0a1f6c4、CI checkoutc88（树684782c1一致）通过CI38855315874全部10项和Docker38855486670全部6项；1498单元、71核心、54高级、六兼容、三库/386/root/relaykit/race均成功。证据driver3820baf在手动补验38857597290中实际通过source guard，复用同c88构建20旅程再过；居中settled后TwoFA失败卡图标、标题与Retry完整。报告分列buildHead/driverHead，完整CI原security报告另存。产品源文件从0a起保持不变，后续归档只文档与证据。

已实际运行只读CI构建预览4174并核对200读取、405写入/生成Token；非实现者审查最终按钮44px、暗色/手机长文/错误恢复、窗口/模型差异图及双writer代表图，原P2关闭。凭据轮换与迁移只交付能力和四项必要决定，未实施。原M哈希、现有登录/Key、缺usage成功/Token未知与近似误差语义保持；历史账务和生产数据/凭据/备份继续冻结，无合并/标签/发布/部署。

归档清单和SHA256见evidence索引；GitHub同步沿用现有功能分支/Draft PR17，归档自动CI与上述准确资格运行分别报告。用户已有计划/设计差异保留。
