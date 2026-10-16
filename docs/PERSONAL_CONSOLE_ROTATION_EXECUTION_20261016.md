# 个人控制台原 ID 单值轮换：续批执行记录

## 批次目标与现状

承接已验收 beta.18、分支 `codex/subscription-key-share-20261014`、归档 `4c21bfba5448b40d8d3a78ee7e931c5da0be2658`。维护者回复“继续”，沿用原Token ID单值轮换、接受短暂中断及证明前不开放提交的决定。原M、柔和蓝和visual-v0.2保持，不再次确认视觉。

现状：认证即使命中Redis仍按主库当前Key核对；风险在认证后的旧secret快照、legacy的HMAC缓存地址及持久batch locator。现有inflight计数仅进程内、非逐Token，不能证明集群已无在途。上一批迁移、unknown恢复及原图保留，不从main重置。

本批采用有界资格：先证明现有authoritative持久结算模式的TokenID连续性，合格配置才允许轮换；legacy/bridge、旧batch/usage/投影等义务未终态或未知仍阻断。不自动切换writer模式，不清队列、不补余额、不更改金额算法或历史账务。保留Key owner、ID、预算、分额、模型/IP/路由配置。

## 实施顺序与验收

1. 只读核对sync/stream/realtime、重试/故障转移、free/按次及异步入口；不能用核心账本按ID的事实覆盖仍依赖旧secret的边界。
2. 增加凭据版本和实际发送前的版本复核；Realtime轮换后禁止旧连接继续发送新帧。已经发送的合格authoritative工作仍按原TokenID收尾，不猜用量、不强制失败成功回答。
3. 实现DB-only轮换预检、Root本人真实会话、专用二次验证、CAS/幂等、不可变脱敏回执和原ID的单值替换；回执/重放不含secret，首次成功新值仅内存展示。无法确认的提交只回读原operation，不自动重发或回滚。
4. 沿用维护卡与Dialog视觉，覆盖阻断/就绪/确认/验证/成功一次性显示/未知恢复；同tab持久记录仅非秘密ID，actor/Token变更或关闭清理秘密。
5. 先缓存定向检查，再现有Actions完整构建、类型/单元、三库/Redis/race、实际Go/浏览器与桌面/手机原图；非实现者审查后同步现有分支与Draft PR #17。候选beta.19，未发布。

验收必须证明旧Key新请求拒绝、新Key同ID可用、已受理合格结算连续、并发与重复操作唯一换值、未知恢复无重复、配置/历史不变和unsupported模式零轮换。证据不能齐备时不开放对应模式，不以按钮可点击作为资格。

## Goal、所有权与资源

新持久Goal承载此批，未指定预算。最多4角色、无递归；实现与审查交叉，重型验证优先现有Actions。本机不安装node_modules/Chromium，不运行生产应用自动迁移。

| 角色 | 配置 | 文件所有权 |
| --- | --- | --- |
| credential_plan | gpt-5.6-sol / ultra | 只读总体规划后，model/token_credential_rotation*.go、专属测试及token.go凭据版本字段；共享模型改动先定接口 |
| credential_guard | gpt-5.6-sol / high | ingress、重试/dispatch、Realtime版本复核与相应测试；不争写模型 |
| credential_ui | gpt-5.6-sol / high | 新轮换API/UI/pending helper/测试、现有轮换卡及父Dialog必要小改；后续明确接管全部locale，只经受控脚本写入并sync |
| 主线程 | 当前主会话模型 | 方案/集成、controller/service/routes/proof、schema注册、版本、CI、隔离运行、文档和同步；不与UI角色争写locale |

共享接口在写入前冻结。权限、JSON wrapper、三库、独立relaykit及账务安全约束继续适用。前端沿用shadcn-ui、React最佳实践及i18n技能；locale仅受控脚本写入并sync。cloud runtime技能已读取，当前environment_status工具与network-policy文件不可用，不据此推断网络权限，远端仍按已有权限流程。

## 冻结范围与停止条件

生产数据/Redis/真实凭据/备份及历史账务冻结；不合并main、不标签、不发布、不部署。用户已有MYAPI_MASTER_PLAN、NEXT_SESSION_HANDOFF和设计草稿保持未提交。既有PR base保持codex/personal-core-20261014。

