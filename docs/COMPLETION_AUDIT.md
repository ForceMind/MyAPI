# MyAPI 完成度与证据矩阵

本文档把总体计划中的目标映射到可复核证据。`已验证` 只表示代码、合同或 CI
已经提供证据；真实设备、生产副本、法律和正式发布不会因静态检查而自动变成完成。

| 领域 | 已验证证据 | 当前状态 | 仍需外部条件 |
| --- | --- | --- | --- |
| API 兼容 | `relay/` 转换器与后端 CI | 已验证 | 上游版本变化时继续回归 |
| API/响应日志 | `web/src/features/usage-logs/`、`web/src/features/full-content-logs/`、移动集成测试、脱敏测试、移动内容高度修复；列表与 Full Content Logs 查询缓存均按 user/session 隔离 | 代码已验证 | 真实手机视觉验收 |
| 运行构建可见性 | 管理员「系统信息」中的只读 Runtime build 标识、`build-metadata.ts` DOM/global 元数据及 `build-metadata.test.ts`；Docker/Release/Electron 构建注入 commit SHA；历史 Linux 副本曾切换到 `local/new-api:myapi-c283c1d` 并健康 | 代码与历史副本有证据；当前部署未核验 | 真实管理员手机视觉验收仍待完成 |
| 完整独立 UI 系统 | 当前仅有 MyAPI 品牌资产、必要文案和局部额度/日志功能增量 | 尚未开始（代码层先行） | 需在代码合同稳定后另行完成信息架构、视觉系统、Full/LAN/移动端真实画面与设备验收；不得把局部增量误报为 UI 全量替换 |
| 渠道额度历史 | `controller/channel-billing.go`、`controller/codex_usage.go`、历史/聚合测试、权限路由测试；2xx 无有效 Codex rate_limit 时标记 unsupported；历史聚合按 metric/window/source/plan/unit/currency/window_seconds 隔离；快照按渠道/系列/观测时间桶幂等保留首条，并以 nullable SHA-256 唯一键抵抗并发重复写入 | 已验证 | 真实登录账号和采样数据演练 |
| 概览额度变化 | `account-quota-changes-panel.tsx`、`codex-account-quota-chart.tsx`、60 秒前台刷新、Codex 账户选择与可配置时间范围/颗粒度/指标/折线、面积或柱状图、稳定额度 0 值解释、旧传输错误行抑制、错误/plan type/只读告警状态测试；`a2528a2` 的跨登录身份查询缓存隔离与认证刷新回归测试；系列按计划/单位/窗口隔离；后台采样默认开启并可在监控设置中调整 | 代码、前端类型检查、生产构建与定向测试已验证 | 需要由具备 `channel.read` 的真实管理员在最新镜像中验收；真实手机视觉仍待完成 |
| 账户等级/Key 访问方案 | `model/access_profile.go`、Key/UI/API 测试、策略注册表及旧 Key profile 保留测试；Key 表单显式提交稳定 `access_profile_id` 并保留 legacy `group`；`setting/access_profile.go` 校验 fallback 目标存在、去空格后的 ID 唯一性和循环依赖 | 兼容层已验证 | 强制路由迁移评审 |
| 设置引导 | Full/LAN Lite/权限条件、生命周期测试 | 已验证 | 多设备视觉检查 |
| 品牌与旧元数据 | `tools/branding/check.mjs`，最近运行 `blocking_count: 0` | 阻断项已清零 | NOTICE、源码头和兼容标识法律审查 |
| 静态官网 | `tools/website/check-static.mjs`、Chromium smoke、artifact workflow | 自动化已验证 | 真实移动视觉与独立域名发布决策 |
| LAN Lite/桌面 | `lan:check`、`desktop:check`、Electron 安全边界；`electron/test/desktop-probe-contract.test.mjs`、`runtime-config.js` 和 `tools/desktop/check.mjs` | 合同已验证（生产探针为 `/api/status`，要求 HTTP 2xx 且 JSON `success=true`；开发首页仍仅校验 HTTP 状态）；CLI 与托盘对通配监听均仅展示发现的 RFC1918 地址 | macOS/Windows 实机安装、LAN 请求、防火墙；真实设备运行结果不得由合同测试代替 |
| 多语言关键文案 | `web/src/i18n/locales/{fr,ja,ru,vi,zh-TW}.json`、`web/src/i18n/__tests__/locale-key-parity.test.ts` | English key parity 已验证（5 locales / 5 tests） | 真实设备文字长度与视觉审查 |
| GHCR/升级 | `release:workflow:check`、`upgrade:check`、不可变 digest 合同；版本与架构 tag 构建前检查并 fail-closed 拒绝覆盖 | 自动化已验证 | 脱敏副本升级、数据库恢复、人工审批 |
| 计费安全 | `service/violation_fee.go` 使用 checked quota rounding，并在饱和时拒绝收费、保留 `relayInfo.QuotaClamp`；对应正常值、溢出、`Inf`、`NaN` 与审计捕获回归测试 | 当前工作树代码与定向 Go 测试已验证 | 当前 CI 已通过；真实业务账本/生产额度仍待相应条件 |
| 新开发环境数据库默认值 | `48abce6`、`docker-compose.dev.yml`、`makefile` | 代码与模板已验证（新开发默认数据库为 `myapi`） | 接管既有数据库必须显式设置 `MYAPI_DEV_POSTGRES_DB`/`DEV_POSTGRES_DB` 并在副本验证；该变更不执行重命名或迁移 |
| Claude 组织用量 | `docs/CLAUDE_USAGE_REPORT.md`，官方 Usage Report 边界 | 设计已验证 | Admin 凭据、权限、保留策略和实际接入 |
| Google Antigravity | `relay/channel/gemini/antigravity_client.go`、`antigravity_client_test.go`、`docs/ANTIGRAVITY_INTEGRATION.md`、`docs/ANTIGRAVITY_PUBLIC_RELAY_GATE.md`；`1827358` | 专用 transport 代码、边界测试及有界 Docker Go 回归已交付（create/get/poll/cancel/delete、usage、动态 agent/continuation 约束、大小上限、终态和脱敏） | 仍需完成公共 Relay 闸门中的持久化、权限、计费、工具策略和完整测试评审；稳定官方余额接口不存在时保持 `unsupported` |
| NPM 正式发布 | CLI/打包/版本合同检查 | 发布前检查已验证 | 版本确认、tag、清单、用户明确确认与 `npm publish` |
| macOS 开发迁移 | `docs/DEVELOPMENT_ON_MACOS.md`、`docs/CODEX_HANDOFF_PROMPT.md`、README 导航 | 文档已补齐 | 新 Mac 的工具安装、依赖测试和实机 Electron/LAN 验收需在新设备执行 |

## S2-A 支付与订阅事务（2026-09-03，已完成当前确认范围）

