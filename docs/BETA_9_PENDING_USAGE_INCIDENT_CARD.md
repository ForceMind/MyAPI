# beta.9 用量待核对：已复现原因修复卡

2026-10-09。优先于个人核心新功能；沿 PR #5 分提交实施和独立审查，最终准确 HEAD
重新跑全部适用 CI。beta.9 标签/制品不可覆盖。本卡不宣称已确定任何生产请求的根因，
也不把已有“结算归属/人工表单”补丁当作以下两个原因的修复。

## 已证实的两个条件组合

### A. 完整流结束之后的下游取消

真实 controller、隔离 TLS 上游、完整 usage 和 HTTP200/[DONE] 的合成对照：普通 Key
在 DONE 后关闭连接仍结算；严格 native Chat Token/USD Key 在连接保持时结算，恰在
DONE 后取消则进入 usage_unknown/usage_pending_review。两 writer 均复现。
取消判断早于 beta.9，但 beta.9 让 Playground 使用真实选定 Key，新增了可触达组合。
外部 SDK 读 DONE 后关闭也需要同样验证；不能只按客户端名称修补。

修复只允许：已有可信的完整上游终态、无解析/下游写错误、完整匹配模型/计数/档位证据时，
忽略随后下游请求取消，沿已有有时限 detached context 持久化结算。未知、矛盾、缺 usage、
中途取消/EOF、写失败不能伪装成功。保留实际 Token/USD 与普通账户腿的一次性、幂等与未知保留。

上游真实消耗并不取决于客户端是否收到回复；下游 DONE 不是费用证据。本次只修复“可信完整 usage
和原来要求的无错误完成边界均满足，随后断开却被误拒”的案例。对其他取消/交付失败继续保留
原 unknown/hold 与人工证据恢复，并非判断没有成本或自动退款。写观察器的取消时序记录必须
使用既有并发安全状态，并接受 race 回归，不新增无锁共享状态。

### B. Legacy 批量额度启用，但没有 Redis

隔离 SQLite 对照证实：legacy + trusted 零预留 + 正实际用量 + batch=true + Redis disabled
会在最终结算写 retryable fact，原因为 batch quota cache is unavailable；余额未改，日志
usage_settlement_pending。batch=false 正用量、batch=true 明确0用量可成功。
authoritative writer 无 Redis 仍能完成持久结算，只留下可恢复缓存投影告警，不能混为欠扣。

已发布 `deploy/docker-compose.yml` 硬编码 batch=true，却不提供 Redis 服务/连接，默认组合
与上述复现一致。根目录 Compose 有 Redis。当前用户的实际安装路径/配置仍需单独确认。

## 配置修复的安全约束

- 新安装显式选择正确默认，并生成 `MYAPI_ACCOUNTING_CONFIG_VERSION=1`；不把已有配置缺字段解释成 false。
  CLI/install 严格校验版本，raw Compose 只保证缺失/空确认字段在插值阶段阻断，未知非空值须走预检。
- 不能用 `${BATCH_UPDATE_ENABLED:-false}` 静默切换旧实例；Redis 缺失、遗失缓存或其他活跃节点
  的内存队列不能仅靠“数据库暂无 pending”排除。没有自动 DB fallback 或自动迁移。
- 旧 .env（包括 --force）、既有 DB/身份/历史账务应保留；缺失或不明确选择需要处理提示。
- **升级 preflight 必须先于停止旧容器/改写配置。** 不合格时拒绝升级，旧服务继续运行；不能先 down
  再因新启动检查失败。指引只要求确认非敏感模式信息，不要求公开完整 env 或 Redis URI。
- Runtime 在业务 worker/HTTP 前检查 legacy batch 与缓存依赖，给去敏且可操作诊断。
  不误阻止 authoritative 的独立持久结算；不改 writer、不清账、不忽略待恢复增量。
- 显式运维切换需先核历史事实/队列与现有节点，备份并按已验证流程操作；本源码修复不执行生产切换。

## 必需回归

1. 两 writer、普通/严格 Key、流保持/DONE后断开、外部 API/Playground；完整 usage 正确结算一次。
2. 前置取消、断流无usage、partial/invalid、错误档位、DONE写失败仍保留未知；重试/并发不双扣。
3. 将两类原 overlay 复现纳入正式仓库测试，保留失败到通过证据，不只报告口头结果。
4. 覆盖全部部署入口：新安装 false、root Compose 有 Redis 组合、已有显式设置保留、旧缺变量
   在 stop 前拒绝升级/不写旧 env、--force 不绕过、authoritative 不误拦、历史事实不删除。
5. 复跑 PR5 原结算投影/人工恢复、全部适用三库、race、前端/真实 Chromium；截图只含合成数据。
6. 独立代码审查、准确新 head 与 tree 回读，Draft 保留。任何旧 CI 取消/失败不计通过。

## 诊断与交付边界

新旧安装与安全处理见[诊断及升级指南](PENDING_USAGE_RECOVERY_GUIDE.md)。

费用徽标仅说明保存的日志 pending_review。实际区分依赖该条 Content 和当前只读结算证据：
usage_pending_review 是计量/价格证据未确认，usage_settlement_pending 是结算未确认。
历史日志不会自动回写，不因修复更改审计事实。新调用持续写新橙标必须调查原因，不能只隐藏。

读取所需最小字段：部署版本、最新时间、Content、actual_quota（保留null）、reserved_quota、
允许的状态/原因码；只在必要时查看 writer 与 fact applied 标记。不得公开真实请求、截图、Key、
headers、Redis连接串、数据库或任意 LastError 原文。诊断展示若扩展，只用白名单短原因。

当前交付为源码修复与验证；不覆盖已发布 beta.9，不自动合并/发版/部署/修改生产配置或账目。
个人模式/结构精简继续保留在原卡，待本事故修复闭环后恢复。
