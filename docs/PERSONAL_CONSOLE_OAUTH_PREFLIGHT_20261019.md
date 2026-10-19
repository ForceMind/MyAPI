# 第二阶段前置核查：OAuth 与隔离运行控制

## 当前目标与授权

维护者于2026-10-19要求先完成无需真实账号的 OAuth 兼容性核对，以及刷新、采样、请求计数和停止机制的隔离验证。发现产品代码适配需要时，先交付具体文件、行为影响、验证和候选版本方案，不直接实施适配。

起点：`codex/subscription-key-share-20261014`，HEAD `3b5f5b26431a51b018252a2b61b51dbd8f5fe90d`，应用版本 `0.2.0-beta.20`。第一阶段准确应用/验收来源为 `cdeef8b52792a09b341d074521d59cb5cc7f62fb`，CI39203586608与Docker39203596121已通过。本轮不重复完整构建或视觉确认，不将旧合成结果当作官方新协议或真实账号资格。

不接入真实账号，不操作真实凭据、生产数据库/Redis/备份或历史账务。不合并 main、不打标签、不发布、不部署。项目只写本核查记录、合成证据及既有交接入口；产品源码、版本、运行配置、工作流和数据库结构保持。用户现有两个计划文档及设计草稿不编辑、不提交。

## 分工、方法与资源

沿用本轮持久 Goal。主代理负责范围、现有调度/运行控制链核查、记录和证据集成；`oauth_protocol_audit` 使用 `gpt-5.6-sol/ultra` 只读核对官方协议并独审；`oauth_isolation_probe` 使用 `gpt-5.6-sol/high` 在 `/tmp/myapi-oauth-preflight-*` 干净当前源码副本中运行合成 Go 验证。最多三个角色同时工作、不递归、不争写。探针代理完成首批后因模型capacity退出，主代理接管唯一临时测试写入和Go执行，不另开并发Go。协议代理已完成主要审阅和修正清单后同样遇capacity，最终由`oauth_preflight_final_review`按既定高风险独审配置`gpt-6-astra/high`只读接续。原审阅不冒称已最终通过。

1. 以当前官方 OpenAI Docs 的注册、账户/刷新、Token、错误恢复和推理合同，与实际旧Codex通道逐项对照；记录来源URL和日期。官方新入口的差异不直接证明旧通道已失效，也不把 connected-app cap 与当前 WHAM 原生窗口混为同一数据合同。
2. 先运行已有相关测试，再对实际缺口添加临时测试/探针。只使用 fake transport、literal loopback 和新临时SQLite，不调用官方认证/用量/推理 endpoint。源码副本通过当前 HEAD 的 git archive 建立，不复制用户工作区私有目录。
3. 分别验证已有互斥/CAS、超时/取消、采样数量/轮次限制与隐式刷新；统一总请求预算和停止能力若不存在，记录明确缺口或预期失败，不把通道数当作物理HTTP次数，也不将取消等同服务端撤销。
4. 原始测试源码/输出、准确来源和摘要分别保存；实际通过、已确认缺口和未验证项分别列出。对适配提出最小分组、文件与接口影响、兼容/版本和验收计划，供维护者审核后再开发。

本机只跑缓存定向测试，`GOCACHE=/tmp/myapi-go-cache`、`GOMAXPROCS=2`、`GOMEMLIMIT=768MiB`、`GOPROXY=off`、`-p 1`；不安装依赖/浏览器或运行Docker。遇冷构建/缺依赖先报告，不通过真实网络补验。代码适配未批准前版本保持beta.20；正常docs-only归档沿用现有分支/Draft PR同步授权，不重复重型CI。

## 核查结论

当前结论分三层：旧Codex链路的已有保护可用合成测试验证；官方新SIWC（Sign in with ChatGPT，允许应用使用ChatGPT订阅的授权入口）与旧链路存在明确合同差异；真实账号受控试验需要的统一请求预算和停止控制尚不完整。不能将旧测试通过解释为新SIWC已兼容，也不能凭新文档宣布旧路线已经失效。真实供应商授权、用量及429仍未验证。

