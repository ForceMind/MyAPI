# 历史金额兼容候选方案

状态：设计参考；用户已要求按收敛计划继续，实际金额表示兼容已进入本地候选，保留各writer既有业务政策，不把下面的政策问题视为新增授权。实施与验收的最新事实见[历史核查记录](HISTORICAL_ACCOUNTING_COMPATIBILITY.md)。源码基线为main `92fa1182e265924312d70917a0a637965aebc617`；SQL模拟、实际Go私有副本及合成检查均不能代替生产账务对账或部署批准。

## 要解决的问题与证据范围

本实例4个持久字段已超过int32，SQLite实际INTEGER可以精确保存原值；原值均在`±(2^53−1)`内。钱包额度有精确匹配的管理员加额度记录，宽Key累计值等于8363条单笔int32内历史消费日志之和。Key原始剩额、更早来源、用户累计与日志差额及历史缓存仍未核清。

问题包含两个独立条件：当前legacy/batch=true/no Redis在余额事务前阻断结算；已有宽余额又不满足普通结算的int32结果检查。修正金额表示不会自动解除缓存配置故障，也不能证明应扣金额。保留当前配置及原记录，不能以新增空Redis、关闭batch或改变writer绕过。

目标是保留已有余额、累计值、原结算身份与每次收费上限，使合资格历史值能被应用精确读写，并让后续小额消费/退款继续工作。方案既覆盖原来宽的字段，也覆盖今后普通字段合法累计跨越int32的情形；不能仅为8306条离线模拟事件增加特例。

## 选择与推荐

| 选择 | 对本目标的作用 | 判定 |
| --- | --- | --- |
| 截断/归零或只改applied | 损失原值或产生余额与义务不一致 | 禁止 |
| 全局MaxInt64、仅改表或仅放宽helper | 留下Lua/JSON/输入/其他结算路径的不一致 | 不采用 |
| 仅对封存来源宽字段做一次性SQL修复 | 已可验证受限差额，但后续请求及正常累计仍不兼容 | 保留作演练，不能作为最终修复 |
| 单次费用与累计余额采用分别验证的整数契约 | 保留单次费用限制，完整修复累计、缓存、API与数据库的兼容边界 | 推荐候选，待审阅及全链路验收 |

推荐候选采用`M=2^53−1`作为钱包/Key及共用结算金额的单字段精确界，不改变common.MaxQuota的单次费用含义。各字段是否为账务、统计或其他既有受限额度见下表，并非所有含quota的字段都自动放宽。M来自Lua双精度及JavaScript整数表示能力，并非现金政策或已批准的额度上限。若将来需要超过M，须另审十进制字符串计算及接口方案；本任务不隐式扩展到该范围。

## 可审阅的数据与行为契约

| 对象 | 候选规则 | 不允许的替代行为 |
| --- | --- | --- |
| 单次实际费用、预扣与请求delta | 继续现有int32边界及非负收费、数量/比例验证、饱和审计；退款符号沿用原协议 | 放宽common.MaxQuota、负收费或溢出后变成赠额 |
| 钱包quota、Key remain | 精确有符号整数，原值及每次目标均在[-M,M]；新预扣仍检查可用余额 | 将负值归零、截断、用高额度绕过预扣 |
| Key used | [0,M]账务状态，与remain及对应版本在同事务/CAS成对更新；退款不得令used负 | 把Key used当作可随意补写的统计 |
| 用户/channel累计及quota_data | [0,M]精确整数的尽力更新统计投影，退款型负delta可输入但目标不得负；溢出检查及失败可观察，但不承诺持久重试/恰好一次 | 按日志覆盖历史counter、升级为新的强一致统计系统或用它证明余额守恒 |
| 订阅plan total、instance total/used | 同结算路径检查[0,M]，total=0的unlimited哨兵保留；每次相加先检查，退款不得令used负 | 先溢出再把负值夹成0、把本实例无订阅解释为其他实例路径安全 |
| Affiliate等独立旧额度 | 保留既有int32合同及精确边界；转入钱包时独立检查钱包M目标，单次转移不得绕过原限制 | 顺带把所有奖励/权益字段放宽或让旧int加法溢出后才检查 |
| 原值在int32内的字段 | 合资格累计可跨越int32，仍须满足M、事务和输入限制 | 沿用离线模拟的“原字段未宽则永不许变宽”来冒充长期应用兼容 |
| 原值超M、REAL或非精确字符串 | 保留原始数据并拒绝相关写入，报告人工核查原因；不自动修正 | 经float读入再转换、静默夹紧或让迁移掩盖异常 |
| 单次管理员调整和新Key输入 | 保留现有单次输入上限，新增操作同时检查累计目标；无限Key也验证明确提交的金额 | 用unlimited跳过所有范围检查、自动把既存宽remain改为0 |
| 更新旧Key其他属性 | 元数据更新完全省略余额写入；显式额度调整走持久request_id去重及旧值/版本CAS的独立事务，详见下文 | 元数据表单覆盖并发消费、超时重试重复赠额、浮点往返覆盖宽历史值 |
| 退款 | 原事件/receipt及原writer校验，金额有界；remain与used同步反向更新，used不得负 | 换新身份重放或把已退款部分再次入账 |
| 逐腿与版本 | 原余额/版本CAS及事务保持；更新对应余额时才提交对应applied，另一腿可合法未完成 | marker-only写入、合并intent/fact重复计债 |
| batch聚合 | 单次项仍有界；代次聚合独立精确求和/目标预检，可超过int32但不得超过M | 因聚合变宽而丢弃、拆换已有generation身份或重新扣全用量 |