每步直接解决轮换资格或闭环，复用现有Root会话、安全proof、维护回执及异步按ID结算；不扩大到控制台Access Token、上游凭据轮换或账务模式切换。代码证据出现新的实质架构风险时先记录并限制资格，独立工作继续。当前阶段：实现与隔离资格验证；实际应用尚未注册轮换提交接口。

## 合同冻结与并行实现

模型增加CredentialVersion（bigint/default1，0/负/溢出阻断）；发送前读取成功是该次发送的逻辑接纳点，不保证提交后物理召回已接纳网络请求。HTTP每attempt/shared transport、WSS建连与每client工作帧、task/MJ/video直发均须覆盖；target回包和已发送义务按ID收尾。Root已落新controller/service与独立route registrar，但不接入SetApiRouter；先完成隔离兼容证明，之后才激活实际路由及maintenance资格。

三路接口：POST credential-rotation/preview（严格{}）；POST credential-rotation（op64hex/revision64hex/confirmed true，token.credential_rotation专用proof）；GET credential-rotation/operations/:op。fresh返回receipt/replayed=false/reveal.secret，重放与GET无secret。未知只回读，不自动重发。新值自动展示仅一次，但既有显式Reveal Key能力保留；不能误称回执丢失必须再次轮换。

独立route chain在UserAuth前标记not_accepted，进入model前改unknown；明确预拒绝才回not_accepted。HTTP超时/5xx/无可靠标记仍保守unknown，避免过期会话/Origin拒绝形成永久404写锁，又不将postcommit错误误称零写。

追加待证实边界：扫描旧legacy task终态与并发“已通过version检查、尚未登记Task”的竞态。authoritative仍可因无Idempotency或feature flag进入legacy async，不能从当前Task为空推断没有即将到来的旧义务。模型/guard交叉核查同Token锁、持久admission或更保守门槛；未证明前实际submit保持关闭。


## 资格审查修正与验证阶段

- legacy async 即使免费或未启用durable submission，也必须在authoritative模式先持久登记dispatch，再发送。登记和轮换共用User→Token锁，任务落库后按不可变身份链接；失败/未知不自动过期释放。仅新建登记授权一次发送；既有open崩溃重入返回unknown，同一request不再次调用provider。已创建登记的发送失败不进入旧重试，legacy/bridge既有重试不变。
- MJ的旧TokenId是可变计费/退款标记；新增仅供新任务的DispatchTokenID记录提交身份，不能改写旧行冒充已证明。旧活跃任务无身份时继续unknown。
- Task/MJ终态必须status与progress一致；不以AND筛选漏掉IN_PROGRESS+100%或SUCCESS+50%。durable operation终态与Task也须一致。
- Realtime拒绝旧连接新帧，保留target reader完成已接纳响应与usage；终态向client实际转发成功才可结束。超时/取消/协议不完整保留unknown，不填0。对应新竞态由非实现者指出并定向修正。
- 凭据版本在浏览器JSON精确整数范围内；达到上界仍可使用当前Key，但后续轮换阻断，不转换或重写存量版本。
- 新增专用controller/router链与strict JSON、disposition、fresh/replay/GET无secret合同，UI/API/pending相关21个测试及四个合成浏览器旅程；实际应用路由在完整隔离资格通过前不注册。新的真实Go/Chromium两设备driver已准备，只在第二阶段激活后运行。

旧归档4c21bfb的CI38915021533中10项作业成功，后端race失败：TokenMaintenance路由权限测试结束后全局DB恢复，与后台admin audit读取DB竞争。该测试只验证授权，采用仓库已有ContextKeyAuditLogged测试夹具约定隔离后台审计；生产审计不变，不删除权限/Origin/proof断言。本批完整CI重新证明，不能把旧归档报告为全绿。


### 第一轮资格候选（路由仍关闭）

模型终审修正：accepted MJ code1/21/22归一稳定身份，21/22轮询变1不改变提交身份；code23/24确定拒绝按官方midjourney-proxy API合同分别绑定，不给其它结果猜终态。open登记缺少所有者存活证明，资格如实unknown。回执和登记同时纳入现有GORM表写保护，Model/Table/alias/UpdateColumn/Delete绕过均拒绝；raw SQL保持特权迁移/测试边界，不承诺数据库权限级防篡改。来源：https://raw.githubusercontent.com/novicezk/midjourney-proxy/main/docs/api.md。