### OAuth兼容性矩阵

核对日期：2026-10-19。以下官方文档是当前新SIWC入口的合同，不是旧Codex endpoint的弃用公告。

| 项目 | beta.20实际能力 | 新SIWC合同与结论 |
| --- | --- | --- |
| 注册和client身份 | `service/codex_oauth.go`固定公共Codex client与`/oauth/authorize`、`/oauth/token` | 初次动态注册、稳定host ID，回调保存issued client；之后按该注册复用。旧client不能直接替代新注册。 |
| 回调和权限 | 旧`localhost:1455/auth/callback`，有state、PKCE、10分钟一次性会话；控制台会话、channel和凭据摘要绑定 | 新入口要求literal `127.0.0.1`、nonce、resource及订阅scope；同次请求与换码URI必须一致。`access_denied`校验state后停止且不换码；新注册成功必须带issued client，重授权可省略client，但出现不同client则拒绝。现入口未实现这些新字段。 |
| ID与授权校验 | 本地JWT仅解析；已有账户连续性依赖固定供应商TLS响应和独立provider-confirmed证据，不信任任意导入JWT | 新入口还要求ID Token签名/JWKS、issuer、aud、exp、nonce、返回scope以及注册身份连续性。不能把现有JWT解析器当作该验证器。 |
| Token持久化 | `CodexOAuthKey`含IDToken字段；换码/刷新结果仅传Access/Refresh/Expiry | 新入口需保存issued client、host关联、验证身份、tokens、granted scopes及expiry；请求/换码/刷新携带resource并校验aud。将固定resource写入凭据是实现选择。当前结果结构不传递验证后的ID Token或`earliest_refresh_at`；后者官方仅列字段，单位及具体调度语义仍待核实。 |
| 刷新 | channel级gate、DB lease、ExpectedKey和CAS；固定旧client；自动任务启动即跑、每10分钟检查、24小时阈值 | 新入口按issued client和resource刷新、接近到期并串行同一session。当前channel级保护不能证明同session跨channel/独立数据库互斥；1小时Token会落在每轮24小时阈值内。 |
| 推理 | `relay/channel/codex/adaptor.go`走`/backend-api/codex/*`和旧账户header | 新入口为公共`POST /v1/responses`；需独立评估模型/能力、`store=false`和`stream=true`，不能只替换host或client。 |
| 限流与恢复 | `service/codex_quota_limit.go`识别旧429码/文本和WHAM恢复证据；本地近似分额错误为`subscription_share_*` | 新入口有`subscription_sharing_*`等恢复语义，部分失败在HTTP200后以`response.failed`到达。两类码不能合并。共享Responses目前会转发failed但未保证以错误返回；错误脱敏还可能归一新码。当前旧429判断不等于完整新入口支持。 |
| 原生窗口/分额 | 已有WHAM规范化窗口、账户身份和近似Key分额 | 新入口的每应用限额不证明存在与WHAM相同的窗口API。官方还区分Plus跨应用共享五小时限制及不适用该限制的Pro；不能据此把所有套餐或窗口合并。429可能来自应用自身限额，不能据此断言整个订阅池耗尽或推断reset。 |

