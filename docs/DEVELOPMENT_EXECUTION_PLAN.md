# MyAPI 开发执行计划

> 2026-09-04 代码验收基线：`2d6acab`（CI `33814136556` 七项成功）；最新适用镜像证据仍为 `237c0da` 的 Docker smoke `33790513455`，Full/LAN 均成功。本文件管理完整路线与当前授权；历史证据保留在
> [完成度审计](COMPLETION_AUDIT.md)，产品定义见[主计划](MYAPI_MASTER_PLAN.md)。
> **S0＋S1 及 S1-R1、S2-A 六项及 S2-A-R1** 已完成当前确认范围。目标及风险边界内的实现、测试修正、审查修复、复验和阶段同步持续推进，不把每次小修拆为新的确认；核心架构/业务取舍、范围扩大与发布仍单独确认。

## 目标、保留能力与完成规则

面向 Full 管理员/普通用户、LAN Lite 同事和桌面用户，完成可靠的调用、计费、日志、
额度观测、配置、安装升级与独立产品体验。保留现有 Codex/Chat/Responses/Claude、附件、
SSE、脱敏日志、额度分析、账户/Key 兼容字段、CLI/Electron 和发行合同，不整体重写。

状态使用 `未开始`、`进行中`、`待验证`、`审查未通过`、`已完成`；阻塞原因与恢复条件单列。
每项必须有责任、范围、验收、证据 SHA/命令/结果与未覆盖风险。CI、SQL DryRun、合成
浏览器 fixture、历史镜像分别标记，不能互相替代，更不能充当真实设备或生产验收。
完成意味着确认范围内的需求逐项满足、已知阻断缺陷清零、适用测试和独立审查通过；
未获授权/缺外部条件的事项保留，不用省略范围或增加无关清理来制造完成度。

## 完整路线与依赖

| 阶段 | 状态/授权 | 责任 | 交付与退出条件 |
| --- | --- | --- | --- |
| S0 计划与证据 | 已完成／已确认范围 | terra 起草，主代理整合 | 需求、缺陷、验证、决策分开；更新主计划、审计、macOS、额度 OpenAPI；链接、参数和事实一致。 |
| S1 安全与一致性 | 已完成／已确认范围 | sol 实现；独立 sol 复审 | 原六项及追加 R1 均通过回归、独立复审和 CI；同进程配置保存/后台重载的发布顺序、双写交错与失败释放已有证据，不推导跨实例一致性。 |
| S2 核心业务合同 | 进行中／已验收项见下表 | sol 实现与独立复审，terra 回归矩阵 | A01–A06/R1、B1、C01/C02/C03a/C04/C05/C06/C07/C08、D01–D10 已完成当前范围并通过同提交 CI；此前批次 CI 见各自记录。B2/B3 任务持久化/恢复、C03b 缓存恢复与 C09 实施仍未完成。完整验证仍包括请求→预扣→上游→结算/退款→日志与 Provider 边界，relaykit 独立。 |
| S3 账户/Key 实际策略 | 未开始／迁移设计待确认 | sol 设计，分模块实现 | 明确账户权益、模型限制交集、route_groups、disabled、fallback、计费归属；旧 Key 兼容/差异报告/启用/回滚经确认后落实。不得把已有元数据当强制策略。 |
| S4 运行与恢复 | 进行中／S4-01、S4-02 已完成当前范围 | 工程/制品与独立 sol 审查；sol 数据与恢复 | Full/LAN 测试镜像均不推送；SQLite/MySQL/PostgreSQL 临时库旧结构升级、重复迁移、多连接竞争、备份恢复；登录/Key/权限/日志/额度探针；原生桌面构建、安装升级与第二设备 LAN。 |
| S5 额度闭环与选定扩展 | 未开始／各扩展分别决策 | sol 领域/安全，terra 常规实现 | 测试实例真实账户连续采样、任务/API/管理员页面一致；失败/重置/缺口和大数据量性能。通知、多 Key 身份、Claude 组织用量等按决策登记，不混入平台账本。 |
| S6 完整独立 UI | 未开始／设计待确认 | sol 信息架构/复杂交互，terra 页面实施；独立视觉审查 | 角色化 IA→设计系统→登录/引导→Key/调用→渠道/额度→日志→计费/运维；Full/LAN/移动、七语言、键盘、空/错/加载/权限状态；实际渲染/操作验收，官网同步。 |
| S7 交付就绪 | 未开始／发布不在当前授权内 | 主代理、独立审查者、负责人 | 逐需求证据、已知阻断清零、恢复可执行、文档/接口/制品一致；版本、签名、NOTICE、域名及正式发布另行审批。 |

**2026-09-04 额度概览图表修复（进行中）：** 主计划要求概览默认显示 Codex
可用额度折线，但实际入口继承了通用详情的“消耗柱图”默认值。本批仅向
概览入口注入 `available` 初始指标，保留渠道详情原有消耗默认和手动切换。
失败回归已证明旧实现仍渲染柱图，修复后定向 Vitest 22/22、`build:check`
通过；本机浏览器脚本因未安装隔离 Playwright 模块未执行，由本次精确 SHA
的 GitHub Frontend job 补验。发布版本仍为受保护的 `0.1.1`，页面运行时构建
标识由 commit SHA 区分本批。

S0/S1 先行；S1 通过后的 S2、S3 设计和 S4 独立准备可并行，重型构建不得并行占满主机。
S6 启动闸门是核心缺陷关闭、API/权限/数据模型稳定、Full/LAN 测试实例可运行、外部
验收责任与条件明确；不是等待正式发布或所有真实设备先完成。完整 UI 是必做后置工作，
不是把现有品牌/局部面板再次标成全量 UI。

## S1 缺陷与最低验收矩阵

| ID | 优先级/状态 | 责任与范围 | 最低验收 |
| --- | --- | --- | --- |
| S1-01 显式身份被回填覆盖 | P1／已完成 | sol：`model/access_profile.go`、启动迁移 | 首次缺列/部分缺列、失败重试、空值补齐、重复启动；显式 standard/custom 保留，旧 group 不改。不能猜测或自动恢复过去已被覆盖的 ID。 |
| S1-02 Zhipu 流生命周期 | P1／已完成 | 主代理 sol：`relay/channel/zhipu` | 内容与尾 metadata 有序；空/缺 usage/解析失败/scanner 错误不得成功；取消/写失败关闭上游并释放生产者；仅正常完成发送 DONE。 |
| S1-03 Zhipu 凭据日志 | P1／已完成 | 主代理 sol：同上 | 错误格式/空段/无效边界拒绝构造认证；不写原始 key/JWT/secret；合法签名与缓存兼容。测试仅合成秘密。 |
| S1-04 注册表锁绕过 | P2／已完成 | sol：`model/option.go`、`setting/access_profile.go`、配置管理器 | 单一同步发布，读取/导出为受保护快照；真实 UpdateOption 并发读写通过 race，不能只测专用 setter。 |
| S1-05 配置持久化误报成功 | P2／已完成 | sol：Option 模型与控制器验收 | 新建/已有配置写失败返回错误；事务回滚且内存不发布；成功后数据库、缓存与导出一致。 |
| S1-06 ID 规范化不一致 | P2／已完成 | sol：profile 注册表 | key/fallback 存储与查询一致；规范化碰撞拒绝；未知引用/循环继续拒绝，合法禁用值和切片不被外部修改。 |