最终本机定向：模型非Redis7.216s、miniredis1.108s、版本JSON边界1.442s；Realtime6项0.041s；relay/service增量0.020s/1.098s；controller异步重试0.076s；最新controller/router0.026s/0.750s。Node语法、浏览器driver格式/lint、YAML与diff检查通过。MySQL5.7、PostgreSQL9.6、Redis7和完整前端/浏览器尚待Actions，不以本机SQLite/miniredis代替。

采用逻辑接纳合同的独立发送路径复审通过：每次外层generation failover均重验，未发现生产provider内部新增generation retry；HTTP/2同Do透明重试、Coze已接纳任务轮询和转换上传属于原attempt。非free authoritative持有BillingSession/receipt按ID收尾；真正free的quota delta为0。成功DB版本读取至物理发送之间的窗口保留为明确限制，不承诺撤回已接纳网络工作。混合旧版本worker不属于当前隔离资格证明，未来部署须先统一兼容版本；本批不部署。


### 第一轮Actions失败与修正

资格源码c8cf0a2430dc3ecd1e797f7ce7e054e0b8ed0418，CI38924455597，merge checkout6fd9eea3c86ac7ac52a7fb07b01b7f800bfbecfb（branch tree5f062c2566345549b1669f512d366cc4e92e3bb2）。实际应用仍不注册rotation。

- Redis7四合同实际通过（0.47s）：old/new fence/cache、subject lock拒绝零写、precommit cache失败零写、postcommit cache失败unknown与secret-free恢复。
- B2新三库SQLite/MySQL通过；PG9.6在Complete回读open admission时Invalid/Unknown。可为空的LinkedIdentityHash原用CHAR(64)，PG按定长类型补空格；改VARCHAR(64)，不Trim、不放宽linked摘要。configured直接从DB回读精确空串并再次完整校验，下一CI实际PG回归才算关闭。依据：[PostgreSQL9.6 Character Types](https://www.postgresql.org/docs/9.6/datatype-character.html)。本机修复后模型定向7.751s通过，仅SQLite/miniredis，不外推PG。
- 新轮换浏览器已完成desktop合成fresh commit，但New Key value模糊label同时匹配保存复选框而失败。两个新driver采用exact label，完整value/clear断言保留。实际原PNG还显示mask隐藏checkbox造成遮标题黑条；capture改为仅遮可见非checkbox/radio/hidden的文本/密码输入和textarea，旧PNG不编辑，CI重新拍摄。失败报告统一遮敏，成功/未知恢复断言不放宽。
- 两新driver项目oxfmt、oxlint、node语法、diff检查通过。完整前端/其它后端回归尚在原运行中，不宣称全绿；下一资格CI继续，入口保持关闭。


### 第二轮Actions结果与第三轮修正

资格源码9dd92c89392c99cc58278fe29a8daa56720f3bee，CI38925857491，merge checkout7e66c8d40210b89456e255efa909653011e2b1cb。空摘要故障已由PG真实回读/Complete证明修复；PG继续执行真实commit时暴露commonKeyCol依赖全局方言，configured PG拿到反引号。两处条件改为GORM map，由连接方言引用key；Unscoped全局去重和id/user_id/旧key/version四字段CAS、RowsAffected1保持。真实PG最终合同仍等下一轮。

浏览器desktop fresh/show/hide/copy/关闭清除与390读取失败重试已通过；320 unknown在刷新后共4次GET而脚本预计3。来源是remount后台恢复GET及用户首次打开恢复弹窗的显式GET，receiptInFlight禁止并发，所有原op、preview/commit各1；脚本精确改为4并解释触发，不放宽为范围值。原图P2：BaseUI透明native input仍被:visible误判，遮罩仅保留本流程新Key/2FA/password/textarea；320越南语长标题增加局部关闭按钮保留区。旧原图不改，下一CI重拍。

额外收紧MySQL已支持配置的重放：clientFoundRows=true可让noop UPDATE报告matched行，因此Begin不再按OnConflict.RowsAffected判新建；同Token锁下回读持久admission，只有其AdmissionID匹配本次CSPRNG且state=open才授权发送。configured普通MySQL保留，另从安全DSN创建foundRows连接，验证旧open只回同row+PreconditionUnknown、总计一行。定向模型8.138s通过（SQLite/miniredis），真实foundRows等下一CI；无生产连接。

当前资格和实际路由仍分开。第三轮继续验证全量回归，不因先前两个数据库、部分页面或本机测试通过而提前开放提交。
