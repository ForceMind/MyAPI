# 历史账务只读审计

状态：受限离线工具已完成合成验证和独立审查，尚未完成生产历史账务对账。报告不能作为直接部署、启动恢复 worker 或自动补扣的依据。

工具仅使用 Python 标准库及原生 SQLite，不导入 MyAPI。只接受明确指定且来源已获认可的封存副本，不默认连接运行实例。副本、manifest 和报告放在仓库外的私有目录，不提交或上传。

运行需要 Python 3.10 或更新版本。修复候选 `8fce1b4` 的 [CI 38158081563](https://github.com/ForceMind/MyAPI/actions/runs/38158081563) 中，63 项合成检查通过并经独立审查：只读读取及证据分类 36 项、离线配置探针 8 项、第二副本模拟 19 项，均使用合成账务数据。早期记录的 36 项、44 项分别对应尚未加入探针、尚未加入模拟的阶段，不代表当前测试集，也不能作为生产对账结论。

## 输入

manifest 格式如下。路径必须是绝对规范路径；摘要是实际文件的完整 SHA-256；来源说明须记录取得方式及时间，不写 DSN、密码、Key 或连接 URL。

```json
{
  "snapshot": {
    "path": "/private/approved-copy/accounting.db",
    "sha256": "实际的64位小写十六进制摘要",
    "source": {
      "description": "用户认可的一致性副本，取得方式与UTC时间"
    }
  }
}
```

禁止将 live 主文件复制后直接宣称一致快照。SQLite WAL 中可能仍有已提交事务；应用停止或等待一个批次间隔也不能证明缓存已落盘。应采用认可的一致性备份流程，例如原生 backup API，并另行记录应用版本、有效配置及缓存证据。

本工具在建立连接前拒绝 WAL/SHM/journal 附属文件及 WAL 模式文件头。使用封存副本的 rollback journal 格式；需要转换时只处理新的派生副本并重新记录摘要，保持原始副本不变。工具不会调用 checkpoint 或修改 journal 模式。

## 运行与验证

先执行合成测试，测试仅自行创建临时数据库：

```bash
cd tools/accounting-audit
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -v
```

来源认可、工具审查通过后才运行审计：

```bash
PYTHONDONTWRITEBYTECODE=1 python3 audit.py \
  --snapshot /private/approved-copy/accounting.db \
  --manifest /private/approved-copy/manifest.json \
  --output /private/reports/accounting-report.json
```

输出文件必须是新的私有文件，不能覆盖输入或已有报告。工具检查完整性，使用 `mode=ro`、`query_only` 和读取事务，并核对前后摘要。金额读取为文本及 SQLite 存储类型，精确值输出为十进制字符串；NULL 不是零，REAL 不作为精确账务证据。

## 解释范围

intent 与 fact 通过原事件关联，只代表一份结算义务。legacy fact 的 `actual_quota=0` 是存储约定，不能当实际用量。结算代码中的 delta 是实际用量与预扣之差；若 journal 中 actual 与 reserved 均有精确记录，工具只校验记录值公式 `delta = actual − reserved`，不把预留记录当作余额实际扣款的证明。缺旧日志时不能推定预扣为零。

资金与 Key 两腿分别报告，已应用一腿不意味着另一腿也完成。batch generation 不含请求归属，不能分摊为某个请求的预扣或债务。没有期初余额、完整资金流和缓存证据，不声明最终余额守恒。身份冲突、缺列、缺日志、不精确金额和缓存未知须保留，不能自动补扣。

完整内容日志（full-content）记录的是网关向客户端写出的响应。beta.3 及当前修复候选均存在本地估算、协议转换和生成客户端 usage 的路径；相关路径在输出前移除内部 `BillingUsage` 来源信息。因此，客户端响应中的显式用量数字不能单独证明上游实际用量、历史计费用量或结算金额；客户端响应缺少 usage 也不能证明上游没有提供用量。

用量证据须能关联原 request/event，并证明实际生产路径、版本及上游报告或估算来源；金额核定另需可追溯的请求冻结计价。仅提取响应中的数字不能填补这些来源缺口，也不能替代资金、预扣、缓存及全部写入者的一致性证据。

完整 authoritative receipt 指纹、generation payload、writer/config 历史有效性和外部缓存不在本分类器的已验证范围内。报告固定 `ledger_reconciled=false`、`upgrade_eligible=false`、`automatic_actions_allowed=false`，原始状态与未验证项继续保留。

## 离线启动配置探针

`preflight.py` 可在启动任何 MyAPI 进程之前，对认可的封存副本检查一条配置规则。它只读取 `quota_writer_epochs` 的 id、schema_version、mode、epoch、lock_version，不读取余额，不初始化缺失表或默认 writer。

```bash
PYTHONDONTWRITEBYTECODE=1 python3 preflight.py \
  --snapshot /private/approved-copy/accounting.db \
  --manifest /private/approved-copy/manifest.json \
  --batch true \
  --redis-status absent
```

`--batch` 必须显式选择 `true`、`false` 或 `unset`，表示既有配置，不建议修改配置。`--redis-status` 的 `available`、`absent`、`unknown` 是操作员声明，探针不连接或验证 Redis，也不接收 Redis URL/凭据。

| 条件 | 该规则的结果 |
| --- | --- |
| legacy/bridge，batch=true，Redis absent | blocked |
| authoritative，batch=true，Redis absent | not_blocked_by_this_rule |
| batch=true，writer 缺失/不支持/重复，或 legacy/bridge 的 Redis unknown | unknown |
| batch=false/unset | not_blocked_by_this_rule；writer 仍可为 unknown |
| legacy/bridge，batch=true，Redis available | not_blocked_by_this_rule；可用性仅为声明 |

输出到标准输出，包含前后摘要；规则退出码分别为未被该规则阻断 0、阻断/非法配置 2、未知 3。退出码 0 只对应这一规则，不能作为启动或迁移许可。所有报告固定 `application_start_approved=false` 和 `migration_approved=false`，运行配置来源、账务、缓存和升级条件均未核清。

该 Python 探针不接入应用启动流程。修复分支的 Go 门禁已接入：[main.go](../../main.go) 在 `InitDB` 前验证 Redis 有效配置，[model/main.go](../../model/main.go) 在普通数据库连接及自动迁移前，依次执行 writer/batch、金额表示和历史执行许可预检；首次失败即停止，worker 前仍有第二次 batch 检查。这些门禁通过也不表示历史账务已核清或允许升级。

远端 main `92fa1182` 与已发布 beta.10 尚无上述迁移前门禁集成，仍先初始化数据库再执行 batch 检查。启动失败不能证明数据库未被写入；应使用原生只读诊断，不能启动这些版本来试探历史库。该 Python 探针本身不改变任何版本的启动行为，合成测试通过也不能作为生产问题已解决的证明。

完整目标、来源缺口和下一阶段见[核查记录](../../docs/HISTORICAL_ACCOUNTING_COMPATIBILITY.md)。金额兼容修复与第二份可写副本演练须基于对账证据；此工具不执行恢复。

## 第二副本离线模拟（工具受限审查通过，禁止生产回灌）

`rehearsal.py` 只接受封存输入及一个尚不存在的私有输出目录，自行创建新副本；CLI 不接受已有可写数据库，不提供生产恢复入口。输出数据库禁止回灌或用于部署。普通 Go worker、int32 请求契约、batch/Redis 配置均未改变。

```bash
PYTHONDONTWRITEBYTECODE=1 python3 rehearsal.py \
  --snapshot /private/approved-copy/accounting.db \
  --manifest /private/approved-copy/manifest.json \
  --new-private-output-directory /private/new-simulation
```

当前保守候选条件：唯一 canonical legacy_wallet fact、正 delta、两腿未应用，匹配且唯一的 request/event/主体/状态日志明确报告 actual=delta。这仅满足记录值公式中 preconsume=0，不证明资金实际预扣为零或缓存已落盘。日志仅用于候选资格，扣差额唯一取原 fact.delta，不按实际总用量重新计算。其他事件冻结；重复身份或不精确金额拒绝。只支持已验证 legacy writer、没有重叠 intent/refund/generation 的 SQLite 副本。日志 request 与 event 均须唯一；关联 prepared journal 的 actual 必须 NULL。拒绝触发器、外键、相关对象为 view 或重复主体/fact ID；副本报告记录输出 SHA-256。

每腿在同一事务内按旧余额/版本及 fact lock_version 比较更新余额和 applied；原 event/request/fingerprint 不变。正常 int32 原余额不能借模拟跨成新的宽余额；已有宽余额资格固定在封存源的具体字段上，仅在原值/当前/目标绝对值均不超过 `2^53−1` 时精确演练，不截断或归零。Key 已用不允许负值。首次写前按主体聚合候选差额与版本增量做范围预检；逐腿仍按同一源资格校验。这是工具资格限制，不是新增全局业务规则。

模拟核对选定差额的资金减少、Key 剩额减少、Key 已用增加，重复执行不追加扣款，冻结 facts 不变及输入摘要不变。负钱包/Key 余额明确报告，不截断、归零或隐去。新增 19 项合成测试覆盖事务中断回滚、资金腿提交后重开续跑、并发重放、原身份冲突、精确存储与范围、只读源及输出不覆盖输入、负余额报告；不能据此声称实际 Go worker、完整历史账本或生产缓存已验收。

报告始终为 `simulation_only=true`、`cache_evidence=unknown`、`production_eligible=false`、`ledger_reconciled=false`。`selected_difference_conservation=true` 仅指本次选定差额模拟的恒等式，完整历史守恒仍未验证。遇到中断保留私有副本供核查；CLI 不原地续跑它，可从未变的封存来源创建另一份新模拟。