上述修复不启用 Profile 路由强制，不新增 Provider 协议，不改变额度观测与平台计费的隔离。

复审追加项 **S1-R1（P2，已完成／已确认范围）**：两个配置写请求，以及后台重载取得旧
快照后再发布，均可能造成数据库与当前进程状态不同。范围覆盖同进程 UpdateOption /
UpdateOptionsBulk 的事务提交到内存发布、loadOptionsFromDatabase / SyncOptions 的
读取到发布，以及初始化的顺序边界；避免递归加锁，内存读锁不跨数据库 I/O。

验收使用确定性双写、写与重载交错、数据库读/写失败释放测试及相关 race / 回归。
本项不保证跨实例一致性、不承诺所有配置读取的全局原子快照、不扩展 S2-S7，且不改变
原六项已完成状态。修复提交 `8dfcfba` 通过本机定向/全量/race、独立复审及 CI
`33743669737` 五个 job（含新增 R1 race 步骤），本阶段可标为完成。

失败边界：普通后台重载查询失败时不发布该次部分结果，保留现值；初始化仍按既有逻辑
先构造默认配置，失败时不保证调用前 Map 原样保留。当前生产没有 ConfigManager.SaveToDB
回调保存入口；未来增加此类入口需复核锁顺序，不由 R1 自动覆盖。

## S2-A 支付与订阅事务