单行Go持久金额及已证明有界的中间计算使用明确int64；入口和收费计算继续现有费用类型/边界。凡暂留int参数的边界必须先证明平台和数值范围，不能依赖64位机器上的隐式可用性。加减先按目标上下界检查，再运算；多行合计可能连int64也超出，必须逐项checked-add，超过能力则精确拒绝或在已审的只读汇总合同中用big.Int/十进制字符串，不能先SQL SUM溢出或扫描到int再检查。转换不得通过float64。普通费用饱和逻辑与账务余额拒绝逻辑分开：余额不能饱和后写入。

用户/channel/quota_data统计保持现有尽力更新的持久性语义，不在本任务增加强一致统计账本；需要修正宽类型、算术及失败观测，仍不得据统计counter证明资金已落盘。request_count及其他计数保留独立计数合同，不随金额M一并重定义。用户累计与日志差额继续作为历史未知项，不能用这个设计选择宣称差额已解决。

1608基线已实现元数据省略金额SQL列和余额版本/元数据CAS，仍缺持久请求身份。本批在现有PUT `/api/token/`实现获审合同：显式`remain_quota`（含等于当前值）要求规范小写UUID `request_id`、规范十进制字符串`quota_version`、`metadata_fingerprint`，null明确拒绝；仅元数据请求省略金额及上述身份字段。一次表单修改名称、权限和金额仍为一个事务，不拆成两次HTTP。收据`token_quota_adjustments`以`(user_id,request_id_hash)`唯一，并精确核验原身份、Key ID及包含字段存在性的客户端白名单指纹；持久指纹版本与v1规范编码明确，未来缺席字段不能改变旧摘要，指纹在当前数据库归一化之前确定，不存Key秘密。事务锁Key、重查收据、核验writer/旧版本/元数据，原子写整次表单、版本及不可变成功结果，used从锁定行保留。相同身份同参数在当前owner校验后返回原收据、`replayed=true`及当前持久数据库行的脱敏Key（非未drain的有效缓存余额），后续消费、状态和权限变化不改原结果，不发布旧缓存；异参数冲突，失败事务零残留。只有确认提交成功才建立持久结果，commit失败不自行证明已完成；现有缓存屏障与错误政策保留。额度版本是并发前提，不能替代request_id幂等。旧客户端缺身份的显式额度覆盖不得静默成功；unlimited与单次输入上限保持。当前实现及三库/前端回归待准确后继CI，历史义务不因此取得许可。

管理员override的输入是目标值，其内部差额`target−current`可能超过单次费用上限；不能误用费用验证器使历史宽余额无法按显式目标调整。内部差额另按精确调整合同验证，绝对值超过M时明确拒绝且零写入，不拆换原请求身份或自动分笔。例如从-M覆盖到正的int32目标，其差额也可能超M；不能因原值与目标分别合格就忽略该边界。receipt/审计JSON和缓存同步都必须覆盖这类内部差额。

负钱包/Key仅是已保存或候选差额推算的带符号配额值，不能自动解释为现金债务。现SQL模拟出现1负钱包/2负Key；是否能实际应用这些差额，仍取决于额度性质、历史预扣/缓存及逐请求证据。即使用户说明只是自用配额，也不能因此清除旧义务或自动把历史用户改为无钱包模式。

