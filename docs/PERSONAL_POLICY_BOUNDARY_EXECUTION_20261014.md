# 下一片：个人准入策略与商业资金依赖边界

承接已经批准的个人核心卡步骤3及用户“继续下一版本”授权。当前未知用量跨重开组合准确 CI
通过后再写生产代码；现有 API 的 model/service 隔离测试可先在本地准备，暂不推送或执行重型验证。
这一片与部署的数据范围选择、冻结历史账务独立。

目标：个人无钱包准入的决定可独立构建和验证；原 model 事务只读取已有权威状态并映射原错误，
运行证据证明 `NewBillingSession` admission/pre-consume 不会访问被守卫的商业表，同时有限 Key 仍扣减。现有 FundingSource 与
BillingSettler 已足够，继续复用，不新建资金生命周期接口。

## 实施与所有权

1. 沿用独立 `codex/personal-restart-20261014` 与 Draft PR #13 追加下一阶段，保留 `eed63b5` 首轮失败和 `068a012` 修订提交及各自验收记录；PR #12 部署 head 固定。
2. 实现者 A 独占新增 pkg/personalusage 的纯策略及表驱动测试、model/user_usage_policy.go、
   service/self_use_request.go；仅提取已有个人策略和请求资格，不修改 schema、writer、receipt、退款。
3. 实现者 B 独占 service/personal_usage_boundary_test.go 与 model/personal_usage_boundary_test.go；
   两 writer、wallet=0、subscription_only、有限 Key 下用真实 SQLite 模型/GORM，对 `user_subscriptions`、
   `subscription_plans`、`subscription_pre_consume_records`、`subscription_orders`、`top_ups` 的
   Query/Create/Update/Delete/Row/Raw 回调注入错误并计数（Raw 如实际SQL带表名也守卫）。
   用合成正对照证明回调生效，后续本次 admission/pre-consume 命中0、source=SelfUse、Key100→40/used60、钱包0。
   model 必需回归：对 funding-state options 观察/失败注入，证明 no_balance=false 与 revision<=0 都0次读取、
   有效策略只读 authoritative state key 一次（loader 的 projection/fallback 可额外SQL）、storage error原样 errors.Is，
   invalid state精确映射原错误；不把一回 loader 等同一条SQL。
4. 非实现者复核纯策略依赖和原调用事务，主代理集成、文档、正常推送现有独立 Draft PR #13；
   所有行为/race/三库回归继续 Actions，不安装依赖、产生本地 DB 副本或编译缓存。

并发最多四槽，不递归。复用当前 Goal，文档记录阶段；没有另设时间或 token 预算。

## 策略合同

`EvaluateAdmission` 返回既有策略的领域决定：ConfiguredFunding、NeedFundingState、SelfUseFunding、
InvalidPolicy、FundingUnavailable。输入只有 no_balance、revision、可空最小 FundingState{Valid,Disabled}。
no_balance=false 立即返回 ConfiguredFunding，既不能因 revision 无效失败，也不新增持久状态查询；
no_balance=true 且 revision<=0 在读取资金状态前拒绝；否则缺状态才请求读取，invalid 拒绝，
只有 valid+disabled 才选择 SelfUse。valid enabled/retirement 保持原配置资金方式。
model 在原有 admission 事务/调用位置按 NeedFundingState 调用一次 `readUserFundingStateTx(tx,false)` 并重评，
保留 loader 缺行/legacy projection fallback 和原 storage error；InvalidPolicy→ErrUserUsagePolicyInvalid、
FundingUnavailable→ErrUserFundingUnavailable；NeedFundingState 不越过 model adapter。
ClaimLegacyUsageRequest、selectAccountBillingSource 中原 user→token 校验/锁后的位置不动；
不读 runtime 缓存，不改变 legacy Claim 和 authoritative Reserve 的事务/锁/冻结字段。

`SupportsMeteredRequest(method,path,rawQuery)` 只复用现有空 rawQuery 的 POST Chat、Responses、compact；
service 公开函数保留 nil request/URL 检查并薄适配。GET models 特例留在 middleware，
个人资格与严格预算资格继续不同，不新增任何模型或端点许可。

## 验收、风险与停止条件

- 最小纯策略表驱动验证覆盖配置资金免查询、无效 revision、缺/无效状态、disabled/enabled/retirement；
  请求资格覆盖现有三个端点、其他 method/path/rawQuery 和 service nil 边界。
- 依赖合同只约束新的纯策略包生产依赖为标准库；不声称这已拆除整个 model/service 的商业依赖。
  依赖检查只针对非测试生产文件，使用 go list 的 Standard 元数据或等价权威信息，
  不靠导入路径字符串启发，不绑定源文件名字或内部常量值。
- 商业故障 canary 仅声明 SQLite 的 admission/pre-consume 边界，有确定错误注入与精确数值；
  现有三库合同不冒充新 canary 三库执行。复用既有 self-use settle/cancel/unknown/到限/策略后改、
  订阅错误 fail-closed、三库 R1UsageReview/TokenBudget，不重复堆测试。
- 交易、数据表示和 API/UI 行为保持；只做结构边界时沿用已准备的未发布候选版本，准确 SHA 标识批次。
- 独审发现语义改变直接修正，Actions 首个实际失败保留，不用删断言或改变账务行为造绿灯。
  若必须修改身份/schema/旧记录/协议范围，停止该变化并另立补充合同，独立工作继续。
- GitHub 正常分支/Draft PR 同步已授权；main 合并、公开发布、生产历史变更不从该片推导。

## 自行复核

每个步骤对应独立策略边界或运行故障隔离证据；没有重复建资金接口，也不把同文件位置当作全部耦合。
前置条件是 PR #13 准确新组合 CI 成功；原事务和缓存边界可从现有代码直接保持。
纯包依赖保护只覆盖提取后的策略，商业表 canary 补运行边界；二者适用范围分别写明。
无 UI、schema、生产或额外临时数据库服务变更；可回滚源码提取，不触碰历史副本。

非实现者 deploy_boundary_20261014 只读审查要求的 loader/错误顺序、model免查询行为和canary声明范围，已补入以上合同。

并行准备修订：修订恢复 CI 在途时，仅允许 B 写上述两个现有API测试文件；生产策略文件由 A 等准确 CI 通过后写。测试准备不移动远端 head，不影响正在执行的源码身份。

## 准备检查点

恢复组合修订 head `068a012e98b92023b519a5fdea20d12c0e53ec28`，tree `379691ff4576f7a521a7e3ac56f7e34aa56a8783`；
[CI 38436481872](https://github.com/ForceMind/MyAPI/actions/runs/38436481872) 已后端失败：真实渠道归因缺口正在本片之前修复；未开始个人策略生产提取。
实际合并检出 `b09884271da4debb843423e58616144bfed1af80` 已回读同一 tree。
部署候选 PR #12 固定 `0000e865`，Docker 38430541538五项 success；生产未部署，数据范围选择待答。

分工：主代理文档/CI/Git；next_core_plan_20261014（派工Sol/high）只写两个隔离测试；
生产策略实现后续由非该测试作者承担；deploy_boundary_20261014（派工Sol/ultra）独立审查。
使用当前Goal，不创建新目标或后台自动化；无用户指定预算。

状态：两个隔离测试已完成静态复审（已补revision负数及先撤guard再Refund），行为待验证；纯策略/adapter未开始；部署选择待答。
当前实际渠道修复按AGENTS准备beta.12源码元数据，此片之后沿用该候选版本；没有创建标签或Release。