开始基线 `c82d0f1` / CI `33744326429` 五项通过，旧绿灯不是本轮修改的验收证据。
交付 `2777021` / CI `33749764180` 六项 success，本机完整 Go/race/relaykit/发行合同
及独立静态复审通过；该次原始 MySQL 实库日志暴露中文日志插入错误，未予验收。
追加 R1 `767b17b` / [CI 33751536908](https://github.com/ForceMind/MyAPI/actions/runs/33751536908)
六项成功，MySQL5.7/PostgreSQL9.6 各七场景未 skip，精确日志数量、完整中文内容与历史
日志不变断言通过，原始输出无 `Error 1366` 或日志写入失败。S2-A 现已完成当前确认范围。

| ID | 状态 | 范围 | 最低验收 |
| --- | --- | --- | --- |
| A01 | 已完成 | 订阅预扣退款及 request ID 幂等记录同一事务 | 首次/重复退款；保存失败时额度与退款标记一起回滚。 |
| A02 | 已完成 | 订阅事务内套餐查询与数据库时钟使用同一连接 | 冷/热缓存与单连接完成；不旁路事务或发布脏缓存；DB 查询故障不伪装“无订阅”。 |
| A03 | 已完成 | Creem 手动补单使用最终 quota 单位 | 五种 provider 正常/手动入账额度一致；重复无第二笔日志；非法数量/溢出/钱包上限回滚。 |
| A04 | 已完成 | Stripe/Pancake 临时 DB 失败返回可重试 HTTP 错误 | 查询/写入/提交失败不得 ACK 成功；恢复重送一次入账，真正缺单/身份拒绝保持既有行为。 |
| A05 | 已完成 | Stripe failed 通知的数据库 pending 条件更新 | 陈旧读取及竞争通知不能覆盖 success；只修改 status，不保存旧整行。 |
| A06 | 已完成 | Stripe/Creem 仅订阅配置的 callback 可用性 | 无充值商品时签名订阅事件可履约；充值入口仍关闭；缺凭据/密钥/合规确认继续拒绝。 |

**S2-A-R1：已完成／已确认**。原 CI 专库的 DSN 编码并未配置 schema 默认字符集，中文
日志插入报 `Error 1366`；旧测试没断言日志，导致绿色不能支撑完整验收。
仅调整 `model/payment_database_test.go`：在严格 loopback/专名/空库检查后配置 utf8mb4，
迁移前后调用既有启动校验，补精确日志条数/中文内容及重复/回滚断言。实库 CI 已复验，
本机 model 全量/vet、SQLite 七场景与专项 race、relaykit 独立构建、Node22 release:check 通过。
不修改生产 schema、业务行为或放宽现有检查，详细证据见[完成度审计](COMPLETION_AUDIT.md)。

本批无生产 schema 迁移；R1 的 DDL 仅配置已确认空的测试专库。验证包括临时 SQLite 故障注入、签名 HTTP fixture、独立复审及
CI 专属 `myapi_s2a_test` MySQL5.7/PostgreSQL9.6 单/多连接测试；不复用 S1 实库证据。
本机新 SQLite 入口复用七场景，但后三项明确是顺序重放，不模拟 MySQL/PG 行锁。
Pancake 使用已验签的合成事件验证履约/ACK，公开路由另测非法签名拒绝；没有真实
provider 私钥，不将此写为完整网关签名或真实付款验收。提交故障 fixture 是明确回滚，
不是网络断开后的不确定提交恢复。真实账单、续费/争议与生产对账未验收。

后续范围：S2-B 的 B1 冻结任务计费快照已验收；响应前持久化与终态账务恢复仍未实施；S2-C 的已确证
数值/审计/缓存脚本/HTTP 隐私已完成所列范围，剩余恢复设计见下表。
需要新增领域或恢复策略的部分先设计确认，不用本轮支付修复替代完整路线。

## S2-C 数值、缓存与 HTTP 隐私

本轮保留正常计价/取整、验签原始字节、S2-A 回调重试与幂等、视频鉴权/SSRF 和生产
数据库结构。实现与复审分离；只操作本机 fixture 与一次性 CI 服务，不调用真实支付/上游。

交付 `6bae734`，收尾统计修正 `451bee3` / [CI 33760303619](https://github.com/ForceMind/MyAPI/actions/runs/33760303619)
七项成功；真实 Redis 7 的 36 场景实跑，无 skip/Lua 错误。根模块全量/vet/build、专项
race、relaykit 独立构建/测试、Node22 发行合同及干净源码 pack 已验证，独立复审闭环。

| ID | 状态 | 已确定写入边界 | 最低验收 |
| --- | --- | --- | --- |
| C01 | 已完成 | 文本结算 token 合计；`service/text_quota.go` 与回归 | `MaxInt+1` 不溢负后清零收费/退款；正常、零值、边界及真实结算/管理员审计。 |
| C02 | 已完成 | Kling 首次 clamp 经内部 TaskInfo 到结算日志；公共 AuditMap | 正常 ceil/倍率不变；极值、NaN/Inf、±1e309、缩小倍率、零差额仍保留首个审计；JSON 不丢 other，普通用户不见 admin_info。零差额为 System 审计，不增加消费 RPM/TPM/导出。 |
| C03a | 已完成 | 四个 quota Lua 脚本、预扣 Go 参数边界、非额度缓存刷新参数索引 | 写前验证所有额度字段/结果，不留下半次 HINCRBY；非法操作不从预扣入口回退 DB 绕过；正常欠费/退款保留。metadata 刷新不创建缺 Quota hash。 |
| C03b | 未开始／待决策 | 批量模式缺损缓存恢复及高层增减的 cache/DB 一致性 | 明确故障时新预扣是否 fail-closed、冷缓存/已丢缓存如何区分、未落库增量如何恢复；不默默以旧 DB 水合覆盖余额。 |
| C04 | 已完成 | 私有视频控制器及真实路由回归 | HTTP/data URL 与 handler 错误为 private/no-store，上游 CDN 缓存头不覆盖；owner/非 owner/匿名与 SSRF 不回退。匿名中间件响应不冒称具有 controller 的响应头。 |
| C05 | 已完成 | 四种支付控制器、必要的定向访问日志过滤及测试 | INFO/WARN/ERROR 和四个已匹配 webhook 的生产访问日志不输出原始正文/签名/query/客户资料；验签/ACK/幂等保持；Pancake 验证边界如实标注。 |
| C06 | 已完成／测试隔离、本机/CI race 与独立复审通过 | 生产 logger 计数/轮转预约状态及轮询测试共享对象 | 两个渠道并发记录日志无 data race；保留日志格式与轮转、多渠道并发；测试使用独立快照/通知，不用串行化或 sleep 掩盖。 |
| C07 | 已完成当前范围 | `common` 请求正文替换生命周期、Kling/Jimeng 兼容路由、Full Content 入口身份、Provider 模型/时长边界 | 所有正文读取路径同版本；原始客户端日志与转换后分发分离；`model_name`/`req_key`、Kling duration/mode、Jimeng frames 不得绕过已验证/已计费字段；内存/磁盘清理、race 与同提交 CI 通过。 |
| C08 | 已完成当前范围 | Settings JSON、持久化前验证、限流快照、倍率与配置发布 | 失败解析/DB 失败不改 runtime；负/非有限倍率拒绝；rate 单请求快照、溢出/内存/动态窗口回归；本地普通/race/全量及 `2d6acab` 同 SHA CI 通过。 |
| C09 | R3 已完成当前范围 | Settings 控制面并发与硬限额合同 | R1/R2 当前范围及同提交 CI 已完成；R3 `e3cd185` 同 SHA 八项 CI 完成 GroupRatio 私有快照、canonical/alias 兼容与写入回滚合同，独立最终复审无 P1/P2/P3；前端 bulk/跨多 HTTP 事务、历史 DB 清理、payment/Passkey/hard limit/global config 仍待。 |

C03a 不等于整个账务缓存一致性完成。现有缺损 hash 的 miss→hydrate/DB fallback 仍未改；
高层 User/Token quota 增减仍异步更新缓存、失败只记录而数据库继续写，须在 C03b 单列。
已向负责人询问批量模式故障时拒绝新预扣的取舍；在决定前不实施恢复策略。

本机 miniredis 用于先复现和回归；CI 真实 Redis 7 矩阵（36 场景）已通过。它要求
显式开关、字面 loopback、整个实例为空，仅在 DB 15 写测试键，不 FLUSH/删除；用
PEXPIRETIME 比较绝对过期时刻，不靠 sleep 或相对 TTL 的时间差判定。它不证明跨实例
丢失增量恢复。首次红测和独立复审发现/修正均记入[完成度审计](COMPLETION_AUDIT.md)。

**S2-C09-R1 已完成当前范围。** 最终 `ae07527` /
[CI 33824814509](https://github.com/ForceMind/MyAPI/actions/runs/33824814509) 八项成功；独立最终复审确认当前范围无 P1/P2。payment compliance 五字段
由单次 `UpdateOptionsBulk` 原子提交；SQLite 成功与保留旧值 rollback 已回归，同一 helper 已由 MySQL 5.7/
PostgreSQL 9.6 CI engine 流程执行。工具价格 source/index 以不可变同代际快照原子发布，公开 DTO 兼容保留，严格 `MapConfig` 与历史
宽松 loader 分离。`ConfigManager.SaveToDB` 完整快照后锁外 callback，覆盖重入及错误释放。Redis limiter 移除首 client
singleton；`redis.Script` 在 `NOSCRIPT` 后恢复；TTL 自然复满而不截断至 24 小时；Go 在触碰 Redis 前拒绝非正、
超过 2^53 与 `Requested > Capacity` 的参数，Lua 还在写前拒绝非整数，拒绝路径零写。`common/limiter` 导出并复用 `MaxExactInteger`/`ValidateConfig`；
Settings 默认及每个 group 在 DB 写入前按实际 `capacity=total*durationSeconds`、`rate=total`、`requested=durationSeconds`
使用同一 2^53-1 精确边界，覆盖大于 2^53 且不超过 MaxInt64 的拒绝，runtime/OptionMap 不发布；保留 `total=0` 和
disabled `duration=0`。miniredis 与独立真实 Redis 7 CI job 均通过。

**S2-C09-R2（已完成当前范围）：** 最终 `2575f5b` / [CI 33828024982](https://github.com/ForceMind/MyAPI/actions/runs/33828024982)
八项成功。Backend 原始日志确认改名后的 `Verify settings, rate-limit, and request snapshots` 12 包均为 `ok`，
包括 `setting/model_setting`、`operation_setting`、`relay/common`，不是 no tests；其余七个 job 成功。`ValidatingMapConfig` 纯验证，既有 `MapConfig` 保持
unsupported。Claude 使用私有 atomic 完整代；getter 深拷贝三层 map/slice、保留 `[]`/`null` 形状；`null`/`{}` 仅在读取
副本补 8192，不污染 export，严格失败不发布。`GenRelayInfo` 捕获 request-private Claude 代，handler/header/converter
同代。Monitor 使用私有 atomic 代，保留 env frequency 再 enabled 优先序；env 移除恢复 DB；mode/concurrency 只作用
effective，不污染 export；`runChannelTestTask` 一次快照，并发 partial 不丢。测试为单次屏障，无 sleep/概率循环。
独立 Sol 最终复审当前范围无 P1/P2；P3 是 `GlobalConfig.Get` 动态具体类型变私有 manager，公开 DTO/getter 签名兼容、
仓内无断言。

本机通过受影响包普通、workflow 同款 12 包 race（`common`、`common/limiter`、`types`、五个 setting 相关包、`model`、
`middleware`、`controller`、`relay/common`）、`go test -p 1 ./...`、vet、build、relaykit `GOWORK=off` vet/build/test、
gofmt、diff、YAML/JSON 门禁。R2 不需且未做真实 Redis、三数据库、前端、上游；无页面/schema，`VERSION` 0.1.1。
Passkey/ServerAddress、payment runtime/密钥轮换、`GroupRatioSetting` alias/前端 bulk、成功 hard limit、跨族事务仍待。
R1 文档收尾 `3af738e` / [CI 33825533002](https://github.com/ForceMind/MyAPI/actions/runs/33825533002) 八项成功。
R2 文档收尾 `3c63e02` / [CI 33828752473](https://github.com/ForceMind/MyAPI/actions/runs/33828752473) 八项成功。

**S2-C09-R3（已完成当前范围）：** 最终 `e3cd185` / [CI 33831492021](https://github.com/ForceMind/MyAPI/actions/runs/33831492021)
八项成功。原始 S1 日志确认 MySQL 5.7 与 PostgreSQL 9.6 均执行
`group-ratio-alias-contract/create-rollback/{mixed-existing-canonical-and-missing-alias,both-missing-second-create}`，全 PASS、无 skip；
Backend `Verify settings, rate-limit, and request snapshots` 13 包均为 `ok`，含 ratio/model/controller/service/relay-common。
GroupRatio 三图私有 writer 加 atomic 单快照、嵌套深拷贝；
detached 公开 DTO 保持三字段 unkeyed/JSON 兼容、receiver-local，NaN/Inf 导出错误传播；注册表动态类型改私有 manager，
公开 DTO/函数不变且仓内无生产类型断言。special 空 user/target 和 direct、
`+:`/`-:` 同目标冲突写前拒绝；service 走 detached special getter，`+`/`-` 语义不变。平面 `GroupRatio`/`GroupGroupRatio`
基于当前 UI 为 canonical，分层键兼容；新 JSON 规范化并事务双写，bulk 冲突写前拒绝，OptionMap/runtime 双键同值。历史加载有效
canonical 优先；invalid canonical fallback 有效 alias；双方 invalid 保留最后有效 runtime、warning、不改 DB。SQLite 覆盖反向
行序、alias-only/conflict、update/mixed/create rollback；同一合同接入 MySQL 5.7/PostgreSQL 9.6 gated 子测试，并由同提交 CI 通过。

独立 Sol 最终复审无 P1/P2/P3。本机通过 ratio/model/service/controller 普通、workflow 同款 13 包 race、
`go test -p 1 ./...`、vet、build、relaykit `GOWORK=off` vet/build/test、gofmt、diff、YAML/JSON 门禁。首次 service/controller
race 仅磁盘满链接失败；只清理 7.9GB 可重建 `/private/tmp/myapi-gocache` 后原命令通过，仓库/DB 未触碰。R3 不完成前端 bulk/
跨多 HTTP 事务、历史 DB 清理、payment/Passkey/hard limit/global config；无页面/schema，`VERSION` 0.1.1。

本机扩展 Settings/C09 同 CI race、根模块全量 test/vet/build、relaykit 独立 vet/build/test、格式、YAML 和
根 JSON 静态门禁均通过；同提交 CI 原始日志确认真实 Redis 7 的短/25 小时 TTL、拒绝与脚本恢复，S1
MySQL/PostgreSQL engine 以及 11 包扩展 race 均成功。

本轮明确不完成：成功限额仍 check→execute→record，未实现 reservation/rollback；总量仍令牌桶，不改产品语义；
generic 21 模块热读、跨族事务、Passkey/null/未知 key/`GroupRatioSetting` 审计仍待；payment runtime 逐字段/活指针
仍未解决；真实付款、生产、设备和发布未做。`VERSION` 保持 0.1.1。

C06 使用私有状态锁保护计数与轮转预约，不跨日志 I/O；只有自动轮转任务释放自己的
预约，手动 SetupLogger 不误清。logger 全包 race（2.595s）与全部 UpdateVideoTasks
关联 race（3.326s）通过，独立复审通过；真实临时文件轮转和并发输出已测，无百万循环。
轮询 fixture 不再读取后台 GORM 正在更新的同一 Task 指针，保留共同 500ms 非阻塞门限。

## S2-B 已有快照与任务持久化

**B1 已完成当前范围**：`7f1913e` / CI `33781560507` 七项成功，MySQL5.7/PG9.6 各五种快照与 NULL 读回实跑通过，原七支付场景保持通过。`e7fffc2` 曾因 PG JSON Valuer 被编码为 bytea 报 SQLSTATE 22P02，已以 JSON 文本传值修复；不改 schema/协议/空 Data，失败和本机 codec 红绿证据保留。复用已有 TaskBillingContext，不另造快照，使完整的提交时模型/实际分组/
附加倍率成为结算依据，避免配置变更后重新定价。新增 JSON 内版本/完整性标记区分合法
零费率；完整历史快照与缺失/零值歧义分别处理，不能猜测历史价格。保持按次计价与 C02
最早 clamp/System 审计。对应 `model/task.go`、`controller/relay.go`、`service/task_billing.go`。

JSON 内使用 version/complete 标记，不新增 SQL 列。完整正倍率历史快照继续使用历史价格；
缺失/零值有歧义的旧记录才走带来源审计的 legacy_current，读取当前用户组到任务组的实际
倍率；不能获得配置时保留预扣，不能猜测默认倍率。新版本显式零模型/分组倍率允许免费，
未知版本或非法倍率保留预扣并记录 System 来源审计。按次计价、正常取整与 C02 审计不变。
补提交边界、SQLite 序列化读回、结算余额/日志断言；现有专库矩阵增加快照 JSON 往返测试，
MySQL5.7/PG9.6 已由最终 CI 真正执行，原始日志确认无 skip 或旧 JSON 错误。

独立复审补项已修：纯快照校验在 token/轮询早返回、按次与 adaptor override 之前执行，
未知/非法快照不会被 adaptor 错误退款；System 审计只记一次，非有限价格不会破坏 JSON。
合法旧按次即使当前配置缺失也不重新查价。此保证仅针对新代码执行结算；旧轮询进程仍
可能使用旧逻辑，混合版本生产切换不在本批验收中。扩大 race 检查发现的 C06 独立跟踪。

B2/B3 需核心合同决定：上游接受结果未知时是否不自动重发/退款；提交意图和可查询记录
何时持久化；终态 CAS 与唯一账务事件、同步主库入账、分库日志 outbox 的边界。当前已有
终态 CAS/系统任务租约，须复用；不能只延迟 HTTP 响应就宣称恢复完成，不能把批量队列
“已入队”当作事务落账，也不能批量推断历史终态记录应退款。三库迁移及混合版本启用
闸门需在设计中明确，生产切换仍另行批准。C03b 的缓存恢复决定不由此代替。

## S2-D 根模块 JSON wrapper 合规

2026-09-04 对根模块生产 Go 代码重新做结构扫描；排除 `*_test.go`、wrapper 实现
`common/json.go`、独立模块 `relaykit/**` 和只使用 `json.RawMessage`/`json.Number` 类型或
`json.Valid` 的合法位置后，共有 **67 个直接序列化调用、27 个文件**。这里的扫描是政策
门禁，不代替行为测试；迁移只做 `json.Marshal`→`common.Marshal`、
`json.Unmarshal`→`common.Unmarshal`、单次非严格 decoder→`common.DecodeJson` 的等价替换，
不引入第二套 wrapper，不让 relaykit 依赖根模块。

| 批次 | 状态 | 调用/文件 | 写入边界与最低验收 |
| --- | --- | ---: | --- |
| D01 OAuth | 已完成当前范围 | 9/4 | GitHub、Discord、OIDC、Linux DO；GitHub JSON 请求/响应与宽松单值 decoder 回归，包测试、race、根模块全量、独立审查及同提交 CI。 |
| D02 请求 middleware | 已完成当前范围 | 2/2 | Jimeng/Kling 请求重写；显式 0/false、嵌套 metadata、malformed、缓存一致性、模型与时长边界。 |
| D03 Relay 输入归一化 | 已完成当前范围 | 3/3 | OpenAI/Replicate/model mapping；RawMessage 解码、循环/非法映射和 import alias；定向/race/vet、独立审查与同提交 CI 通过。 |
| D04 Provider 响应/Vertex token | 已完成当前范围 | 5/3 | SiliconFlow、Tencent、Vertex；合法/错误/malformed 响应，Vertex 固定安全错误、非空 token 与 HTTP/provider error 边界；同提交 CI 通过，不访问真实上游。 |
| D05 Midjourney | 已完成当前范围 | 8/1 | 持久化 Buttons/VideoUrls/Properties、Notify 与 object/array/`[]` 响应形状；保留静默解析及历史错误字符串；同提交 CI 通过。 |
| D06 Controller | 已完成当前范围 | 8/3 | Vertex key、model metadata、Uptime Kuma/Ollama 边界；保留合法 `json.Valid`/RawMessage，规则 endpoint 稳定排序；同提交 CI 通过。 |
| D07 Settings / C08 | 已完成当前范围 | 13/5 已清零；根余量 19/6 | 设置反射及 fresh 发布、群组倍率、集合 null 规范化、generic config 原子发布与纯验证；配置/模型/限流并发边界回归，新增 D07 race 门禁并于同 SHA CI 实跑。 |
| D08 io.net 核心 | 已完成当前范围 | 8/2 已清零；根余量 11/4 | `pkg/ionet/client.go` 和 `jsonutil.go` 各 4 处实际 stdlib JSON 调用迁移至 `common` wrapper；离线 fake client 覆盖 HTTP body/query、API error 与 flexible time。普通/race `-count=2`/vet/diff-check/gofmt、根全量、relaykit 独立矩阵及 `9193ada` 同 SHA CI 通过。 |
| D09 io.net endpoints | 已完成当前范围 | 9/3 已清零；根余量 2/1（仅 `cachex`） | container/deployment/hardware 全部 path segment 使用 `PathEscape`；stream options 局部复制；nil、null、空、缺 ID、mutation/hardware/location 与 `false` 合同回归；默认 HTTP client 禁止 301/302/303/307/308 重定向，避免 `X-API-KEY` 和敏感 body 泄露；`8228203` 同 SHA CI 含 io.net 整包 race 实跑。 |
| D10 cachex | 已完成当前范围 | 2/1 已清零；根余量 0/0 | `codec.go` 的 `Marshal`/`Unmarshal` 已改用 `common` wrapper，Decode 保留 `[]byte(s)` 复制；覆盖 round trip、空白、错误输入、不可编码值及会改写输入的自定义 unmarshaler；`bf03cba` 同 SHA CI 实跑新增根生产代码静态门禁及 io.net/cachex race。 |

D01 已将四个 OAuth 文件的 9 处直接调用清零，结构余量为 **58 处/23 文件**。本机
`go test ./common ./oauth`、`go test -race ./oauth`、`go vet ./oauth` 与低并行根模块
`go test ./... -count=1` 通过；测试仅用合成 RoundTripper，不访问真实 OAuth endpoint 或
凭据。独立审查无 P1/P2；P3 是另外三个 provider 未机械复制 GitHub 的协议测试，当前由
等价替换、全包编译/race 和静态门禁承担，后续若改 DTO/endpoint 再补专用行为回归。
该批无页面变化，`VERSION` 仍为 0.1.1。最终 `6b3042a` /
[CI 33793219733](https://github.com/ForceMind/MyAPI/actions/runs/33793219733) 七项成功，
Backend 原始步骤包含根/relaykit vet、build、全量 test 与两组既有 race；D01 已完成当前范围。

**S2-C08 / D07 已完成当前范围。** Settings 五个目标文件的 13 处直接 JSON 调用已
清零，根模块结构余量由 32 处/11 文件降为 **19 处/6 文件**。同批修复 fresh 发布与
验证合同：RWMap、10 倍率、Chats/UserGroups/AutoGroups/PayMethods、负倍率和非有限值、
model DB 前完整验证及批量 rate 聚合、generic config 全对象失败原子性和纯验证、
ConfigManager registry 回调锁、集合 `null` 规范化；Claude/Gemini 各自语义保持不被通用
配置路径改写。模型成功请求限流改为单请求快照，防止溢出；Enabled 且 0 分钟不能启用，
成功请求才记录，内存路径不把成功计入失败，避免巨额预分配；动态窗口按 key 自身过期，
并在计数缩小时 prune。exposed cache 代际也纳入可观察回归。

本机实际通过受影响包普通测试；最终 CI 同款 race（`common`、`types/config`、
`model_setting`、`operation_setting`、`ratio_setting`、`setting/model`、`middleware`、
`controller`）；限流 race `-count=2`；根 `go test -p 1 ./...`、`go vet ./...`、
`go build -p 1 ./...`；以及 `relaykit` 的 `GOWORK=off` vet/build/test、gofmt、diff-check
和 YAML 解析。独立 Sol 首审问题均已修复，复审确认当前范围无剩余 P1/P2。本批无前端
页面或 schema 变化，`VERSION` 保持 0.1.1；未跑真实上游、生产、真实设备或发布。最终 `2d6acab` /
[CI 33814136556](https://github.com/ForceMind/MyAPI/actions/runs/33814136556) 七项成功；Backend 原始日志确认新增 D07 race
步骤对 10 个包逐个返回 `ok`。同 SHA 常规 MySQL/PostgreSQL jobs 成功，但本批无专用三数据库
Settings 行为场景，不得以其代替热更新验收。

**明确未完成的 S2-C09：** generic config 对象业务热读仍缺统一快照/锁；成功限额的内存
与 Redis 都是 check→execute→record 的近似语义，并发可超发，硬限额须另定
reservation/rollback 合同；跨配置族 reload 仍是 best-effort，非全量事务；历史 DB raw
`null`、未知分层 key、Passkey 懒写与 `GroupRatioSetting` 可变指针仍待审计。C09 的只读设计已完成、
实施仍待分批收敛；D09 与 D10 均已完成当前范围并通过同提交 CI。

**S2-D08 io.net 核心已完成当前范围。** `pkg/ionet/client.go`
与 `pkg/ionet/jsonutil.go` 的各 4 处实际 stdlib JSON 调用已迁移至 `common` wrapper，根模块结构余量
由 **19 处/6 文件**降为 **11 处/4 文件**。无网络 fake client 回归覆盖请求 body、headers、method、URL，
NaN marshal，transport/API detail fallback，query slices、HTML escape、空值/零值/false、`time.Time` 与
`*time.Time`；flexible time 覆盖对象/数组、直接或 `data` 包装、无时区 UTC、带时区 offset、未知/普通
字符串、malformed、错误类型与尾随值。普通测试、race `-count=2`、vet、gofmt 与 diff-check，以及
根模块全量测试/vet/build 和 `relaykit` 独立 vet/build/test 已通过。最终 `9193ada` /
[CI 33816756504](https://github.com/ForceMind/MyAPI/actions/runs/33816756504) 七项成功。独立 Sol 审查无
P1/P2，数组和 `*time.Time` 两项 P3 已补。保留 `interface{}`→`float64` 的大整数精度风险与递归识别
“看似时间”字符串的既有语义；endpoint path 逃逸、nil response 等 D09 边界另审。本批无页面、schema、
真实 io.net、凭据或网络访问，`VERSION` 保持 0.1.1。

**S2-D09 io.net endpoints 已完成当前范围。** `container`、`deployment`、
`hardware` 三文件的 9 处实际 `Unmarshal` 已全部迁移至 `common.Unmarshal`，根模块结构余量由
11 处/4 文件降为 **2 处/1 文件**，仅余 `pkg/cachex` 的 D10 codec。所有动态 deployment/container/
cluster path segment 均经 `PathEscape`；stream options 使用局部复制，调用者输入不被修改。`makeRequest`
覆盖 nil 边界，仅 2xx 视为成功，非 2xx 固定为 `APIError` 且不回显 raw response 或 detail；controller
以 `errors.As` 识别该错误。默认 HTTP client 拒绝 301/302/303/307/308 重定向，以免 `X-API-KEY` 或敏感
body 跟随跳转泄露。

离线 fake 与 loopback `httptest` 回归覆盖合法及 malformed 响应、mutation/hardware/location、null/空/缺 ID
防伪成功、合法 `false`、五种重定向状态、path 逃逸、stream options 不变性和错误映射；不访问真实 io.net、
凭据或公网。本机 `pkg/ionet` race `-count=2`、controller 定向 race、vet、gofmt、diff-check、根模块
全量 test/vet/build 及 relaykit 独立 vet/build/test 已通过。独立 Sol 审查无 P1/P2；P3 为未公开文档
的 hardware/location 必填回显，仍需真实脱敏响应或官方 schema 补验。无页面或 schema 改动，`VERSION` 保持
0.1.1。最终 `8228203` / [CI 33819117410](https://github.com/ForceMind/MyAPI/actions/runs/33819117410)
七项成功；Backend 新增 io.net 整包 race `-count=2` 步骤成功。

**S2-D10 cachex 已完成当前范围。** `pkg/cachex/codec.go` 最后两处生产
JSON 调用已等价迁移为 `common.Marshal` 与 `common.Unmarshal([]byte(s), ...)`，根模块生产实际
`Marshal`/`Unmarshal`/`Decoder`/`Encoder` 直调余量为 **0/0**；扫描继续排除 `common/json.go`、测试、
`relaykit`、合法类型/`json.Valid` 及一条注释。`[]byte(s)` 不可改为无复制转换：独立审查发现
`UnmarshalJsonStr` 的 unsafe 别名可让自定义 `UnmarshalJSON` 改写调用者 string，codec 路径已改回复制并以 mutating
unmarshaler 回归锁定输入不变。

新增 codec 回归覆盖嵌套 round trip（含显式 `0`/`false`）、空白、malformed、类型错误、多个 JSON 值、
尾随空白、func 不可编码及上述输入不变性。本机 `cachex` race `-count=2`、根模块全量 test/vet/build、
`relaykit` `GOWORK=off` vet/build/test、gofmt、diff-check 与静态门禁均通过；独立复审最终无 P1/P2。
最终 `bf03cba` / [CI 33821142971](https://github.com/ForceMind/MyAPI/actions/runs/33821142971) 七项成功；
Backend 原始步骤确认 `git grep` 门禁和 io.net/cachex race `-count=2` 均实际成功。无页面、schema、Redis
或真实缓存服务改动，`VERSION` 保持 0.1.1。

C07/D02 在 D01 的 58/23 基线上将两个 middleware 直接 Marshal 清零，结构余量为
**56 处/21 文件**。只读 Sol ultra 追踪确认旧 `KeyBodyStorage` 会遮蔽兼容适配器写入的
统一正文：Jimeng `req_key` 和 Kling 仅 `model_name` 请求在分发时读不到 `model`；同时
Provider metadata 可覆盖已经路由/计费的 Kling `model_name`/duration 和 Jimeng
`req_key`/frames。修复增加原子 `ReplaceRequestBody`，同步 Body/GetBody/ContentLength/cache，
关闭旧内存/磁盘资源；路由按 TokenAuth→原始 Full Content 日志→转换→Distribute，所有
日志阶段冻结同一入口身份。兼容 envelope 保留通用 Task 顶层字段，provider 字段按顶层
覆盖 nested metadata，受保护别名不得下传；Kling/Jimeng adaptor 最终恢复权威模型，
Kling duration/mode 与 Jimeng frames 只能来自已验证的顶层合同。Jimeng 官方顶层 frames
只接受 121/241 并正规化为 5/10 秒。

红测先复现下游 GetBody 为空/旧 storage 仍被读取，以及 metadata 覆盖两个 Provider 的
权威模型；修复后受影响六包及 router、common/middleware 全包 race、受影响 vet 和根模块
全量测试通过。独立 Sol 首轮指出 duration/frames 旁路与日志身份缺口，修正后第二轮无
P1/P2，图片权威字段 P3 断言也已补齐。最终 `f7cc5c3` /
[CI 33798508808](https://github.com/ForceMind/MyAPI/actions/runs/33798508808) 七项成功；Backend
完成根/relaykit vet、build、全量 test 和既有 race 门禁，C07/D02 已完成当前范围。无页面
变化，`VERSION` 保持 0.1.1。Jimeng 查询 handler 选择与 metadata JSON 字符串兼容仍单独
审计，不在本批伪称完成。

D03 将 OpenRouter Anthropic `THINKING`、Replicate `OutputFormat` 和模型映射的三个直接
Unmarshal 等价改为 `common.Unmarshal`，结构余量由 56/21 降至 **53 处/18 文件**。
OpenAI 继续保留 `encoding/json` 仅作 RawMessage 类型，Replicate 删除无用 import，
model helper 用 `rootcommon` 避免与 `relay/common` 命名冲突。测试冻结 thinking 的双重
门控、enabled/budget/malformed/disabled 行为，Replicate 原字符串/静默忽略与 Extra 覆盖
顺序，以及映射直链/多跳/自映射/真循环/非法/空/null 的成功和部分失败状态。
三包普通测试及 race、三包 vet、diff-check 与独立 Sol 审查通过；无网络、数据库、凭据或
全局设置修改。非阻断 P3 是未单列“非 OpenRouter＋Anthropic”轴，以及不锁定自映射时
SetModelName 的内部调用细节；代码门控和可观察业务结果已覆盖。最终 `615fbbd` /
[CI 33800052236](https://github.com/ForceMind/MyAPI/actions/runs/33800052236) 七项成功，D03
完成当前范围。无页面变化，`VERSION` 保持 0.1.1。

D04 将 SiliconFlow rerank 的解码/编码、Tencent 非流响应解码和 Vertex 两个 token decoder
共五处统一到 `common` wrapper，结构余量由 53/18 降为 **48 处/15 文件**。Sol 安全审计
同时确认 Vertex P1：不可信 token endpoint 响应 map 可能被格式化进 API 错误；以及 P2：
空/纯空白 token 会被当成功并可能缓存。两个 exchange 现共用单一安全 parser：nil/body、
非 2xx、provider error、malformed、missing/null/错误类型/空白 token 均返回固定错误，绝不
包含正文、token、error_description 或 map；未知字段和首个 JSON 后的尾随值保持兼容。

SiliconFlow/Tencent 真实 handler 回归锁定 status/header/usage/统一响应、read/malformed 与
既有 provider error 行为；Vertex 纯内存响应覆盖 provider error 与 token 同时出现、所有
JSON 类型、未知字段、尾随值和 body ownership，不生成 JWT、不连接 Google/代理/cache。
三包普通测试、race、vet、gofmt、diff-check 和独立 Sol 审查通过。非阻断 P3：非 2xx 不
drain 正文会降低错误连接复用，两 exchange 尚无各自 transport 测试；安全和共用接线已由
代码/纯 parser 测试确认。最终 `544f83b` /
[CI 33802572396](https://github.com/ForceMind/MyAPI/actions/runs/33802572396) 七项成功，D04
完成当前范围。无页面变化，版本仍 0.1.1。

D05 将 `relay/mjproxy_handler.go` 的 8 处直接调用机械迁移到 `common.Marshal/Unmarshal`，
结构余量从 48/15 降为 **40 处/14 文件**。Notify 仍忽略 VideoUrls marshal error；三种
持久 JSON 仍只在解析成功时赋值；SwapFace、ImageSeed、单 task 与列表的 marshal 失败仍
使用历史 `unmarshal_response_body_failed`，不借迁移改协议。

回归精确覆盖 Buttons 的 0/false/空/null、Buttons/VideoUrls 非 nil 空 slice、Properties
`null` 的非 nil 零值历史语义，三字段各自 malformed 静默且不影响其他字段。隔离 SQLite
真实验证 Notify 将 `videoUrls:[]` 存为精确 `[]`，Task handler 单项为 camelCase object、
条件项为 array、空条件为 `[]` 并设置 JSON Content-Type；全局 DB 单连接、关闭并恢复。
relay 定向、全包普通/race、vet、diff-check 与独立 Sol 审查通过。JSON-safe DTO 的四个
marshal 错误分支无法自然构造，源码确认错误字符串未变，不为覆盖率引入注入钩子。最终
`b2b60fd` / [CI 33804146311](https://github.com/ForceMind/MyAPI/actions/runs/33804146311)
七项成功，D05 完成当前范围。无网络、上游、计费、转发设置或页面变化，版本仍 0.1.1。

D06 将 Controller 的 8 处实际编解码迁移到 `common` wrapper；`channel.go` 仍只为
`json.Valid` 和 RawMessage 类型保留 stdlib import。规则模型 endpoint 并集从 map 转 slice
后按字符串排序再编码，修复同集合在响应/缓存中顺序漂移；精确模型集合不变。结构余量从
40/14 降为 **32 处/11 文件**。

离线测试覆盖 Vertex key 数组的 string trim、object/array/0/false/null 紧凑表达及 malformed/
非数组/空输入；Uptime helper 验证 GET、200、未知字段与尾随值宽松解码、malformed、非 200、
transport error 和 body close。Controller 全包普通/race、vet、gofmt、diff-check 与独立 Sol
审查通过。Ollama 三种 SSE frame 仍忽略不可达 marshal error、frame 与 `[DONE]` 顺序不变；
endpoint enrich 和 Ollama 未新增专用集成测试列为非阻断 P3，由直接排序、机械等价、全包
回归与后续 CI 承担。最终 `02a0aaf` /
[CI 33805743908](https://github.com/ForceMind/MyAPI/actions/runs/33805743908) 七项成功，D06
完成当前范围。无外网、数据库、页面或 relaykit 变化，版本仍 0.1.1。

## S4-01 无发布 Full/LAN 镜像测试（已完成当前范围）

修复原 `docker-smoke.yml` 等待 health 状态却没有配置 healthcheck 的确定缺口；分别构建
Full/LAN，保留手动 dispatch、contents:read、load:true/push:false，无 registry 登录。
BuildKit 上限 2 CPU/4 GiB，矩阵串行；应用 1 CPU/768 MiB，回环端口和临时 SQLite。
显式隔离开关、字面 loopback、全新 SQLite/root 未初始化检查全部满足后才创建合成管理员。
复用认证/额度/日志探针，另检查匿名拒绝、SelfUse 模式及真实浏览器启动与精确 SHA 构建标识。
不输出凭据/原始响应，不上传数据库，只清理本次 SHA 标记的测试容器。

最终 `7f1913e` / Docker smoke `33781637372` Full/LAN 两项均成功，实际登录表单与三处
构建标识精确对应该 SHA；修订探针要求登录 success:true，成功项不再附错误码。
Node 合成 API 13 项、YAML/Bash 语法检查及独立复审通过。历史 `540cf32` 的手动测试
`33766140801` 两种镜像均构建/启动成功，但实际表单就绪后构建标识检查失败，未予验收。
Rsbuild 产物证实间接读取 env 丢失注入值而回退为 0000/local；已修为直接属性读取，
已经真实镜像复验，不放宽 SHA 校验。此项只验证 linux/amd64、新安装与前端启动，不是三库升级恢复、
完整角色/Key/上游调用、macOS Docker Desktop、arm64 或真实桌面/手机验收。

构建标识最小修复已通过独立复审、两项明确注入值的单测、typecheck、涉及文件 lint 及
生产 build（6.64s，合成 ID `fixture-s4-build`）；产物确认实际注入，不再读取缺失 env 别名。
前端完整测试为 64 文件/336 项通过（101.96s，maxWorkers=2）。本机合成 ID 不冒充 GitHub
提交 SHA；最终 Full/LAN 在 GitHub 镜像里通过真实浏览器三处一致标识检查，image ID 见完成度审计。

2026-09-04 最终整合根模块全量/vet/build、定向 race、relaykit 独立 build/test 已通过；
B1/C06/S4-01 的功能、实库、镜像和 2190 文件源码发行包证据已补齐。封版追加的
7f1913e 整合 race 又发现 Kling 测试清理与异步缓存回调竞争；测试生命周期已固定，新增
CI race 接线；`6fd8ae4` / CI `33783792231` 七项成功，新 backend race 步骤原始日志
确认 logger/model/service/Kling 全部执行通过。未更改生产 C03b 策略，也不以此前 race
通过覆盖新失败。未将 S2-B2/B3、C03b、
S3–S7 或真实设备范围列为完成。

## 待决策与外部条件

**S4-02 已完成当前范围**：
在无 Redis、非 batch、关闭渠道内存缓存的全新 SQLite Full/LAN 镜像中创建合成普通用户、钱包、受限 Key 和普通
OpenAI 渠道；假上游与应用共享隔离网络 namespace，绝不调用真实 Provider。通过管理
API 显式固定测试模型/补全/组倍率为 1，单次非流请求返回 usage 10+5，精确验证钱包与
Key 剩余减 15、用户/Key/渠道用量加 15、请求数加 1，以及唯一关联消费日志。
只验证已定义的请求凭据字段/请求与响应头脱敏和日志权限；响应正文当前按原样保留，
测试上游只返回无敏感合成正文，不宣称通用全内容脱敏。

实现范围为 `tools/runtime/fake-openai.mjs`、`docker-smoke.mjs` 及测试、无发布 workflow：
固定 digest Bun sidecar 与应用共享网络，root 仅管理，普通用户真实登录并使用受限 Key；
fake control 在业务前为 0、成功调用后为 1，匿名拒绝后仍为 1，只保存请求形状布尔值。
应用/sidecar 只发布 runner 回环端口，日志和 SQLite 位于 tmpfs；sidecar 只读、非 root、
cap-drop/no-new-privileges，并按 SHA 标签先于应用清理。Node22 17 项、Bun 回环入口、
YAML 与全部 shell 块、runtime/release workflow 合同及独立审查通过。首个 `b54ce36`
实跑的 app 与 sidecar 均运行，但 sidecar 只监听 namespace loopback，宿主 NAT 健康请求
持续 reset，业务探针未执行；已改为仅 CI sidecar 显式监听容器全接口，宿主仍只发布
127.0.0.1，本机默认 loopback 不变。

最终 `237c0da` 的本机 `release:check` 全链通过（runtime 17/17、合同检查及 2191 文件/
20,024,635 bytes 包检查）；[CI 33790468336](https://github.com/ForceMind/MyAPI/actions/runs/33790468336)
七项成功。[Docker 33790513455](https://github.com/ForceMind/MyAPI/actions/runs/33790513455)
在同一精确 SHA 上 Full 4m21s、LAN 4m30s 均成功：fresh SQLite、身份/权限、精确 15 quota
账本、唯一关联日志、普通用户 admin_info 隔离与 Full Content 403、请求/响应头脱敏、匿名
relay 401 且不上游、真实登录表单和三处一致 revision 全部通过；失败诊断未运行，SHA 标签
清理完成。Full image ID 为 `sha256:5bff88fc38c973f86d0966aff1539ddae5e1c658627a7bb7a9e8bf471343d1ae`，
LAN 为 `sha256:6eb322e85047c9bb6f5087717527c5b9b0e1d91cf3c631fff7fb373f539ba1ce`。
该批不改产品页面；`VERSION` 保持 0.1.1，临时 image ID 不是 GHCR digest。

下一项复用相同 HTTP 合同扩展 MySQL5.7/PG9.6：应用启动前证明独立专库为空，正确配置
字符集，不把 setup/root_init 标记当作全库空证明，也不直接放宽 SQLite-only 安全门。
完整备份/升级/恢复、Redis/batch、失败退款、SSE、真实账户仍分别验收；不由一次调用
成功推导上述能力。B2/B3、C03b 的账务恢复决策独立保留。

| 项目 | 当前事实 | 恢复/进入条件 |
| --- | --- | --- |
| GitHub CI | 最新 `237c0da` / `33790468336` 七项 success；测试门禁 `6fd8ae4` / `33783792231` 七项 success。两库各七支付＋五快照及新增四包 race 的原始日志已核对 | 历史假绿、统计回归、PG JSON 与追加 race 失败保留；后续仍核对各自 SHA。 |
| Docker | Mac 仍无 Docker CLI/App；`237c0da` / `33790513455` 的 Full/LAN 全新 SQLite 合成计费 smoke 已通过 | GitHub 临时镜像不发布；image ID 不是 GHCR digest，也不是三库恢复、本机 Docker Desktop 或真实设备证据。 |
| 工具链 | macOS 26.2 arm64，Go 1.27.0、Bun 1.4.0、Node 22.23.2 可用 | 最低/固定验证基线以 `go.mod` 1.25.1、CI Bun 1.3.14 为准；本机可运行不等于最低版本或永久环境配置已验。 |
| 三数据库 | SQLite fixture 与历史副本已有；S1 身份迁移/Option 事务已在 MySQL5.7/PostgreSQL9.6 实跑 | 不等于完整应用升级、备份恢复或多连接业务验证；S4 仍需对应测试，不能连接生产替代。 |
| 真账户/设备 | 缺当前版本完整真实采样、手机、macOS/Windows 安装、第二设备 LAN 证据 | 测试实例、显式配置的上游账户、管理员权限、设备/网络与回滚方案；不读取本机凭据文件。 |
| Profile 强制执行 | 当前是兼容字段/元数据注册表 | S3 迁移、拒绝/回退/计费语义及回滚批准。 |
| 外发预警 | 现有阈值、冷却、只读状态；不发送 | 决定接收者、通道、凭据、重试/去重、恢复与审计，速度/耗尽估算明确不确定性。 |
| Claude 组织用量 | 转发已实现，独立组织观测仅有设计 | 确认业务需求、产品形态、专用 Admin 权限/保管/保留规则；组织用量不是余额。 |
| Antigravity 公共 relay | 内部 transport 已有，公共持久异步业务未实施 | 生命周期、权限、工具边界、预算/结算设计批准；没有官方余额能力就保持 unsupported。 |
| 多 Key / TokenHub 扩展 | 当前不把多个上游账户混成单一额度；专用适配边界已有 | 先决定稳定非敏感账户身份、路由/对账需求；不复制其他项目或隐式导入凭据。 |
| 发布与法律 | 版本仍 0.1.1，v0.1.0/v0.1.1 是历史保护 tag | 新版本号、tag rules/environment 审批配置、签名、NOTICE/许可证、域名及发布权限分别确认。 |

## 验证与阶段同步

先运行新增回归，保存真实失败；修复后定向测试、相关包回归、必要 race、根模块
vet/build/test 与 `cd relaykit && GOWORK=off go build ./...`。新增/共享前端行为按 Bun
运行 typecheck、Vitest、build、i18n 和真实浏览器；仅后端修改不冒称做过新的视觉验收。
发布/LAN/升级合同和制品验证在最终提交上执行，生成物保持 ignored。

每阶段由未参与实现的角色复审；记录失败项与修复复验，不把实现者自查写成独立通过。
提交前检查工作树、敏感文件、范围与远端；推送不强制覆盖并发提交，不移动 tag。
文档用精确代码 SHA、实际命令/版本、结果、CI/run/artifact 和未验证范围；历史记录归档
而不重复当作当前。新增范围或重要设计变化先确认。

本批 S0 与 S1-01..06 已交付：代码 `c42eeeb`，CI 表达式修正 `68e930f`，MySQL schema
探测修正 `dc94e81`；成功 CI `33740321133` 五个 job 包含实际 MySQL5.7/PostgreSQL9.6
子项（未 skip），以及前端测试/构建/浏览器、后端与 relaykit、发行和桌面合同。
SQLite 启动/恢复及专项 race 有本机通过证据；sol 独立复审原六项无新增阻断回归，terra
文档/接口合同复审通过。详情见[完成度审计](COMPLETION_AUDIT.md)。

R1 后续交付为 `8dfcfba` / `33743669737`：配置写入与重载序列化，四项确定性回归及
CI race 通过；本机完整 release:check 通过。上述完成状态仅指 S0/S1 原六项及经确认的
R1，不代表 S2-S7、完整应用三库恢复或真实设备/生产验收完成。先前 MySQL 实跑失败
记录保留，不能只保留绿灯而删除失败证据。