负余额必须按操作授权，而非只按表示范围：受余额约束的reserve/admission要求非负费用和足额余额，不能因扩M新增透支权限；unlimited/self-use保留其明确既有资格，不以兼容补丁自动开启。退款/credit可向正方向恢复，Key used不得负。终态实际结算能否把余额推负、管理员subtract能否透支须分别裁定；现legacy管理员subtract允许负目标，authoritative管理员路径拒绝负目标，不能假定各模式原本一致。候选不擅自统一这项业务政策；在用户决定和逐入口语义核对前，不实施涉及负目标的金额政策。

如果上游已产生用量但最终目标会超M，义务不能丢弃、夹紧或改成赠额。按原request/event在现有pending/manual-review义务路径中保留精确费用和失败原因，applied保持事实状态。隔离须按方向和证据处理：阻止会维持/加深越界风险的新reserve/debit及未知重放；只允许有充分事务/缓存证据、经原身份/幂等/CAS验证且使状态回到合法范围的退款、credit或debit，不能一并禁止恢复权益的合法操作。须验证既有人工审核状态是否实际拦截应阻止的后续admission；不足时补齐同范围门禁，不能只记录错误继续运行。该路径须独立审查其身份/持久性，尚未实现或批准自动恢复。

## 数据库及迁移

先用不初始化应用的原生只读检查确认每个实际列的类型、当前范围、默认值、NULL、约束、索引、trigger/view/FK，再计划写入。SQLite现有INTEGER不需要为本实例做32→64位表改造；不是所有SQLite schema都自动合格。

MySQL至少5.7.8、PostgreSQL至少9.6须用原生实库验证有符号BIGINT/兼容类型；int4或其他不足列不能依靠Go类型放宽。需要扩列时采用独立、可审阅、重复执行安全的迁移及回退验证，记录锁表/空间影响。SQLite不使用ALTER COLUMN，不为展示统一而重建已合格的表。

同时调整GORM模型及迁移所有权，防止普通AutoMigrate根据旧`type:int`把已合格宽列重新收窄。金额/schema门禁必须先于User/Token/receipt等任何相关AutoMigrate，不能只先于worker。物理数值列清单包括user quota/used、token remain/used、UserQuotaMutationReceipt的QuotaBefore/QuotaAfter及旧预留/订阅共用数值字段，不能只改主余额列。QuotaMutation的Before/After快照是TEXT JSON：只修Go字段、解码及旧JSON兼容，不把TEXT改成BIGINT、不重写旧快照/receipt/fingerprint。应用迁移前先读版本/schema/config，无法证明安全时阻止迁移写入；合法authoritative且无需金额扩列的配置不得仅因无Redis被误拦。方案提出时Go候选仅完成writer/cache配置门禁；当前本地候选已实施迁移前金额/schema及执行门禁，原生三库与生产历史验收仍未齐，最新实现及证据范围以核查记录为准。