已确认范围与状态见[执行矩阵](DEVELOPMENT_EXECUTION_PLAN.md#s2-a-支付与订阅事务)。
实现包括：退款与幂等标记同事务、套餐与时钟同连接、真实 DB 错误不误报缺单、Creem
补单额度单位及重复日志、Stripe 回调错误 ACK/重复成功与 pending 条件更新、
Pancake 查询错误分类，以及 Stripe/Creem 仅订阅配置的回调闸门。没有 schema 迁移。

当前验收代码 `767b17b` / [CI 33751536908](https://github.com/ForceMind/MyAPI/actions/runs/33751536908)
六个 job 全部成功；MySQL5.7/PostgreSQL9.6 各七场景实际执行，精确日志增量、中文正文和
历史日志不变断言全部通过，原始实库输出未见 `Error 1366`、日志写失败或 skip。
A01–A06 及追加 R1 已验收；S2 总阶段、完整恢复和真实付款/设备并未完成。

本机新增回归先复现冷缓存额外借连接、热缓存错误时间 fallback、退款嵌套事务、
Creem 补单多乘 QuotaPerUnit/重复日志及陈旧 pending 覆盖 success；随后修复。
HTTP 测试最初的直接 handler fixture 未刷新 Gin Status，已改真实 ServeHTTP；
提交故障 wrapper 已改指针以符合 GORM 回滚接口。二者是测试设施修正，不冒称生产缺陷。

已执行并通过：

- `GOMAXPROCS=1 GOWORK=off GOCACHE=/tmp/myapi-gocache GOMODCACHE=/tmp/myapi-gomodcache go test -p 1 ./controller ./service ./model -count=1 -timeout=180s`；
  需要的 SMTP/HTTP/Redis 均为 loopback 临时 fixture，sandbox 不允许监听时改在审批环境运行。
- 同一 Go 环境下新增 `TestSubscriptionTransaction` 八项、既有订阅分组/会话回归及新事务 `-race`；
  手动补单五 provider 单位、额度非法/边界与重复日志；Stripe/Pancake 查询/写入/明确提交失败及重送；
  Stripe/Creem 仅订阅配置的真实 Gin 路由、本地签名、重复履约、非法签名与禁用矩阵。
- Node22 `npm run release:workflow:check`（18/18）、`npm run quota:openapi:check`（3/3）、`git diff --check`。
- sol 独立只读复审生产改动、测试与 CI fixture 未发现本批阻断回归，YAML 解析通过。
- 代码提交 `2777021` 的本机根模块 `go test -p 1 ./... -count=1 -timeout=180s`、
  `go vet -p 1 ./...`、`go build -p 1 ./...` 通过；同目录/缓存环境与上项一致。
- `go test -race -p 1 ./controller ./model -run 'Test(SubscriptionTransaction|SubscriptionOnlyPaymentWebhooks|StripeWebhook|WaffoPancakeWebhook|ManualCompleteTopUp|UpdatePendingTopUpStatus)' -count=1 -timeout=120s` 通过。
- 在 `relaykit/` 使用 `GOWORK=off` 独立 `go build -p 1 ./...` 与 `go test -p 1 ./... -count=1` 通过；
  Node22 干净提交 `npm run release:check` 全部通过，包括 2177 文件的本地 pack 校验；未发布。

CI 新增独立 `S2-A payment and subscription database regression`，目标仅空库
`myapi_s2a_test` 的 MySQL5.7/PostgreSQL9.6；明确开关、loopback 与空库检查，无 drop。
`2777021` 的 [CI 33749764180](https://github.com/ForceMind/MyAPI/actions/runs/33749764180)
六个 job 均为 success，包含后端、前端构建/浏览器、S1/S2-A 实库、发行与桌面合同。
实库日志确认 MySQL/PostgreSQL 各七个子项实际运行（未 skip）。双连接屏障保证事务
重叠及最终状态检查，第二屏障在 SQL 发送前，不能声称直接观察到了数据库锁等待。

**历史日志复核失败（已由 R1 处理）**：当时不能据此宣布 S2-A 验收完成。MySQL 的订阅/充值中文日志插入出现
`Error 1366 (HY000): Incorrect string value ... for column 'content'`，但测试未断言
日志写入，因此 job 仍为绿色。fixture 直接 AutoMigrate，没有按真实启动路径调用
`model/main.go` 的 `checkMySQLChineseSupport`；连接 DSN 的 `charset=utf8mb4` 不等于
数据库/表的默认字符集已配置。生产启动已有字符集拒绝保护，本次不修改该保护或生产库。

追加 **S2-A-R1（已完成／已确认）**，开始基线 `5db9558`，交付 `767b17b`：仅修正专用、已确认空库的 MySQL fixture 字符集，
复用真实启动的中文支持检查；补齐成功/重复/回滚与并发付款的准确日志条数、中文内容
断言，再重跑两种实库及相关回归。范围为 `model/payment_database_test.go` 和验收文档，
不改变生产付款语义、schema 迁移或发布配置。本轮独立静态复审通过不替代这一运行时发现。

R1 已通过实库 CI：固定 `ALTER DATABASE myapi_s2a_test` 仅在目标及空库检查后
配置 utf8mb4；复用中文支持检查，在 AutoMigrate 前后验证。七个业务场景均比较历史
日志快照、新增数量及完整中文正文，订阅退款保持不新增充值日志。新增
`TestS2APaymentSQLite` 复用同一矩阵，但后三项仅顺序重放，明确不模拟 MySQL/PG 行锁。
本机 `go test -p 1 ./model -run '^TestS2APayment(SQLite|DatabaseTargetSafety|ConfiguredDatabases)$' -count=1 -v`
已通过本地七场景和目标安全检查；无 DSN 的外部入口明确 skip，不能算实库通过。
本机同一低并行 Go 环境下 `go test -p 1 ./model -count=1 -timeout=180s`（9.003s）、
`go vet -p 1 ./model`、`go test -race -p 1 ./model -run '^TestS2APayment(SQLite|DatabaseTargetSafety)$' -count=1 -timeout=120s`（5.070s）通过。
`relaykit/` 独立 `GOWORK=off go build -p 1 ./...` 与 Node22 干净提交 `npm run release:check`
通过（本地 pack 2177 文件）；未发布。sol 独立复审未发现阻断问题，确认日志缺失现在会失败；旧 Error 1366 证据保留。

Pancake 履约使用已验签合成事件，公开入口另验非法签名拒绝，未替换官方公钥。
所有支付数据/签名均为 fixture，没有真实付款。明确回滚的提交失败不代表不确定提交
恢复；完整三库备份恢复、真实网关重试、续费/争议/生产对账及 S2-B/C 仍未验收。

## S2-C 已验收项与剩余恢复设计（2026-09-03）

开始基线 `2d705a7` / CI `33752536294` 六项成功。当前不是完整 S2 验收，具体状态见
[执行矩阵](DEVELOPMENT_EXECUTION_PLAN.md#s2-c-数值缓存与-http-隐私)。

最终代码 `451bee3` / [CI 33760303619](https://github.com/ForceMind/MyAPI/actions/runs/33760303619)
七项成功。C01/C02/C03a/C04/C05 已完成当前范围，C03b 未完成。最终 Redis job 的原始
步骤日志再次确认 36 个 PASS，无 skip/Lua/整数错误，实际检查 hash 全字段与绝对过期时间。

已实际复现的红测：

- 文本 `PromptTokens=MaxInt, CompletionTokens=1` 导致总数溢负、收费归零、测试预扣 384 被退；
  clamp 的 NaN/±Inf 又使 `other` JSON 编码成空串。
- Kling 首次过大扣减乘小倍率后丢失 clamp；零差额和非正 token 缺审计。
  独立复审追加 ±1e309（ParseFloat 返回 ErrRange），红测后修复，普通语法错误语义保留。
- token hash `UsedQuota=bad` 时，旧 reserve/delta 在报错前已把 `RemainQuota=100` 改成 90/110；
  metadata-only 用户刷新因 ARGV 索引错误创建缺 Quota hash，使实际 123 被读成 0。
- 视频返回 public 缓存；支付日志暴露合成正文/签名/query/客户资料。独立复审还指出真实
  Gin 访问日志会拼入 query，已通过挂载生产日志中间件的红测复现并修复；只对四个真实
  webhook 路由记录模板，普通路由既有日志语义与请求原文不变。

C01/C02 定向 common/service/Kling/relay-common 回归和独立复审已通过；C04/C05 定向、
middleware/controller 全量与 vet 通过，含生产访问日志补项，独立复审闭环。C03a 本机脚本/预扣/
补偿/围栏定向通过；真实 Redis 7 CI 已执行，本机无 Redis server/Docker，不冒充本机实测。
新增 Redis fixture 只接受显式开启、字面 loopback、空实例及 DB 15；不删除已有数据。

本机使用 `GOMAXPROCS=1 GOWORK=off GOCACHE=/tmp/myapi-gocache GOMODCACHE=/tmp/myapi-gomodcache`：
`go test -p 1 ./model -run 'Test(QuotaCache|QuotaScripts|UserCacheMetadata)' -count=1 -timeout=120s`
通过（1.533s）；`go vet -p 1 ./common ./model ./service ./controller ./middleware ./relay/channel/task/kling`
通过。`go test -p 1 ./... -count=1 -timeout=180s`、根模块 vet/build、relaykit 独立
build/test、Node22 `npm run release:check` 通过；最终干净提交 `source:manifest` +
`pack:check` 通过（2183 文件）。新增 model/controller/middleware/Kling 专项 race 通过，
文本/JSON审计 common/service race 通过；统计修正后 service/Kling 两项整链 race 再通过
（2.693s / 2.670s）。这些本机结果与最终 CI 相互补充，不代替真实供应商或生产验收。

首批代码 `6bae734` 的 CI `33757378962` 七项成功，真实 Redis 7 的 36 个场景已实际
通过，原始日志无 skip、Lua/整数错误；本机根模块 vet/build、relaykit 独立构建/测试、
Node22 release:check 和新增专项 race 均通过。收尾再次核对“请求次数不变”时发现：
零差额 clamp 审计虽未改 User.RequestCount，却以 Consume 类型进入 `SumUsedQuota`
的 RPM（真实统计红测 1→2）。已修正为已有 System 类型；不改变正差额 Consume 或负
差额 Refund，不改统计 SQL。消费日志关闭时异常系统审计仍保留，且不进入消费导出。
新增真实统计、消费筛选、开/关消费日志与真实导出缓存快照回归通过；service/Kling 全量
及 vet 通过（3.386s / 2.091s），独立复审闭环。补项已由 `451bee3` 和最终 CI 复验，
不以首批七个绿色 job 代替新增统计合同的验收。System 审计可在全部/系统日志中查看，
消费筛选不包含它，普通用户始终去除 admin_info；关闭消费日志仍保留异常系统审计。

仍未改变 C03b 的缓存恢复和高层异步增减/数据库更新语义；正在请求故障策略决定。
S2-B 持久化恢复、真实付款/供应商、生产与设备验收均未包含在本批。

## S2-B1 与 S4-01

**2026-09-04 最终验收：B1、C06、S4-01 已完成当前范围。** 生产代码 `7f1913e` 的
[CI 33781560507](https://github.com/ForceMind/MyAPI/actions/runs/33781560507) 七项成功，
[Docker smoke 33781637372](https://github.com/ForceMind/MyAPI/actions/runs/33781637372)
Full/LAN 两项成功。专库 job `100736213883` 原始日志确认 MySQL5.7/PG9.6 各七支付＋
五种快照/NULL 读回实跑，无 skip、SQLSTATE 22P02 或旧中文日志错误。

两种最终镜像均为 linux/amd64、新 SQLite；安全报告全部检查通过且成功项无错误码。
实际 revision 为 `rv.0.1.1.7f1913ef8dd9fdeea66b3ce18792c758c870f78b.2k6e8r7p`。
Full image ID：`sha256:0f32aed5e44b646b7f00f2916a6222b8d4737a8695077f8a045101e1bac55572`；
LAN image ID：`sha256:cd5ce63849d3f55f3c46345a00d38092d70de4df8dcf2ce401202b7fbd700a0d`。
它们是 CI 本地 image ID，不是 GHCR digest；没有发布、生产连接或真实设备验收。
该代码提交的 Node22 完整 release:check 通过，manifest sourceCommit 精确匹配、
sourceTreeDirty=false，pack 为 2190 文件（19,984,976 字节）。

追加按最终 SHA 对齐的整合 race：logger 2.625s、model 4.545s、controller 4.633s、
service 5.792s 通过，但 Kling 3.020s 失败。原始报告指向测试清理写 RedisEnabled 与
合法异步 cache 回调读取竞争；此前 4.976s 的 Kling 通过属于上一轮，不覆盖本次失败。
测试配置生命周期已固定在进程级内存 SQLite 与 disabled cache，task/logger/Kling 定向
race 已接入 CI；不改变生产缓存恢复或异步更新策略。本机同命令四包通过：logger
2.264s、model 2.358s、service 3.705s、Kling 2.644s；独立复审通过。测试/CI 收尾
`6fd8ae4` / [CI 33783792231](https://github.com/ForceMind/MyAPI/actions/runs/33783792231)
七项成功；backend 新步骤原始日志为 logger 1.042s、model 1.461s、service 3.755s、
Kling 1.662s，均真正执行通过，无 DATA RACE/FAIL。
不把普通 CI/镜像绿灯或前一次偶然通过当本次 race 通过。

以下保留实现、失败与复验过程；其中“待验证”描述当时状态，不覆盖上述最终结论。

本轮基线 `f8aa3d8` / CI `33761897224` 七项成功。B1 只消费已有任务计费快照：提交时
存实际模型/分组/附加倍率，JSON 版本区分显式零费；完整历史快照不按新配置重算，旧
缺失记录使用明确审计的当前配置回退。非法/未知版本或不可解析的旧配置保留预扣并
System 审计；不实现响应前持久化、账务事件、outbox 或批量缓存恢复。
新增提交、SQLite/实库 JSON 往返与精确结算/日志回归；MySQL/PG 新场景最终已实跑。

B1 初始红测：快照 `2×0.5×3×100=300` 被当前配置算成 1200；修复后完整新/旧快照冻结。
独立复审追加早返回组合，红测确认未知版本可被 adaptor 把预扣 500 改成 300，并漏掉
来源审计；非有限价格还会清空 Other JSON。共享纯快照守卫已在真实轮询及 token 入口
修复这些路径，独立复审通过；新守卫专项 race 通过（service 4.272s）。
初版根模块全量/vet/build、relaykit 独立 build/test 通过；扩展 race 的 model/controller/
Kling 通过，但 service 的旧 SlowChannel 测试暴露共享 Task 读取及生产 logger.logCount
竞争，当次不能标全量 race 通过。测试同步与日志状态列为 C06，下列记录保留修复证据。

C06 已修复并独立复审通过：状态锁只保护计数/预约，自动轮转自行释放，手动 SetupLogger
不误清他人预约；原输出格式、阈值与 writer 锁不变。logger 全包 race 2.595s、全部
UpdateVideoTasks race 3.326s 通过，含两并发真实日志、I/O 阻塞隔离及临时文件轮转。
fixture 保留共同 500ms 门限，以不可变 ID/事件和后台退出后的 DB 快照消除竞争；无 sleep
或串行化绕过。主代理最终同一条整合 race 已通过：logger 1.793s、model 3.309s、controller
4.033s、service 6.561s、Kling 4.976s；这是 e7fffc2 之前的整合轮次，包含原失败轮询、B1
快照/守卫和 C02 审计；最终 SHA 的追加 race 结果及隔离补项见本节开头。

S4-01 修复现有 Docker smoke 的 healthcheck 缺失及直接运行时 session 环境变量名称，
增加串行 Full/LAN 构建、资源限制、回环端口、隔离新 SQLite 初始化、认证与实际浏览器
精确构建版本检查。仅手动构建加载本地镜像，push:false，无发布或生产连接。
Node 合成 12 项与 YAML/Bash 语法通过。首轮 `540cf32` /
[Docker smoke 33766140801](https://github.com/ForceMind/MyAPI/actions/runs/33766140801) 两种
镜像构建、健康检查、初始化/认证及登录表单就绪已执行，但最终均以
`SMOKE_FRONTEND_BUILD_MISMATCH` 失败，未验收。原生产 chunk 中 env 别名仅保留 Rsbuild
内建变量，VITE 版本/SHA 读取为 undefined；已修为可被编译替换的直接属性读取，
需要新提交的真实镜像复验。初始单测只检查 revision 格式，所以未发现此问题。
修复后本机两项元数据单测、typecheck、文件 lint、完整 64 文件/336 项 Vitest（101.96s）
通过；生产 build 6.64s，产物已包含合成 `fixture-s4-build` 字面值，替代旧缺失 env 查找。
实际产品 SHA 仍须由后续 GitHub smoke 验证。独立复审确认 local fallback、白名单/长度
限制、表单与 SHA 检查未放宽。CLI 的 runtime 脚本合同最初因新增测试 glob 未同步失败，
已同步声明并通过 runtime:check（13/13）、release:workflow:check（18/18）及运行探针 12 项。

2026-09-04 收尾：最终代码的根模块全量、vet/build 及同一条整合 race 已通过，独立代码
与文档复审闭环。使用 `GOMAXPROCS=1 GOWORK=off GOCACHE=/tmp/myapi-gocache GOMODCACHE=/tmp/myapi-gomodcache`：

```sh
go test -p 1 ./... -count=1 -timeout=180s
go vet -p 1 ./...
go build -p 1 ./...
go test -race -p 1 ./logger ./model ./controller ./service ./relay/channel/task/kling -run 'Test(ConcurrentLog|BlockedInfo|ManualSetupLogger|AutomaticRotation|NewTaskBillingContext|TaskBillingSnapshotRoundTrip|TaskJSONScan|TaskSubmissionBillingContext|Recalculate|RefundTask|Settle_|SettleTask|TaskToken|TaskFreeSnapshot|TaskBilling|TaskRecalculate|TaskPollingAudits|ParseTaskResultFinalUnitDeduction|KlingPollingPreservesDeductionSaturationAudit|CASGuarded|NonTerminalUpdate|UpdateVideoTasks|S2APaymentSQLite)' -count=1 -timeout=180s
```

`relaykit/` 的 `GOWORK=off go build -p 1 ./...` / `go test -p 1 ./... -count=1` 独立通过。
上述不替代新提交的 MySQL/PG 快照实跑、Full/LAN 镜像及完整发行包复验，提交后继续补证。

`e7fffc2` 复验结果：本机干净源码 `npm run release:check` 全链通过，manifest 的
sourceCommit 为该完整 SHA、sourceTreeDirty=false；pack 为 2190 文件。普通 CI
`33778745451` 六项通过，但专库 job `100726920898` 未通过：MySQL 七支付及五快照均通过，
PG 七支付通过、五快照 INSERT 均报 SQLSTATE 22P02，B1 未予验收。与生产一致的
simple-protocol 下，两处 JSON Valuer 的字节数组被编码为 bytea 十六进制文本；实际 GORM
参数配合同一 pgx codec 的红测已复现，空 Data/NULL 控制组正常；两处 Value 已改为成功
编码后返回 JSON 文本，不修改 schema、协议或补假数据绕过。四个受影响包完整测试
（model 9.838s、service 2.148s、controller 3.658s、Kling 1.201s）、相关 vet 及根模块 build
通过，独立复审通过。按复审建议以五场景真实 NULL 读回断言代替 SQL 括号格式断言，
SQLite/codec 复跑 1.942s 通过。修正仍待新的 MySQL/PG 实跑。

第二轮 [Docker smoke 33778810531](https://github.com/ForceMind/MyAPI/actions/runs/33778810531)
在 `e7fffc2` 的 Full/LAN 均成功，原始报告确认实际初始化/认证、匿名拒绝、模式及登录
表单，三处 revision 均为 `rv.0.1.1.e7fffc223ec9a73bae5606cfad681b9c9c795555.2k6e8r7p`。
本地 image ID（不是 GHCR digest）：Full `sha256:3990bc161879273143d9b15fdc05dde29c862403f0faad61efd1115069187d7c`；
LAN `sha256:9426fc35bcb84509772e857adc1892ff550c5bf0c6967cb8ad388ceafa041fa6`。均为 linux/amd64、
全新 SQLite，不代表三库恢复。首轮 BuildMismatch 失败记录保留。

核对该报告时发现原认证探针对成功项误附 UNCONFIRMED code，另用红测复现登录
`success:false` 即使含 token 也被接受。已修为成功无错误码、登录必须 HTTP 2xx＋success:true＋
非空 token，否则不调用管理 API；新增红绿回归后 Node 13 项、runtime 合同及独立复审通过。
新探针随 PG 修正再次提交和镜像复验，不撤销旧有效登录/镜像证据，也不将旧报告当新版通过。
它也不覆盖三库恢复、真实上游、完整 UI 或 macOS/Windows 安装。

## S4-02 合成业务链（2026-09-04，已完成当前范围）

在 S4-01 的 Full/LAN 全新 SQLite 镜像入口上增加固定 digest Bun sidecar，只运行纯合成
OpenAI 接口。应用先健康，sidecar 再共享其网络 namespace；宿主 18080/19090 均只绑定
127.0.0.1。sidecar 为只读文件系统、UID 1000、cap-drop ALL、no-new-privileges、0.25 CPU、
128 MiB、64 PID，Bun 转译缓存关闭；应用仍为 1 CPU/768 MiB，full-content 日志限制为
tmpfs 内 1 MiB×2。失败诊断只读状态/退出码/OOM，清理校验 SHA 并先移除 sidecar。

业务探针只接受固定 loopback app/fake URL，并在任何 setup 写入前拒绝缺失/非法 fake；
fake control 必须从 0 开始。root token 只更新倍率、创建合成普通用户和渠道、读取管理员
日志；18 字符随机普通用户密码和生成的 API Key 仅驻留内存。普通用户实际登录，创建
余额 1,000,000、限 `smoke-model` 的 Key，向 type=1 渠道发一次 `max_tokens=8` 非流请求。
fake 只保存 method/path/model/max_tokens/stream/Bearer 的布尔摘要，不保存原始 header/body；
返回 usage 10+5。探针精确要求用户和 Key 余额为 999,985，用户/Key/渠道 used 为 15、
request_count 为 1；普通与 root 通过同 request ID 各看到唯一 Consume 日志，普通日志无
admin_info。Full Content 普通用户为 403；root 看到 Authorization、请求 api_key 和响应
X-Api-Key 已脱敏，固定无敏感响应正文按既有合同原样保留。匿名 relay 后 fake count 仍为 1。

本机首批命令：Node22 `npm run runtime:probe:test` 为 16/16（672 ms）；固定入口另由 Bun
在 127.0.0.1:19090 启动，health 和全 false 零状态读回后立即停止。YAML、全部九个 shell
块、`runtime:check` 13/13、`release:workflow:check` 18/18、diff-check 与独立 Sol 审查通过。
这些是提交前证据，当时不替代 sidecar 容器联通、真实 API 字段及资源余量，因此没有提前
标记完成。无页面或产品版本改动，0.1.1 与受保护 tag 不动；临时镜像仍由 SHA 区分。

`b54ce36` 的普通 CI `33788688465` 七项成功。两次重复手动触发
`33788701235` / `33788890484` 均被取消且无验收结论；随后唯一运行 `33789030687` 的
Full app/sidecar 均保持 running、exit=0、OOM=false，但宿主健康请求连续 connection reset。
根因是 sidecar 在共享 namespace 只绑定 127，而 Docker publish 将宿主流量 NAT 到该
namespace 的非 loopback 接口；业务探针未执行，LAN 为避免重复消耗而取消。修复保持
本机默认 127，仅在 CI sidecar 通过受限 env 显式监听 0.0.0.0；宿主 publish 仍为
127.0.0.1，state 仅合成布尔值。Node22 更新后 17/17（491 ms）、YAML/九个 shell 块及
独立复审通过。

监听修复提交 `237c0da16ee2ced3a0f00aca700f23cdb405bb75` 的本机完整
`npm run release:check` 通过：CLI 22/22、branding 2/2、quota OpenAPI 3/3、LAN 68/68、
desktop 32/32、upgrade 18/18、runtime contract 13/13、runtime probe 17/17、release
workflow 18/18，源码 manifest 2190 项，最终包 2191 文件/20,024,635 bytes。
[CI 33790468336](https://github.com/ForceMind/MyAPI/actions/runs/33790468336) 同 SHA 七个 job
成功，包含根/relaykit vet/build/test、四包 race、前端真实 quota 图表、Redis 与 MySQL5.7/
PostgreSQL9.6 既有回归、desktop 和 distribution 合同。

[Docker 33790513455](https://github.com/ForceMind/MyAPI/actions/runs/33790513455) 同 SHA 成功：
Full job `100765715038` 为 4m21s，LAN job `100765714033` 为 4m30s；每项 build、health、
fake health、浏览器安装、完整业务探针、image identity 和清理均成功，失败诊断因无失败跳过。
两份原始安全报告均为 `passed:true`，并验证 fresh SQLite、匿名 self 401、正确 edition、
root 登录/身份/基础额度日志、精确 15 quota 的普通用户/Key/渠道账本、唯一 Consume 日志且
普通视图无 admin_info、普通用户 Full Content 403、root 请求/响应头和请求字段脱敏、匿名
relay 401 且 fake count 保持 1、真实登录表单。两者 revision 均为
`rv.0.1.1.237c0da16ee2ced3a0f00aca700f23cdb405bb75.2k6e8r7p`；Full image ID
`sha256:5bff88fc38c973f86d0966aff1539ddae5e1c658627a7bb7a9e8bf471343d1ae`，LAN image ID
`sha256:6eb322e85047c9bb6f5087717527c5b9b0e1d91cf3c631fff7fb373f539ba1ce`。S4-02 当前范围完成。
这些临时 image ID 不是发布 digest；无 Redis/batch、真实 Provider、三库恢复、本机 Docker
Desktop、真实手机/桌面安装或生产证据。

## S2-D01 OAuth JSON wrapper（2026-09-04，已完成当前范围）

Sol ultra 只读复审按根模块生产代码重新计数：排除测试、`common/json.go`、`relaykit/**`、
合法 `json.Valid` 及仅类型用途后，标准库直接 Marshal/Unmarshal/Decoder/Encoder 共
67 处、27 文件；旧 54 处计数漏掉 11 个 decoder 和 2 个 RawMessage 解码。执行按十个
互不争写的小批次进行，完整分解见[执行计划](DEVELOPMENT_EXECUTION_PLAN.md#s2-d-根模块-json-wrapper-合规)。

D01 仅修改 `oauth/github.go`、`discord.go`、`oidc.go`、`linuxdo.go`：1 个 Marshal 和 8 个
单次 decoder 分别等价改为 `common.Marshal` / `common.DecodeJson`，删除只为调用存在的
`encoding/json` import。未知字段、尾随第二个 JSON 值、错误传播、endpoint、DTO 和返回
映射不变；没有切换 strict decoder。新增 GitHub 合成 RoundTripper 回归，验证 token 请求
JSON/header、token/user 映射及宽松单值语义；全局 transport 和合成 client 配置均恢复，
不访问真实网络或凭据。

本机实际通过：`go test ./common ./oauth`（common 1.301s、oauth 0.654s）、
`go test -race ./oauth`（2.134s）、`go vet ./oauth`、低并行
`go test -p 1 ./... -count=1`。结构门禁从 67/27 降为 58/23，OAuth 四文件为 0。
独立审查无 P1/P2；保留 P3：Discord/OIDC/Linux DO 没有为机械替换复制同类协议测试，
不以重复测试制造覆盖率。最终 `6b3042ac021183111240915f00ead6fd16b5c986` /
[CI 33793219733](https://github.com/ForceMind/MyAPI/actions/runs/33793219733) 七项成功；Backend
明确通过根/relaykit vet、build、全量 test、配置发布 race 和 task billing/logging race，
其余 Frontend、S1/S2-A 实库、S2-C Redis、Desktop、Distribution 也成功。D01 当前范围完成。
无页面变更，`VERSION` 保持 0.1.1；不发布、不调用真实 Provider、不读取本机凭据。

## S2-C07 / D02 视频正文与 Provider 边界（2026-09-04，已完成当前范围）

D02 只读扫描最初只计划把 `middleware/jimeng_adapter.go`、`kling_adapter.go` 的两个
`json.Marshal` 等价替换为 `common.Marshal`；Sol ultra 数据流追踪进一步确认真实 P1：
两 adapter 首次 `UnmarshalBodyReusable` 建立旧 `KeyBodyStorage`，随后只更新直接 Body 和
旧 `KeyRequestBody`，而所有后续读取优先旧 storage。Jimeng 标准 `req_key` 与 Kling 仅
`model_name` 请求因此在 Distributor 读不到统一 `model`；Full Content 开启与否均可达。

先写红测：compat middleware 下游 `Request.GetBody` 为 nil、预期 204 实际 200；Kling
converter 实际把权威上游模型改为 `metadata-model-name`，Jimeng 四个 v3 分支实际改为
`metadata-req-key`。审查再发现 Kling `metadata.duration` 和 Jimeng `metadata.frames`
仍可绕过顶层边界，未把首轮绿色当完成。最终实现：

- `common.ReplaceRequestBody` 先创建新 storage/独立 reader，全部成功后同步 cache、Body、
  GetBody、ContentLength，再关闭旧 reader/storage；Cleanup 同时清两 key，内存和强制磁盘
  测试验证旧文件 unlink、预先打开 reader 的独立生命周期、最终统计回基线与重复清理。
- 两兼容 adapter 生成统一 envelope：通用 Task 字段在顶层，nested metadata 先复制、
  provider 顶层字段后覆盖，显式 0/false/空数组保留；模型别名和通用字段不再留在 metadata。
- Kling/Jimeng Provider 对 metadata 使用副本，并在应用其他 provider 参数后恢复权威
  UpstreamModelName、prompt 及受保护资源字段。Kling mode/duration/image 不可被 metadata
  改写；Jimeng frames 不可覆盖已验证 duration。官方顶层 frames 仅接受 121/241，按
  [火山引擎接口合同](https://www.volcengine.com/docs/85621/1791184?lang=zh) 映射为 5/10 秒。
- Kling/Jimeng 路由改为 TokenAuth→FullContentLogger→adapter→Distribute；request 保存
  客户端原始正文，request/chunk/end 共用入口 request ID、method/path、用户/Token/IP。
  标准视频路由不变，专项说明同步更新。

最终本机实际通过：受影响 `common`、`middleware`、`relay/common`、Kling、Jimeng、router
包；common/middleware 全包 race 分别 2.302s/1.956s；受影响六包及 router 的 `go vet -p 1`；
低并行 `go test -p 1 ./... -count=1` 根模块全量。结构门禁从 58/23 降至 56/21。
独立 Sol 首轮列出两项 P1 和身份/测试 P2，全部修正；第二轮无新 P1/P2，末项图片权威
P3 断言已补。最终 `f7cc5c326a725170663e035a56b3573883ab198e` /
[CI 33798508808](https://github.com/ForceMind/MyAPI/actions/runs/33798508808) 七项成功；Backend
完成根/relaykit vet、build、全量 test、配置发布 race 与 task billing/logging race，其他
Frontend、S1/S2-A 实库、S2-C Redis、Desktop、Distribution 作业也成功。C07/D02 当前范围
完成。无真实 Provider/生产数据/凭据、数据库迁移、relaykit 或页面改动，`VERSION` 仍为
0.1.1。Jimeng GET handler 候选、metadata JSON 字符串兼容、真实上游费用和完整端到端任务
仍未验证。

## S2-D03 Relay 输入 JSON wrapper（2026-09-04，已完成当前范围）

仅修改三处生产调用：OpenRouter 将 Anthropic `THINKING` RawMessage 交给
`common.Unmarshal`，仍保留 `encoding/json` 类型 import；Replicate `OutputFormat` 改用
wrapper 并删除 stdlib import；`ModelMappedHelper` 以 `rootcommon` 别名解码配置，避免与
`relay/common` 冲突。wrapper 当前直接委托标准库，因此字段、null、错误类型和数字语义不变。

新增真实 converter/helper 的 testify 表驱动回归：thinking 覆盖 enabled、缺 budget、
malformed、disabled 与非 Anthropic；Replicate 保留带空格原字符串，空/null/数字/布尔/
数组/对象/malformed 均静默忽略，ExtraFields 与 `Extra["input"]` 的原覆盖顺序不变；模型
映射覆盖直达、链式、起点/链尾自映射、真循环、malformed、空、`{}`、null 和空目标，
同时断言 request 与 RelayInfo 成功结果及现有错误时部分状态。

本机 `go test -p 1 ./relay/channel/openai ./relay/channel/replicate ./relay/helper -count=1`
分别 2.016s/1.138s/3.742s；同三包 race 分别 2.795s/2.506s/5.255s；三包 vet、gofmt、
diff-check 通过。结构门禁从 56/21 降至 53/18，独立 Sol 审查无 P1/P2。两个 P3 仅为
额外门控轴和内部调用顺序测试，不改变可观察合同。最终
`615fbbd48c2f7627d6f3dc513c97b8845ba3074e` /
[CI 33800052236](https://github.com/ForceMind/MyAPI/actions/runs/33800052236) 七项成功，Backend
包含根/relaykit vet、build、全量 test 和既有 race 门禁；D03 完成当前范围。无网络、
数据库、凭据、relaykit 或页面变更，`VERSION` 仍为 0.1.1。

## S2-D04 Provider 响应与 Vertex token（2026-09-04，已完成当前范围）

机械范围为 SiliconFlow rerank 一次 Unmarshal/一次 Marshal、Tencent 非流一次 Unmarshal、
Vertex 两个 decoder，共五处全部改用 `common` wrapper。Sol 只读审计发现不能只机械替换：
Vertex 两路径在 token 类型断言失败时把整个不可信 response map 写入 error，该错误可进入
同步 relay 或异步任务 API；字符串空 token 也会被接受并可能缓存。因此 D04 同批增加共享
`decodeAccessTokenResponse` 安全边界。

parser 先验证 response/body 与 HTTP 2xx，再用非严格 `common.DecodeJson` 解第一值；存在
provider error（包括 null 或与 token 同时出现）、malformed、missing/null、number/bool/
array/object、空或纯空白 token 一律固定失败。错误测试逐项禁止出现合成 token、provider
error、description、原始 JSON 或 map；未知字段和尾随第二个 JSON 值仍兼容，合法非空 token
原样返回，body 仍由调用者关闭。两个 exchange 直接共用该 parser，不修改 cache 或 transport。

SiliconFlow/Tencent 测试调用真实 handler，验证正常 status/header/usage/统一正文、reader/
malformed 与既有 provider error 行为；结构合法的 SiliconFlow provider error 仍保持历史
零值响应，本批不借 wrapper 改协议。全部 fixture 为内存合成值，无网络、数据库或凭据。

本机三包普通测试分别 1.775s/1.327s/1.003s，race 分别 2.560s/2.809s/2.478s；新增 Vertex
类型用例复验普通 1.844s、race 2.625s。三包 vet、gofmt、diff-check 通过，结构门禁从
53/18 降至 48/15，独立 Sol 审查无 P1/P2。P3 为非 2xx 不 drain 的错误连接复用效率，以及
未给两个 exchange 各建 transport 测试；固定安全 parser 与直接接线已覆盖当前风险。最终
`544f83b54e44e4747998297e9eb45a1ffa368489` /
[CI 33802572396](https://github.com/ForceMind/MyAPI/actions/runs/33802572396) 七项成功，Backend
包含根/relaykit vet、build、全量 test 和既有 race 门禁；D04 完成当前范围。无真实 Google/
代理/cache/JWT、relaykit 或页面变更，`VERSION` 仍为 0.1.1。

## S2-D05 Midjourney JSON wrapper（2026-09-04，已完成当前范围）

`relay/mjproxy_handler.go` 的 VideoUrls 持久化 marshal、Buttons/VideoUrls/Properties 三个
持久字段 unmarshal，以及 SwapFace、ImageSeed、单任务、条件列表四个 response marshal
共 8 处迁移到 `common` wrapper，删除 `encoding/json`。Notify 的 marshal error 继续忽略；
malformed 持久字段继续分别静默；四个 response 失败仍返回历史误命名字符串
`unmarshal_response_body_failed`。

新增 testify 回归不只检查“可执行”：精确断言 ActionButton 的 0/false/空/null、两个非 nil
空 slice、Properties=`null` 仍得到非 nil 零值指针；Buttons、VideoUrls、Properties 各自
malformed 时另外两个合法字段不受影响。隔离 SQLite 单连接并精确恢复 `model.DB`，真实调用
Notify 验证 `videoUrls:[]` 存为 `[]`；真实 Task handler 验证单项为 camelCase object、条件
查询为 array、空 IDs 精确为 `[]`，三者 Content-Type 均为 JSON。

本机定向 handler 测试 1.259s、relay 全包普通 0.561s、race 1.888s，relay vet、gofmt、
diff-check 通过；结构门禁从 48/15 降至 40/14。独立 Sol 审查无 P1/P2；剩余 P3 仅为
JSON-safe DTO 的 marshal error 无法自然触发，未为此引入生产测试钩子。最终
`b2b60fd22432ba01d24eeff01e1e2b96a05d0d23` /
[CI 33804146311](https://github.com/ForceMind/MyAPI/actions/runs/33804146311) 七项成功，Backend
包含根/relaykit vet、build、全量 test 与既有 race 门禁；D05 完成当前范围。无网络、真实
渠道、计费、转发 URL、relaykit 或页面变化，版本仍 0.1.1。

## S2-D06 Controller JSON wrapper（2026-09-04，已完成当前范围）

`controller/channel.go` 五处、`model_meta.go` 两处、`uptime_kuma.go` 一处实际编解码改用
`common.Marshal/Unmarshal/DecodeJson`。`channel.go` 的两个 `json.Valid` 和 RawMessage 类型
按规则保留；另两文件删除 stdlib JSON import。Ollama progress/error/success 仍忽略不可达
marshal error，SSE frame、flush 与 `[DONE]` 不变。

只读检查确认规则模型 endpoint union 来自 map，原 JSON 顺序不稳定；本批在序列化前按
EndpointType 字符串排序，不改变去重集合。新增 testify 离线测试精确覆盖 Vertex array key
的字符串 trim、空过滤、object/array/0/false/null 表达和非法/非数组/空错误；Uptime 使用
实例级 RoundTripper 验证 GET、200、未知字段/尾随值、malformed、非 200、transport error
与 body close，不修改全局 client。

本机定向测试 2.292s、Controller 全包 3.259s、race 9.101s，Controller vet、gofmt、
diff-check 通过；结构门禁从 40/14 降至 32/11。独立 Sol 审查无 P1/P2；P3 是没有单列
endpoint enrich 与 Ollama SSE 集成测试，现由直接确定性排序、机械等价和全包回归覆盖。
最终 `02a0aaf0cb761bdaab44dbe1ff786039960cbc63` /
[CI 33805743908](https://github.com/ForceMind/MyAPI/actions/runs/33805743908) 七项成功，Backend
包含根/relaykit vet、build、全量 test 与既有 race 门禁；D06 完成当前范围。无外网、
数据库、relaykit 或页面变化，版本仍 0.1.1。

## S2-C08 / D07 Settings（2026-09-04，已完成当前范围）

本批 Settings 五个目标文件 13 处直接 JSON 编解码全部迁移至 `common` wrapper，根模块合规余量
由 32 处/11 文件降为 19 处/6 文件。除机械迁移外，实际修复和回归覆盖 RWMap/10 倍率、
Chats/UserGroups/AutoGroups/PayMethods 的 fresh 发布、负倍率及 NaN/Inf 防护、模型在访问 DB
前完整验证与批量 rate 聚合、generic config 的全对象失败原子性/纯验证、Claude/Gemini 既有
语义、ConfigManager registry 回调锁、集合 `null` 规范化和 exposed cache generation。

模型成功请求限流以单请求配置快照运行，防止窗口计算溢出；Enabled 与 0 分钟组合拒绝启用；
仅成功请求记录额度，内存成功不再计入失败；大窗口不巨额预分配。动态窗口改为每个 key
独立过期，配置 count 缩小时同步 prune。内存与 Redis 的成功限额仍是
check→execute→record 的近似语义，并发可超发，未误报为硬限额。

本机真实通过：受影响包普通测试；最终 CI 同款 race（`common`、`types/config`、
`model_setting`、`operation_setting`、`ratio_setting`、`setting/model`、`middleware`、
`controller`）；限流 race `-count=2`；根 `go test -p 1 ./...`、`go vet ./...`、
`go build -p 1 ./...`；`relaykit` `GOWORK=off` vet/build/test；gofmt、diff-check 与 YAML
解析。独立 Sol 初审问题修复后复审无当前范围 P1/P2。最终 `2d6acab` /
[CI 33814136556](https://github.com/ForceMind/MyAPI/actions/runs/33814136556) 七项成功；Backend 原始日志中新增 D07 race
步骤的 `common`、`types`、五个 `setting` 相关包、`model`、`middleware`、`controller`
均实际返回 `ok`。不得把这些证据写成发布验收；本批无 schema/前端页面变化，
`VERSION` 仍为 0.1.1，未运行真实上游、生产或真实设备。同 SHA 常规 MySQL/PostgreSQL jobs 成功；
本批无专用三数据库 Settings 行为场景，不会把其绿灯写成热更新验收。

剩余独立 S2-C09：generic config 对象业务热读缺统一快照/锁；跨不同配置族 reload 仍为
best-effort 而非全量事务；历史 DB raw `null`、未知分层 key、Passkey 懒写和
`GroupRatioSetting` 可变指针待审计。C09 只读设计已完成、实施仍独立保留；D08 已完成当前范围，
D09 下一批，D10 其后。

## S2-D08 io.net 核心（2026-09-04，已完成当前范围）

`pkg/ionet/client.go` 与 `pkg/ionet/jsonutil.go` 的各 4 处实际 stdlib JSON 调用已等价迁移至
`common` wrapper，根模块结构余量由 19 处/6 文件降为 11 处/4 文件。无网络 fake client 覆盖请求 body、
headers、method、URL，NaN marshal，transport/API detail fallback，query slices、HTML escape、空值、
零值、false、`time.Time` 和 `*time.Time`；flexible time 覆盖对象/数组、直接或 `data` 包装、无时区 UTC、
带时区 offset、未知/普通字符串、malformed、错误类型与尾随值。

普通测试、race `-count=2`、vet、gofmt 和 diff-check，根模块全量 test/vet/build 及 relaykit
独立 vet/build/test 已通过。最终 `9193ada` /
[CI 33816756504](https://github.com/ForceMind/MyAPI/actions/runs/33816756504) 七项成功。独立 Sol 审查无 P1/P2；数组和 `*time.Time` 的两个
P3 已补。保留 `interface{}`→`float64` 大整数精度风险和递归识别看似时间字符串的既有语义；endpoint path
逃逸、nil response 等 D09 边界另审。无页面/schema、真实 io.net、凭据或网络访问，`VERSION` 保持 0.1.1。

## 最近 CI 证据

- S2-D08 最终提交 `9193ada`：[CI 33816756504](https://github.com/ForceMind/MyAPI/actions/runs/33816756504)
  七项成功；io.net 核心 8 处 wrapper 和离线 HTTP/flexible-time 回归闭环，余量 11/4。
- S2-C08/D07 最终提交 `2d6acab`：[CI 33814136556](https://github.com/ForceMind/MyAPI/actions/runs/33814136556)
  七项成功；Backend 全量、旧配置顺序 race、新 D07 十包 race 和既有计费/日志 race 均成功，余量 19/6。
- S2-D06 最终提交 `02a0aaf`：[CI 33805743908](https://github.com/ForceMind/MyAPI/actions/runs/33805743908)
  七项成功；Controller 8 处 wrapper 与 endpoint 稳定排序闭环，余量 32/11。
- S2-D05 最终提交 `b2b60fd`：[CI 33804146311](https://github.com/ForceMind/MyAPI/actions/runs/33804146311)
  七项成功；Midjourney 8 处 wrapper、持久化与响应形状闭环，余量 40/14。
  文档提交 `0818ea1` / [CI 33804869951](https://github.com/ForceMind/MyAPI/actions/runs/33804869951)
  也为七项成功。
- S2-D04 最终提交 `544f83b`：[CI 33802572396](https://github.com/ForceMind/MyAPI/actions/runs/33802572396)
  七项成功；五处 wrapper 和 Vertex token 安全边界闭环，余量 48/15。
  文档提交 `30d1d13` / [CI 33803224195](https://github.com/ForceMind/MyAPI/actions/runs/33803224195)
  也为七项成功。
- S2-D03 最终提交 `615fbbd`：[CI 33800052236](https://github.com/ForceMind/MyAPI/actions/runs/33800052236)
  七项成功；OpenRouter/Replicate/model mapping wrapper 与行为回归闭环，余量 53/18。
  文档提交 `4e71ec2` / [CI 33800837776](https://github.com/ForceMind/MyAPI/actions/runs/33800837776)
  也为七项成功。
- S2-C07/D02 最终提交 `f7cc5c3`：[CI 33798508808](https://github.com/ForceMind/MyAPI/actions/runs/33798508808)
  七项成功；正文缓存、视频模型/时长边界、原始日志身份和 wrapper 余量 56/21 已闭环。
  文档提交 `3133495` / [CI 33799035547](https://github.com/ForceMind/MyAPI/actions/runs/33799035547)
  也为七项成功。
- S2-D01 最终提交 `6b3042a`：[CI 33793219733](https://github.com/ForceMind/MyAPI/actions/runs/33793219733)
  七项成功；OAuth 9 处 wrapper 清零，结构余量 58/23，定向/race/全量和独立复审均通过。
- S4-02 最终提交 `237c0da`：[CI 33790468336](https://github.com/ForceMind/MyAPI/actions/runs/33790468336)
  七项成功；[Docker 33790513455](https://github.com/ForceMind/MyAPI/actions/runs/33790513455)
  Full/LAN 两项均成功，精确 SHA、revision、image ID 与安全业务报告已核对。
- 测试隔离与 CI 门禁 `6fd8ae4`：[CI 33783792231](https://github.com/ForceMind/MyAPI/actions/runs/33783792231)
  七项成功；新增 task/logger/Kling backend race 四包实跑，原始日志已核对。生产代码未再变。
- 最终修复 `7f1913e`：[CI 33781560507](https://github.com/ForceMind/MyAPI/actions/runs/33781560507)
  七项成功，MySQL/PG 各五快照与七支付实跑且原始日志已核对；
  [Docker 33781637372](https://github.com/ForceMind/MyAPI/actions/runs/33781637372) 两种 edition
  真实探针成功，精确 SHA/revision/image ID 已记录。B1/C06/S4-01 当前范围完成，不代表剩余路线完成。
- 首批实现 `e7fffc2`：[CI 33778745451](https://github.com/ForceMind/MyAPI/actions/runs/33778745451)
  六项成功、PG 快照 JSON 写入失败；同提交 Docker `33778810531` 两项成功。
  原失败由上项修复，不以镜像绿灯代替 PG 验证。

- S4-01 测试入口 `540cf32`：[普通 CI 33766050871](https://github.com/ForceMind/MyAPI/actions/runs/33766050871)
  七项成功；同 SHA 的 [Docker smoke 33766140801](https://github.com/ForceMind/MyAPI/actions/runs/33766140801)
  两种 edition 均因真实构建标识不匹配失败。两类证据不可混用，修复后的新提交尚待 CI/镜像验证。
- S2-C 文档收尾 `f8aa3d8`：[CI 33761897224](https://github.com/ForceMind/MyAPI/actions/runs/33761897224)
  七项成功，作为 B1/S4-01 开始基线，不证明新增代码与镜像测试已通过。

- S2-C 统计补项 `451bee3`：[CI 33760303619](https://github.com/ForceMind/MyAPI/actions/runs/33760303619)
  七项成功，最终 Redis 36 场景实跑且原始日志已核对；完成 C01/C02/C03a/C04/C05，C03b 保留。
- S2-C 首批 `6bae734`：[CI 33757378962](https://github.com/ForceMind/MyAPI/actions/runs/33757378962)
  七项成功，但随后真实统计红测发现审计-only日志增加RPM；该回归已由上项关闭。

- R1 文档收尾 `2d705a7`：[CI 33752536294](https://github.com/ForceMind/MyAPI/actions/runs/33752536294)
  六项成功，作为 S2-C 开始基线，不代表当前未提交的 C 系列修复已通过 CI。

- S2-A-R1 `767b17b`：[CI 33751536908](https://github.com/ForceMind/MyAPI/actions/runs/33751536908)
  六项 success；实库两种数据库各七场景均含准确日志/中文读回断言，无 skip 或旧日志错误。
  此结果支持 S2-A/A-R1 当前确认范围验收，不代表 S2-B/C、S3–S7 或生产验收完成。

- S2-A 代码 `2777021`：[CI 33749764180](https://github.com/ForceMind/MyAPI/actions/runs/33749764180)
  六项 success；但实库原始日志发现 MySQL 中文日志写失败且缺断言，详见上节。
  这是当时“CI 绿但验收未通过”的记录，不得省略警告；其缺口现由上项 R1 关闭。

- S2-A 开始基线 `c82d0f1`：[CI 33744326429](https://github.com/ForceMind/MyAPI/actions/runs/33744326429)
  五项成功。已确认 A01–A06，目前修改待验收；此旧运行不证明本轮代码、实库或真实付款通过。

- R1 交付 `8dfcfba`：[CI 33743669737](https://github.com/ForceMind/MyAPI/actions/runs/33743669737)
  五项成功，backend 新增配置发布顺序 race 步骤已实际执行通过。该结果及本机完整回归、
  独立复审支持 S1-R1 在确认范围内完成；不推导跨实例一致性或 S2-S7 完成。

- R1 开始基线 `6fabc98` 的 [CI 33740901999](https://github.com/ForceMind/MyAPI/actions/runs/33740901999)
  五项通过。这是用户确认并开始 S1-R1（配置保存/后台重载发布顺序）时的记录；该旧 CI 不作为
  R1 新修改已验证的证据。原 S1-01..06 完成状态不变。

- S0＋S1 交付代码 `dc94e81`（2026-09-03）：[CI 33740321133](https://github.com/ForceMind/MyAPI/actions/runs/33740321133)
  五个 job 全部成功；相比旧基线新增 S1 临时数据库实跑。详情和先失败后修复记录见本页
  S0＋S1 章节；原六项与追加待确认风险分开，不代表整项目或生产完成。

- 当前重新核对基线 `a36e529`（2026-09-03）：CI `33721694305` 的 Backend、Frontend、
  Desktop、Distribution 四个 job 均成功，包含真实构建的额度浏览器 fixture、截图工件；
  官网检查 `33721694330`、官网工件 `33721694316` 成功。以下 Billing/runner `steps: []`
  为历史记录，不再是当前阻塞。该 CI 不替代 Docker、三数据库恢复、真设备或生产验收。

- `33377504590`（提交 `4151c41`，2026-08-31）仍在 GitHub Actions runner 启动前失败：
  Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`，约 5 秒内结束。
  该结果继续按 GitHub Billing/runner 外部阻塞处理，不能据此判断当前文档提交或源码失败；
  runner 恢复后只需重跑最新提交。
- `33338149233`（提交 `a9dcbae`）、`33338069839`（提交 `2abb2e1`）、`33337364868`（提交 `f487ac0`）、`33337161936`（提交 `3bae897`）和 `33336527035`（提交 `eae3d30`）在 GitHub Actions runner 启动前失败：四个作业均无 steps，无法据此判断代码失败；需待 runner 恢复后重新运行同一提交。
- `33339052745`（提交 `149155b`）仍在 runner 启动前失败：Backend、Frontend、Desktop
  和 Distribution 四个作业均为 `steps: []`；这不能作为代码失败证据，需待 runner
  恢复后重新运行当前提交。
- `33339157977`（提交 `cf47ea7`）继续呈现相同 runner 启动前失败：四个作业均为
  `steps: []`，因此仍需 runner 恢复后重新运行当前提交。
- `33341868806`（提交 `f4fc17e`）仍是同一外部故障：Desktop、Backend、Frontend、
  Distribution 四个作业均在启动后立即失败且 `steps: []`，不能据此判定代码失败；
  本轮已用受限本机/Docker 回归替代验证，待 runner 恢复后仍应重跑远端 CI。
- `33342523430`（提交 `4c2267c`）及同期官网运行 `33339379741` 的检查注释已明确
  为账户付款失败或 spending limit 阻断；四个作业均 `steps: []`、无 runner，根因在
  GitHub Billing & plans，不是 workflow 或代码。额度恢复后只重跑最新提交，避免重跑历史。
- `33344526525`（提交 `1695688`）仍在 runner 启动前失败：Backend、Frontend、Desktop、
  Distribution 四个作业均为 `steps: []`；当前仍应按 GitHub Billing & plans 外部阻塞处理，
  不将其视为代码测试失败。
- `33344621039`（提交 `51f3fec`）及同提交的官网 workflow 均在 runner 启动前失败，
  CI 四个作业和官网检查/浏览器 smoke 均无执行 steps；继续按同一 Billing 外部阻塞处理。
- `33351658149`（提交 `94a9eba`）在本轮自动触发后仍呈现相同状态：Frontend、Backend、
  Desktop 和 Distribution 四个 job 均 `steps: []`，在启动阶段失败；该结果不能作为
  代码失败证据，需 GitHub Billing/runner 恢复后只重跑最新提交。
- `33352270443`（提交 `4524224`）和 `33352740848`（提交 `085e475`）继续呈现同一
  外部启动故障：Frontend、Backend、Desktop 和 Distribution 四个 job 均为 `steps: []`，
  无可用 job 日志。应按 GitHub Billing/runner 阻塞处理，不能视作代码失败；恢复后只重跑
  最新提交。
- `33356196350`（提交 `a56966f`）及其前序 `33356139117`、`33355935758`、
  `33355371586` 仍在 runner 启动阶段失败；最新 CI 的四个 job 均为 `steps: []`，
  约 2 秒内结束。该结果继续按 GitHub Billing/runner 外部阻塞处理，本轮以本机
  前端 62/278、Go 回归和发行合同检查作为替代证据，不重跑历史 workflow。
- `33357189957`（提交 `f04f97b`，2026-08-31）仍为同一外部启动故障：Backend、Frontend、
  Desktop 和 Distribution 四个 job 均为 `steps: []`，约 2 秒内结束；不能据此判断本轮
  文档提交或源码失败，待 GitHub runner/Billing 恢复后只需重跑最新提交。
- `33358768662`（提交 `7479b2a`，2026-08-31）继续呈现同一外部启动故障：Backend、
  Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`，约 3 秒内结束；当前仍以
  本机资源受限回归作为替代证据，不将该 CI 红灯归因于源码。
- 2026-08-31 历史记录：本机运行副本曾使用 `local/new-api:myapi-9dc11d4`，容器
  `new-api` healthy，回环 `GET /api/status` 返回成功且版本 `0.1.1`；该记录对应当时的
  只读复核，已由下方的迁移修复和新镜像演练更新。
- `33335484167`：完成度矩阵一致性修正后的完整 CI，Backend、Frontend、Desktop
  和 Distribution 四个作业全部成功。
- `33335299578`：本机部署旧镜像诊断证据提交后的完整 CI，Backend、Frontend、
  Desktop 和 Distribution 四个作业全部成功。
- `33334940514`：安装环境导出安全修复、官网 metadata 和 LAN 注入回归后的完整 CI，
  Backend、Frontend、Desktop 和 Distribution 四个作业全部成功。
- `33334940499`：同一提交对应的静态官网 Chromium smoke，成功。
- `33334175656`：完成度证据矩阵提交后的完整 CI，Backend、Frontend、Desktop
  和 Distribution 四个作业全部成功。
- `33333993651`：README 多语言导航更新后的完整 CI，全部成功。
- `33333825592`：README 导航与合同更新后的完整 CI，全部成功。
- `33332856633`：静态官网 Chromium smoke，成功。

- `48abce6`：新开发环境的 compose 与 make 默认 PostgreSQL 数据库标识切换为
  `myapi`；保留显式变量覆盖旧数据库的路径，不代表既有生产数据库已迁移或重命名。
- `957d4ca`：Electron 生产后端探针改为请求 `/api/status`，并仅接受 2xx；后续
  `94707b0` 增加 JSON `success=true` 校验，避免业务失败响应误判为就绪。
  HTTP 状态；对应 runtime-config、探针合同测试和 desktop check 已纳入源码证据。
  这些合同证据不等同于 macOS/Windows 实机安装、局域网请求或生产运行验证。
- `a2528a2`：额度概览和渠道额度面板恢复统一的 `401` 认证刷新路径，并将用户
  ID、session SID 与权限能力纳入查询缓存 key；新增跨登录身份回归测试，避免
  同一标签页复用上一会话的额度数据。该修复不绕过服务端权限检查。
- `cb419c1`：CLI `up`/`upgrade`/`doctor` 与 installer 统一拒绝未显式允许的
  非回环 LAN 监听；installer 同时严格校验 Full edition HTTPS origin、LAN 私网
  origin 和至少 48 字符的 `SESSION_SECRET`。新增 CLI 回归 21/21、LAN Lite
  合同 66/66，并为 NPM workflow 增加旧 `v0.1.0` tag 保护；概览额度汇总按
  provider `source`/`plan_type` 隔离，避免不同账户语义混算。
- `0e77a85`：渠道页移动端关闭固定高度表格，避免额度面板与渠道列表形成
  嵌套滚动或内容裁剪；该提交当时的前端有界回归为 59 个测试文件、266 个测试（历史证据）。
- `1827358`：Claude adaptor 增加 nil/base URL 防护和默认 JSON/Anthropic 版本头
  测试；Antigravity transport 固定 dynamic agent/continuation 字段边界、限制
  interaction 响应大小、支持 `requires_action` 终态并保持错误正文脱敏。Go 回归已在
  有界 Docker 容器中执行：`GOWORK=off go test ./relay/channel/gemini ./relay/channel/claude`
  通过（gemini 0.140s、claude 0.018s）。
- `8a7741a`：PR 质量检查补齐 anti-slop 所需的最小 PR/issue 写权限，并监听
  `pull_request_target.synchronize`，确保后续提交重新审查；未授予 contents 写权限。
- `618327c`：LAN 通配监听移除 `<private-LAN-IP>` 占位，按 RFC1918 IPv4
  地址发现并输出候选端点；补齐五个非中文 locale 的额度/账户/访问方案关键文案，
  并加入 locale key parity 回归测试（5/5）。
- `24a44f1`：Docker、Release、NPM workflow 和 release-state 统一保护已存在的
  `v0.1.0`/`v0.1.1` tag，并把 SemVer tag 自动 GHCR 构建、Full/LAN 仓库和版本固定
  拉取纳入 release contract（15/15）。
- `f965e04`：Codex 额度采样不再把 2xx 登录页/无 rate_limit JSON 误计为成功样本，
  新增 unsupported 分类回归；Go 测试因依赖下载资源限制未宣称通过。
- `3338846`：Full Content Logs 查询缓存加入 user/session 身份隔离，新增 query-key
  回归；相关 Vitest 5 文件/14 测试、tsgo 均通过。
- `9871564`：Codex WHAM usage/reset/consume 响应统一限制为 1 MiB，并以额外 1 字节
  探测超限；边界测试覆盖三条接口。gofmt 通过，Go 测试因依赖下载资源限制未宣称通过。
- `5cfb046`：Electron 托盘和 LAN 状态对 `0.0.0.0` 展开实际 RFC1918 IPv4 端点，
  无候选时显示明确提示并移除占位符；Desktop 合同更新为 32/32。
- `bdf1d62`：旧的未过期管理员会话若缺少 `admin_permissions`，认证引导现在会
  强制进入一次服务端 refresh，避免额度变化面板因旧权限快照被误判为无权限；新增
  `requiresCapabilityRefresh` 回归测试，明确的空权限矩阵仍按拒绝处理。相关前端测试
  （认证会话与额度面板）21/21、`tsgo -b` 和格式检查均通过。

CI 运行号会随新提交变化；发布前应重新查询当前提交对应的运行结果，不应永久依赖
上述历史编号。

## 当前源码合同复核（2026-08-31）

### 当前工作树阶段证据（2026-08-31）

- 本阶段提交为 `a620246`（基于 `HEAD=9b65ac4`）；代码、测试与文档已写入本地提交，远端同步状态需以本阶段 push 后的核对为准。
- `deploy/install.sh` 已移除 macOS 系统 Bash 3.2 不支持的 `${var,,}` 展开，并将 `MYAPI_PORT` 限制为 `1..65535`；`bash -n deploy/install.sh` 通过。
- 本机合同回归：CLI 22/22、LAN Lite 68/68、Desktop 32/32、Upgrade 18/18、Runtime probe 13/13（测试 4/4）、Release workflow 16/16、Brand 2/2；Website 静态检查通过。
- 本机前端回归（Node 22）：typecheck、Vitest 62 个测试文件/280 个测试和 production build 全部通过。Node 26 的 localStorage 不兼容只属于不符合项目要求的运行环境，改用 Node 22 后未重现。
- 本机 Go 回归：`GOWORK=off go test ./... -count=1` 通过；额度/渠道与 Gemini/Claude 定向回归通过；`cd relaykit && GOWORK=off go build ./...` 通过。计费饱和回归覆盖正常值、溢出、无穷与 NaN 输入。
- 上述均为源码、合同和本机受限资源验证；尚未替代 Docker Desktop Compose 实跑、SQLite/MySQL/PostgreSQL 副本恢复、真实管理员手机、macOS/Windows 安装、局域网/防火墙和生产环境验收。
- 本阶段没有执行 push、tag、GHCR/NPM 发布、生产重启或生产数据操作。GitHub Actions 仍可能受 runner/Billing 启动阶段故障影响；恢复后应只重跑最新提交。NOTICE/法律审查和正式版本号仍需负责人确认。
- 阶段提交 `e0ca670` 推送后的 CI run `33397392112`（2026-08-31）中，Frontend、Desktop、Backend 和 Distribution 四个 job 均在 runner 启动阶段失败且 `steps: []`；按既有规则归类为 GitHub Billing/runner 外部阻塞，不归因于源码。恢复后只需重跑最新提交。
- 随后的文档同步提交 `52f1d03` 对应 CI run `33397495873` 仍为同一启动阶段故障，四个 job 均为 `steps: []`；继续按 GitHub Billing/runner 外部阻塞处理，不修改无关源码。
- 阶段 2 的 macOS/Docker/发行合同只读审查未发现新的可直接修复缺陷；Docker Desktop/Compose、真实 LAN 启动、健康检查和数据库演练仍待本机安装与外部验收。
- 当前执行批次已把代码优先目标写入总体计划；本机尝试安装 Docker Desktop 时下载长时间无进度后中止，Docker/Compose 仍视为未安装的外部环境阻塞。
- 在干净提交 `54fe197` 上重新生成被忽略的 `SOURCE_MANIFEST.json`，`npm run pack:check` 通过（2119 个文件，19,238,938 bytes）；清单只作为发布前证据，不代表已执行 NPM/GHCR 发布。
- 新增 `.github/workflows/docker-smoke.yml`：GitHub runner 使用 `push: false`、不登录 GHCR 的 Buildx 构建临时 LAN 镜像，启动隔离 SQLite 容器并检查 `/api/status`；`tools/release/check.mjs` 已增加对应静态合同。该 workflow 仅用于测试，不会创建 tag 或发布镜像，首次真实运行需等待 GitHub runner/Billing 恢复。
- Docker smoke 手动运行 `33403818364`（提交 `0e4f912`）在 runner 启动阶段失败，唯一 job 为 `steps: []`；已将 workflow 限定为 `workflow_dispatch`，避免普通文档/代码 push 重复触发同一外部阻塞。恢复 runner 后再手动重跑。
- JSON wrapper 阶段提交 `83ba011` 将 common 工具、额度告警和 model JSON 持久化路径统一到 `common/json.go`，新增 strict decoder 并保留原有 `JsonRawMessageToString` 回归；完整根 Go 回归与 relaykit 独立构建通过。
- Provider 阶段提交 `6b160c8` 修复 Midjourney 上传响应 fallback 丢失结果、Tencent 签名 payload 错误被忽略和 Cohere JSON wrapper 绕过；`service`、Tencent、Cohere 定向回归及完整根 Go 回归通过。
- Provider 阶段推送后的 CI run `33404395878`（提交 `46697c6`）仍在 runner 启动阶段失败，Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`；不归因于本轮源码。
- 迁移/Provider 阶段提交 `b90a342` 对应 CI run `33406547782` 延续同一 runner 启动故障，四个 job 均为 `steps: []`；待 Billing/runner 恢复后只重跑最新提交。
- JSON helper 增量提交 `f6655ca` 将 `common.Any2Type` 的序列化/反序列化改用项目 wrapper，并保留 `0`、`false`、嵌套值和错误传播回归；common/model 测试通过。
- 迁移与 Provider 增量提交 `2fab8a3`：SQLite 旧 `subscription_plans` 表新增必需列时使用兼容默认值并支持重复迁移；`migrateDBFast` 改为串行并补齐 Casbin/Authz 模型；Cloudflare/Dify 业务 JSON 统一走 wrapper，Dify 附件字段、上传失败和 SSE 错误不再静默吞掉。完整根 Go 回归、model/cloudflare/dify 定向测试与 relaykit 独立构建通过。
- model 锁/迁移增量提交 `5ec3822`：`lockForUpdate` 按实际 `tx.Dialector` 选择方言，避免全局配置误加 SQLite `FOR UPDATE`；`migrateSubscriptionPlanPriceAmount` 传播 DDL 错误，防止迁移失败后继续启动；model 全量回归和完整根 Go 回归通过。
- Dify/迁移增量提交 `f201f6d`：Dify 缺失 usage 时仅按实际输出文本估算，不再按 reasoning 事件数虚构 token；完整上游 usage 保持不被额外增加；迁移转换 helper 按实际连接方言执行，model_sync JSON 走 wrapper。Dify、controller、model 和完整根 Go 回归通过。
- Controller JSON 增量提交 `791ed6c`：io.net 部署测试与 Creem 支付 products/checkout 路径改用项目 JSON wrapper，保留原错误语义；controller 完整回归通过。
- 邮箱一致性阶段提交 `9801b57`：新增 nullable `users.email_normalized`，迁移先回填并检测含软删除记录的冲突/超长值，再创建可重复的唯一索引；创建、更新、支付/OAuth 内部 map 更新和解绑同步规范化值，读路径保留缺列时的旧库回退。model 全量与完整根 Go 回归通过；MySQL/PostgreSQL 实库并发和恢复演练仍待外部副本验证。
- 邮箱一致性阶段推送后的 CI run `33413426111`（提交 `2ff9f3e`）四个 job 均在 runner 启动阶段失败且 `steps: []`；继续按 GitHub Billing/runner 外部阻塞处理。
- 渠道测试 billing 阶段提交 `e528d34`：`settleTestQuota` 使用 checked quota 转换，饱和值拒绝写入并通过集中化 helper 保留 admin-only 审计标记；controller 与完整根 Go 回归通过。该路径仍不替代真实上游/生产计费验收。
- Claude billing 阶段提交 `f004bd6`：统一 `max_tokens`/`max_tokens_to_sample` 有效值，限制默认值与 thinking budget 比例，拒绝 thinking 请求中低于 1280 的显式上限，并防止超界配置绕过 validator；relay、setting、relaykit 及完整根 Go 回归通过。
- 图片计数边界阶段提交 `f53ce34`：MiniMax 与 Vertex Imagen 适配器复用 `dto.MaxImageN`，拒绝超限/负数/非整数/Inf/NaN，并保留合法 0/默认值语义；Provider 定向与完整根 Go 回归通过。
- Baidu/Coze JSON 阶段提交 `b265e6f`：Provider 响应、SSE 和访问令牌路径统一使用项目 JSON wrapper，malformed 响应不会继续输出或进入 usage 处理；Baidu/Coze 定向与完整根 Go 回归通过。
- 数据库并发/控制器增量提交 `2ae74db`：规范化邮箱锁与可用性检查统一使用 `LOWER(email)`，快照去重索引在 MySQL 并发创建时回检并传播真实错误，io.net 部署测试请求改用 JSON wrapper；model/controller 定向与完整根 Go 回归通过。
- 代码优先阶段提交 `9427656` 修复并覆盖了 OpenRouter cache-create quota 饱和、topup ratio 原子更新与有限值校验、Gemini Imagen `N` 边界以及图片 token 面积/最终 quota 转换；`go test ./common ./service ./controller ./relay/channel/gemini`、完整根 Go 回归与 `cd relaykit && GOWORK=off go build ./...` 均通过。
- 同一阶段的 Node 合同复核：LAN Lite 68/68、Desktop 32/32、Upgrade 18/18、Release workflow 17/17；Docker workflow 版本写入已断言为无 `v` 的 SemVer。真实 Docker Compose、数据库副本和跨平台设备仍未验证。
- 代码阶段提交 `d0cd478` 推送后的 CI run `33402289671`（2026-08-31）仍在 runner 启动阶段失败，Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`；继续按 GitHub Billing/runner 外部阻塞处理。

- 当前 `02bcc16` 增量复核：在 `web/` 以
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 完整运行前端回归，62 个测试
  文件、279 个测试全部通过（约 125 秒）。本条只更新当前提交的前端证据；CLI、品牌、官网、
  LAN Lite、Desktop、Upgrade 和打包合同仍以各自最近一次明确标注的成功运行作为证据，不能
  用本地前端测试替代这些合同或真实设备/副本验收。
- 当前 `02bcc16` 合同增量复核：`npm run lan:check -- --skip-docker` 通过 66/66，
  `npm run desktop:check` 通过 32/32，`npm run upgrade:check -- --json` 通过 18/18。
  LAN 检查跳过了 Docker Compose 解析，三项结果均不能替代真实跨平台安装、局域网请求、
  防火墙或脱敏数据库升级/恢复演练。
- 当前 `35ed22b` 后复核：`npm run release:workflow:check` 通过 16/16，确认语义版本
  Tag 自动触发 Full/LAN GHCR、既有 Tag 拒绝覆盖、manifest 使用已校验的不可变 digest，
  以及发布闸门和构建超时/并行度约束仍然生效；这不等于真实 GHCR 拉取或发布操作已执行。
- 审计提交 `ac2a1d8` 的发行包复核：在允许 Node 子进程的受限环境中重新运行
  `npm run source:manifest && npm run pack:check`，清单记录当前源提交，包含 2110 个文件；
  打包检查通过（2111 个文件，19,180,475 bytes），未发现敏感文件、构建目录或凭据模式。
- 当前工作树快照幂等增量：`ChannelQuotaSnapshot` 使用可迁移的 nullable `dedupe_key`
  唯一索引，并在并发唯一冲突时回读赢家记录；模型定向回归在 `--cpus=1.5 --memory=3g`
  的 Go 容器中通过；随后完整 `GOWORK=off go test ./model -count=1` 也在同样资源限制下
  通过（约 7 秒），额度历史/同步/Codex quota 相关的 `GOWORK=off go test ./controller
  -run "ChannelQuota|CodexQuota|Quota" -count=1` 也通过（约 0.19 秒）。旧数据的完整字段
  查询仍作为兼容回退。
- 访问方案注册表增量复核：`setting/access_profile.go` 的 JSON 解析统一使用
  `common.UnmarshalJsonStr`，不再绕过项目 JSON wrapper；`GOWORK=off go test ./setting ./model
  -run "AccessProfile|AccountTier" -count=1` 在资源受限 Go 容器中通过。

- 在代码提交 `02bcc16` 的验证时点，`HEAD` 与 `origin/main` 已核对为同一提交；上述
  前端、发行合同、清单和访问方案回归证据均对应该提交。`SOURCE_MANIFEST.json` 仍是
  被忽略的生成文件，发布前应在最终版本提交上重新生成，不要将其加入 Git。

- 推送后的 CI `33359526462`（提交 `d422511`，2026-08-31）仍在 runner 启动阶段失败：
  Backend、Frontend、Desktop 和 Distribution 四个 job 均为 `steps: []`，约 3 秒内结束。
  该结果与前序记录一致，不能归因于源码；GitHub Billing/runner 恢复后只需重跑最新提交。

- 最新 CI `33359611153`（提交 `cf5d326`，2026-08-31）仍在 runner 启动阶段失败：四个 job
  均为 `steps: []`，约 2 秒内结束；继续按 GitHub Billing/runner 外部阻塞处理，不将其
  视为源码测试失败。

- `npm run release:check` 在提交 `4169778` 上通过：CLI 21/21、品牌 2/2（115 条分类
  引用、0 blocking）、Website、LAN Lite 66/66、Desktop 31/31、Upgrade 18/18、
  Release workflow 12/12，以及 SOURCE_MANIFEST/package check 均通过。该命令在
  外部受限执行环境中运行，避免 CLI 子进程被沙箱拒绝。
- 本轮有界复核：Go `gemini`/`claude` 测试通过；前端 `tsgo -b` 通过，Vitest
  通过 61 个测试文件、274 个测试。测试容器限制为 `--cpus=1.5 --memory=3g
  --memory-swap=4g`；图表零尺寸和 React 非布尔属性仅为既有测试环境警告，不影响
  断言结果。真实手机/桌面设备仍按外部验收顺序执行。
- `618327c` 后增量复核：CLI 22/22、LAN Lite 66/66、Desktop 31/31、Website
  静态检查通过；国际化 parity 测试 5/5 通过。前端完整构建与真实设备视觉仍待
  受限环境/外部设备执行。
- `24a44f1`/`f965e04`/`3338846` 历史增量：release contract 15/15、upgrade contract
  18/18、Full Content Logs 相关 Vitest 14/14 和 tsgo 通过；当时新增 Codex Go 测试仅
  完成 gofmt/静态审阅，未完成依赖下载后的运行验证；该缺口已由后续 Go 容器回归补齐。
- `5cfb046` 后增量：Electron runtime tests 当前 17/17 个 Node 子测试（分布在 2 个
  test files）、Desktop contract 32/32；真实平台安装和局域网请求仍待实机验收。
- `28e7bb2` 后增量：在允许 Node 子进程的受限环境中重新运行完整
  `npm run release:check`，CLI 22/22、品牌 2/2、Website、LAN Lite 66/66、Desktop
  32/32、Upgrade 18/18、Release workflow 15/15 以及 SOURCE_MANIFEST/pack check
  全部通过；未执行任何发布、tag 或生产操作。
- `c8a0681`：权限快照刷新仅针对非超级管理员的旧会话；`SUPER_ADMIN` 继续使用后端
  隐式全权限路径，不因缺少矩阵而增加不必要的 refresh。认证会话回归 11/11、格式
  检查通过；该边界不会把缺失权限当作 allow。
- `c4793ae` 后增量：在单 CPU、Node 堆上限 2GB 的受限环境中运行完整前端回归，
  61 个测试文件、273 个测试全部通过；仅有既有图表零尺寸和 React 非布尔属性警告，
  没有断言失败。真实手机视觉仍需按实机清单执行。
- `085e475` 后增量：额度变化面板在管理员面板挂载时按 `user.id + session.sid`
  最多刷新一次 `/api/user/self`，解决 SPA 内权限策略更新后旧快照导致的误隐藏；新增
  会话刷新回归测试。单 CPU、Node 堆上限 2GB 的完整前端回归通过 61 个测试文件、274
  个测试；仅有既有图表零尺寸和 React 非布尔属性警告。
- `8478f0f` 后增量：额度历史聚合与概览变化按 metric/window/source/plan/unit/currency/
  window_seconds 隔离；未指定系列的频道历史会锁定最新系列并支持显式筛选，避免多订阅
  计划混合绘图和速率计算。Full Content Logs 与 Usage Logs 在 401/403/网络错误时清空
  旧缓存行/正文并保留 Retry；单 CPU、Node 堆上限 2GB 的完整前端回归通过 62 个测试
  文件、278 个测试。
- `ee04de8` 后增量：Key 表单显式维护并提交稳定 `access_profile_id`，同时保留 legacy
  `group`；账户等级和访问方案的映射测试扩展后，完整前端回归通过 62 个测试文件、279
  个测试。
- `98133ce` 后复核：在 `web/` 使用
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 重新执行完整前端回归，62 个
  测试文件、279 个测试全部通过（约 125 秒）；该结果仍只证明代码和状态分支回归，不能
  替代真实管理员手机上的登录、权限和视觉验收。
- 当前提交 `fc4c7b4` 后复核：在 `web/` 使用 `taskset -c 0` 和
  `NODE_OPTIONS=--max-old-space-size=2048 npm test -- --run` 重新执行完整前端回归，62 个
  测试文件、279 个测试全部通过（约 158 秒）。本次限制为单 CPU，未出现断言失败；该结果
  仍不能替代真实管理员手机上的登录、权限和视觉验收。

## 版本与远端 tag 只读核对（2026-08-31）

- 本轮只读核对确认 `main` 与 `origin/main` 指向同一提交；精确提交值应以
  `git rev-parse HEAD origin/main` 的当前输出为准，避免文档提交后产生漂移。
- `origin` 的 `v0.1.1` 仍指向历史提交 `5007c6c`；本次工作没有移动或覆盖该 tag。
- 本地历史 `v0.1.0` 仍保留在旧提交；正式发布前仍需由负责人决定新版本号并创建
  指向目标提交的新 tag。
- `npm run release:state` 当前按预期 fail-closed：工作树版本 `0.1.1` 对应的 `v0.1.1`
  是受保护历史 tag，因此工具拒绝继续，要求先决定新的版本号；本轮没有修改
  `VERSION`/`package.json`，也没有移动或覆盖任何 tag。

## 外部验收顺序

1. 在脱敏测试副本记录镜像 digest、数据库备份校验和及资源余量。
2. 使用具备 `channel.read` 的管理员账号，在手机浏览器验证日志和额度页面。
3. 分别在 macOS 和 Windows 验证默认回环、显式 LAN 绑定、API Key 请求和回滚。
4. 完成副本升级/恢复后，再由负责人决定是否进行生产变更、版本 tag、NPM 或其他发布。

## 代码优先阶段增量（2026-09-01）

- Palm、Zhipu、Ali rerank 和 MiniMax 的业务 JSON 序列化/反序列化已统一走
  `common/json.go` wrapper；Palm/Zhipu/Ali 增加 malformed 响应边界回归，Zhipu
  malformed `meta:` SSE 与 scanner 错误不再继续输出或伪造 usage。
- Provider 定向回归：`go test ./relay/channel/palm ./relay/channel/zhipu
  ./relay/channel/ali ./relay/channel/minimax -count=1` 通过；在允许本地测试端口的
  条件下，`GOWORK=off go test ./... -count=1` 全量通过。受限沙箱首次运行的 SMTP
  测试因 `127.0.0.1:0` bind 权限失败，升级权限后已复核通过。
- relaykit 独立构建 `cd relaykit && GOWORK=off go build ./...` 通过（依赖下载在允许网络
  的执行环境完成）。
- 前端/发行合同本轮未能启动：本机 `/opt/homebrew/bin/node` 及 `merve/tsgo` 因缺失
  `simdutf` 动态库而在启动阶段 SIGABRT；代码未因此修改。修复 Homebrew Node 运行时后
  需重跑 `bun run typecheck`、前端测试、production build 与 `bun/npm run release:check`。
- 本阶段没有执行 tag、NPM/GHCR publish、生产重启或生产数据操作；完整 UI 替换仍为
  `尚未开始（代码层先行）`。真实 Docker Desktop、MySQL/PostgreSQL 副本、手机及
  macOS/Windows 安装仍按外部验收清单待验证。
- 提交 `2db1a58` 已推送到 `origin/main`；GitHub Actions CI run `33419182037` 的
  Backend、Frontend、Desktop、Distribution 四个 job 均在 runner 启动前失败且
  `steps: []`。按规则归类为 GitHub Billing/runner 外部阻塞，不归因于本轮源码；恢复后
  只需重跑该提交的 CI。
- 随后使用已安装的 Node 22.23.2（`PATH=/opt/homebrew/opt/node@22/bin:$PATH`）并将
  `NPM_CONFIG_CACHE` 指向临时目录复核：`bun run typecheck` 通过，前端 Vitest 62 个文件/
  280 个测试通过，`bun run build` 通过；`npm run release:check` 通过（CLI 22/22、LAN
  68/68、Desktop 32/32、Upgrade 18/18、Runtime 13/13+测试 4/4、Release workflow
  18/18、源码清单和打包检查通过）。原生 `/opt/homebrew/bin/node` 的 simdutf 链接问题仍
  需在开发主机永久修复，当前验证通过不代表 Node 26 环境符合项目要求。

## Provider JSON 审计增量（2026-09-01，第二轮）

- Xunfei 与 Volcengine 的业务 JSON 编解码已统一走 `common/json.go` wrapper；Xunfei
  增加响应解码错误边界，Volcengine 保留 `json.RawMessage` 类型依赖并补充 TTS/metadata
  malformed 输入测试。
- 定向回归 `GOWORK=off go test ./relay/channel/xunfei ./relay/channel/volcengine -count=1`
  通过；未改变上游协议、发布流程或生产数据。
- 随后在允许本地 SMTP/回环测试端口的环境中重新运行 `GOWORK=off go test ./... -count=1`，
  全部根模块包通过。提交 `f0b2195` 已推送；CI run `33420183217` 的四个 job 仍均为
  runner 启动前失败、`steps: []`，继续归类为 GitHub Billing/runner 外部阻塞。
- 按请求手动触发 Docker smoke workflow `33420575494`（提交 `5493cca`）；唯一 job
  `Build local image and probe SQLite runtime` 同样在 runner 启动前失败且 `steps: []`。
  workflow 仍保持 `push: false`、不登录 GHCR 的安全边界；待 GitHub runner/Billing 恢复后
  重跑即可，当前不能把该结果当作镜像或 `/api/status` 验收。

## Provider JSON 审计增量（2026-09-01，第三轮）

- AWS、Jimeng、MokaAI 的业务 JSON 路径已统一使用 `common/json.go` wrapper；保留必要的
  `json.RawMessage` 类型依赖，并补充 malformed 响应/输入回归。
- `GOWORK=off go test ./relay/channel/aws ./relay/channel/jimeng ./relay/channel/mokaai -count=1`
  通过；未改变 Provider 协议、发布闸门或生产环境。

## Linux 实例历史部署记录（2026-09-01）

本节来自当时获准执行的 Linux 实例记录，不表示当前 Mac 的部署或本轮生产验证。

- `/root/new-api/docker-compose.yml` 当时配置的是本地镜像
  `local/new-api:myapi-c283c1d`，容器名为 `my-api`，监听回环地址；旧的 `new-api` 仅保留在历史演练记录中。
- 2026-09-01 远端同步：本地 `main` 已快进到 `3ea1e5b`，与 `origin/main` 一致；该批次包含
  provider JSON 解码路径收敛、请求包装/参数边界、计费与并发安全、数据库迁移和 CI/发布合同更新。
- 2026-09-01 本机更新：使用 `MYAPI_BUILD_PARALLELISM=1` 构建
  `local/new-api:myapi-c283c1d`，并通过 compose 强制重建本机容器；容器健康、`/api/status`
  返回 HTTP 200、`success=true`、`version=0.1.1`、`system_name=MyAPI`。
  数据和日志绑定仍为 `/root/new-api/data` 与 `/root/new-api/logs`，没有修改其中内容。
- 后台额度采样默认开启；本机 Compose 不注入覆盖环境变量，因此管理员可在「系统设置 → 运维 →
  监控与告警」修改采样开关、间隔和每轮最大渠道数。首次任务已成功执行并记录 1 个 Codex 采样点；
  当时按默认 15 分钟间隔继续产生历史点；这是历史部署记录，2026-09-03 修复版本的代码默认值已改为 1 分钟，见下文。
- 2026-09-01 本机容器已从历史名称 `new-api` 重命名为 `my-api`；仅重建容器实例，继续使用原有
  `/root/new-api/data` 和 `/root/new-api/logs` 挂载，未删除卷或迁移数据。
- 2026-08-31 Codex 额度折线图更新：源码提交 `ff5feb8` 已使用单核、2GB 内存构建为
  `local/new-api:myapi-ff5feb8`，并通过 compose 强制重建本机容器；容器健康、首页返回
  HTTP 200，`/api/status` 返回 `success=true`、`version=0.1.1`、`brand=MyAPI`。
  数据和日志绑定仍为 `/root/new-api/data` 与 `/root/new-api/logs`，本次没有修改或复制其中内容。
- 2026-08-31 后续核对确认该容器仍在运行且健康；该镜像包含 SQLite 迁移修复，并在
  临时 SQLite 副本上完成新镜像启动、旧镜像回滚和 `/api/status` 健康演练。
- 新镜像构建使用 `MYAPI_BUILD_PARALLELISM=1`，未拉取或发布外部 MyAPI 镜像，也未读取
  生产环境密钥。
- 容器健康检查通过，`/api/status` 返回 HTTP 200、`version=0.1.1`；本次源码提交和镜像
  构建分别由 `ff5feb8` 与 `local/new-api:myapi-ff5feb8` 记录。
- 新镜像内嵌前端已确认包含 `Account quota changes`、`/api/channel/quota/changes`
  和 `Runtime build`，并已在本机正式回环容器中运行。
- 未携带凭据请求额度接口返回 HTTP 401（`AUTH_UNAUTHORIZED`），权限门禁正常；本轮
  没有使用真实登录凭据，因此仍不能证明管理员账户在手机上已看到数据或样本。
- 仍需使用具备 `channel.read` 的管理员账号在手机浏览器登录，核对 Runtime build
  revision、额度面板和 API 日志正文；本轮没有使用真实登录凭据，因此未替代该项
  真实视觉/权限验收。
- 最终镜像运行时链路增量探针：在 `local/new-api:myapi-921ca38` 上使用 1 CPU、1 GB
  内存和匿名 SQLite 数据卷完成初始化，使用仅用于探针的合成管理员登录后，
  `/api/user/self`、`/api/channel/quota/changes?range=24h` 与 `/api/log/` 均返回
  HTTP 200 且 `success=true`；该副本没有真实渠道因此额度项为 0，日志列表返回 1 条
  结果。探针结束后容器、数据卷和临时文件均已清理；这验证认证/权限链路，不替代真实
  管理员手机上的视觉与敏感日志正文验收。
- 针对移动端日志可见性和额度面板的增量回归：在 `web/` 使用 `taskset -c 0` 运行移动
  日志卡片、日志表格集成、概览额度面板和渠道额度面板 4 个测试文件，共 22 个测试全部
  通过（约 13 秒）。这验证移动 slot、加载/错误/空态与权限分支，但仍不能替代真实
  手机浏览器的触控、滚动和视觉验收。
- 当前后端增量回归：使用本机缓存的 Go 1.26.1 容器，在 `--cpus=1.5`、`--memory=3g`
  限制下运行 `GOWORK=off go test ./controller -run 'ChannelQuota|CodexQuota|Quota' -count=1`，
  结果 `ok github.com/ForceMind/MyAPI/controller`；该结果验证额度采样、聚合和 Codex
  用量相关控制器回归，不替代真实上游账户或生产数据库演练。
- 当前提交 `0735b20` 后完整发行检查：在单 CPU 限制下运行 `npm run release:check`，CLI
  22/22、品牌审计 115 条分类引用且 0 blocking、Website、LAN Lite 66/66、Desktop
  32/32、Upgrade 18/18、Runtime probe contract 13/13、Runtime probe 测试 3/3、Release
  workflow 16/16、源码清单和发行包检查全部通过。该结果不替代真实跨平台安装、手机视觉
  验收、数据库恢复或外部 GitHub runner 验证。
- `fb920c5` 后增量：新增 SQLite 迁移回归测试，在 Go 1.26.1 Alpine 容器中以单 CPU、1 GiB
  内存运行通过；验证旧表添加 `dedupe_key`、唯一索引创建及重复迁移幂等性。
- `80f722e` 后本机副本演练：`local/new-api:myapi-4b08bdb` 完成 SQLite 迁移并健康，恢复
  原数据库副本后旧镜像 `local/new-api:myapi-9dc11d4` 也通过健康检查；临时容器、端口和
  副本已清理，正式容器未使用副本数据。
- 当前文档基线（`b9cf87e`）：`main` 与 `origin/main` 已同步；在该干净提交上重新运行
  `npm run release:check`，CLI 22/22、LAN Lite 66/66、Desktop 32/32、Upgrade 18/18、
  Runtime probe 4/4、Release workflow 16/16、品牌/官网检查和 2115 文件打包检查均通过。
  `SOURCE_MANIFEST.json` 已生成但按约定保持 ignored。
- 推送 `4cc5e2b` 后的最新 CI `33376520189` 仍在 runner 启动前结束，四个 job 均为
  `steps: []`；该结果继续按 GitHub Billing/runner 外部阻塞处理，不能归因于源码。
- 推送 `7f4e184` 后的 CI `33376622066` 延续相同状态：四个 job 均在 runner 启动前结束且
  `steps: []`；待 GitHub Billing/runner 恢复后只需重跑最新提交。
- 当前 HEAD `6e96836` 的完整 `npm run release:check` 已重新通过：CLI 22/22、LAN Lite
  66/66、Desktop 32/32、Upgrade 18/18、Runtime probe 4/4、Release workflow 16/16，
  品牌/官网检查和 2115 文件打包检查均通过；该命令在单 CPU 限制下执行。
- 2026-09-02 本机额度 UI 更新：基于提交 `d69b556` 使用单 CPU、2 GiB 内存构建
  `local/new-api:myapi-quota-ui-20260902`，并通过 `/root/new-api/docker-compose.yml`
  重建 `my-api`。容器健康检查通过，继续复用 `/root/new-api/data` 和
  `/root/new-api/logs` 挂载；`/api/status` 返回 HTTP 200、`success=true`、`version=0.1.1`。
  本次加载了概览/渠道额度折线、面积、柱状模式、时间颗粒、指标切换和中文状态徽标。

## 额度消耗修复与本地回归（2026-09-03，未部署）

此前“控件已加载”的部署记录不能证明真实功能可用。本轮按真实路由、浏览器渲染、原始快照算法及后台任务重新核验；完整规则和使用路径见 [额度与消耗分析](QUOTA_ANALYTICS.md)。

- 概览、渠道详情和渠道行 Codex 历史弹窗共用历史分析组件；支持折线/面积/柱状、额度/消耗/每分钟估算、时间颗粒、预设及自定义时间范围。
- 统一在服务端从相邻原始有效样本计算消耗、加权平均速率和原始峰值，再进行展示聚合。失败不形成第二个账户，不跨重置、缺口、恢复或基准变化拼接消耗；最新采样状态与历史统计分离。
- 修复真实中文浏览器中的非法 `Intl` 语言标识异常，以及固定高度渠道容器裁切。新增文案覆盖 7 个语言包；货币、额度百分比与消耗百分点分开格式化。
- 默认开启 1 分钟采样，保留管理员和环境变量优先级；按任务创建时间调度、公平轮转，并限制上游请求、刷新、保存、进度及租约操作的时间预算。旧的显式配置不自动覆盖。
- 只读检查实际历史规范化快照发现 `reset_at` 秒级抖动，已补 Codex 限定容错及匿名回归测试。实际跨越重置仍断开；未修改生产数据或凭据。
- 前端完整回归 **64 文件 / 336 测试通过**，TypeScript 与生产构建通过；Go 1.26.1 的 `service`、`controller`、`model` 三包完整回归通过。测试使用单核/半核和明确内存限额，没有无约束并行构建。
- 真实构建的浏览器回归通过：三个入口的实际 Recharts SVG、样式/范围/颗粒/自定义日期请求、失败历史保留，以及 320×740、390×844、1280×600 下滚轮到达图表时间轴和渠道操作。已实际查看截图。浏览器使用合成 API fixtures，不冒充真实账户端到端验证。
- 新增 `npm run quota:browser` 和 CI 浏览器回归、截图工件；原始本地验收时尚未提交/推送，
  后续提交为 `a36e529`，其 CI 已成功（见本页当前证据）。MySQL/PostgreSQL 当次只有
  SQL 生成层检查，真实运行未验证。
- 原始本地验收没有修改 tag、发布 NPM 或重启生产容器。本轮审计未访问生产；获准部署后，
  还需等待至少两个新样本，核对任务结果、实际 API 和管理员页面，才能确认线上修复完成。

## S0＋S1 执行（2026-09-03，原六项已验收）

用户已明确确认 S0＋S1，完整阶段/依赖、六项缺陷与授权边界见
[开发执行计划](DEVELOPMENT_EXECUTION_PLAN.md)。新增回归先复现显式 ID 迁移覆盖、配置
持久化失败误报成功、profile 规范化/共享状态问题、Zhipu 缺 usage 伪成功及合成凭据日志
泄露，再完成修复与复验。

- S1-01：在 AutoMigrate 前给缺失 identity 列添加 nullable 无默认值列，仅回填空值；DDL
  不依赖事务回滚，部分加列或事务失败可以重试。显式 standard/custom、旧 group 及重复
  启动保留。此前版本已经覆盖的 ID 无法可靠推断恢复，不自动猜测改写。
- S1-02/03：单一有序 SSE 通道，取消/下游写失败关闭上游并等待生产者退出；缺失/非法
  usage、解析/读取错误不发送 DONE、不以 nil usage 报成功；无效凭据不写日志且拒绝认证
  构造，合法 JWT/缓存协议保留。测试使用合成数据，不是上游实测。
- S1-04/05/06：配置写入检查事务错误、成功后才发布内存；profile 专有同步导入/快照
  导出与深拷贝不暴露共享可变对象，ID/fallback 实际存储规范化并拒绝碰撞。
- 独立 sol 对两条实现线静态复审，原六项修复未发现新增阻断回归；另发现既有双写者
  提交/发布顺序风险 S1-R1（P2），已征求用户确认，未授权前不实现，也不声称多写者
  最终一致性已验证。terra 对文档/额度 OpenAPI/本地链接复审通过。

实际本机验证（Go 1.27.0，以下 Go 命令使用 `GOMAXPROCS=1 GOWORK=off` 与临时缓存）：

- `go test -p 1 ./... -count=1 -timeout=180s`：根模块全量通过。
- `go vet -p 1 ./...`、`go build -p 1 ./...`：通过。
- `cd relaykit && go build -p 1 ./... && go test -p 1 ./... -count=1`：独立构建/测试通过。
- `go test -race -p 1 ./relay/channel/zhipu -count=1 -timeout=60s`：通过。
- `go test -race -p 1 ./model ./setting -run 'TestUpdateOptionAccessProfileConcurrentReadAndExport|TestAccessProfileReadersReturnDetachedSnapshots|TestAccessProfileConfigManager' -count=1`：专项通过；不代替双写者逻辑顺序测试。
- Node 22 下 `npm run release:check`：CLI 22/22、Brand 2/2（116 条分类/0 blocking）、
  Website、Quota OpenAPI 3/3、LAN 68/68（跳过 Docker）、Desktop 32/32、Upgrade 18/18、
  Runtime 13/13＋4/4、Release workflow 18/18 均通过；最后 pack 检查按预期拒绝 dirty
  源码树，需在干净提交上重新生成 manifest/pack，不放宽此保护。

新增 S1 专用 CI 服务库验证入口 `TestAccessProfileConfiguredDatabases`，仅显式启用、
loopback 与 `myapi_s1_test` 库名可用，先拒绝已有 users/tokens/options 表，不删除既有表；
MySQL 5.7/PostgreSQL 9.6 容器由 CI 生命周期清理，不发布镜像、不读取实际部署 DSN。
首次提交时尚待真实 CI，不把本机未配置而 skip 或 SQLite 成功写成三数据库已通过；其范围
仅本次 identity/Option 迁移与事务，不替代 S4 的完整应用升级、备份恢复及多连接业务验收。

阶段提交 `c42eeeb` 与 CI 修正 `68e930f` 已推送；`c42eeeb` 干净树的 manifest/pack
复验通过（2169 文件，19,730,795 bytes）。CI `33739643051` 的后端、前端（含生产构建、
真实图表 fixture 与截图）、桌面、发行四项成功；新增数据库任务实际启动 MySQL5.7/
PostgreSQL9.6 后，在 MySQL 的 HasColumn 探测发生 panic：当前 GORM 基础实现不能用纯
表名字符串替代带 schema 的模型。该结果是代码兼容回归，**不是 Billing/runner 阻塞**；
该次失败时 S1-01 保持待验证，要求修正探测参数后重跑同一实库用例，不跳过引擎或放宽断言。

上述两段记录初次提交/失败时点；最终在 `dc94e81` 修正为携带模型 schema 的列探测，
独立复核机制后重跑 [CI 33740321133](https://github.com/ForceMind/MyAPI/actions/runs/33740321133)：

- 五个 job 全通过：Backend、Frontend、Desktop、Distribution、S1 database regression。
- 数据库 job `100600463870` 日志确认 `TestAccessProfileConfiguredDatabases/mysql` 和
  `/postgres` 及各自 create-new/save-new/save-existing 全部 PASS，没有 SKIP；对应
  MySQL5.7、PostgreSQL9.6 一次性服务库，仅涵盖本批 identity/Option 行为。
- CI 采用 `go.mod` 的 Go 1.25.1、Bun 1.3.14；根模块及 relaykit vet/build/test 通过。
  前端 64 文件/336 测试、生产构建、真实图表合成 fixture 与截图工件成功；截图工件名
  `myapi-quota-browser-dc94e81f21dca9378fdb76a4f38be7cdcc57a29d`。
- S0 计划/接口文档和 S1-01..06 原六项可标已完成。S1-R1 双写者最终一致性仍待用户
  决策；不把单写者 race 或以上实库事务测试当作双写顺序证明。
- 本轮未改生产数据库/服务、未移动 tag、未发布 GHCR/NPM；生成清单保持 ignored。
  完整应用三库备份恢复、Full/LAN 镜像矩阵、真实上游/设备和独立 UI 仍按 S2-S7 推进。

## S1-R1 配置发布顺序（2026-09-03，已验收）

用户已确认在同一进程内统一配置保存及后台重载的发布顺序。实现覆盖数据库读取/提交到
内存发布的完整区间，保持 OptionMap 锁不跨 DB I/O；初始化调用已持锁 helper，避免递归
加锁。原代码红测直接观察到 DB=Second、Map/注册表=First，以及 DB=Newer、旧重载
覆盖为 Initial，另有失败读取仍发布部分快照；无测试超时，不以随机调度或 sleep 作为证明。

新增四项确定性回归已通过，sol 独立静态复审未发现阻断；本机完整回归与最终 CI 结果如下。
后台重载读失败保留现值；初始化仍先构造默认 Map，读失败不保证调用前 Map 不变。
本项不处理跨实例一致性或所有配置读取原子快照，未来增加 ConfigManager.SaveToDB
回调保存的生产入口时须另审锁顺序。

本机集成回归（Go 1.27.0；前缀 `GOMAXPROCS=1 GOWORK=off`，构建/模块缓存位于临时目录）：

- `go test -p 1 ./... -count=1 -timeout=180s`、`go vet -p 1 ./...`、
  `go build -p 1 ./...`：全部通过。
- `cd relaykit && go build -p 1 ./... && go test -p 1 ./... -count=1`：独立构建/测试通过。
- `go test -race -p 1 ./model -run '^TestOption(WritesPublishInCommitOrder|ReloadCannotOverwriteNewerWrite|SaveFailureDoesNotPublishOrHoldSequence|ReloadFailureDoesNotPublishOrHoldSequence)$' -count=1 -timeout=120s`：四项通过；worker 另跑 S1 持久化/规范化/注册表读取回归通过。
- Release workflow 合同 18/18、Quota OpenAPI 合同 3/3 通过；CI YAML 语法解析通过。
  新 race 步骤已接入 backend job。
- 干净提交 `8dfcfba`、Node22 下 `npm run release:check` 全部通过，包含 CLI、品牌、官网、
  OpenAPI、LAN（跳过 Docker）、Desktop、Upgrade、Runtime、Release 和源码清单/打包；
  包含 2170 个文件、19,749,735 bytes。生成物保持 ignored，未执行发布。
- [CI 33743669737](https://github.com/ForceMind/MyAPI/actions/runs/33743669737) 的 Backend、
  Frontend、Desktop、Distribution 与 S1 数据库五项成功；新增 R1 race 步骤通过，原
  MySQL5.7/PostgreSQL9.6 临时库迁移/配置事务用例继续通过，未扩大其验收范围。
- CI 前端 64 文件/336 测试、生产构建及浏览器 fixture 回归通过；本轮未修改 UI，未将
  这些结果冒充新的真实设备视觉审查。
- R1 生产仅修改 `model/option.go`，新增确定性测试、CI 入口及文档；无数据 schema、
  Provider、UI、发布、tag 或生产环境操作。全项目目标仍包含 S2-S7，不能把 S1 完成
  当作整个产品或所有配置并发/原子语义已验证。