官方依据：[注册与登录](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)、[账户与刷新](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions)、[Token参考](https://developers.openai.com/siwc/token-sharing-open-source/token-reference)、[模型与推理](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)、[错误与恢复](https://developers.openai.com/siwc/token-sharing-open-source/errors-and-recovery)、[预览限制](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)。只读取公开文档，没有向认证、Token、用量或推理服务发出业务请求。

新入口精确scope为身份`openid profile email`与订阅`offline_access resource.invoke chatgpt.tokens.use.direct`，resource为`https://api.openai.com/v1`；换码和refresh也携带resource，refresh省略scope保留grant。上表描述当前差异，未向旧Codex流程追加这些参数。

### 已有控制与停止边界

| 控制 | 实际范围 | 不足以证明的能力 |
| --- | --- | --- |
| 采样开关 | `controller/system_task_handlers.go`的`Enabled()`控制scheduler是否新建任务；默认开启 | `Run`和claim不复查开关。关闭配置不取消已入队/在途任务，也不阻断手动查询、OAuth刷新或推理。 |
| 数量上限 | 一轮默认100、上限1000个凭据尝试；多Key展开有独立界限，去重/轮询防止永远漏采 | 不是物理HTTP总数、生成请求数、刷新数或整个验收session的总预算。 |
| 时间和取消 | 单轮截止按间隔，最高5分钟；单次后台Codex采样共享15秒上下文，尊重更短caller期限；每候选/等待间隔检查取消 | 手动用量查询的刷新后重试会重新派生15秒deadline，与后台采样不同。lease失效可取消本轮，但没有面向验收session的停止接口；下一轮仍可能新建。关闭浏览器不能作为后台停止证据。 |
| 自动刷新 | `service/codex_credential_refresh_task.go`单进程不重入；每channel串行及跨进程同DB lease | 独立于采样开关，使用Background上下文，缺专用验收暂停/计数。不能用`NODE_TYPE=slave`代替局部停止，它会影响其他主节点职责。 |
| 取消后的写入 | 已成功旋转的Token用独立最多5秒上下文完成安全落库；失败观察/任务终态也可用有限独立上下文保存 | 停止不等于撤销已发出的上游操作，也不应通过取消必要落库丢失旋转Token。 |

## 实际隔离验证

准确基线为`3b5f5b26431a51b018252a2b61b51dbd8f5fe90d`（tree `a4268033b5abe8cc0eeccdefaba865e14a44795e`），从git archive解包到`/tmp/myapi-oauth-preflight-1WJhBO`。只增加两个临时`*_probe_test.go`；仓库应用源码没有加入探针或改造。供应商交互均由fake RoundTripper或本机httptest替代；数据库均是新建临时SQLite。没有启动带已有配置/数据的MyAPI实例。

最终重现命令、所选测试名、原始stdout和退出码见[证据索引](evidence/personal-console-oauth-preflight-20261019/README.md)。最终三个包共**68个顶层测试通过，118个叶子案例通过**，所有包退出码0；父测试和子测试不重复相加。四个新增顶层探针包含五个叶子案例，既有回归64个顶层测试。本机无新增构建依赖，无Docker/浏览器/重型构建，未重复第一阶段Actions。

| 验证组 | 实际结果 | 证据说明 |
| --- | --- | --- |
| service | 15顶层/57叶子通过 | PKCE/state URL、合成换码、18个有效期边界、20个重定向拒绝、正常返回；并发刷新/可信subject/CAS、取消后旋转落库、task期限/续租收尾、旧429与generic429区别。 |
| controller | 38顶层/45叶子通过 | OAuth控制台绑定与新建/替换、元数据与凭据写入区别、采样共享deadline/短期限/持久化失败、数量限制/多Key去重与轮询、证明随凭据生命周期变化、有界任务终态。 |
| model | 15顶层/16叶子通过 | 凭据CAS/缺DB/取消后缓存一致性、可信历史subject、gate等待、AuthFlow绑定/一次性/过期/事务回滚、named lease。未声称新增跨channel同session锁。 |
| 自动刷新探针 | 25小时有效期：0次；1小时有效期：连续两次显式run共2次refresh | 未等待10分钟或改变时钟，直接调用实际单轮函数；每次保存合成替换Token。确认“有效但小于24小时”仍会刷新，不是对SIWC真实端点的兼容性证明。 |
| 手动401/403探针 | 各自usage→refresh→usage，共3次fake HTTP派发；均保存合成旋转Token | 调用实际`FetchCodexWhamUsage`及刷新链，而非只统计stub调用。两例还确认手动重试重新生成deadline。403的旧行为不能当新SIWC政策拒绝的正确恢复。 |
| 取消与收尾探针 | 首channel usage=1、refresh=1；第二channel usage=0；failed=1、deferred=1；旋转凭据已落库 | 取消发生在合成Token响应返回时；证明不进入下一候选且必要落库完成，不证明取消可撤销上游操作或提供全局停止。 |
| 关闭定时采样探针 | `Enabled=false`时直接运行一次batch仍派发1次HTTP并采样成功 | 对现有行为的通过断言，明确暴露“调度开关不是统一停止”；未伪造预期失败或新增停止实现。 |

首次控制器测试在受限沙箱因`socket: operation not permitted`无法建立httptest loopback（退出1），同一源码在获准的loopback测试环境重试9个顶层测试通过（退出0）。原失败/重试输出均保留；没有通过修改产品消除该环境限制。后续最终68项也使用该获准隔离测试环境。最终结果只统计三个final日志，不把早期重复结果或环境失败算成新增通过。

本轮测试使用SQLite，不是新增MySQL/PostgreSQL行为验收；本轮没有数据库或产品改动，第一阶段准确应用源码的三库CI仍按原记录引用。公开文档核对、合成行为测试、旧CI、真实业务资格四者分开。

## 代码适配建议与候选版本（均未实施）

先完成beta.21的隔离验收控制，再决定是否需要独立SIWC入口。候选号为计划，不创建版本或标签；本轮纯核查继续beta.20。现有旧Codex、控制台登录、Key身份/单值轮换资格、近似分额和冻结账务均保持。

### 候选beta.21：受控验收机制

目标是让后续真实试验具备可核验的账号范围、物理请求计数和停止证据。只在显式开启的隔离验收运行中生效；默认不启动、不入队、不发送请求，普通实例保持现有行为。先完成纯合成回归，不引入真实凭据。

| 具体文件/模块范围 | 拟改行为 | 影响和验收条件 |
| --- | --- | --- |
| 新`service/upstream_acceptance_run.go`、`model/upstream_acceptance_run.go`及测试；`model/main.go`注册 | 新增验收run记录：允许channel/账户、截止、状态、分类计数、总预算、停止锁存；状态/计数原子持久化，重启不自动续跑或清零预算 | 只写新增验收表，不写历史账务；SQLite/MySQL/PostgreSQL兼容。持久化或身份判断失败即拒绝发送。 |
| `service/codex_oauth.go`、`service/codex_wham_usage.go`、`service/codex_models.go`、`service/codex_credential_refresh.go`、`controller/codex_usage.go`、`controller/channel-billing.go`，以及实际生成发送点`relay/channel/api_request.go`/`relay/channel/codex/adaptor.go` | OAuth换码/refresh、usage/模型每次查询、重试、生成均在实际发送前申请同一预算；在验收专用transport或共用发送点统一拦截，分类与HTTP总数分别记录 | 统计MyAPI进程的上游HTTP派发，失败/重试也计数；浏览器内IdP登录跳转不在进程计数范围。禁止重定向/自动重试绕过；不存Token/URL查询内容。仅覆盖明确受验通道，不改变其他provider。 |
| `service/codex_credential_refresh_task.go`、`controller/system_task_handlers.go`、`service/system_task.go`、`main.go`相关启动点 | 验收模式默认暂停该范围自动刷新和采样；手动启动也受同run约束；停止后不再claim/新建/重试/刷新/生成 | 运行中的出站context取消；已经成功旋转的凭据仍允许有界安全落库并报告drain结果。停止终态确认无新出站，而非只更新UI或开关。 |
| 新隔离验收控制controller/路由与命令行驱动；相关安全测试 | 仅既有维护者会话/来源保护可开始、查看、停止；不可从Key或普通外部调用开启；记录停止原因、计数和最后一次派发 | 首批使用隔离驱动及机器可读报告，复用现有视觉；若后续增加控制台操作，单独列页面范围并完成七语/桌面手机验证。 |
| 新验收脚本/定向CI；现有refresh/sampling/OAuth回归 | fake transport/loopback、临时DB与并发精确计数；到限、deadline、停止、401/403、429、重启和落库失败 | 合成环境先证明限制不可绕过；按既有Actions完成必要三数据库/Redis验证。没有真实账号前不能写“真实429/窗口通过”。 |

发送预算与现有发送凭证必须按“预留、临发送确认、未发送收尾”对齐；禁止在现有逻辑已经标记可能发送后，直接新增拒绝却遗留未决凭证。需沿用既有未发送收尾与安全不变量，验证停止/到限/持久化失败不会制造账务义务或误记供应商调用；不修改历史账务或切换writer绕过。

建议后续首轮真实方案仍为单账号、单生成并发、主动试验20分钟、最多10次短生成，推理重试默认关闭。OAuth、refresh、sample和其他HTTP另列预算且同时受HTTP总上限约束；**具体各类额度和总上限需在真实试验前根据路线单独确认**，不能将“10次生成”误写成10次总HTTP，也不能拿未用完的采样额度增加生成次数。没有自然429就保留待验，不主动烧空订阅。观察下一次原生窗口重置需另开明确观察时段。

### 后续候选beta.22：独立SIWC入口

仅在维护者确认需要支持新入口后实施；旧Codex可能继续独立使用。必须先解决新注册身份、回调可达性、刷新session范围、API可用模型与窗口数据来源，不能用统一client或邮箱合并注册。

| 范围 | 拟改内容与影响 |
| --- | --- |
| 新独立SIWC OAuth service/controller、DTO、维护者路由及测试 | 动态注册/issued client、stable host、state/nonce/PKCE、literal loopback同URI、callback错误/client错配、JWKS签名/issuer/aud/exp/nonce、授权scope。复用安全原语，保持旧Codex endpoint/流程。 |
| 独立凭据记录/刷新service及model迁移 | account/registration/client/host/token/grant/resource/expiry关联；保留验证后的ID Token，原子更新旋转Token；按同session跨进程互斥。保留`earliest_refresh_at`并先核单位和语义；错误码区分终止重登与暂态退避，避免401/403一律refresh。兼容旧数据读取，不推测缺失元数据或自动改写旧凭据。 |
| 新relay adapter/明确通道能力；`relay/channel/openai/relay_responses.go`、`service/error.go`、`service/http.go`/`relay/channel/api_request.go`的必要定向修正 | 公共Responses路由、模型/工具/参数约束，显式处理completed/failed/incomplete/中断；保留新错误code、前置detail与request ID并脱敏。避免错误allowlist吞码；本地`subscription_share_*`和上游`subscription_sharing_*`分开。旧`service/subscription_share.go`只认可旧backend入口，新路由分额资格须独立证明。不把应用限额当作整个原生池；保持其他provider语义及“完整回答缺usage仍成功、Token未知”，不改历史账务。 |
| UI、七语言及规范说明 | 明确显示入口/授权能力/账户注册；未知用量保持未知；已有个人页面不显示金额余额。沿用已确认M logo、柔和蓝和布局，实施才递增应用版本并交付实际页面/桌面手机图。 |
| 测试/资格 | 全部先用合成JWT/JWKS、端点和DB证明；真实账号仅在另行提供资料、预算与授权后验收。官方公共文档不保证WHAM原生窗口能力，缺数据不得伪造或退化成Token/金额池。 |

## 尚未验证及下一步

核查、最终隔离测试和独立只读审查已完成，原始证据已归档。[独审记录](evidence/personal-console-oauth-preflight-20261019/independent-review.md)无阻断发现；审阅指出的字段/发送点/错误链精度及118叶子计数已修正，原始日志未改。没有统一预算/停止能力或新SIWC支持的完成声明。任何真实账号接入、真实凭据写入、生产/历史账务、main合并、发布及部署均未执行，后续适配等待维护者对本节具体范围的决定。

真实服务仍待验证：旧Codex当前授权/refresh是否被供应商接受；issued client的真实注册与JWKS轮换；刷新Token重用/吊销行为；原生窗口、重置、新鲜度和429；真实多进程/多库同session；2FA/实体Passkey与生产恢复。下一批推荐先批准上列beta.21控制范围，在合成环境证明预算和停止后再提供真实测试资料；是否增加beta.22独立SIWC入口另作决定，不作为本轮已批准开发。