旧版与候选金额写入器不能并发操作同一份升级中的账务库。不得擅自改变writer模式、epoch或已有指纹来解决混合版本问题。物理扩列不能反向收窄仍有宽值的列；回退必须连同同点余额、逐腿记录、代次及缓存一起验证，见[回滚合同](HISTORICAL_ACCOUNTING_COMPATIBILITY.md#未来实际恢复的回滚合同方案未实测)。

## Redis、代次与版本比较

`model/quota_reserve.go`现同一quotaInteger同时限制单次amount与缓存余额，需分别验证：单次amount仍int32，余额/累计按M，代次聚合按独立合同。所有字段、目标、key类型、epoch/锁/代次及格式检查在第一次余额写入前完成；Redis脚本后续报错不会回滚已发生的HINCRBY。

Lua中先按`maximum−delta`等边界预检，不能仅算出可能已舍入的`current+delta`再判断。保留规范十进制字符串，整数输出不得使用会产生指数或舍入文本的tostring。Key remain/used的一对更新及journal/compensation需共同覆盖；缓存缺失、旧schema或结果不明保持未知，不猜测成功。

前置检查只能保证可预见的输入/类型/范围错误在余额写入前被拒绝，不能保证Redis运行时不会在部分写入后失败。运行时错误、连接中断或丢失响应均须保留原journal/generation并进入现有未知结果处理，不能自动重放或补偿，也不能把Redis脚本原子执行误写成报错后事务回滚。

`model/user_auth_cache.go`和`model/token_cache.go`用tonumber比较QuotaVersion/writer epoch，金额界M不能修复超过双精度精确范围的版本比较。候选应在规范非负十进制字符串上比较长度/字典序，保留现有int64版本上限及CAS语义，不重置/缩小历史版本。同样检查AuthVersion等共享缓存比较，避免改金额时遗留同一投影失效边界。

缓存协议变更不得通过清空缓存启动：先封存旧schema/代次，核清未确认应用与所有写入者，明确旧格式读取、投影和恢复边界。现生产没有Redis配置这一事实，不证明历史内存/Redis已结清，也不构成新增空实例的授权。

宽语义采用新代次协议时，新v4 payload/聚合为int64，新fingerprint包含schema/domain tag；保留v3原算法作为旧代次核验依据。现初始化会把低SchemaVersion行提升至current的逻辑必须先修正：v3 pending/applying/failed/compensating不得自动改成v4，旧终态也保留版本，generation key/payload/fingerprint/state不重写。按原版本验证执行或先证明旧代次已结清再启用新写入，不能因升级批量重解释历史义务。user/token缓存schema亦需显式版本化；旧缓存按已审协议读取/隔离，不以清空替代结清。

## JSON与现有界面

主金额字段若以JSON number保留，必须在后端按精确整数验证在M内；不能通过map[string]float64解析后再检查。现有客户端返回类型尽量保留，输入拒绝非整数及不精确表示；接口/缓存序列化仍使用common JSON包装。

单字段安全不等于接口安全。例如`controller/token.go`的`total_granted=remain_quota+used_quota`，两个合格字段的和可超过M；跨用户/多行汇总还可能超int64。禁止返回舍入number或靠新增remain+used≤M的财务约束隐藏问题。本批获审接口合同：`GetTokenUsage`先独立校验remain∈[-M,M]、used∈[0,M]，可表示的派生和保持number并返回`total_granted_representable=true`；超M则`total_granted=null`、标志false，合法组件保持精确number。非法组件仍失败；不改成string或增加财务总和约束。Key列表/详情/搜索不为显示而扩充DTO；前端在真实API边界校验组件，先按符号检查边界再相加，超界显示已有翻译并保留两组件、隐藏派生比例。若调用方必须取得超M精确总和，则另审版本化string DTO。本批实现及云端验收状态见[云端记录](CLOUD_VALIDATION_PROGRESS.md)。

前端金额编辑、精确配额展示及货币/单位换算分别检查，不把显示用浮点结果提交回账务。已有宽值的只读显示及无金额变更表单必须保真；任何必要的UI修正只限此兼容缺陷，沿用i18n及现有样式，不扩展新功能。跨行汇总不能靠单字段M保证精确。

## 实施顺序、文件与最低验收

写点核对按目的列，而非只改一个helper。以下是候选实施清单，尚未声称已完成全仓写点审计：

| 目的状态 | 需要核对的写入来源与投影 | 范围与不变量 |
| --- | --- | --- |
| users.quota | reserve/context、settlement/refund、任务/account mutation、管理员、topup/redemption/checkin/credit_edge、affiliate转入、订阅购买 | M目标检查、原业务身份及逐腿/CAS；正常入金也不能先int溢出再走authoritative |
| tokens.remain/used | reserve/extension/context、settlement/refund、任务/account mutation、Key额度编辑、batch及token cache | remain/used同事务；额度授权编辑used不变，消费/退款二者反向，原代次/请求幂等 |
| subscriptions total/used | 计划/实例输入、购买、预扣/延长、终态结算/退款及对应snapshot | 精确范围、total=0哨兵、先检查后加减，不把溢出或过量退款夹0 |
| users.used、channels.used | 请求统计、task/usage-review投影、utils批量统计 | bounded best-effort，失败可观察，不作余额腿或全账守恒凭据 |
| quota_data | usedata内存累计、usedata_flow及SQL聚合、dashboard接口 | 只读统计/展示精确及超界明确行为，不制造新的强一致账本 |
| users.aff_quota/aff_history | 邀请奖励、转入钱包、对应缓存/API | 维持原int32独立合同，转入wallet的目标独立按M检查 |
| receipts/历史快照/传递DTO | quota_mutation、user_quota_mutation、user_usage_policy、legacy reservation、billing/relay context、缓存投影及格式化 | 宽状态不得经int；旧记录/指纹不重写，单次费用仍有int32证明 |

持久Goal沿用当前完整任务，不新建缩小目标。获审方案后才进入金额实施；主智能体负责接口、阶段集成和交付；核心模型/算术及只读规划请求Sol/ultra，缓存/投影请求Sol/high，独立审查请求Sol/high，沿用可用角色及实际并发上限，不递归派工。以上均为请求值，工具未暴露实际模型配置。核心model与Redis文件分别归对应实现者，controller/UI在数值接口固定后串行处理；共享迁移/模型文件由主智能体集成，测试者不并发改同一实现文件，独立审查者只读。该分工不表示执行代理已启动。

验证只使用合成环境与新的私有可写副本，原始封存源保持只读；每阶段先预估新副本/三库磁盘和进程资源，有界运行并回收自有测试进程，不启后台自动化。容器、真实数据库及生产操作各遵循实际授权，现阶段不启动服务。出现资源不足或验证目标不明时保留成果、停止依赖操作，不借用未知外部DSN。

| 阶段 | 文件/模块范围 | 必须证明的结果 |
| --- | --- | --- |
| 数据及政策核清 | 私有snapshot报告、现有核查记录 | 504未知事件、用户累计差额、历史写入者/cache及额度性质各有证据或明确禁止操作的未知项；形成实际差额可执行资格 |
| 数值合同与三库模型 | common金额检查边界、user/token/channel/subscription、quota_mutation snapshots、user_quota_mutation receipt before/after、user_usage_policy及迁移入口 | 收费仍int32，余额/累计精确；普通累计跨int32、历史宽值、小额加减及边界拒绝；三库不收窄；GOARCH=386编译与定向测试 |
| 应用所有写入路径 | quota_reserve/context、legacy_usage_extension、settlement/refund facts、account_quota_mutation、quota_mutation/settle、user_quota_mutation、admin_quota_adjustment、token；topup/redemption/checkin/credit_edge/subscription及对应controller/service | 由目的列反查消费、退款、充值、兑换、签到、奖励、转入和购买的全部写点；原identity/receipt/CAS不变；旧宽余额下后续正常小额请求可工作 |
| 统计投影与计数 | user/channel/utils、usedata/usedata_flow及dashboard接口 | 金额类型/逐项聚合/JSON精确，失败可观察；明确仍为尽力更新，不用于账务守恒；计数合同不变 |
| 缓存/代次 | quota_reserve、quota_batch_drain、quota_projection、user/token/auth cache、utils | Lua原生精确边界、输入/类型错误先于余额写入、运行时部分写入保持未知、宽聚合、原代次重放/补偿、未知隔离、版本超过2^53仍正确比较 |
| 接口/界面/传递 | controller token/user/playground/subscription、user_cache、relay/common/relay_info及context、receipt snapshot JSON、web/src/lib/format.ts与users/keys/wallet/playground/订阅types/forms | exact JSON往返、派生和超M有明确行为、Key调整幂等并发、编辑旧值保真、32位日志/通知格式化保真、无浮点写回；旧receipt/fingerprint不重写 |
| 应用级副本恢复 | 新的私有可写副本、实际Go结算入口及隔离测试环境 | 原身份逐腿、部分完成重开、并发、重复零追加、选定差额守恒、冻结集合不变、退款方向；不是Python SQL替代Go |
| 独立验收/回滚 | 独立代码及结果审查、同点回滚演练 | 三库与必要原生Redis证据、完整结果/未解决项、数据库/缓存/义务一致回退；原始封存源摘要不变 |

具体数值反例至少包含：原宽正余额小扣、按获审政策从正宽经过int32到负宽、正常累计跨int32、M边界±1、退款used不足、两个M内字段合计超M/多行超int64、管理员override内部差额超过int32/超过M、batch宽聚合、v3未完与v4不重解释、超过2^53的相邻版本、事务第二腿失败、缓存第一写后错误风险、Key元数据与消费并发/调整重试、订阅溢出不得夹0、单次费用最小负值取反保护、相同身份异参数及已完成重放。金额计算不得制造负收费、无意赠额或重复扣款。32位验证不只构建根模块，需覆盖宽状态/payload/receipt/cache DTO及格式化的定向行为。

## 依赖与停止条件

尚未收到额度性质说明、beta.3一致性备份路径及历史Redis/多写入节点证据。503个prepared journal及另1个缺journal事件没有日志旁证；8306个有日志事件也没有独立余额/缓存事务证明。用户累计差额未解释。这些条件阻止实际恢复资格判定，不能把日志相等写成全账守恒。

外部DSN测试此前被自动审批拒绝，已改用本地安全验证；申请全新隔离MySQL 5.7/PostgreSQL 9.6服务仍待答复，不重试未知DSN。本方案还要求必要的原生Redis合同验收，现无此验证结果；合成cache测试不能代替全部Lua生产语义。Git作者身份缺失，不改Git配置、不伪造身份；本地候选按完整补丁归档，未推送。

任何范围/政策未定、来源不精确、身份冲突、缓存结果未知、数据库无法表示或必需验证缺失，都保留原值及义务并停止相关写入；独立可推进的分析继续。方案审阅及实现授权不等于实际补扣、发布或部署授权。完整任务未满足全部应用、账务和回滚验收前，不建议换镜像，也不发布beta.11。
